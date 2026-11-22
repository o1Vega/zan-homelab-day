package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"example.com/zan-homelab-day/internal/ai"
	"example.com/zan-homelab-day/internal/atmosphere"
	"example.com/zan-homelab-day/internal/store"
)

// ---- the LLM-themed day ----
//
// A day (public post or private memory) is themed by the configured LLM:
// given the day's title and description it picks a palette, an ambient
// effect, an opening animation, a background, a short tagline, a mini-story
// developed from the text, and a seal. The result is cached in the day's
// folder as theme.json, so the model is called once per day (or after ?force=1).
// Private days are themed automatically when they are created or rewritten.

// PostTheme is what the front end applies to the day's page.
type PostTheme struct {
	Mood       string   `json:"mood"`       // one short word, e.g. "cozy"
	Palette    []string `json:"palette"`    // 3-5 hex colors, [0] = accent
	Effect     string   `json:"effect"`     // sakura|hanabi|kintsugi|ma|rain|snow|embers|stars|none
	Intensity  float64  `json:"intensity"`  // 0..1, how present the effect is
	Background string   `json:"background"` // a CSS background value for the page
	Tagline    string   `json:"tagline"`    // a 3-7 word poetic line
	Story      string   `json:"story"`      // a 3-6 sentence mini-story in the day's language
	Opening    string   `json:"opening"`    // how the page breathes in: ink|petals|rise|fade|brush|wave|ma
	Seal       string   `json:"seal"`       // one kanji that fits the day
}

// allowedEffects is the closed set the model may pick (the front end renders
// exactly these layers).
var allowedEffects = map[string]bool{
	"sakura": true, "hanabi": true, "kintsugi": true, "ma": true,
	"rain": true, "snow": true, "embers": true, "stars": true, "none": true,
}

// allowedOpenings is the closed set of page-opening animations the model may
// pick; the front end renders exactly these, so each day gets its own one.
var allowedOpenings = map[string]bool{
	"ink": true, "petals": true, "rise": true, "fade": true,
	"brush": true, "wave": true, "ma": true,
}

// designClient returns the LLM client for post theming, or nil when unconfigured.
func (s *Server) designClient() *ai.Client {
	if s.cfg.AIDesignKey == "" {
		return nil
	}
	return ai.New(s.cfg.AIDesignBaseURL, s.cfg.AIDesignKey, s.cfg.AIDesignModel, "")
}

// handlePostTheme serves the theme of a shared (public) day.
func (s *Server) handlePostTheme(w http.ResponseWriter, r *http.Request) {
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad post id")
		return
	}
	m, err := s.store.GetMemoryByID(r.Context(), id)
	if err != nil || !m.IsPublic {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.serveTheme(w, r, m)
}

// handleMemoryTheme serves the theme of the owner's own day. Private days are
// themed automatically: when there is no theme yet, the model is called
// synchronously and the result is cached, so the page is unique per day from
// the first open.
func (s *Server) handleMemoryTheme(w http.ResponseWriter, r *http.Request) {
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	m, err := s.store.GetMemory(r.Context(), userFrom(r.Context()).ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	s.serveTheme(w, r, m)
}

// serveTheme answers a theme request for one day: cache → model → fallback.
func (s *Server) serveTheme(w http.ResponseWriter, r *http.Request, m *store.Memory) {
	dir, err := s.memoryFolder(m)
	if err != nil {
		s.fail(w, err, "open day folder")
		return
	}
	cache := filepath.Join(dir, "theme.json")
	if r.URL.Query().Get("force") != "1" {
		if b, rerr := os.ReadFile(cache); rerr == nil {
			var th PostTheme
			if jerr := json.Unmarshal(b, &th); jerr == nil {
				writeJSON(w, http.StatusOK, map[string]any{"theme": th, "cached": true})
				return
			}
		}
	}
	c := s.designClient()
	if c == nil {
		// the model is not configured: the measured art direction still themes the day
		writeJSON(w, http.StatusOK, map[string]any{"theme": fallbackTheme(m), "cached": false, "error": "the theming model is not configured (AI_DESIGN_KEY)"})
		return
	}
	th, usage, err := s.themeForDay(r.Context(), m)
	if err != nil {
		s.log.Warn("day theme", "memory", m.ID, "err", err)
		// fall back to the art direction already measured from the day
		writeJSON(w, http.StatusOK, map[string]any{"theme": fallbackTheme(m), "cached": false, "error": err.Error()})
		return
	}
	s.logUsage(r.Context(), m.UserID, "post_theme", usage)
	if b, merr := json.MarshalIndent(th, "", "  "); merr == nil {
		if werr := os.WriteFile(cache, b, 0o644); werr != nil {
			s.log.Warn("post theme cache", "err", werr)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"theme": th, "cached": false})
}

// generateThemeIfMissing kicks off a background theming right after a day was
// created or rewritten, so the page is already unique when the owner opens it.
// It never blocks or fails the request: when the owner's GET /theme arrives
// before the background job finishes, serveTheme rolls the theme synchronously
// (at worst one extra model call lands in the cache).
func (s *Server) generateThemeIfMissing(m *store.Memory) {
	if s.designClient() == nil {
		return
	}
	dir, err := s.memoryFolder(m)
	if err != nil {
		return
	}
	if _, rerr := os.Stat(filepath.Join(dir, "theme.json")); rerr == nil {
		return // already themed
	}
	mid, uid := m.ID, m.UserID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		th, usage, err := s.themeForDay(ctx, m)
		if err != nil {
			s.log.Warn("auto theme", "memory", mid, "err", err)
			return
		}
		s.logUsage(ctx, uid, "post_theme", usage)
		if b, merr := json.MarshalIndent(th, "", "  "); merr == nil {
			if werr := os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644); werr != nil {
				s.log.Warn("auto theme cache", "err", werr)
			}
		}
	}()
}

// themeForDay asks the model for a page theme that fits the day's words.
func (s *Server) themeForDay(ctx context.Context, m *store.Memory) (PostTheme, ai.Usage, error) {
	var ad atmosphere.ArtDirection
	if len(m.ArtDirection) > 0 {
		_ = json.Unmarshal(m.ArtDirection, &ad)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", m.Title)
	text := strings.TrimSpace(m.Text)
	if rn := []rune(text); len(rn) > 500 {
		text = string(rn[:500])
	}
	if text != "" {
		fmt.Fprintf(&b, "The day:\n%s\n", text)
	}
	if len(ad.Palette) > 0 {
		fmt.Fprintf(&b, "Colors measured from the day's photos: %s\n", strings.Join(ad.Palette, ", "))
	}
	if ad.Technique != "" {
		fmt.Fprintf(&b, "Art style: %s. Mood: %s.\n", ad.Technique, m.MoodCluster)
	}
	system := `You design the look of a single day's page in a private photo journal. From the day's words choose a palette, an opening animation, one ambient effect, a background, a tagline and a mini-story. Rules: colors must suit the described scene; the opening is the page's entrance animation and must be one of ink (ink spreading on paper), petals (a gentle drift in), rise (elements rising into place), fade (a slow breath of light), brush (a painted brushstroke wipe), wave (a soft wave of light), ma (a held silence before the page appears) — pick the one that best matches the day's mood, so that every day opens differently; the effect must be one of sakura, hanabi, kintsugi, ma, rain, snow, embers, stars, none; the background is a single CSS value (a soft, dark-friendly gradient is ideal); the tagline is a poetic line of 3 to 7 words written in the SAME language as the day's text (never translate it); the story is a short mini-story of 3 to 5 sentences written in the SAME language as the day's text: it is the day's own words remembered out loud, so use ONLY the people, places and objects that are already in the day's text, do not invent new events, in the first person, short simple sentences of at most 12 words each, sensory and concrete; the seal is exactly ONE single CJK ideograph (one kanji/han/한자 character, no spaces, no Latin letters, no punctuation) that captures the day. Answer with a single JSON object: {"mood": string, "palette": [3-5 hex colors], "effect": string, "intensity": number from 0 to 1, "background": css string, "tagline": string, "story": string, "opening": string, "seal": one_cjk_character}. No comments, no markdown, no text outside the JSON.`
	out, usage, err := s.designClient().Complete(ctx, system, b.String(), true)
	if err != nil {
		return PostTheme{}, usage, err
	}
	th, usage, err := themeFromJSON(out, usage)
	if err != nil {
		return PostTheme{}, usage, err
	}
	// kazllm tends to answer in English; the tagline and the story must stay
	// in the day's language, so when the model drifted away from a Cyrillic
	// day, keep the words that are already in that language.
	if containsCyrillic(m.Text) {
		if containsLatinWord(th.Tagline, 2) {
			th.Tagline = ad.Caption
		}
		if storyDrifts(m.Text, th.Story) {
			story := m.Text
			if rn := []rune(story); len(rn) > 900 {
				story = string(rn[:900])
			}
			th.Story = story
		}
	}
	// when the model did not manage to write a tagline, the day's own first
	// words do the job — they are already in the right language
	if strings.TrimSpace(th.Tagline) == "" {
		if ad.Caption != "" {
			th.Tagline = ad.Caption
		} else {
			th.Tagline = firstSentence(m.Text, 60)
		}
	}
	return th, usage, nil
}

// themeWire mirrors PostTheme but reads intensity as raw JSON: the model
// sometimes sends it as a string ("0.8", "high") and a strict float would
// discard the whole theme.
type themeWire struct {
	Mood       string          `json:"mood"`
	Palette    []string        `json:"palette"`
	Effect     string          `json:"effect"`
	Intensity  json.RawMessage `json:"intensity"`
	Background string          `json:"background"`
	Tagline    string          `json:"tagline"`
	Story      string          `json:"story"`
	Opening    string          `json:"opening"`
	Seal       string          `json:"seal"`
}

func (w *themeWire) toTheme() PostTheme {
	return PostTheme{
		Mood: w.Mood, Palette: w.Palette, Effect: w.Effect,
		Intensity: parseFloatLoose(w.Intensity),
		Background: w.Background, Tagline: w.Tagline,
		Story: w.Story, Opening: w.Opening, Seal: w.Seal,
	}
}

// parseFloatLoose reads a 0..1 number that the model may have sent as a JSON
// number, a quoted number or a word; it falls back to a middle value.
func parseFloatLoose(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0.5
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		for _, r := range s {
			if r >= '0' && r <= '9' || r == '.' || r == '-' {
				// accumulate the numeric part of words like "0.8 strong"
				var num []byte
				seen := false
				for _, c := range []byte(s) {
					if c >= '0' && c <= '9' || c == '.' || (c == '-' && len(num) == 0) {
						num = append(num, c)
						seen = true
					} else if seen {
						break
					}
				}
				if len(num) > 0 {
					if v, err := strconv.ParseFloat(string(num), 64); err == nil {
						return v
					}
				}
			}
		}
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "high", "strong", "full":
			return 0.9
		case "medium", "mid", "medium_high":
			return 0.6
		case "low", "soft", "subtle", "light":
			return 0.3
		}
	}
	return 0.5
}

// themeFromJSON parses the model answer, clamping fields to the closed set.
func themeFromJSON(out string, usage ai.Usage) (PostTheme, ai.Usage, error) {
	var w themeWire
	if err := unmarshalLooseJSON(out, &w); err != nil {
		return PostTheme{}, usage, fmt.Errorf("model returned no JSON object: %w", err)
	}
	th := w.toTheme()
	if !allowedEffects[th.Effect] || th.Effect == "" {
		th.Effect = "none"
	}
	if len(th.Palette) > 5 {
		th.Palette = th.Palette[:5]
	}
	for i, p := range th.Palette {
		if !hexColorRe.MatchString(p) {
			th.Palette = append(th.Palette[:i], th.Palette[i+1:]...)
			i--
		}
	}
	if len(th.Palette) == 0 {
		th.Palette = []string{"#8a7a66"}
	}
	if th.Intensity < 0 {
		th.Intensity = 0
	}
	if th.Intensity > 1 {
		th.Intensity = 1
	}
	th.Seal = firstCJK(th.Seal)
	if th.Seal == "" {
		th.Seal = "残"
	}
	if !allowedOpenings[th.Opening] || th.Opening == "" {
		th.Opening = "fade"
	}
	th.Story = strings.TrimSpace(th.Story)
	// a story must stay a story: hard cap so a chatty model cannot blow the page
	if rn := []rune(th.Story); len(rn) > 900 {
		th.Story = string(rn[:900])
	}
	lb := strings.ToLower(strings.TrimRight(strings.TrimSpace(th.Background), "; \t"))
	if !strings.Contains(lb, "gradient") && !strings.HasPrefix(lb, "#") && !strings.HasPrefix(lb, "rgb") {
		th.Background = "radial-gradient(120% 90% at 50% 0%, #1b1b26 0%, #101018 60%, #0b0b12 100%)"
	} else {
		th.Background = lb
	}
	return th, usage, nil
}

var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// firstCJK extracts the first single CJK ideograph from the model's seal
// answer; empty string when it returned nothing usable.
func firstCJK(s string) string {
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF, // CJK unified
			r >= 0x3400 && r <= 0x4DBF, // extension A
			r >= 0x3040 && r <= 0x30FF, // kana
			r >= 0xAC00 && r <= 0xD7AF: // hangul
			return string(r)
		}
	}
	return ""
}

// unmarshalLooseJSON parses a JSON object from model output, tolerating
// surrounding prose and code fences.
func unmarshalLooseJSON(out string, dst any) error {
	s := strings.TrimSpace(out)
	if s == "" {
		return fmt.Errorf("empty model answer")
	}
	if start := strings.IndexByte(s, '{'); start >= 0 {
		if end := strings.LastIndexByte(s, '}'); end > start {
			s = s[start : end+1]
		}
	}
	return json.Unmarshal([]byte(s), dst)
}

// fallbackTheme derives a theme from the already-measured art direction when
// the model is unreachable: its palette, an effect per mood cluster, and an
// opening chosen from the day's date so every day still opens its own way.
func fallbackTheme(m *store.Memory) PostTheme {
	var ad atmosphere.ArtDirection
	if len(m.ArtDirection) > 0 {
		_ = json.Unmarshal(m.ArtDirection, &ad)
	}
	pal := ad.Palette
	if len(pal) == 0 {
		pal = []string{"#8a7a66"}
	}
	bg := fmt.Sprintf("radial-gradient(120%% 90%% at 50%% 0%%, %s33 0%%, #101018 55%%, #0b0b12 100%%)", pal[0])
	e := "ma"
	switch strings.ToLower(m.MoodCluster) {
	case "rainy_calm", "cold_clear":
		e = "rain"
	case "deep_focus", "quiet_warm":
		e = "embers"
	case "bright_gentle":
		e = "sakura"
	}
	seed := 0
	for _, r := range m.Date {
		seed = seed*31 + int(r)
	}
	openings := []string{"ink", "petals", "rise", "fade", "brush", "wave", "ma"}
	story := m.Text
	if rn := []rune(story); len(rn) > 900 {
		story = string(rn[:900])
	}
	return PostTheme{Mood: m.MoodCluster, Palette: pal, Effect: e, Intensity: 0.5, Background: bg, Tagline: ad.Caption, Story: story, Opening: openings[seed%len(openings)], Seal: "残"}
}

// containsCyrillic reports whether s has any Cyrillic letter.
func containsCyrillic(s string) bool {
	for _, r := range s {
		if r >= 0x0400 && r <= 0x04FF {
			return true
		}
	}
	return false
}

// containsCJK reports whether s has any CJK ideograph.
func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3040 && r <= 0x30FF) {
			return true
		}
	}
	return false
}

// containsLatinWord reports whether s has at least n words made of Latin
// letters (a sign the model answered in a Latin-script language).
func containsLatinWord(s string, n int) bool {
	words := 0
	in := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			if !in {
				words++
				if words >= n {
					return true
				}
			}
			in = true
		} else {
			in = false
		}
	}
	return false
}

// storyDrifts reports whether the story has drifted into another language:
// a small model sometimes writes a Latin-script word that does not occur in
// the day's own text (a Russian story with "aylık", for instance). Any such
// word is drift: the story then reverts to the day's own words.
func storyDrifts(source, s string) bool {
	src := strings.ToLower(source)
	for _, word := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || (r >= '0' && r <= '9'))
	}) {
		if len(word) < 3 {
			continue
		}
		if !strings.Contains(src, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

// firstSentence returns the first sentence of the day's text (capped), for
// the cases when the model did not manage to write a tagline.
func firstSentence(s string, cap int) string {
	s = strings.TrimSpace(s)
	if rn := []rune(s); len(rn) > cap {
		return string(rn[:cap])
	}
	cut := len(s)
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			cut = i + 1
			break
		}
	}
	return s[:cut]
}

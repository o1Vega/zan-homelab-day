// Package atmosphere develops one day into an art direction: from the
// person's text, the photos' colors and the track's features it derives the
// mood cluster, a palette, a technique, a short title, a handwritten caption,
// the voice for the narration and the motion rhythm.
//
// The rules are fixed (the journal's visual language); the LLM only picks
// among them and explains the choice in reasons[], so every day can be
// accounted for: a slow minor track means more air and muted colors.
package atmosphere

import (
	"context"
	"fmt"
	"strings"

	"example.com/zan-homelab-day/internal/ai"
)

// MoodCluster is one of the four fixed moods of the journal's visual language.
type MoodCluster string

// The four moods. Each carries a named Japanese palette and a technique.
var (
	QuietWarm   MoodCluster = "quiet_warm"   // тёплый тихий вечер — Моне: мазки, свет, воздух
	TenseNight  MoodCluster = "tense_night"  // напряжённая ночь — Кандинский: жёсткая сетка, линии, контраст
	LightBright MoodCluster = "light_bright" // лёгкий день — японский минимализм: воздух, один акцент
	Melancholy  MoodCluster = "melancholy"   // ностальгия, дождь — приглушённый тон, дымка
)

// All is the fixed set, in prompt order.
var All = []MoodCluster{QuietWarm, TenseNight, LightBright, Melancholy}

// ValidCluster reports whether c is one of the four moods.
func ValidCluster(c string) bool {
	for _, x := range All {
		if string(x) == c {
			return true
		}
	}
	return false
}

// Palettes are the fixed palettes of the four moods: five colors each,
// the last is the vermillion accent used for the hanko seal.
var Palettes = map[MoodCluster][]string{
	QuietWarm:   {"#D8C8B8", "#F2E8D5", "#C97B4A", "#2B2B2B", "#E34234"},
	TenseNight:  {"#1E2A4A", "#16181D", "#6B7A8F", "#F5F6F8", "#E34234"},
	LightBright: {"#F2E8D5", "#F2B8C6", "#BFD9E8", "#2B2B2B", "#E34234"},
	Melancholy:  {"#7D8A80", "#C5C9C2", "#3A4A5A", "#1C1D21", "#E34234"},
}

// Techniques name the artistic treatment per mood, in the language the
// renderer (canvas) and the T2I prompt both understand.
var Techniques = map[MoodCluster]string{
	QuietWarm:   "monet",     // мягкие мазки, тёплый свет, приглушённые тона
	TenseNight:  "kandinsky", // жёсткая сетка, линии и пятна, контрастная типографика
	LightBright: "minimal",   // плоские формы, много воздуха, один акцент
	Melancholy:  "mist",      // дымка, blur, капли, приглушённый color-grade
}

// Voice is the narration's parameters, chosen by the mood and the track.
type Voice struct {
	Rate   float64 `json:"rate"`   // 0.6..1.2, slow for quiet days
	Pitch  float64 `json:"pitch"`  // 0.7..1.1
	Warmth string  `json:"warmth"` // soft | neutral | bright
}

// Motion is the rhythm of the rendered remembrance.
type Motion struct {
	FrameSec float64 `json:"frame_sec"` // one photo on screen, 4..16 seconds
	Grain    float64 `json:"grain"`     // film grain strength 0..1
}

// ArtDirection is what the LLM returns and what the renderer consumes.
type ArtDirection struct {
	Title     string      `json:"title"` // 3-7 words, the day's folder name
	Cluster   MoodCluster `json:"mood_cluster"`
	Palette   []string    `json:"palette"`   // five hex colors, fixed per cluster
	Technique string      `json:"technique"` // monet | kandinsky | minimal | mist
	Caption   string      `json:"caption"`   // handwritten line, 3-7 words
	Voice     Voice       `json:"voice"`
	Motion    Motion      `json:"motion"`
	Reasons   []string    `json:"reasons"` // why the day looks exactly this way
}

// Track is what the client measured from the audio (essentia.js) or typed in.
type Track struct {
	Title   string  `json:"title"`
	Artist  string  `json:"artist"`
	BPM     float64 `json:"bpm"`     // 0 when unknown
	Key     string  `json:"key"`     // e.g. "Am", "" when unknown
	Major   *bool   `json:"major"`   // major/minor, nil when unknown
	Energy  float64 `json:"energy"`  // 0..1, 0 when unknown
	Valence float64 `json:"valence"` // 0..1, 0 when unknown
}

// PhotoColors is one photo's dominant colors, normalized to 0..1.
type PhotoColors struct {
	Name   string    `json:"name"`
	Colors []float64 `json:"colors"` // [r,g,b] triples
}

// Input is everything the analysis gets.
type Input struct {
	Text     string        `json:"text"`
	Date     string        `json:"date"`
	Photos   []PhotoColors `json:"photos"`
	Track    Track         `json:"track"`
	Language string        `json:"language"` // the person's UI language for title/caption
	// Notes is text read off the day's photos (OCR): messages, signs,
	// labels — concrete material the direction may lean on.
	Notes string `json:"notes,omitempty"`
}

const systemPrompt = `You develop the atmosphere of one day into a remembrance for a personal art journal.
The journal has ONE visual language, shared by every day, so the archive reads as a series:
a 4:5 poster on washi paper, a vertical date (tategaki), a handwritten caption, a small vermillion hanko seal.
You do not invent; you translate what the person wrote, the photo colors and the track into the fixed language.

The four moods, fixed (pick exactly one):
- "quiet_warm" — a slow, warm, safe day (evenings, coffee, conversations, autumn parks). Palette: tsukihagi #D8C8B8, kinari #F2E8D5, kōchi #C97B4A, sumi #2B2B2B, vermillion #E34234. Technique "monet": soft brush strokes, warm light, air, muted tones.
- "tense_night" — a night of deadlines, rush, office, electronics. Palette: ai-iro #1E2A4A, sumi #16181D, steel #6B7A8F, white #F5F6F8, vermillion #E34234. Technique "kandinsky": hard grid, lines and patches, contrasting type, sharp rhythm.
- "light_bright" — a light, clear, small-joy day (walks, sunshine, simple pleasure). Palette: kinari #F2E8D5, sakura #F2B8C6, sky #BFD9E8, sumi #2B2B2B, vermillion #E34234. Technique "minimal": flat shapes, lots of negative space, one accent.
- "melancholy" — rain, nostalgia, farewell, grey afternoons. Palette: grey-green #7D8A80, mist #C5C9C2, indigo #3A4A5A, sumi #1C1D21, vermillion #E34234. Technique "mist": haze, blur, droplets, muted grade.

Fixed rules (apply them, and state each applied one in reasons):
- A slow minor track (bpm < 80 or key minor) adds air and mutes the palette: rate 0.8, grain up, frame_sec up to 12-14.
- A fast or energetic track (bpm > 110 or energy > 0.7) sharpens the grid and the cut: frame_sec 4-6, grain down, for tense days use kandinsky even if the text is short.
- Warm photo colors (orange/red/gold dominant) pull quiet_warm; cold blue-grey colors pull tense_night or melancholy; pink/sky/light colors pull light_bright.
- Voice follows the mood: quiet and warm → rate 0.82, pitch 0.9, warmth "soft"; tense → rate 1.05, pitch 0.95, warmth "neutral"; light → rate 1.0, pitch 1.05, warmth "bright"; melancholy → rate 0.88, pitch 0.85, warmth "soft".
- The title is the day's name in 3-7 words, concrete, in the person's language (given in the input), e.g. "Осенний парк и какао". No quotes, no date.
- The caption is one handwritten line, 3-7 words, in the person's language, the most sensory detail of the day.
- reasons: 2-4 short strings, in the person's language, each one = input signal → visual consequence.

Return ONE json object, no markdown:
{"title": "...", "mood_cluster": "quiet_warm", "palette": ["#...", "#...", "#...", "#...", "#..."],
 "technique": "monet", "caption": "...", "voice": {"rate": 0.82, "pitch": 0.9, "warmth": "soft"},
 "motion": {"frame_sec": 10, "grain": 0.35}, "reasons": ["..."]}`

// Analyze runs the fixed rules through the LLM and returns the direction.
// When the LLM is unavailable it degrades to a rule-based direction built
// from the same signals, so a day is never lost.
func Analyze(ctx context.Context, c *ai.Client, in Input) (ArtDirection, error) {
	if c != nil && c.Configured() {
		user := userPrompt(in)
		var out ArtDirection
		if _, err := c.JSON(ctx, systemPrompt, user, &out); err == nil && ValidCluster(string(out.Cluster)) {
			out.Palette = Palettes[out.Cluster]
			out.Technique = Techniques[out.Cluster]
			return out, nil
		}
	}
	return Fallback(in), nil
}

// userPrompt is the material the LLM translates.
func userPrompt(in Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Date: %s. The person's text about the day:\n\"%s\"\n", in.Date, strings.TrimSpace(in.Text))
	if len(in.Photos) > 0 {
		fmt.Fprintf(&b, "Photos: %d attached. Dominant colors (rgb 0..1): ", len(in.Photos))
		for i, p := range in.Photos {
			if i > 0 {
				b.WriteString(" | ")
			}
			fmt.Fprintf(&b, "%v", p.Colors)
		}
		b.WriteString("\n")
	}
	if n := strings.TrimSpace(in.Notes); n != "" {
		fmt.Fprintf(&b, "Text visible on the photos (read by OCR):\n\"%s\"\n", n)
	}
	t := in.Track
	if t.Title != "" || t.Artist != "" {
		fmt.Fprintf(&b, "Track: %s", t.Title)
		if t.Artist != "" {
			fmt.Fprintf(&b, " — %s", t.Artist)
		}
		if t.BPM > 0 {
			fmt.Fprintf(&b, ", %.0f bpm", t.BPM)
		}
		if t.Key != "" {
			mode := "major"
			if t.Major != nil && !*t.Major {
				mode = "minor"
			}
			fmt.Fprintf(&b, ", key %s (%s)", t.Key, mode)
		}
		if t.Energy > 0 {
			fmt.Fprintf(&b, ", energy %.2f", t.Energy)
		}
		if t.Valence > 0 {
			fmt.Fprintf(&b, ", valence %.2f", t.Valence)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Answer in the person's language for title and caption: %s.", in.Language)
	return b.String()
}

// Fallback derives a direction from the same signals without the LLM.
func Fallback(in Input) ArtDirection {
	cluster, reasons := classify(in)
	t := in.Track
	rate, pitch, warmth := 0.82, 0.9, "soft"
	frameSec, grain := 10.0, 0.35
	switch cluster {
	case TenseNight:
		rate, pitch, warmth = 1.05, 0.95, "neutral"
		frameSec, grain = 5, 0.2
	case LightBright:
		rate, pitch, warmth = 1.0, 1.05, "bright"
		frameSec, grain = 7, 0.3
	case Melancholy:
		rate, pitch, warmth = 0.88, 0.85, "soft"
		frameSec, grain = 11, 0.45
	}
	if t.BPM < 80 || (t.Major != nil && !*t.Major) {
		frameSec = 13
		reasons = append(reasons, "slow/minor track → more air and slower cuts")
	}
	if t.BPM > 110 || t.Energy > 0.7 {
		frameSec = 4
		grain = 0.15
		reasons = append(reasons, "fast/energetic track → sharper grid, quicker cuts")
	}
	return ArtDirection{
		Title:     fallbackTitle(in),
		Cluster:   cluster,
		Palette:   Palettes[cluster],
		Technique: Techniques[cluster],
		Caption:   fallbackCaption(in),
		Voice:     Voice{Rate: rate, Pitch: pitch, Warmth: warmth},
		Motion:    Motion{FrameSec: frameSec, Grain: grain},
		Reasons:   reasons,
	}
}

// classify runs the fixed rules on the signals.
func classify(in Input) (MoodCluster, []string) {
	var reasons []string
	t := in.Track
	text := strings.ToLower(in.Text)
	warm := anyWord(text, "тёпл", "тепл", "cozy", "warm", "осен", "autumn", "fall", "какао", "cocoa", "coffee", "чай", "walk", "парк", "парк", "друзья", "girlfriend", "бойфренд", "девушк", "парн")
	coldNight := anyWord(text, "ночь", "night", "office", "офис", "deadline", "сдач", "work", "work", "электрон", "electronic", "sprint", "смен")
	light := anyWord(text, "солн", "sun", "ярк", "bright", "walk", "прогулк", "море", "sea", "бег", "run", "bike", "велосип", "лёгк", "light", "smile", "улыб")
	rainy := anyWord(text, "дожд", "rain", "тоск", "nostalg", "прощан", "farewell", "сер", "grey", "gray", "хмур", "fog", "туман")

	// Audio signals.
	if t.Valence > 0 && t.Energy > 0 {
		if t.Energy < 0.3 && t.Valence < 0.4 {
			rainy = true
			reasons = append(reasons, "low-energy, low-valence track → rain and nostalgia")
		}
		if t.Energy > 0.7 {
			coldNight = coldNight || anyWord(text, "ночь", "night", "office", "офис")
		}
	}

	// Photo colors: warm vs cold dominance.
	warmPix, coldPix := 0, 0
	for _, p := range in.Photos {
		for i := 0; i+2 < len(p.Colors); i += 3 {
			r, _, b := p.Colors[i], p.Colors[i+1], p.Colors[i+2]
			if r > 0.45 && r > b+0.1 {
				warmPix++
			}
			if b > r+0.08 || (b > 0.4 && r < 0.45) {
				coldPix++
			}
		}
	}
	if warmPix > coldPix {
		warm = true
		reasons = append(reasons, "warm dominant photo colors → quiet_warm palette")
	} else if coldPix > warmPix {
		rainy = true
		reasons = append(reasons, "cold dominant photo colors → muted or night palette")
	}

	switch {
	case coldNight && !warm:
		return TenseNight, append(reasons, "night/rush signals in the text → hard grid and contrast")
	case rainy && !warm:
		return Melancholy, append(reasons, "rain/nostalgia signals → haze and muted tones")
	case warm:
		return QuietWarm, append(reasons, "warm, safe, slow signals → brush strokes and light")
	case light:
		return LightBright, append(reasons, "light and open signals → negative space and one accent")
	}
	// Default: the text decides by valence of the track, else a calm day.
	if t.Valence > 0 && t.Valence < 0.4 {
		return Melancholy, append(reasons, "the track's valence is low → muted tones")
	}
	return LightBright, append(reasons, "no strong signal → the light, open default")
}

func anyWord(s string, words ...string) bool {
	for _, w := range words {
		if w != "" && strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// fallbackTitle is a short day name from the text's first concrete line.
func fallbackTitle(in Input) string {
	for _, line := range strings.Split(in.Text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len([]rune(line)) > 70 {
			line = string([]rune(line)[:70])
		}
		return line
	}
	if in.Date != "" {
		return in.Date
	}
	return "Один день"
}

// fallbackCaption is the shortest sensory line of the text.
func fallbackCaption(in Input) string {
	best := ""
	for _, line := range strings.FieldsFunc(in.Text, func(r rune) bool { return r == '\n' || r == '.' || r == '!' || r == '?' }) {
		line = strings.TrimSpace(line)
		if n := len([]rune(line)); n >= 6 && (best == "" || n < len([]rune(best))) {
			best = line
		}
	}
	if best != "" {
		if len([]rune(best)) > 60 {
			best = string([]rune(best)[:60])
		}
		return best
	}
	return in.Date
}

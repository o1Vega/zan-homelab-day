package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/zan-homelab-day/internal/ai"
	"example.com/zan-homelab-day/internal/store"
)

// aiClient returns the configured client, or nil.
func (s *Server) aiClient(ctx context.Context, u *store.User, st store.Settings) *ai.Client {
	c := ai.New(st.AIBaseURL, st.AIKey, st.AIModel, st.AITranscribeModel)
	c.ImageKey = st.AIImageKey
	c.TranscribeKey = st.AITranscribeKey
	c.OCRKey = st.AIOCRKey
	c.RerankKey = st.AIRerankKey
	c.TTSKey = st.AITTSKey
	c.ImageModel = st.AIImageModel
	c.OCRModel = st.AIOCRModel
	c.RerankModel = st.AIRerankModel
	c.TTSModel = st.AITTSModel
	if !c.Configured() {
		return nil
	}
	return c
}

// aiOrFail returns settings and a client, or writes 400.
func (s *Server) aiOrFail(w http.ResponseWriter, r *http.Request) (store.Settings, *ai.Client, bool) {
	st, _ := s.store.GetSettings(r.Context(), userFrom(r.Context()).ID)
	c := s.aiClient(r.Context(), userFrom(r.Context()), st)
	if c == nil {
		writeErr(w, http.StatusBadRequest, "add an OpenAI-compatible key under Settings → AI")
		return st, nil, false
	}
	return st, c, true
}

func (s *Server) logUsage(ctx context.Context, userID int64, feature string, u ai.Usage) {
	s.store.LogUsage(ctx, userID, feature, u.Model, u.PromptTokens, u.CompletionTokens)
}

const voice = `You help someone keep a private daily journal. The material is what they wrote themselves, the moments they noted and the photos they attached. Rules: use only what is in the material; never invent people, places, feelings or reasons; prefer the concrete over the generic; no therapy-speak, no exclamation marks, no emojis, no motivational tone. Times are local.`

// ---- draft the day ----

func (s *Server) handleAIDraft(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok || date > store.Today() {
		writeErr(w, http.StatusBadRequest, "pick a day that has happened")
		return
	}
	st, c, ok := s.aiOrFail(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	u := userFrom(ctx)
	brief := s.dayBrief(ctx, u, date, st.AISendText, true, 2000)
	if !hasContent(brief) {
		writeErr(w, http.StatusBadRequest, "there is nothing about this day yet to draft from")
		return
	}
	sys := voice + `
Task: draft the person's journal entry for the day in the FIRST PERSON, past tense, as if they wrote it that evening. 60–160 words, two short paragraphs at most. Weave the three to five most telling things into sentences (what the moments mention, a photo they attached, a place, a small scene); do not list every detail. Never state how something felt, or why something happened, unless their own words say so — describe what happened and leave the feelings to them. If there is little material, write two or three sentences.
If the material contains "Wrote:", the person has already written; then "text" must contain ONLY new sentences about what their text does not cover (it will be appended below theirs) — never paraphrase or repeat what they wrote; return an empty "text" if nothing is missing.
Also propose a one-line highlight (max 12 words) that names the single thing the day will be remembered by.
` + s.languageRule(ctx, u, st) + `
Reply as JSON: {"highlight": "...", "text": "..."}`
	var out struct {
		Highlight string `json:"highlight"`
		Text      string `json:"text"`
	}
	usage, err := c.JSON(ctx, sys, brief, &out)
	s.logUsage(ctx, u.ID, "draft", usage)
	out.Text = strings.TrimSpace(out.Text)
	out.Highlight = strings.TrimSpace(out.Highlight)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- tonight's question ----

// aiQuestion returns a question about the day, cached for three hours on
// today and for good on older days.
func (s *Server) aiQuestion(ctx context.Context, u *store.User, date string, force bool) (string, error) {
	st, _ := s.store.GetSettings(ctx, u.ID)
	c := s.aiClient(ctx, u, st)
	if c == nil {
		return "", errors.New("AI is not configured")
	}
	key := fmt.Sprintf("aiq:%d:%s", u.ID, date)
	if !force {
		if v, at, ok := s.store.CacheGet(ctx, key); ok && v != "" {
			fresh := date != store.Today()
			if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 3*time.Hour {
				fresh = true
			}
			if fresh {
				return v, nil
			}
		}
	}
	brief := s.dayBrief(ctx, u, date, st.AISendText, true, 800)
	if !hasContent(brief) {
		return "", nil
	}
	sys := voice + `
Task: ask ONE question that would help the person write about this day. Pick exactly one specific thing from the material — the most unusual or human one (a place they went, a small scene, a moment they noted, a photo they attached) — and invite a story, not a yes/no. Do not combine two topics, do not quote clock times or numbers. Max 18 words. No greeting, no preamble, no quotation marks — just the question.
` + s.languageRule(ctx, u, st)
	q, usage, err := c.Complete(ctx, sys, brief, false)
	s.logUsage(ctx, u.ID, "question", usage)
	if err != nil {
		return "", err
	}
	q = strings.Trim(strings.TrimSpace(q), `"“”`)
	_ = s.store.CachePut(ctx, key, q)
	return q, nil
}

func (s *Server) handleAIQuestion(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok || date > store.Today() {
		writeErr(w, http.StatusBadRequest, "pick a day that has happened")
		return
	}
	if _, _, ok := s.aiOrFail(w, r); !ok {
		return
	}
	q, err := s.aiQuestion(r.Context(), userFrom(r.Context()), date, queryBool(r, "refresh"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"question": q})
}

// ---- highlight / tags / mood from what was written ----

func (s *Server) handleAISuggest(w http.ResponseWriter, r *http.Request) {
	if _, ok := dateParam(r); !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	st, c, ok := s.aiOrFail(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		Highlight string `json:"highlight"`
		Text      string `json:"text"`
	}
	_ = decode(r, &in)
	if strings.TrimSpace(in.Text+in.Highlight) == "" {
		writeErr(w, http.StatusBadRequest, "write something first; suggestions come from your words")
		return
	}
	tags, _ := s.store.Tags(ctx, u.ID)
	var vocab []string
	for i, t := range tags {
		if i >= 40 {
			break
		}
		vocab = append(vocab, t.Tag)
	}
	user := "What they wrote:\nHighlight: " + in.Highlight + "\nText: " + in.Text
	sys := voice + `
Task: from what the person WROTE, suggest:
- "highlight": the one line the day will be remembered by, max 12 words, in their words where possible (keep theirs if they gave one and it fits);
- "tags": 1–5 short lower-case tags; reuse these existing tags when they fit: ` + strings.Join(vocab, ", ") + `;
- "mood": 1 (rough) to 5 (great), judged from the writing only, 0 if it gives no clue;
- "why": max 15 words on the mood.
` + s.languageRule(ctx, u, st) + ` Tags stay in the language of the existing tags.
Reply as JSON: {"highlight": "...", "tags": ["..."], "mood": 0, "why": "..."}`
	var out struct {
		Highlight string   `json:"highlight"`
		Tags      []string `json:"tags"`
		Mood      int      `json:"mood"`
		Why       string   `json:"why"`
	}
	usage, err := c.JSON(ctx, sys, user, &out)
	s.logUsage(ctx, u.ID, "suggest", usage)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if out.Mood < 0 || out.Mood > 5 {
		out.Mood = 0
	}
	out.Tags = strings.Split(store.NormalizeTags(strings.Join(out.Tags, ",")), ",")
	if len(out.Tags) == 1 && out.Tags[0] == "" {
		out.Tags = []string{}
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- dictation ----

// handleAITranscribe writes out a recording through the person's OpenAI endpoint.
func (s *Server) handleAITranscribe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	c := ai.New(st.AIBaseURL, st.AIKey, st.AIModel, st.AITranscribeModel)
	c.TranscribeKey = st.AITranscribeKey
	if c.Key == "" {
		writeErr(w, 400, "Dictation needs your OpenAI key in Settings → AI")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "send the recording as multipart field “file” (max 25 MB)")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no file")
		return
	}
	defer f.Close()
	buf := make([]byte, 0, hdr.Size)
	tmp := make([]byte, 32<<10)
	for {
		n, err := f.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	name := hdr.Filename
	if name == "" || !strings.Contains(name, ".") {
		name = "dictation.webm"
	}
	text, err := c.Transcribe(ctx, buf, name, r.FormValue("language"))
	s.logUsage(ctx, u.ID, "dictation", ai.Usage{Model: c.TranscribeModel})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

// ---- stories ----

// periodRange turns a story period into dates: week 2026-W42, month 2026-10, year 2026.
func periodRange(kind, period string) (string, string, string, error) {
	switch kind {
	case "week":
		var y, wk int
		if _, err := fmt.Sscanf(period, "%d-W%d", &y, &wk); err != nil || wk < 1 || wk > 53 {
			return "", "", "", errors.New("week must look like 2026-W42")
		}
		jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
		off := int(jan4.Weekday())
		if off == 0 {
			off = 7
		}
		mon := jan4.AddDate(0, 0, 1-off+(wk-1)*7)
		from, to := mon.Format("2006-01-02"), mon.AddDate(0, 0, 6).Format("2006-01-02")
		return from, to, fmt.Sprintf("Week %d, %s – %s", wk, mon.Format("2 Jan"), mon.AddDate(0, 0, 6).Format("2 Jan 2006")), nil
	case "month":
		t, err := time.Parse("2006-01", period)
		if err != nil {
			return "", "", "", errors.New("month must look like 2026-10")
		}
		return t.Format("2006-01-02"), t.AddDate(0, 1, -1).Format("2006-01-02"), t.Format("January 2006"), nil
	case "year":
		y, err := strconv.Atoi(period)
		if err != nil || y < 1970 || y > 2200 {
			return "", "", "", errors.New("year must look like 2026")
		}
		return period + "-01-01", period + "-12-31", period, nil
	}
	return "", "", "", errors.New("kind is week, month or year")
}

// currentWeek returns the ISO week label of a date.
func currentWeek(date string) string {
	t, _ := store.ParseDate(date)
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

func (s *Server) handleGetStory(w http.ResponseWriter, r *http.Request) {
	kind, period := chiParam(r, "kind"), chiParam(r, "period")
	if _, _, _, err := periodRange(kind, period); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := s.store.GetStory(r.Context(), userFrom(r.Context()).ID, kind, period)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"story": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"story": st})
}

func (s *Server) handleListStories(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.ListStories(r.Context(), userFrom(r.Context()).ID, queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "stories")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteStory(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteStory(r.Context(), userFrom(r.Context()).ID, chiParam(r, "kind"), chiParam(r, "period")); err != nil {
		s.fail(w, err, "delete story")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMakeStory(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.aiOrFail(w, r); !ok {
		return
	}
	story, err := s.makeStory(r.Context(), userFrom(r.Context()), chiParam(r, "kind"), chiParam(r, "period"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"story": story})
}

// makeStory writes (and stores) the story of a week, a month or a year.
func (s *Server) makeStory(ctx context.Context, u *store.User, kind, period string) (*store.Story, error) {
	from, to, label, err := periodRange(kind, period)
	if err != nil {
		return nil, err
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	c := s.aiClient(ctx, u, st)
	if c == nil {
		return nil, errors.New("AI is not configured")
	}
	today := store.Today()
	if from > today {
		return nil, errors.New("that period has not started yet")
	}
	if to > today {
		to = today
	}
	var material strings.Builder
	words := "180–320 words"
	switch kind {
	case "week", "month":
		maxText := 700
		if kind == "month" {
			maxText = 280
		}
		n := 0
		for d := from; d <= to; d = store.AddDays(d, 1) {
			b := s.dayBrief(ctx, u, d, st.AISendText, kind == "week", maxText)
			if hasContent(b) {
				material.WriteString(b + "\n")
				n++
			}
		}
		if n == 0 {
			return nil, errors.New("there is nothing recorded in that period yet")
		}
		if kind == "week" {
			words = "120–220 words"
		}
	case "year":
		y, _ := strconv.Atoi(period)
		rec, err := s.buildYear(ctx, u, y)
		if err != nil {
			return nil, err
		}
		if rec.Entries == 0 {
			return nil, errors.New("there is nothing recorded in that year yet")
		}
		material.WriteString(yearMaterial(rec))
		entries, _, _ := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, From: from, To: to, Limit: 400, Asc: true})
		if len(entries) > 0 {
			material.WriteString("\n# The days, one line each\n")
			for _, e := range entries {
				line := e.Date
				if e.Mood > 0 {
					line += " · " + moodWords[e.Mood]
				}
				if st.AISendText {
					if e.Highlight != "" {
						line += " · " + e.Highlight
					} else if e.Text != "" {
						line += " · " + clip(e.Text, 120)
					}
				}
				if e.Pinned {
					line += " ★"
				}
				material.WriteString(line + "\n")
			}
		}
		words = "350–550 words"
	}
	sys := voice + `
Task: write the story of ` + label + ` for the person, in the SECOND person ("you"), warm but plain, like a thoughtful friend who read their journal. ` + words + `, in short paragraphs, no headings, no bullet lists. Lead with what defined the period (a run of good days, a hard stretch, a moment they keep returning to), then the texture (the places, scenes and moods the entries mention) and end with one quiet observation. Mention specific days by weekday or date when it helps. Also give a short title (max 8 words) that is specific to this period, not generic.
` + s.languageRule(ctx, u, st) + `
Reply as JSON: {"title": "...", "text": "..."}`
	var out struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	usage, err := c.JSON(ctx, sys, material.String(), &out)
	s.logUsage(ctx, u.ID, "story-"+kind, usage)
	if err != nil {
		return nil, err
	}
	story := &store.Story{UserID: u.ID, Kind: kind, Period: period, Title: strings.TrimSpace(out.Title), Text: strings.TrimSpace(out.Text), Model: usage.Model}
	if err := s.store.PutStory(ctx, story); err != nil {
		return nil, err
	}

	return story, nil
}

func yearMaterial(rec *yearRecap) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %d in numbers\nDays written: %d, words %d, moments %d, average mood %.1f, longest streak %d\n", rec.Year, rec.Entries, rec.Words, rec.Moments, rec.Mood, rec.Longest)
	var top []string
	for i, t := range rec.Tags {
		if i >= 10 {
			break
		}
		top = append(top, "#"+t.Tag)
	}
	if len(top) > 0 {
		fmt.Fprintf(&b, "Most used tags: %s\n", strings.Join(top, ", "))
	}
	if len(rec.Best) > 0 {
		var best []string
		for i, e := range rec.Best {
			if i >= 6 {
				break
			}
			line := e.Date
			if e.Highlight != "" {
				line += " — " + e.Highlight
			} else if e.Text != "" {
				line += " — " + clip(e.Text, 80)
			}
			best = append(best, line)
		}
		fmt.Fprintf(&b, "Brightest days: %s\n", strings.Join(best, " | "))
	}
	if len(rec.Pinned) > 0 {
		var pin []string
		for i, e := range rec.Pinned {
			if i >= 6 {
				break
			}
			line := e.Date
			if e.Highlight != "" {
				line += " — " + e.Highlight
			}
			pin = append(pin, line)
		}
		fmt.Fprintf(&b, "Pinned: %s\n", strings.Join(pin, " | "))
	}
	b.WriteString("Per month (days written · moments · mood):\n")
	for _, m := range rec.Months {
		if m.Entries == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s: %d · %d · %.1f\n", m.Month, m.Entries, m.Moments, m.Mood)
	}
	return b.String()
}

// ---- ask your journal ----

var dateRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	st, c, ok := s.aiOrFail(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		Question string `json:"question"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Question) == "" {
		writeErr(w, http.StatusBadRequest, "ask a question")
		return
	}
	q := strings.TrimSpace(in.Question)
	today := store.Today()
	first := today
	if stt := s.store.Stats(ctx, u.ID, today); stt.FirstDate != "" {
		first = stt.FirstDate
	}
	tags, _ := s.store.Tags(ctx, u.ID)
	var vocab []string
	for i, t := range tags {
		if i < 40 {
			vocab = append(vocab, t.Tag)
		}
	}
	// 1. plan: which days and which words to look for
	planSys := `You turn a question about someone's personal journal into a search plan. Today is ` + today + ` (` + time.Now().Weekday().String() + `). The journal covers ` + first + ` to ` + today + `. It holds what they wrote, the moments they noted and the photos they attached. Their tags: ` + strings.Join(vocab, ", ") + `.
Return JSON {"from":"YYYY-MM-DD","to":"YYYY-MM-DD","keywords":["..."]}.
- from/to: the narrowest range that surely contains the answer. "last week" = the 7 days before today; "this year" = Jan 1 to today; "ever", "last time", "how often" with no period = the whole journal.
- keywords: 0–6 lower-case words or names likely to appear literally in the text (names, places, moments, tags; include both the English and the person's own-language form when they differ). No keywords when the question is about the period in general.`
	var plan struct {
		From     string   `json:"from"`
		To       string   `json:"to"`
		Keywords []string `json:"keywords"`
	}
	usage, err := c.JSON(ctx, planSys, q, &plan)
	s.logUsage(ctx, u.ID, "ask-plan", usage)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if !validDate(plan.From) || plan.From < first {
		plan.From = first
	}
	if !validDate(plan.To) || plan.To > today {
		plan.To = today
	}
	if plan.From > plan.To {
		plan.From, plan.To = plan.To, plan.From
	}
	var kws []string
	for _, k := range plan.Keywords {
		if k = strings.ToLower(strings.TrimSpace(k)); len([]rune(k)) >= 2 {
			kws = append(kws, k)
		}
	}
	// 2. gather: keyword hits get a detailed brief, the rest an overview line
	material, cited := s.askMaterial(ctx, u, st, plan.From, plan.To, kws)
	scope := plan.From + " … " + plan.To
	if len(kws) > 0 {
		scope += " · " + strings.Join(kws, ", ")
	}
	if material == "" {
		a := &store.Ask{UserID: u.ID, Question: q, Answer: "There is nothing recorded between " + plan.From + " and " + plan.To + " to answer that from.", Scope: scope, Model: c.Model}
		_ = s.store.CreateAsk(ctx, a)
		writeJSON(w, http.StatusOK, a)
		return
	}
	ansSys := voice + `
Task: answer the person's question about their own life from the journal material below (today is ` + today + `). Be direct and brief: one to five sentences, or a short list when they ask for several things or counts. Count carefully when asked "how many" — count the days in the material, and say if the material may be incomplete. Cite every day you rely on as [YYYY-MM-DD] (cite days, not the edges of the search window). Always speak to the person as "you" and never about yourself. If the material does not contain the answer, say so plainly and, if useful, name the closest thing you did find. Address them as "you".
` + s.languageRule(ctx, u, st) + ` (Answer in the language of the question if it differs.)
Reply as JSON: {"answer": "...", "dates": ["YYYY-MM-DD", ...]}`
	var ans struct {
		Answer string   `json:"answer"`
		Dates  []string `json:"dates"`
	}
	usage, err = c.JSON(ctx, ansSys, "Question: "+q+"\n\nSearch window: "+scope+"\n\n"+material, &ans)
	s.logUsage(ctx, u.ID, "ask", usage)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	seen := map[string]bool{}
	var dates []string
	for _, d := range append(ans.Dates, dateRe.FindAllString(ans.Answer, -1)...) {
		if validDate(d) && !seen[d] && cited[d] {
			seen[d] = true
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	a := &store.Ask{UserID: u.ID, Question: q, Answer: strings.TrimSpace(ans.Answer), Dates: dates, Scope: scope, Model: usage.Model}
	if err := s.store.CreateAsk(ctx, a); err != nil {
		s.fail(w, err, "save answer")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// askMaterial builds the context for an answer and the set of dates it shows.
func (s *Server) askMaterial(ctx context.Context, u *store.User, st store.Settings, from, to string, kws []string) (string, map[string]bool) {
	const budget = 90000
	cited := map[string]bool{}
	entries, _, _ := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, From: from, To: to, Limit: 100000, Asc: true})
	byDate := map[string]store.Entry{}
	for _, e := range entries {
		byDate[e.Date] = e
	}
	moments, _ := s.store.ListMoments(ctx, u.ID, from, to, 100000)
	momentsBy := map[string][]string{}
	for _, m := range moments {
		momentsBy[m.Date] = append(momentsBy[m.Date], strings.ToLower(m.Text))
	}
	// days that mention a keyword anywhere
	var hits []string
	if len(kws) > 0 {
		for d := from; d <= to; d = store.AddDays(d, 1) {
			var hay strings.Builder
			if e, ok := byDate[d]; ok {
				if st.AISendText {
					hay.WriteString(e.Highlight + " " + e.Text + " ")
				}
				hay.WriteString(strings.ReplaceAll(e.Tags, ",", " ") + " ")
			}
			if st.AISendText {
				hay.WriteString(strings.Join(momentsBy[d], " ") + " ")
			}
			h := strings.ToLower(hay.String())
			for _, k := range kws {
				if strings.Contains(h, k) {
					hits = append(hits, d)
					break
				}
			}
		}
	}
	var b strings.Builder
	if len(hits) > 0 {
		if len(hits) > 60 { // keep the most recent
			hits = hits[len(hits)-60:]
		}
		fmt.Fprintf(&b, "# Days that mention %s (%d days)\n", strings.Join(kws, " / "), len(hits))
		for _, d := range hits {
			brief := s.dayBrief(ctx, u, d, st.AISendText, true, 1200)
			if b.Len()+len(brief) > budget*2/3 {
				break
			}
			b.WriteString(brief + "\n")
			cited[d] = true
		}
	} else if len(kws) > 0 {
		fmt.Fprintf(&b, "# No day in the window mentions %s literally.\n\n", strings.Join(kws, " / "))
	}
	// overview of the window: one line per day that has anything
	days := 0
	for d := from; d <= to; d = store.AddDays(d, 1) {
		days++
	}
	detailed := days <= 21 && len(hits) == 0
	var ov strings.Builder
	n := 0
	for d := to; d >= from; d = store.AddDays(d, -1) { // newest first so the budget keeps recent days
		if detailed {
			brief := s.dayBrief(ctx, u, d, st.AISendText, true, 1500)
			if hasContent(brief) {
				ov.WriteString(brief + "\n")
				cited[d] = true
				n++
			}
			continue
		}
		e, hasE := byDate[d]
		if !hasE {
			continue
		}
		if !hasE && days > 200 { // long windows: only written days in the overview
			continue
		}
		line := d
		if hasE {
			if e.Mood > 0 {
				line += " · mood " + moodWords[e.Mood]
			}
			if st.AISendText && e.Highlight != "" {
				line += " · “" + e.Highlight + "”"
			} else if st.AISendText && e.Text != "" {
				line += " · “" + clip(e.Text, 100) + "”"
			}
			if e.Tags != "" {
				line += " · #" + strings.ReplaceAll(e.Tags, ",", " #")
			}
		}
		if b.Len()+ov.Len()+len(line) > budget {
			break
		}
		ov.WriteString(line + "\n")
		cited[d] = true
		n++
	}
	if n > 0 {
		fmt.Fprintf(&b, "# Every recorded day in the window, newest first (%d days)\n%s", n, ov.String())
	}
	return strings.TrimSpace(b.String()), cited
}

func (s *Server) handleListAsks(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.ListAsks(r.Context(), userFrom(r.Context()).ID, queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "asks")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteAsk(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteAsk(r.Context(), userFrom(r.Context()).ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- usage ----

// handleAIUsage is the caller's own token use (each person brings their own key).
func (s *Server) handleAIUsage(w http.ResponseWriter, r *http.Request) {
	uid := userFrom(r.Context()).ID
	month, _ := s.store.Usage(r.Context(), uid, time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339))
	all, _ := s.store.Usage(r.Context(), uid, "0")
	writeJSON(w, http.StatusOK, map[string]any{"last_30_days": month, "all_time": all})
}

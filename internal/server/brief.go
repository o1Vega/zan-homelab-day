package server

import (
	"context"
	"fmt"
	"strings"

	"example.com/zan-homelab-day/internal/store"
)

var moodWords = []string{"", "rough", "meh", "okay", "good", "great"}

// dayBrief is one day for the AI: the user's words (when allowed), the
// moments they noted and the things they kept.
func (s *Server) dayBrief(ctx context.Context, u *store.User, date string, sendText, detail bool, maxText int) string {
	var b strings.Builder
	t, _ := store.ParseDate(date)
	fmt.Fprintf(&b, "## %s (%s)\n", date, t.Weekday())
	if e, err := s.store.GetEntry(ctx, u.ID, date); err == nil {
		if e.Mood > 0 {
			fmt.Fprintf(&b, "Mood: %s\n", moodWords[e.Mood])
		}
		if sendText {
			if e.Highlight != "" {
				fmt.Fprintf(&b, "Highlight: %s\n", e.Highlight)
			}
			if e.Text != "" {
				fmt.Fprintf(&b, "Wrote: %s\n", clip(e.Text, maxText))
			}
		}
		if e.Tags != "" {
			fmt.Fprintf(&b, "Tags: %s\n", e.Tags)
		}
		if e.Pinned {
			b.WriteString("Starred as a day worth remembering\n")
		}
	}
	if sendText {
		if ms, _ := s.store.ListMoments(ctx, u.ID, date, date, 0); len(ms) > 0 {
			var l []string
			for _, m := range ms {
				x := m.Text
				if m.Time != "" {
					x = m.Time + " " + x
				}
				if m.Kind != "note" {
					x += " (" + m.Kind + ")"
				}
				l = append(l, x)
			}
			fmt.Fprintf(&b, "Moments: %s\n", strings.Join(l, "; "))
		}
	}
	if ps, _ := s.store.ListPins(ctx, u.ID, date, date); len(ps) > 0 {
		var l []string
		for _, p := range ps {
			l = append(l, strings.TrimSpace(p.Title+" "+p.Subtitle))
		}
		fmt.Fprintf(&b, "Kept: %s\n", strings.Join(l, "; "))
	}
	return b.String()
}

// hasContent reports whether a brief says anything beyond its heading.
func hasContent(brief string) bool { return strings.Count(brief, "\n") > 1 }

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// languageRule tells the model which language to write in.
func (s *Server) languageRule(ctx context.Context, u *store.User, st store.Settings) string {
	if strings.TrimSpace(st.AILanguage) != "" {
		return "Write in " + st.AILanguage + "."
	}
	recent, _, _ := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, Limit: 5})
	var sample []string
	for _, e := range recent {
		if t := strings.TrimSpace(e.Highlight + " " + e.Text); t != "" {
			sample = append(sample, clip(t, 160))
		}
	}
	if len(sample) == 0 {
		return "Write in English."
	}
	return "Write in the same language (and script) the person uses in their own entries — for example: “" + strings.Join(sample, "” / “") + "”."
}

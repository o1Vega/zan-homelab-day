package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"

	"example.com/zan-homelab-day/internal/store"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

type monthRecap struct {
	Month   string  `json:"month"`
	Entries int     `json:"entries"`
	Moments int     `json:"moments"`
	Mood    float64 `json:"mood"`
}

type yearRecap struct {
	Year        int              `json:"year"`
	Months      []monthRecap     `json:"months"`
	Entries     int              `json:"entries"`
	Moments     int              `json:"moments"`
	Words       int              `json:"words"`
	Mood        float64          `json:"mood"`
	Moods       [6]int           `json:"moods"`
	Tags        []store.TagCount `json:"tags"`
	Pinned      []store.Entry    `json:"pinned"`
	Best        []store.Entry    `json:"best"`
	Longest     int              `json:"longest_streak"`
	WeekdayMood [8]float64       `json:"weekday_mood"`
}

func (s *Server) handleYears(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	rows, err := s.store.DB.QueryContext(r.Context(), `SELECT substr(date,1,4) y, COUNT(1) FROM entries WHERE user_id = ? GROUP BY y ORDER BY y DESC`, u.ID)
	if err != nil {
		s.fail(w, err, "years")
		return
	}
	defer rows.Close()
	type yr struct {
		Year    int `json:"year"`
		Entries int `json:"entries"`
	}
	out := []yr{}
	seen := map[int]bool{}
	for rows.Next() {
		var y string
		var n int
		if rows.Scan(&y, &n) == nil {
			yi, _ := strconv.Atoi(y)
			out = append(out, yr{yi, n})
			seen[yi] = true
		}
	}
	rows.Close()
	sort.Slice(out, func(i, j int) bool { return out[i].Year > out[j].Year })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleYear(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(chiParam(r, "year"))
	if err != nil || year < 1970 || year > 2200 {
		writeErr(w, http.StatusBadRequest, "year must be a number")
		return
	}
	rec, err := s.buildYear(r.Context(), userFrom(r.Context()), year)
	if err != nil {
		s.fail(w, err, "year")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// buildYear computes the year in review for one user.
func (s *Server) buildYear(ctx context.Context, u *store.User, year int) (*yearRecap, error) {
	ys := strconv.Itoa(year)
	from, to := ys+"-01-01", ys+"-12-31"
	rec := yearRecap{Year: year, Months: make([]monthRecap, 12), Pinned: []store.Entry{}, Best: []store.Entry{}}
	for i := range rec.Months {
		rec.Months[i] = monthRecap{Month: ys + "-" + pad2(i+1)}
	}
	entries, _, err := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, From: from, To: to, Limit: 100000, Asc: true})
	if err != nil {
		return nil, err
	}
	moodSum, moodN := 0.0, 0
	monthMood := make([]float64, 12)
	monthMoodN := make([]int, 12)
	wdSum := [8]float64{}
	wdN := [8]int{}
	for _, e := range entries {
		mi := monthIndex(e.Date)
		rec.Entries++
		rec.Months[mi].Entries++
		rec.Months[mi].Moments += e.Moments
		rec.Moments += e.Moments
		rec.Words += e.Words
		if e.Mood > 0 {
			rec.Moods[e.Mood]++
			moodSum += float64(e.Mood)
			moodN++
			monthMood[mi] += float64(e.Mood)
			monthMoodN[mi]++
			wd := store.WeekdayISO(e.Date)
			wdSum[wd] += float64(e.Mood)
			wdN[wd]++
		}
		if e.Pinned {
			rec.Pinned = append(rec.Pinned, e)
		}
		if e.Mood == 5 && len(rec.Best) < 12 {
			rec.Best = append(rec.Best, e)
		}
	}
	if moodN > 0 {
		rec.Mood = round1(moodSum / float64(moodN))
	}
	for i := range rec.Months {
		if monthMoodN[i] > 0 {
			rec.Months[i].Mood = round1(monthMood[i] / float64(monthMoodN[i]))
		}
	}
	for wd := 1; wd <= 7; wd++ {
		if wdN[wd] > 0 {
			rec.WeekdayMood[wd] = round1(wdSum[wd] / float64(wdN[wd]))
		}
	}
	_, rec.Longest = s.store.Streak(ctx, u.ID, store.Today())
	tags, _ := s.store.Tags(ctx, u.ID)
	if len(tags) > 15 {
		tags = tags[:15]
	}
	rec.Tags = tags

	return &rec, nil
}

func monthIndex(date string) int {
	if len(date) < 7 {
		return 0
	}
	m, _ := strconv.Atoi(date[5:7])
	if m < 1 || m > 12 {
		return 0
	}
	return m - 1
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

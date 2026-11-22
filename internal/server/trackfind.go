package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/zan-homelab-day/internal/store"
)

// ---- finding an open-licensed track for a day ----
//
// A day gets a piece of music that suits it, found on the internet and
// downloaded into the day's folder. The only legal source we trust:
// Openverse (api.openverse.org), an aggregator of Creative-Commons media.
// We accept exclusively licenses that are open for any purpose:
//
//   - cc0      — public domain, no attribution required
//   - by       — CC-BY, attribution required (we show it on the page)
//   - by-sa    — CC-BY-SA, attribution + share-alike
//
// by-nc / by-nd and anything else are rejected: non-commercial or
// no-derivatives licenses are not "fully open" and we would rather have no
// track than a track that could land us in court. The track's title, artist,
// license and source link are stored in track_meta and shown as attribution.

var trackHTTP = &http.Client{Timeout: 90 * time.Second}

const openverseAudioURL = "https://api.openverse.org/v1/audio/"

// openLicenses is the closed set of licenses we are willing to attach to a day.
var openLicenses = map[string]bool{"cc0": true, "by": true, "by-sa": true}

// licenseNames is the human label for the badges on the page.
var licenseNames = map[string]string{
	"cc0":   "CC0 · public domain",
	"by":    "CC BY",
	"by-sa": "CC BY-SA",
}

type openverseAudio struct {
	Title            string  `json:"title"`
	Url              string  `json:"url"`
	Creator          string  `json:"creator"`
	CreatorURL       string  `json:"creator_url"`
	ForeignLanding   string  `json:"foreign_landing_url"`
	License          string  `json:"license"`
	LicenseURL       string  `json:"license_url"`
	Provider         string  `json:"provider"`
	Filetype         string  `json:"filetype"`
	Filesize         int64   `json:"filesize"`
	Duration         float64 `json:"duration"` // milliseconds
	Attribution      string  `json:"attribution"`
}

type openversePage struct {
	ResultCount int              `json:"result_count"`
	Results     []openverseAudio `json:"results"`
}

// trackQueryForDay maps the measured mood of the day onto a music search in
// English (the catalog is mostly English-tagged).
func trackQueryForDay(m *store.Memory) string {
	q := "music"
	switch strings.ToLower(m.MoodCluster) {
	case "tense_night":
		q = "dark ambient electronic music"
	case "light_bright":
		q = "bright light piano music"
	case "melancholy":
		q = "melancholy ambient music"
	case "quiet_warm", "":
		q = "calm acoustic music"
	}
	return q
}

// currentTrack reads the title/artist the day already plays (empty strings
// when the day has no track), so a re-roll can skip the current pick.
func currentTrack(m *store.Memory) (title, artist string) {
	if len(m.TrackMeta) == 0 {
		return "", ""
	}
	var meta map[string]any
	if err := json.Unmarshal(m.TrackMeta, &meta); err != nil {
		return "", ""
	}
	t, _ := meta["title"].(string)
	a, _ := meta["artist"].(string)
	return t, a
}

// openverseSearch asks Openverse for audio matching q and keeps only the
// results that are actually open: allowed license, playable format, a
// sensible duration and a direct file URL.
func (s *Server) openverseSearch(ctx context.Context, q string) ([]openverseAudio, error) {
	// keyless requests are capped at page_size 20, so walk a few pages
	var out []openverseAudio
	var firstErr error
	for p := 1; p <= 3; p++ {
		u := openverseAudioURL + "?" + url.Values{
			"q":         {q},
			"page_size": {"20"},
			"page":      {strconv.Itoa(p)},
			"file_type": {"audio"},
		}.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", s.cfg.UserAgent)
		res, err := trackHTTP.Do(req)
		if err != nil {
			return nil, err
		}
		var page openversePage
		err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&page)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || err != nil {
			if firstErr == nil {
				if res.StatusCode != http.StatusOK {
					firstErr = fmt.Errorf("openverse answered %s", res.Status)
				} else {
					firstErr = fmt.Errorf("openverse returned no JSON: %w", err)
				}
			}
			continue
		}
		if len(page.Results) == 0 {
			break
		}
		for _, r := range page.Results {
			if !openLicenses[r.License] {
				continue
			}
			if r.Filetype != "mp3" && r.Filetype != "ogg" {
				continue
			}
			// duration is in milliseconds; skip stingers and feature-length files
			if r.Duration != 0 && (r.Duration < 30000 || r.Duration > 15*60*1000) {
				continue
			}
			if r.Url == "" {
				continue
			}
			if strings.HasPrefix(r.Creator, "Unknown") {
				continue
			}
			out = append(out, r)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// attachTrackToDay downloads an open-licensed track for the day and stores it
// in the day's folder with its attribution metadata. salt spreads the pick:
// the background job uses 0 (one stable pick per day), the manual
// "roll another" button uses a random salt so re-rolls actually differ.
func (s *Server) attachTrackToDay(ctx context.Context, m *store.Memory, salt int) (*store.Memory, error) {
	candidates, err := s.openverseSearch(ctx, trackQueryForDay(m))
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no open-licensed track found for “%s”", trackQueryForDay(m))
	}
	// a different pick per day: the day id spreads the days across the list,
	// and a re-roll never lands on the track the day already has
	base := (int(m.ID)*7 + salt) % len(candidates)
	curTitle, curArtist := currentTrack(m)
	pick := candidates[base]
	if pick.Title == curTitle && pick.Creator == curArtist {
		for off := 1; off < len(candidates); off++ {
			c := candidates[(base+off)%len(candidates)]
			if !(c.Title == curTitle && c.Creator == curArtist) {
				pick = c
				break
			}
		}
	}

	dir, err := s.memoryFolder(m)
	if err != nil {
		return nil, err
	}
	ext := ".mp3"
	if pick.Filetype == "ogg" {
		ext = ".ogg"
	}
	name := "track" + ext
	dst := filepath.Join(dir, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pick.Url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	res, err := trackHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download the track: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the track file answered %s", res.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return nil, err
	}
	// 40 MB is far above any real track; a runaway answer is aborted
	if _, err := io.Copy(f, io.LimitReader(res.Body, 40<<20)); err != nil {
		f.Close()
		_ = os.Remove(dst)
		return nil, fmt.Errorf("save the track: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dst)
		return nil, err
	}

	mime, ok := audioExts[ext]
	if !ok {
		mime = "audio/mpeg"
	}
	landing := pick.ForeignLanding
	if landing == "" {
		landing = pick.Url
	}
	meta := map[string]any{
		"title":       pick.Title,
		"artist":      pick.Creator,
		"mime":        mime,
		"url":         landing,
		"source":      "openverse",
		"license":     pick.License,
		"license_url": pick.LicenseURL,
		"attribution": pick.Attribution,
	}
	b, _ := json.Marshal(meta)
	m.TrackPath = name
	m.TrackMeta = b
	m, err = s.store.UpsertMemory(ctx, m)
	if err != nil {
		_ = os.Remove(dst)
		return nil, err
	}
	_ = s.writeMemoryJSON(m)
	s.log.Info("open track attached", "memory", m.ID, "title", pick.Title, "by", pick.Creator, "license", pick.License)
	return m, nil
}

// generateTrackIfMissing finds an open-licensed track in the background right
// after a day is created (or its words rewritten), mirroring the theme job.
// It never blocks or fails the request: a day without a track simply waits.
func (s *Server) generateTrackIfMissing(m *store.Memory, rewrite bool) {
	if m.TrackPath != "" {
		if !rewrite {
			return
		}
		// on a rewrite, only an openverse pick is fair game: the user's own
		// upload stays even if the day's words change
		var meta map[string]any
		if len(m.TrackMeta) > 0 {
			_ = json.Unmarshal(m.TrackMeta, &meta)
		}
		if meta == nil || meta["source"] != "openverse" {
			return
		}
	}
	mid, uid := m.ID, m.UserID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		fresh, err := s.store.GetMemory(ctx, uid, mid)
		if err != nil {
			return
		}
		if _, err := s.attachTrackToDay(ctx, fresh, 0); err != nil {
			s.log.Warn("auto track", "memory", mid, "err", err)
		}
	}()
}

// handleMemoryFindTrack is the manual "find a track" / "roll another" button:
// it runs the search synchronously so the page can show the result at once.
func (s *Server) handleMemoryFindTrack(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	m, err := s.store.GetMemory(ctx, userFrom(ctx).ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	force := r.URL.Query().Get("force") == "1"
	if m.TrackPath != "" && !force {
		writeErr(w, http.StatusConflict, "the day already has a track; use force=1 to roll another")
		return
	}
	if force && m.TrackPath != "" {
		var meta map[string]any
		if len(m.TrackMeta) > 0 {
			_ = json.Unmarshal(m.TrackMeta, &meta)
		}
		if meta == nil || meta["source"] != "openverse" {
			writeErr(w, http.StatusConflict, "the day has your own track; remove it before the model can replace it")
			return
		}
		if dir, derr := s.memoryFolder(m); derr == nil {
			_ = os.Remove(filepath.Join(dir, m.TrackPath))
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	m2, aerr := s.attachTrackToDay(ctx, m, rand.IntN(1<<30))
	if aerr != nil {
		s.log.Warn("find track", "memory", id, "err", aerr)
		writeErr(w, http.StatusBadGateway, "could not find an open track: "+aerr.Error())
		return
	}
	m = m2
	writeJSON(w, http.StatusOK, map[string]any{"track": map[string]any{"file": m.TrackPath, "meta": json.RawMessage(m.TrackMeta)}})
}

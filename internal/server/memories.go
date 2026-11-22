package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"

	"example.com/zan-homelab-day/internal/atmosphere"
	"example.com/zan-homelab-day/internal/store"
)

// ---- the day's folder ----
//
// Every memory owns one folder: data/days/<YYYY-MM-DD>/<slug>/ with the text
// (memory.json), the photos, the track, the developed background and the
// rendered poster. The folder is what the person archives; the rows only
// index it.

// memoryFolder returns (and creates) the day's folder for a memory.
func (s *Server) memoryFolder(m *store.Memory) (string, error) {
	slug := slugify(m.Title)
	if slug == "" {
		slug = fmt.Sprintf("day-%d", m.ID)
	}
	dir := filepath.Join(s.cfg.DataDir, "days", m.Date, slug)
	return dir, os.MkdirAll(dir, 0o755)
}

// memoryDirFor resolves the folder name for a memory with a given title.
func (s *Server) memoryDirFor(m *store.Memory, title string) string {
	slug := slugify(title)
	if slug == "" {
		slug = fmt.Sprintf("day-%d", m.ID)
	}
	return filepath.Join(s.cfg.DataDir, "days", m.Date, slug)
}

// renameFolder moves a day's folder when its title (the folder name) changes.
// Only this memory's own old folder (derived from oldTitle) is moved; other
// people's days on the same date are never touched.
func (s *Server) renameFolder(m *store.Memory, oldTitle string) error {
	target, err := s.memoryFolder(m)
	if err != nil {
		return err
	}
	old := s.memoryDirFor(m, oldTitle)
	if old == target {
		return nil
	}
	entries, _ := os.ReadDir(old)
	if len(entries) == 0 {
		return nil
	}
	for _, e := range entries {
		src := filepath.Join(old, e.Name())
		dst := filepath.Join(target, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			if err := moveDir(src, dst); err != nil {
				return err
			}
			_ = os.Remove(src)
		} else if _, err := os.Stat(dst); os.IsNotExist(err) {
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
	}
	_ = os.Remove(old)
	return nil
}

func moveDir(from, to string) error {
	for {
		entries, _ := os.ReadDir(from)
		if len(entries) == 0 {
			return nil
		}
		for _, e := range entries {
			src := filepath.Join(from, e.Name())
			dst := filepath.Join(to, e.Name())
			if e.IsDir() {
				if err := os.MkdirAll(dst, 0o755); err != nil {
					return err
				}
				if err := moveDir(src, dst); err != nil {
					return err
				}
				_ = os.Remove(src)
			} else if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
	}
}

// writeMemoryJSON mirrors the day's metadata into the folder itself.
func (s *Server) writeMemoryJSON(m *store.Memory) error {
	dir, err := s.memoryFolder(m)
	if err != nil {
		return err
	}
	var art any
	_ = json.Unmarshal(m.ArtDirection, &art)
	var meta any
	_ = json.Unmarshal(m.TrackMeta, &meta)
	photos, _ := s.store.ListMemoryPhotos(context.Background(), m.ID)
	names := make([]string, 0, len(photos))
	for _, p := range photos {
		names = append(names, filepath.Base(p.Path))
	}
	doc := map[string]any{
		"date": m.Date, "title": m.Title, "text": m.Text,
		"mood_cluster": m.MoodCluster, "art_direction": art,
		"track": meta, "track_url": m.TrackURL, "photos": names,
		"video": m.VideoPath, "narration": m.NarrationPath, "poster": m.PosterPath,
		"created_at": m.CreatedAt,
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	return os.WriteFile(filepath.Join(dir, "memory.json"), b, 0o644)
}

// slugify makes a folder name from a day title: letters of any script,
// digits and hyphens, at most 80 chars.
func slugify(title string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.TrimSpace(title) {
		if prevDash && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			prevDash = false
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if r := []rune(s); len(r) > 80 {
		s = string(r[:80])
	}
	return s
}

func memID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func validDateParam(date string) bool {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return false
	}
	return date <= store.Today()
}

// ---- list / get / create / update / delete ----

func (s *Server) handleMemoriesList(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.ListMemories(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err, "list memories")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMemoryGet(w http.ResponseWriter, r *http.Request) {
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
	photos, _ := s.store.ListMemoryPhotos(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"memory": m, "photos": photos})
}

func (s *Server) handleMemoryCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		Date string `json:"date"`
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "send {date, text}")
		return
	}
	if !validDateParam(in.Date) {
		writeErr(w, http.StatusBadRequest, "pick a day that has happened")
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "write a few words about the day first")
		return
	}
	m, err := s.store.UpsertMemory(ctx, &store.Memory{UserID: u.ID, Date: in.Date, Text: in.Text, Status: "draft"})
	if err != nil {
		s.fail(w, err, "create memory")
		return
	}
	if _, err := s.memoryFolder(m); err != nil {
		s.fail(w, err, "create day folder")
		return
	}
	_ = s.writeMemoryJSON(m)
	// the day is themed in the background: when the owner opens it, the page
	// already has its own colors, story and opening animation
	s.generateThemeIfMissing(m)
	// and it is given a piece of music that suits it: an open-licensed track,
	// found on Openverse and downloaded into the day's folder
	s.generateTrackIfMissing(m, false)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleMemoryUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	var in struct {
		Text  *string `json:"text"`
		Title *string `json:"title"`
		Track *string `json:"track_url"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "send {text?, title?, track_url?}")
		return
	}
	oldTitle := m.Title
	textChanged := false
	if in.Text != nil && *in.Text != m.Text {
		m.Text = *in.Text
		textChanged = true
	}
	if in.Title != nil {
		m.Title = strings.TrimSpace(*in.Title)
	}
	if in.Track != nil {
		m.TrackURL = strings.TrimSpace(*in.Track)
	}
	if textChanged {
		// new words, new day: drop the old theme so the background job
		// re-rolls the colors, story and opening for the rewritten text
		if dir, err := s.memoryFolder(m); err == nil {
			_ = os.Remove(filepath.Join(dir, "theme.json"))
		}
	}
	m, err = s.store.UpsertMemory(ctx, m)
	if err != nil {
		s.fail(w, err, "update memory")
		return
	}
	if err := s.renameFolder(m, oldTitle); err != nil {
		s.fail(w, err, "rename day folder")
		return
	}
	_ = s.writeMemoryJSON(m)
	// rewritten words: if the day lost its theme (or never had one), theme it again
	s.generateThemeIfMissing(m)
	// and if the day has no track yet (or only an openverse pick), re-roll it
	// for the new words; a track the user uploaded is never touched
	s.generateTrackIfMissing(m, textChanged)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	if err := s.store.DeleteMemory(ctx, u.ID, id); err != nil {
		s.fail(w, err, "delete memory")
		return
	}
	// The folder is the day's archive: remove it with its files.
	if dir, err := s.memoryFolderFor(m); err == nil {
		_ = os.RemoveAll(dir)
	}
	w.WriteHeader(http.StatusNoContent)
}

// memoryFolderFor is memoryFolder without creating (for deletion).
func (s *Server) memoryFolderFor(m *store.Memory) (string, error) {
	slug := slugify(m.Title)
	if slug == "" {
		slug = fmt.Sprintf("day-%d", m.ID)
	}
	return filepath.Join(s.cfg.DataDir, "days", m.Date, slug), nil
}

// ---- photos ----

var photoExts = map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".heic": "image/heic"}

// handleMemoryPhotos accepts several images as the multipart field "photos"
// plus an optional "colors" JSON field: one [r,g,b] triple set per file.
func (s *Server) handleMemoryPhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "send photos as multipart field “photos” (max 25 MB total)")
		return
	}
	headers := r.MultipartForm.File["photos"]
	if len(headers) == 0 {
		writeErr(w, http.StatusBadRequest, "the “photos” field is empty")
		return
	}
	var colors [][][]float64
	if f := r.FormValue("colors"); f != "" {
		_ = json.Unmarshal([]byte(f), &colors)
	}
	dir, err := s.memoryFolder(m)
	if err != nil {
		s.fail(w, err, "open day folder")
		return
	}
	// Number photos from the existing count so re-uploads do not collide.
	existing, _ := s.store.ListMemoryPhotos(ctx, id)
	saved := []map[string]any{}
	for i, h := range headers {
		ext := strings.ToLower(filepath.Ext(h.Filename))
		mime, known := photoExts[ext]
		if !known {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("photo %d: only jpg, png, webp or heic", i+1))
			return
		}
		f, err := h.Open()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "read the photo")
			return
		}
		name := fmt.Sprintf("photo-%d%s", len(existing)+i+1, ext)
		dst, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			f.Close()
			s.fail(w, err, "save photo")
			return
		}
		if _, err := io.Copy(dst, f); err != nil {
			dst.Close()
			f.Close()
			s.fail(w, err, "save photo")
			return
		}
		dst.Close()
		f.Close()
		var c []float64
		if i < len(colors) {
			c = flattenColors(colors[i])
		}
		p := &store.MemoryPhoto{MemoryID: id, Path: name, Colors: c, Sort: len(existing) + i}
		if _, err := s.store.UpsertMemoryPhoto(ctx, p); err != nil {
			s.fail(w, err, "store photo")
			return
		}
		saved = append(saved, map[string]any{"name": name, "type": mime, "size": h.Size})
	}
	_ = s.writeMemoryJSON(m)
	writeJSON(w, http.StatusOK, map[string]any{"photos": saved})
}

func flattenColors(c [][]float64) []float64 {
	out := []float64{}
	for _, t := range c {
		if len(t) >= 3 {
			out = append(out, t[0], t[1], t[2])
		}
	}
	return out
}

func (s *Server) handleMemoryPhotoDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	pid, ok := memID(r, "pid")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad photo id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	photos, _ := s.store.ListMemoryPhotos(ctx, id)
	for _, p := range photos {
		if p.ID == pid {
			if dir, err := s.memoryFolder(m); err == nil {
				_ = os.Remove(filepath.Join(dir, filepath.Base(p.Path)))
			}
			break
		}
	}
	if err := s.store.DeleteMemoryPhoto(ctx, id, pid); err != nil {
		s.fail(w, err, "delete photo")
		return
	}
	_ = s.writeMemoryJSON(m)
	w.WriteHeader(http.StatusNoContent)
}

// ---- the track ----

var audioExts = map[string]string{".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".wav": "audio/wav", ".ogg": "audio/ogg", ".flac": "audio/flac", ".opus": "audio/opus"}

// handleMemoryTrack accepts the track (multipart field "file") and the
// features the client measured (title, artist, bpm, key, major, energy, valence).
func (s *Server) handleMemoryTrack(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 60<<20)
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "send the track as multipart field “file” (max 60 MB)")
		return
	}
	var header *multipart.FileHeader
	if f := r.MultipartForm.File["file"]; len(f) > 0 {
		header = f[0]
	}
	if header == nil {
		writeErr(w, http.StatusBadRequest, "the “file” field is empty")
		return
	}
	h, err := header.Open()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read the track")
		return
	}
	defer h.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	mime, known := audioExts[ext]
	if !known {
		writeErr(w, http.StatusBadRequest, "only mp3, m4a, wav, ogg, flac or opus")
		return
	}
	dir, err := s.memoryFolder(m)
	if err != nil {
		s.fail(w, err, "open day folder")
		return
	}
	name := "track" + ext
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		s.fail(w, err, "save track")
		return
	}
	if _, err := io.Copy(dst, h); err != nil {
		dst.Close()
		s.fail(w, err, "save track")
		return
	}
	dst.Close()
	meta := map[string]any{
		"title":   r.FormValue("title"),
		"artist":  r.FormValue("artist"),
		"bpm":     formFloat(r, "bpm"),
		"key":     r.FormValue("key"),
		"energy":  formFloat(r, "energy"),
		"valence": formFloat(r, "valence"),
		"mime":    mime,
	}
	if mj := r.FormValue("major"); mj != "" {
		meta["major"] = mj == "true"
	}
	b, _ := json.Marshal(meta)
	m.TrackPath = name
	m.TrackMeta = b
	m, err = s.store.UpsertMemory(ctx, m)
	if err != nil {
		s.fail(w, err, "store track")
		return
	}
	_ = s.writeMemoryJSON(m)
	writeJSON(w, http.StatusOK, map[string]any{"track": map[string]any{"file": name, "type": mime, "meta": meta}})
}

func formFloat(r *http.Request, key string) float64 {
	f, _ := strconv.ParseFloat(r.FormValue(key), 64)
	return f
}

// handleMemoryTrackLink sets (or clears) the day's song as a link to a music
// service — an alternative to the uploaded file. The file, when present,
// still feeds the analysis and the video.
func (s *Server) handleMemoryTrackLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	var in struct {
		URL    string `json:"url"`
		Title  string `json:"title"`
		Artist string `json:"artist"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, `send {"url", "title?", "artist?"}`)
		return
	}
	songURL := strings.TrimSpace(in.URL)
	if songURL != "" {
		if _, err := url.Parse(songURL); err != nil || (!strings.HasPrefix(songURL, "https://") && !strings.HasPrefix(songURL, "http://")) {
			writeErr(w, http.StatusBadRequest, "the song link must be an http(s) url")
			return
		}
	}
	m.TrackURL = songURL
	if songURL != "" {
		// A link replaces the file as "the song" only when there is no file
		// metadata yet; otherwise it is stored next to it.
		if strings.TrimSpace(in.Title) != "" || strings.TrimSpace(in.Artist) != "" {
			var meta map[string]any
			if len(m.TrackMeta) > 0 {
				_ = json.Unmarshal(m.TrackMeta, &meta)
			}
			if meta == nil {
				meta = map[string]any{}
			}
			if strings.TrimSpace(in.Title) != "" {
				meta["title"] = strings.TrimSpace(in.Title)
			}
			if strings.TrimSpace(in.Artist) != "" {
				meta["artist"] = strings.TrimSpace(in.Artist)
			}
			meta["url"] = songURL
			b, _ := json.Marshal(meta)
			m.TrackMeta = b
		}
	} else if len(m.TrackMeta) > 0 {
		// Clearing the link also clears link-only metadata.
		var meta map[string]any
		_ = json.Unmarshal(m.TrackMeta, &meta)
		if meta != nil {
			if _, hasFile := meta["mime"]; !hasFile {
				t, _ := meta["title"].(string)
				a, _ := meta["artist"].(string)
				if strings.TrimSpace(t) == "" && strings.TrimSpace(a) == "" {
					m.TrackMeta = []byte("{}")
				}
			}
		}
	}
	m, err = s.store.UpsertMemory(ctx, m)
	if err != nil {
		s.fail(w, err, "store track link")
		return
	}
	_ = s.writeMemoryJSON(m)
	writeJSON(w, http.StatusOK, map[string]any{"track_url": m.TrackURL})
}

// ---- analyze: text + photos + track → art direction ----

// handleMemoryAnalyze develops the day's atmosphere. It degrades to the
// rule-based direction when no AI is connected, so a day is never blocked.
func (s *Server) handleMemoryAnalyze(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	if strings.TrimSpace(m.Text) == "" {
		writeErr(w, http.StatusBadRequest, "write the day first, then analyze")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	c := s.aiClient(ctx, u, st)
	photos, _ := s.store.ListMemoryPhotos(ctx, id)
	pc := make([]atmosphere.PhotoColors, 0, len(photos))
	for _, p := range photos {
		pc = append(pc, atmosphere.PhotoColors{Name: p.Path, Colors: p.Colors})
	}
	var tm atmosphere.Track
	if len(m.TrackMeta) > 0 {
		_ = json.Unmarshal(m.TrackMeta, &tm)
	}
	in := atmosphere.Input{Text: m.Text, Date: m.Date, Photos: pc, Track: tm, Language: st.AILanguage}
	// OCR the first photo for concrete material (messages, signs, labels).
	// Only when an OCR model is configured; a failure never blocks the day.
	if c != nil && c.OCRModel != "" && len(photos) > 0 {
		if dir, err := s.memoryFolder(m); err == nil {
			if data, err := os.ReadFile(filepath.Join(dir, photos[0].Path)); err == nil {
				ocrCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				txt, oerr := c.OCR(ocrCtx, data, "Read any text visible in this photo (signs, messages, labels, handwriting). If there is no readable text, return an empty string.")
				cancel()
				if oerr == nil {
					if txt = strings.TrimSpace(txt); txt != "" {
						in.Notes = clipRunes(txt, 600)
					}
				} else {
					s.log.Warn("memory photo OCR", "memory", id, "err", oerr)
				}
			}
		}
	}
	dir, err := atmosphere.Analyze(ctx, c, in)
	if err != nil {
		s.fail(w, err, "analyze the day")
		return
	}
	if strings.TrimSpace(dir.Title) == "" {
		dir.Title = m.Title
	}
	oldTitle := m.Title
	m.Title = dir.Title
	m.MoodCluster = string(dir.Cluster)
	art, _ := json.Marshal(dir)
	m.ArtDirection = art
	m.Status = "ready"
	if _, err := s.store.UpsertMemory(ctx, m); err != nil {
		s.fail(w, err, "store the direction")
		return
	}
	if err := s.renameFolder(m, oldTitle); err != nil {
		s.fail(w, err, "rename day folder")
		return
	}
	_ = s.writeMemoryJSON(m)
	writeJSON(w, http.StatusOK, dir)
}

// ---- T2I background ----

// handleMemoryBackground develops the day's paper/texture with the image
// model in the palette of the day. Cached per direction; ?force=1 re-rolls.
func (s *Server) handleMemoryBackground(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	name := "background.png"
	dir, err := s.memoryFolder(m)
	if err != nil {
		s.fail(w, err, "open day folder")
		return
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil && r.URL.Query().Get("force") != "1" {
		writeJSON(w, http.StatusOK, map[string]any{"file": name, "cached": true})
		return
	}
	st, c, ok := s.aiOrFail(w, r)
	_ = st
	if !ok {
		return
	}
	var ad atmosphere.ArtDirection
	if len(m.ArtDirection) > 0 {
		_ = json.Unmarshal(m.ArtDirection, &ad)
	}
	if string(ad.Cluster) == "" {
		writeErr(w, http.StatusBadRequest, "analyze the day first, then develop the background")
		return
	}
	prompt := backgroundPrompt(ad)
	// Square: the day's T2I endpoints (litellm/Qwen-Image) only accept equal
	// dimensions; the square texture is stretched to the 4:5 poster on render.
	img, err := c.Image(ctx, prompt, "1024x1024")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := os.WriteFile(path, img, 0o644); err != nil {
		s.fail(w, err, "save background")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file": name, "cached": false})
}

// backgroundPrompt is the T2I prompt in the fixed visual language.
func backgroundPrompt(ad atmosphere.ArtDirection) string {
	pal := strings.Join(ad.Palette, ", ")
	var style string
	switch ad.Technique {
	case "monet":
		style = "soft impressionist brush strokes in the style of Claude Monet, warm diffuse light, visible paint texture"
	case "kandinsky":
		style = "early abstract composition in the style of Kandinsky, hard geometric grid, thin black lines, flat color patches, high contrast"
	case "minimal":
		style = "flat japanese minimalist graphic, large areas of negative space, one small accent, washi paper grain"
	case "mist":
		style = "soft watercolor mist, faint rain droplets, muted desaturated tones, hazy depth"
	}
	return "Background texture for a personal day journal poster, no text, no people, no objects in the center, " +
		style + ". Palette: " + pal + ". Subtle, quiet, suitable as a backdrop behind a photograph."
}

// ---- the rendered poster (canvas in the browser) ----

// handleMemoryPoster stores the poster the browser rendered, so the day's
// folder holds the finished remembrance, not only the inputs.
func (s *Server) handleMemoryPoster(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(ctx)
	m, err := s.store.GetMemory(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "send the poster as multipart field “file” (png, max 15 MB)")
		return
	}
	h, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the “file” field is empty")
		return
	}
	defer h.Close()
	dir, err := s.memoryFolder(m)
	if err != nil {
		s.fail(w, err, "open day folder")
		return
	}
	name := "poster.png"
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		s.fail(w, err, "save poster")
		return
	}
	if _, err := io.Copy(dst, h); err != nil {
		dst.Close()
		s.fail(w, err, "save poster")
		return
	}
	dst.Close()
	if err := s.store.SetMemoryAsset(ctx, u.ID, id, "poster_path", name); err != nil {
		s.fail(w, err, "store poster")
		return
	}
	_ = s.writeMemoryJSON(m)
	writeJSON(w, http.StatusOK, map[string]any{"poster": name})
}

// ---- asset serving ----

// handleMemoryAsset serves one file of the day's folder (?file=name).
func (s *Server) handleMemoryAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	u := userFrom(r.Context())
	m, err := s.store.GetMemory(r.Context(), u.ID, id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	file := r.URL.Query().Get("file")
	if file == "" {
		writeErr(w, http.StatusBadRequest, "pass ?file=<name>")
		return
	}
	if strings.ContainsAny(file, "/\\") || file == ".." {
		writeErr(w, http.StatusBadRequest, "bad file name")
		return
	}
	dir, _ := s.memoryFolder(m)
	full := filepath.Join(dir, filepath.Base(file))
	http.ServeFile(w, r, full)
}

// clipRunes shortens s to n runes (whole words not required: notes are
// material, not text to read).
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---- the public posts feed ----
//
// A day is private by default. When its owner switches it on, the remembrance
// appears in the shared Posts feed, served through /posts so no owner token is
// needed for the pictures.

// handlePostsList lists the days people shared, newest first. ?follows=1 keeps
// only the viewer's own days and the days of the people they follow; ?author=
// keeps one person's days.
func (s *Server) handlePostsList(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	limit, offset := 24, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v > 0 {
		offset = v
	}
	followsOnly := r.URL.Query().Get("follows") == "1"
	var authorID int64
	if v, err := strconv.Atoi(r.URL.Query().Get("author")); err == nil && v > 0 {
		authorID = int64(v)
	}
	posts, err := s.store.ListPublicFeed(r.Context(), u.ID, followsOnly, authorID, limit, offset)
	if err != nil {
		s.fail(w, err, "list posts")
		return
	}
	total, _ := s.store.CountPublicFeed(r.Context(), u.ID, followsOnly, authorID)
	writeJSON(w, http.StatusOK, map[string]any{"posts": posts, "total": total})
}

// handleMemoryPublic toggles the public flag of one memory (owner only).
func (s *Server) handleMemoryPublic(w http.ResponseWriter, r *http.Request) {
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	var in struct {
		Public bool `json:"public"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, `pass {"public": true|false}`)
		return
	}
	u := userFrom(r.Context())
	if _, err := s.store.GetMemory(r.Context(), u.ID, id); err != nil {
		s.fail(w, err, "get memory")
		return
	}
	if err := s.store.SetMemoryPublic(r.Context(), u.ID, id, in.Public); err != nil {
		s.fail(w, err, "set public")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"is_public": in.Public})
}

// handlePostAsset serves one file of a public day's folder (?file=name).
func (s *Server) handlePostAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := memID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad memory id")
		return
	}
	m, err := s.store.GetMemoryByID(r.Context(), id)
	if err != nil {
		s.fail(w, err, "get memory")
		return
	}
	if !m.IsPublic {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	file := r.URL.Query().Get("file")
	if file == "" {
		writeErr(w, http.StatusBadRequest, "pass ?file=<name>")
		return
	}
	if strings.ContainsAny(file, "/\\") || file == ".." {
		writeErr(w, http.StatusBadRequest, "bad file name")
		return
	}
	dir, _ := s.memoryFolder(m)
	full := filepath.Join(dir, filepath.Base(file))
	http.ServeFile(w, r, full)
}

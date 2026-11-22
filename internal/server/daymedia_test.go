package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/zan-homelab-day/internal/store"
)

// dayDir is the day's folder on disk, the same way the server computes it.
func dayDir(a *connectApp, m *store.Memory) string {
	slug := slugify(m.Title)
	if slug == "" {
		slug = fmt.Sprintf("day-%d", m.ID)
	}
	return filepath.Join(a.dir, "days", m.Date, slug)
}

// png1x1 is the smallest valid PNG: a 1x1 red pixel.
func png1x1(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 220, G: 40, B: 40, A: 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// daysAgo is date n days before today in the server's timezone: the day
// must have already happened.
func daysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("2006-01-02")
}

func (a *connectApp) createDay(t *testing.T, date, text string) int64 {
	t.Helper()
	code, m, err := a.do(http.MethodPost, "/api/v1/memories", bytes.NewReader([]byte(`{"date":"`+date+`","text":"`+text+`"}`)), "application/json")
	if err != nil || code != http.StatusOK {
		t.Fatalf("create day: %d %v", code, err)
	}
	id, _ := m["id"].(float64)
	return int64(id)
}

// ---- the song as a link ----

func TestMemoryTrackLink(t *testing.T) {
	a := newConnectApp(t)
	id := a.createDay(t, daysAgo(1), "a rainy evening at the desk")
	track := fmt.Sprintf("/api/v1/memories/%d/track", id)
	day := fmt.Sprintf("/api/v1/memories/%d", id)

	// A link is stored and visible on the day.
	code, m, err := a.do(http.MethodPut, track, bytes.NewReader([]byte(`{"url":"https://open.spotify.com/track/abc","title":"Rain","artist":"Zan"}`)), "application/json")
	if err != nil || code != http.StatusOK {
		t.Fatalf("set link: %d %v", code, err)
	}
	if m["track_url"] != "https://open.spotify.com/track/abc" {
		t.Fatalf("response %v", m)
	}
	code, got := a.getJSON(day)
	if got["memory"] == nil {
		t.Fatalf("get: %d", code)
	}
	mem := got["memory"].(map[string]any)
	if mem["track_url"] != "https://open.spotify.com/track/abc" {
		t.Fatalf("memory %v", mem)
	}
	meta, _ := json.Marshal(mem["track_meta"])
	if !bytes.Contains(meta, []byte("Rain")) || !bytes.Contains(meta, []byte("Zan")) {
		t.Fatalf("track_meta %s", meta)
	}
	// the mirror in the day folder carries the link too
	mirror := readFolderDoc(t, a, 1)
	if mirror["track_url"] != "https://open.spotify.com/track/abc" {
		t.Fatalf("memory.json %v", mirror)
	}

	// Not a link: refused.
	for _, bad := range []string{"javascript:alert(1)", "ftp://x/y", "not a url"} {
		code, _, _ := a.do(http.MethodPut, track, bytes.NewReader([]byte(`{"url":"`+bad+`"}`)), "application/json")
		if code != http.StatusBadRequest {
			t.Fatalf("%q answered %d, want 400", bad, code)
		}
	}

	// Clearing the link clears it (and the link-only metadata).
	code, m, _ = a.do(http.MethodPut, track, bytes.NewReader([]byte(`{"url":""}`)), "application/json")
	if code != http.StatusOK || m["track_url"] != "" {
		t.Fatalf("clear: %d %v", code, m)
	}
	code, got = a.getJSON(day)
	if got["memory"].(map[string]any)["track_url"] != "" {
		t.Fatalf("still set: %v", got)
	}
}

func readFolderDoc(t *testing.T, a *connectApp, id int64) map[string]any {
	t.Helper()
	ctx := context.Background()
	m, err := a.st.GetMemoryByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dayDir(a, m), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// ---- the day folder gets a real name ----

func TestMemoryFolderRename(t *testing.T) {
	a := newConnectApp(t)
	id := a.createDay(t, daysAgo(0), "morning walk in the rain")
	ctx := context.Background()
	m, _ := a.st.GetMemoryByID(ctx, id)
	oldDir := dayDir(a, m)
	// A file lands in the day's folder before the rename.
	if err := os.WriteFile(filepath.Join(oldDir, "photo.png"), png1x1(t), 0o644); err != nil {
		t.Fatal(err)
	}
	// Renaming the day renames the folder and carries the files over.
	code, _, err := a.do(http.MethodPut, fmt.Sprintf("/api/v1/memories/%d", id), bytes.NewReader([]byte(`{"title":"Rainy morning"}`)), "application/json")
	if err != nil || code != http.StatusOK {
		t.Fatalf("rename: %d %v", code, err)
	}
	m, _ = a.st.GetMemoryByID(ctx, id)
	newDir := dayDir(a, m)
	if newDir == oldDir {
		t.Fatalf("folder not renamed, still %s", newDir)
	}
	if _, err := os.Stat(filepath.Join(newDir, "photo.png")); err != nil {
		t.Fatalf("the file did not move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "memory.json")); err != nil {
		t.Fatalf("memory.json missing after rename: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old folder still there: %v", err)
	}
}

// ---- the spoken day ----

func TestMemoryNarration(t *testing.T) {
	a := newConnectApp(t)
	id := a.createDay(t, daysAgo(2), "we walked to the river and stayed till dark")
	narrate := fmt.Sprintf("/api/v1/memories/%d/narration", id)

	// No AI connected: a clear 400, not a 500.
	code, m, _ := a.do(http.MethodPost, narrate, nil, "")
	if code != http.StatusBadRequest {
		t.Fatalf("no AI: %d %v", code, m)
	}

	// A fake TTS endpoint that records what it was asked.
	var gotBody map[string]any
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/speech" {
			http.Error(w, "unexpected path", 404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fake-key" {
			http.Error(w, "no key", 401)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte{0xFF, 0xF3, 0x00, 0x10}) // a fake mp3 frame
	}))
	t.Cleanup(tts.Close)

	st := a.getSettings()
	st["ai_provider"] = "openai"
	st["ai_key"] = "fake-key"
	st["ai_base_url"] = tts.URL
	st["ai_tts_model"] = "fake-tts"
	st["ai_tts_voice"] = "shimmer"
	code, m2 := a.putJSON("/api/v1/settings", st)
	if code != http.StatusOK {
		t.Fatalf("settings: %d %v", code, m2)
	}

	code, m, err := a.do(http.MethodPost, narrate, nil, "")
	if err != nil || code != http.StatusOK {
		t.Fatalf("narration: %d %v", code, err)
	}
	if m["file"] != "narration.mp3" || m["cached"] != false {
		t.Fatalf("response %v", m)
	}
	// The mood picked a slow, warm voice → the endpoint saw it clamped.
	if gotBody["model"] != "fake-tts" || gotBody["voice"] != "shimmer" {
		t.Fatalf("tts request %v", gotBody)
	}
	if rate, _ := gotBody["rate"].(float64); rate <= 0 || rate > 2 {
		t.Fatalf("rate %v", gotBody["rate"])
	}
	// The audio is in the day's folder and the day knows about it.
	ctx := context.Background()
	mm, _ := a.st.GetMemoryByID(ctx, id)
	dir := dayDir(a, mm)
	if b, err := os.ReadFile(filepath.Join(dir, "narration.mp3")); err != nil || len(b) != 4 {
		t.Fatalf("narration.mp3 %v %d", err, len(b))
	}
	if mm.NarrationPath != "narration.mp3" {
		t.Fatalf("memory %v", mm)
	}
	// The second call serves the cache without a new TTS request.
	before := time.Now()
	code, m, _ = a.do(http.MethodPost, narrate, nil, "")
	if code != http.StatusOK || m["cached"] != true || time.Since(before) > 150*time.Millisecond {
		t.Fatalf("cache: %d %v %v", code, m, time.Since(before))
	}
}

// ---- the day's video ----

func TestMemoryVideo(t *testing.T) {
	a := newConnectApp(t)
	id := a.createDay(t, daysAgo(3), "a slow evening, coffee and a book")
	video := fmt.Sprintf("/api/v1/memories/%d/video", id)
	ctx := context.Background()
	m, _ := a.st.GetMemoryByID(ctx, id)
	dir := dayDir(a, m)
	if err := os.WriteFile(filepath.Join(dir, "poster.png"), png1x1(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.st.SetMemoryAsset(ctx, a.uid, id, "poster_path", "poster.png"); err != nil {
		t.Fatal(err)
	}

	if !ffmpegAvailable() {
		// Without ffmpeg the box says so instead of failing silently.
		code, m, _ := a.do(http.MethodPost, video, nil, "")
		if code != http.StatusServiceUnavailable {
			t.Fatalf("no ffmpeg: %d %v", code, m)
		}
		t.Skip("ffmpeg not installed — the render path is not exercised")
	}
	code, out, err := a.do(http.MethodPost, video, nil, "")
	if err != nil || code != http.StatusOK {
		t.Fatalf("video: %d %v", code, err)
	}
	if out["file"] != "day.mp4" {
		t.Fatalf("response %v", out)
	}
	if st, err := os.Stat(filepath.Join(dir, "day.mp4")); err != nil || st.Size() == 0 {
		t.Fatalf("day.mp4 %v %d", err, st.Size())
	}
	m, _ = a.st.GetMemoryByID(ctx, id)
	if m.VideoPath != "day.mp4" {
		t.Fatalf("memory %v", m)
	}
	// The cached render does not re-roll.
	code, out, _ = a.do(http.MethodPost, video, nil, "")
	if code != http.StatusOK || out["cached"] != true {
		t.Fatalf("cache: %d %v", code, out)
	}
}

// The video graph itself: frames zoom and crossfade, the track plays under.
func TestBuildVideoArgs(t *testing.T) {
	// A trackless day: one frame, no audio input.
	args := buildVideoArgs([]string{"/tmp/a.png"}, "", "/tmp", 8, 0.3, "/tmp/out.mp4")
	s := strings.Join(args, "\x00")
	if strings.Count(s, "-i") != 1 || strings.Count(s, "-map") != 1 || !strings.Contains(s, "libx264") {
		t.Fatalf("trackless args %v", args)
	}
	if !strings.Contains(s, "zoompan") || !strings.Contains(s, "noise") || !strings.Contains(s, "yuv420p") {
		t.Fatalf("filter graph %v", args)
	}
	// Three frames over a track: two crossfades, an audio map, aac.
	args = buildVideoArgs([]string{"/tmp/a.png", "/tmp/b.png", "/tmp/c.png"}, "track.mp3", "/tmp", 8, 0.5, "/tmp/out.mp4")
	s = strings.Join(args, "\x00")
	if strings.Count(s, "xfade") != 2 {
		t.Fatalf("crossfades %v", args)
	}
	if strings.Count(s, "-map") != 2 || !strings.Contains(s, "3:a:0") || !strings.Contains(s, "aac") || !strings.Contains(s, "track.mp3") {
		t.Fatalf("track args %v", args)
	}
}

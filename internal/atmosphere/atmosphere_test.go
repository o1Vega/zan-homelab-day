package atmosphere

import (
	"context"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestFallbackQuietWarmAutumn(t *testing.T) {
	// the brief's example: a park, yellow leaves, cold but warm cocoa, slow Sinatra
	dir := Fallback(Input{
		Text: "Гуляли по парку, всё в жёлтых листьях, было холодно, но мы пили какао и долго разговаривали",
		Date: "2026-10-20",
		Track: Track{Title: "Fly Me to the Moon", Artist: "Frank Sinatra", BPM: 74,
			Key: "Am", Major: boolPtr(false), Energy: 0.25, Valence: 0.55},
		Photos:   []PhotoColors{{Name: "photo-1.jpg", Colors: []float64{0.8, 0.55, 0.2}}},
		Language: "ru",
	})
	if dir.Cluster != QuietWarm {
		t.Fatalf("cluster = %s, want quiet_warm (reasons: %v)", dir.Cluster, dir.Reasons)
	}
	if dir.Technique != "monet" || len(dir.Palette) != 5 || dir.Palette[4] != "#E34234" {
		t.Fatalf("technique/palette = %s %v", dir.Technique, dir.Palette)
	}
	// a slow minor track must add air: slow cuts
	if dir.Motion.FrameSec < 10 {
		t.Fatalf("slow track wants air, frame_sec = %v", dir.Motion.FrameSec)
	}
	if len(dir.Reasons) == 0 || strings.Join(dir.Reasons, " ") == "" {
		t.Fatal("the direction must explain itself")
	}
	if dir.Voice.Warmth != "soft" {
		t.Fatalf("voice = %+v, want soft for a warm quiet day", dir.Voice)
	}
}

func TestFallbackTenseNightOffice(t *testing.T) {
	// the brief's second day: a deadline, an office night, electronic music
	dir := Fallback(Input{
		Text: "Срочная сдача проекта, ночь в офисе, работа до утра",
		Date: "2026-11-02",
		Track: Track{Title: "Strobe", Artist: "deadmau5", BPM: 128,
			Energy: 0.9, Valence: 0.45},
		Photos:   []PhotoColors{{Name: "photo-1.jpg", Colors: []float64{0.2, 0.25, 0.4}}},
		Language: "ru",
	})
	if dir.Cluster != TenseNight {
		t.Fatalf("cluster = %s, want tense_night (reasons: %v)", dir.Cluster, dir.Reasons)
	}
	if dir.Technique != "kandinsky" {
		t.Fatalf("technique = %s, want kandinsky", dir.Technique)
	}
	if dir.Motion.FrameSec > 6 {
		t.Fatalf("fast track wants sharp cuts, frame_sec = %v", dir.Motion.FrameSec)
	}
}

func TestFallbackDefaultIsLight(t *testing.T) {
	dir := Fallback(Input{Text: "обычный день", Date: "2026-07-01", Language: "en"})
	if dir.Cluster != LightBright {
		t.Fatalf("cluster = %s, want light_bright default", dir.Cluster)
	}
	if dir.Title == "" || dir.Caption == "" {
		t.Fatalf("title/caption must exist: %+v", dir)
	}
	if words := len(strings.Fields(dir.Title)); words > 12 {
		t.Fatalf("title is too long: %q", dir.Title)
	}
}

func TestFallbackLowValenceIsMelancholy(t *testing.T) {
	dir := Fallback(Input{
		Text: "день без событий", Date: "2026-03-14", Language: "ru",
		Track: Track{BPM: 84, Energy: 0.3, Valence: 0.25},
	})
	if dir.Cluster != Melancholy {
		t.Fatalf("cluster = %s, want melancholy (reasons: %v)", dir.Cluster, dir.Reasons)
	}
}

func TestAnalyzeDegradesWithoutAI(t *testing.T) {
	// no configured client: the rules still deliver a direction, never an error
	dir, err := Analyze(context.Background(), nil, Input{Text: "дождь, чай, старая песня", Date: "2026-11-10", Language: "ru"})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidCluster(string(dir.Cluster)) || len(dir.Palette) != 5 {
		t.Fatalf("bad direction without AI: %+v", dir)
	}
}

func TestClustersAreClosed(t *testing.T) {
	if !ValidCluster("quiet_warm") || ValidCluster("sunny") {
		t.Fatal("the mood set is fixed: four clusters only")
	}
	if len(All) != 4 {
		t.Fatalf("want four moods, got %d", len(All))
	}
}

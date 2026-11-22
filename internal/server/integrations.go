package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"example.com/zan-homelab-day/internal/ai"
	"example.com/zan-homelab-day/internal/store"
)

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

func (s *Server) publicSettings(ctx context.Context, u *store.User, st store.Settings) map[string]any {
	return map[string]any{
		"ai_provider": st.AIProvider, "ai_configured": st.AIKey != "",
		"title": st.Title, "date_format": st.DateFormat, "time_format": st.TimeFormat, "week_start": st.WeekStart, "home_currency": st.HomeCurrency, "distance_unit": st.DistanceUnit,
		"home_lat": st.HomeLat, "home_lon": st.HomeLon, "home_place": st.HomePlace, "prompt": st.Prompt,
		"notify_url": st.NotifyURL, "notify_format": st.NotifyFormat, "notify_configured": st.NotifyURL != "",
		"notify_evening": st.NotifyEvening, "evening_hour": st.EveningHour, "notify_morning": st.NotifyMorning, "morning_hour": st.MorningHour,
		"ai_key": mask(st.AIKey), "ai_model": st.AIModel, "ai_base_url": st.AIBaseURL, "ai_transcribe_model": st.AITranscribeModel, "ai_language": st.AILanguage,
		"ai_image_key": mask(st.AIImageKey), "ai_image_model": st.AIImageModel, "ai_transcribe_key": mask(st.AITranscribeKey),
		"ai_ocr_key": mask(st.AIOCRKey), "ai_ocr_model": st.AIOCRModel, "ai_rerank_key": mask(st.AIRerankKey), "ai_rerank_model": st.AIRerankModel,
		"ai_tts_key": mask(st.AITTSKey), "ai_tts_model": st.AITTSModel, "ai_tts_voice": st.AITTSVoice,
		"ai_question": st.AIQuestion, "ai_weekly_story": st.AIWeeklyStory, "ai_send_text": st.AISendText,
		"ai_default_model": ai.DefaultModel, "ai_default_transcribe": ai.DefaultTranscribe, "ai_default_tts": ai.DefaultTTS,
		"google_client_id": st.GoogleClientID, "google_client_secret": mask(st.GoogleClientSecret),
		"google_account": st.GoogleAccount, "google_connected": st.GoogleRefreshToken != "", "google_last_sync": st.GoogleLastSync,
		"setup_complete": st.SetupComplete, "base_url": s.cfg.BaseURL, "timezone": time.Now().Location().String(),
	}
}

// handleGetSettings returns the caller's own settings (AI key,
// notifications…) plus the box-wide title.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	st, err := s.store.GetSettings(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), u, st))
}

// handlePutSettings saves the caller's own settings. The box-wide title is
// admin-only; a member sending it gets 403.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	cur, err := s.store.GetSettings(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in store.Settings
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	boxChanged := false
	if keys["title"] && strings.TrimSpace(in.Title) != cur.Title {
		if u.Role != "admin" {
			writeErr(w, http.StatusForbidden, "only an admin can rename the app")
			return
		}
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
		boxChanged = true
	}
	if keys["date_format"] {
		cur.DateFormat = in.DateFormat
	}
	if keys["time_format"] {
		cur.TimeFormat = in.TimeFormat
	}
	if keys["week_start"] && (in.WeekStart == "mon" || in.WeekStart == "sun") {
		cur.WeekStart = in.WeekStart
	}
	if keys["home_currency"] && len(in.HomeCurrency) == 3 {
		cur.HomeCurrency = strings.ToUpper(in.HomeCurrency)
	}
	if keys["distance_unit"] && (in.DistanceUnit == "km" || in.DistanceUnit == "mi") {
		cur.DistanceUnit = in.DistanceUnit
	}
	if keys["home_lat"] {
		cur.HomeLat = in.HomeLat
	}
	if keys["home_lon"] {
		cur.HomeLon = in.HomeLon
	}
	if keys["home_place"] {
		cur.HomePlace = strings.TrimSpace(in.HomePlace)
	}
	if keys["prompt"] {
		cur.Prompt = strings.TrimSpace(in.Prompt)
		if cur.Prompt == "" {
			cur.Prompt = store.DefaultPrompt
		}
	}
	if keys["notify_url"] {
		cur.NotifyURL = strings.TrimSpace(in.NotifyURL)
	}
	if keys["notify_format"] && in.NotifyFormat != "" {
		cur.NotifyFormat = in.NotifyFormat
	}
	if keys["notify_evening"] {
		cur.NotifyEvening = in.NotifyEvening
	}
	if keys["evening_hour"] && in.EveningHour >= 0 && in.EveningHour <= 23 {
		cur.EveningHour = in.EveningHour
	}
	if keys["notify_morning"] {
		cur.NotifyMorning = in.NotifyMorning
	}
	if keys["morning_hour"] && in.MorningHour >= 0 && in.MorningHour <= 23 {
		cur.MorningHour = in.MorningHour
	}
	if keys["ai_key"] && !strings.Contains(in.AIKey, "••") {
		cur.AIKey = strings.TrimSpace(in.AIKey)
	}
	if keys["ai_provider"] && (in.AIProvider == "" || in.AIProvider == "openai") {
		cur.AIProvider = in.AIProvider
	}
	if keys["ai_model"] {
		cur.AIModel = strings.TrimSpace(in.AIModel)
	}
	if keys["ai_base_url"] {
		cur.AIBaseURL = strings.TrimRight(strings.TrimSpace(in.AIBaseURL), "/")
	}
	if keys["ai_transcribe_model"] {
		cur.AITranscribeModel = strings.TrimSpace(in.AITranscribeModel)
	}
	if keys["ai_image_key"] && !strings.Contains(in.AIImageKey, "••") {
		cur.AIImageKey = strings.TrimSpace(in.AIImageKey)
	}
	if keys["ai_image_model"] {
		cur.AIImageModel = strings.TrimSpace(in.AIImageModel)
	}
	if keys["ai_transcribe_key"] && !strings.Contains(in.AITranscribeKey, "••") {
		cur.AITranscribeKey = strings.TrimSpace(in.AITranscribeKey)
	}
	if keys["ai_ocr_key"] && !strings.Contains(in.AIOCRKey, "••") {
		cur.AIOCRKey = strings.TrimSpace(in.AIOCRKey)
	}
	if keys["ai_ocr_model"] {
		cur.AIOCRModel = strings.TrimSpace(in.AIOCRModel)
	}
	if keys["ai_rerank_key"] && !strings.Contains(in.AIRerankKey, "••") {
		cur.AIRerankKey = strings.TrimSpace(in.AIRerankKey)
	}
	if keys["ai_rerank_model"] {
		cur.AIRerankModel = strings.TrimSpace(in.AIRerankModel)
	}
	if keys["ai_tts_key"] && !strings.Contains(in.AITTSKey, "••") {
		cur.AITTSKey = strings.TrimSpace(in.AITTSKey)
	}
	if keys["ai_tts_model"] {
		cur.AITTSModel = strings.TrimSpace(in.AITTSModel)
	}
	if keys["ai_tts_voice"] {
		cur.AITTSVoice = strings.TrimSpace(in.AITTSVoice)
	}
	if keys["ai_language"] {
		cur.AILanguage = strings.TrimSpace(in.AILanguage)
	}
	if keys["ai_question"] {
		cur.AIQuestion = in.AIQuestion
	}
	if keys["ai_weekly_story"] {
		cur.AIWeeklyStory = in.AIWeeklyStory
	}
	if keys["ai_send_text"] {
		cur.AISendText = in.AISendText
	}
	if keys["google_client_id"] {
		cur.GoogleClientID = strings.TrimSpace(in.GoogleClientID)
	}
	if keys["google_client_secret"] && !strings.Contains(in.GoogleClientSecret, "•") {
		cur.GoogleClientSecret = strings.TrimSpace(in.GoogleClientSecret)
	}
	if err := s.store.SaveSettings(r.Context(), u.ID, cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if boxChanged {
		if err := s.store.SaveBoxSettings(r.Context(), cur); err != nil {
			s.fail(w, err, "save settings")
			return
		}
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), u, cur))
}

// handleIntegrationTest checks one integration with the posted credentials or
// the caller's own saved ones (never anybody else's).
func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Which  string `json:"which"`
		URL    string `json:"url"`
		Key    string `json:"key"`
		Extra  string `json:"extra"`
		Format string `json:"format"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u := userFrom(r.Context())
	st, _ := s.store.GetSettings(r.Context(), u.ID)
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	switch in.Which {
	case "ai":
		base, k, model := strings.TrimRight(strings.TrimSpace(in.URL), "/"), in.Key, strings.TrimSpace(in.Extra)
		if base == "" {
			base = st.AIBaseURL
		}
		if k == "" || strings.Contains(k, "••") {
			k = st.AIKey
		}
		if model == "" {
			model = st.AIModel
		}
		c := ai.New(base, k, model, "")
		ids, err := c.Models(ctx)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		for _, id := range ids {
			if id == c.Model {
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Key works; %d models available, “%s” included", len(ids), c.Model)})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("Key works, but “%s” is not among its %d models", c.Model, len(ids))})
		return
	case "notify":
		url := strings.TrimRight(strings.TrimSpace(in.URL), "/")
		fm := in.Format
		if fm == "" {
			fm = st.NotifyFormat
		}
		s.handleNotifyTestWith(w, r, url, fm)
		return
	default:
		writeErr(w, http.StatusBadRequest, "unknown integration")
		return
	}
}

func (s *Server) handleGeocode(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	endpoint := "https://geocoding-api.open-meteo.com/v1/search?name=" + url.QueryEscape(q) + "&count=8&format=json"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "geocoding: "+err.Error())
		return
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "geocoding: "+err.Error())
		return
	}
	defer res.Body.Close()
	var out struct {
		Results []struct {
			Name    string  `json:"name"`
			Admin1  string  `json:"admin1"`
			Country string  `json:"country"`
			Lat     float64 `json:"latitude"`
			Lon     float64 `json:"longitude"`
		} `json:"results"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&out) != nil {
		writeErr(w, http.StatusBadGateway, "geocoding: bad response")
		return
	}
	hits := []map[string]any{}
	for _, p := range out.Results {
		region := p.Admin1
		if region == "" {
			region = p.Country
		}
		hits = append(hits, map[string]any{"name": p.Name, "region": region, "lat": p.Lat, "lon": p.Lon})
	}
	writeJSON(w, http.StatusOK, hits)
}

// ---- system ----

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	u := userFrom(r.Context())
	counts := map[string]int{}
	if u.Role == "admin" { // box-wide row counts (numbers only, no content) are for the admin
		counts = s.store.Counts(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": counts,
	})
}

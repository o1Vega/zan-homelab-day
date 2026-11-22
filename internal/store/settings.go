package store

import (
	"context"
	"fmt"
	"strconv"
)

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "残像 Zanzō"

// DefaultPrompt is the question above the text box.
const DefaultPrompt = "What made today today?"

func defaultSettings() Settings {
	return Settings{Title: DefaultTitle, DateFormat: "auto", TimeFormat: "auto", WeekStart: "mon", HomeCurrency: "EUR", DistanceUnit: "km", Prompt: DefaultPrompt, Language: "auto",
		NotifyFormat: "apprise", NotifyEvening: true, AIQuestion: true, AIWeeklyStory: true, AISendText: true, EveningHour: 21, NotifyMorning: true, MorningHour: 9}
}

// BoxSettingKeys are the settings that belong to the box (admin-only); every
// other key is personal and lives in user_settings.
var BoxSettingKeys = map[string]bool{"title": true, "setup_complete": true}

// GetSettings loads the box-wide settings plus the user's personal ones, with
// defaults applied. userID 0 = box-wide settings only.
func (s *Store) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	st := defaultSettings()
	if s.Defaults != nil {
		s.Defaults(&st)
	}
	kv, err := s.settingRows(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	for k, v := range kv {
		if BoxSettingKeys[k] {
			applySetting(&st, k, v)
		}
	}
	if userID > 0 {
		kv, err := s.settingRows(ctx, `SELECT key, value FROM user_settings WHERE user_id = ?`, userID)
		if err != nil {
			return st, err
		}
		for k, v := range kv {
			if !BoxSettingKeys[k] {
				applySetting(&st, k, v)
			}
		}
	}
	return st, nil
}

func (s *Store) settingRows(ctx context.Context, q string, args ...any) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func applySetting(st *Settings, k, v string) {
	switch k {
	case "title":
		if v != "" {
			st.Title = v
		}
	case "date_format":
		if v != "" {
			st.DateFormat = v
		}
	case "time_format":
		if v != "" {
			st.TimeFormat = v
		}
	case "week_start":
		if v == "sun" {
			st.WeekStart = "sun"
		}
	case "home_currency":
		if v != "" {
			st.HomeCurrency = v
		}
	case "distance_unit":
		if v == "mi" {
			st.DistanceUnit = "mi"
		}
	case "home_lat":
		st.HomeLat, _ = strconv.ParseFloat(v, 64)
	case "home_lon":
		st.HomeLon, _ = strconv.ParseFloat(v, 64)
	case "home_place":
		st.HomePlace = v
	case "prompt":
		if v != "" {
			st.Prompt = v
		}
	case "notify_url":
		st.NotifyURL = v
	case "notify_format":
		if v != "" {
			st.NotifyFormat = v
		}
	case "notify_evening":
		st.NotifyEvening = v != "0"
	case "evening_hour":
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			st.EveningHour = n
		}
	case "notify_morning":
		st.NotifyMorning = v != "0"
	case "morning_hour":
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			st.MorningHour = n
		}
	case "setup_complete":
		st.SetupComplete = v == "1"
	case "ai_provider":
		st.AIProvider = v
	case "ai_key":
		st.AIKey = v
	case "ai_model":
		st.AIModel = v
	case "ai_base_url":
		st.AIBaseURL = v
	case "ai_transcribe_model":
		st.AITranscribeModel = v
	case "ai_image_key":
		st.AIImageKey = v
	case "ai_image_model":
		st.AIImageModel = v
	case "ai_transcribe_key":
		st.AITranscribeKey = v
	case "ai_ocr_key":
		st.AIOCRKey = v
	case "ai_ocr_model":
		st.AIOCRModel = v
	case "ai_rerank_key":
		st.AIRerankKey = v
	case "ai_rerank_model":
		st.AIRerankModel = v
	case "ai_tts_key":
		st.AITTSKey = v
	case "ai_tts_model":
		st.AITTSModel = v
	case "ai_tts_voice":
		st.AITTSVoice = v
	case "ai_language":
		st.AILanguage = v
	case "ai_question":
		st.AIQuestion = v != "0"
	case "ai_weekly_story":
		st.AIWeeklyStory = v != "0"
	case "language":
		st.Language = v
	case "ai_send_text":
		st.AISendText = v != "0"
	case "google_client_id":
		st.GoogleClientID = v
	case "google_client_secret":
		st.GoogleClientSecret = v
	case "google_refresh_token":
		st.GoogleRefreshToken = v
	case "google_account":
		st.GoogleAccount = v
	case "google_last_sync":
		st.GoogleLastSync = v
	}
}

// settingValues flattens settings into key → value.
func settingValues(st Settings) map[string]string {
	b := map[bool]string{true: "1", false: "0"}
	kv := map[string]string{
		"title":                st.Title,
		"date_format":          st.DateFormat,
		"time_format":          st.TimeFormat,
		"week_start":           st.WeekStart,
		"home_currency":        st.HomeCurrency,
		"distance_unit":        st.DistanceUnit,
		"home_lat":             strconv.FormatFloat(st.HomeLat, 'f', -1, 64),
		"home_lon":             strconv.FormatFloat(st.HomeLon, 'f', -1, 64),
		"home_place":           st.HomePlace,
		"language":             st.Language,
		"prompt":               st.Prompt,
		"notify_url":           st.NotifyURL,
		"notify_format":        st.NotifyFormat,
		"notify_evening":       b[st.NotifyEvening],
		"evening_hour":         strconv.Itoa(st.EveningHour),
		"notify_morning":       b[st.NotifyMorning],
		"morning_hour":         strconv.Itoa(st.MorningHour),
		"setup_complete":       b[st.SetupComplete],
		"ai_provider":          st.AIProvider,
		"ai_key":               st.AIKey,
		"ai_model":             st.AIModel,
		"ai_base_url":          st.AIBaseURL,
		"ai_transcribe_model":  st.AITranscribeModel,
		"ai_image_key":         st.AIImageKey,
		"ai_image_model":       st.AIImageModel,
		"ai_transcribe_key":    st.AITranscribeKey,
		"ai_ocr_key":           st.AIOCRKey,
		"ai_ocr_model":         st.AIOCRModel,
		"ai_rerank_key":        st.AIRerankKey,
		"ai_rerank_model":      st.AIRerankModel,
		"ai_tts_key":           st.AITTSKey,
		"ai_tts_model":         st.AITTSModel,
		"ai_tts_voice":         st.AITTSVoice,
		"ai_language":          st.AILanguage,
		"ai_question":          b[st.AIQuestion],
		"ai_weekly_story":      b[st.AIWeeklyStory],
		"ai_send_text":         b[st.AISendText],
		"google_client_id":     st.GoogleClientID,
		"google_client_secret": st.GoogleClientSecret,
		"google_refresh_token": st.GoogleRefreshToken,
		"google_account":       st.GoogleAccount,
		"google_last_sync":     st.GoogleLastSync,
	}
	return kv
}

// SaveSettings persists the user's personal settings (never the box-wide ones).
func (s *Store) SaveSettings(ctx context.Context, userID int64, st Settings) error {
	if userID <= 0 {
		return fmt.Errorf("personal settings need a user")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range settingValues(st) {
		if BoxSettingKeys[k] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_settings (user_id, key, value) VALUES (?, ?, ?) ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value`, userID, k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SaveBoxSettings persists the box-wide settings (title, setup_complete).
func (s *Store) SaveBoxSettings(ctx context.Context, st Settings) error {
	kv := settingValues(st)
	for k := range BoxSettingKeys {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, kv[k]); err != nil {
			return err
		}
	}
	return nil
}

// HasPersonalSettings reports whether the user has saved any personal setting.
func (s *Store) HasPersonalSettings(ctx context.Context, userID int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM user_settings WHERE user_id = ?`, userID).Scan(&n)
	return n > 0
}

// AdoptLegacySettings moves personal keys still sitting in the box-wide table
// (written before there was any user) to the owner. Idempotent.
func (s *Store) AdoptLegacySettings(ctx context.Context, ownerID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_settings (user_id, key, value) SELECT ?, key, value FROM settings WHERE key NOT IN ('title', 'setup_complete')`, ownerID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE key NOT IN ('title', 'setup_complete')`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

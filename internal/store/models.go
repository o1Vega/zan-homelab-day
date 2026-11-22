package store

// User is an account that can sign in. The first user is the admin; admins
// manage other users and the box-wide settings (title). Everything else is
// personal: entries, moments, pins, stories, questions and the settings
// (AI, notifications, home place, journal options).
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Entry is what the user wrote about one day.
type Entry struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Date      string `json:"date"`
	Highlight string `json:"highlight"`
	Text      string `json:"text"`
	Mood      int    `json:"mood"`
	Tags      string `json:"tags"`
	Pinned    bool   `json:"pinned"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	Moments int `json:"moments,omitempty"` // derived on lists
	Words   int `json:"words,omitempty"`
}

// Moment is one short line at a time of day.
type Moment struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Date      string `json:"date"`
	Time      string `json:"time"`
	Text      string `json:"text"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

// MomentKinds the UI offers.
var MomentKinds = []string{"note", "win", "thanks", "idea", "event"}

// ValidKind reports whether k is a moment kind.
func ValidKind(k string) bool {
	for _, x := range MomentKinds {
		if x == k {
			return true
		}
	}
	return false
}

// Pin is something the user kept on the day.
type Pin struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Date      string `json:"date"`
	Source    string `json:"source"`
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Image     string `json:"image"`
	CreatedAt string `json:"created_at"`
}

// Settings are one person's options plus the box-wide title (see BoxSettingKeys).
type Settings struct {
	AIProvider    string  `json:"ai_provider"`
	Title         string  `json:"title"`
	DateFormat    string  `json:"date_format"`
	TimeFormat    string  `json:"time_format"`
	WeekStart     string  `json:"week_start"`
	HomeCurrency  string  `json:"home_currency"`
	DistanceUnit  string  `json:"distance_unit"` // km | mi
	HomeLat       float64 `json:"home_lat"`
	HomeLon       float64 `json:"home_lon"`
	HomePlace     string  `json:"home_place"`
	Prompt        string  `json:"prompt"`   // the question above the text box
	Language      string  `json:"language"` // user interface language
	NotifyURL     string  `json:"notify_url"`
	NotifyFormat  string  `json:"notify_format"`
	NotifyEvening bool    `json:"notify_evening"`
	EveningHour   int     `json:"evening_hour"`
	NotifyMorning bool    `json:"notify_morning"`
	MorningHour   int     `json:"morning_hour"`
	SetupComplete bool    `json:"setup_complete"`

	// AI (OpenAI-compatible). Empty key = every AI feature hidden.
	AIKey             string `json:"ai_key"`
	AIModel           string `json:"ai_model"`
	AIBaseURL         string `json:"ai_base_url"`
	AITranscribeModel string `json:"ai_transcribe_model"`
	// Per-service keys: one endpoint may require a key per feature.
	AIImageKey      string `json:"ai_image_key"`
	AIImageModel    string `json:"ai_image_model"`
	AITranscribeKey string `json:"ai_transcribe_key"`
	AIOCRKey        string `json:"ai_ocr_key"`
	AIOCRModel      string `json:"ai_ocr_model"`
	AIRerankKey     string `json:"ai_rerank_key"`
	AIRerankModel   string `json:"ai_rerank_model"`
	AITTSKey        string `json:"ai_tts_key"`
	AITTSModel      string `json:"ai_tts_model"`
	AITTSVoice      string `json:"ai_tts_voice"`
	AILanguage      string `json:"ai_language"`     // "" = same as the entries
	AIQuestion      bool   `json:"ai_question"`     // tonight's question on the day page + evening push
	AIWeeklyStory   bool   `json:"ai_weekly_story"` // Sunday evening: the week as a story, pushed
	AISendText      bool   `json:"ai_send_text"`    // include what you wrote, not only the moments and kept items

	// Google Photos: the user's own OAuth client (Console project) plus the
	// refresh token the box holds after the OAuth dance.
	GoogleClientID     string `json:"google_client_id"`
	GoogleClientSecret string `json:"google_client_secret"`
	GoogleRefreshToken string `json:"google_refresh_token"`
	GoogleAccount      string `json:"google_account"` // the Google account email, for display
	GoogleLastSync     string `json:"google_last_sync"`
}

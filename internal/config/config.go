// Package config loads runtime configuration from environment variables.
//
// Every option has a sensible default so that `docker run` with a single
// volume works out of the box. See docs/configuration.md for the full list.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	Host           string
	Port           int
	BaseURL        string
	DataDir        string
	DBPath         string
	FilesDir       string
	LogLevel       string
	LogFormat      string // text | json
	TrustProxy     bool
	MetricsEnabled bool

	AuthMode             string // local | proxy | none
	AuthProxyHeader      string
	AuthProxyEmailHeader string
	SessionTTL           time.Duration
	SecureCookies        bool
	AdminEmail           string
	AdminPassword        string
	AdminName            string

	// Sign in with an OpenID Connect provider.
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCName         string // button label
	OIDCAutoCreate   bool   // unknown verified emails get a member account

	DefaultCurrency string
	HomeLat         float64
	HomeLon         float64
	HomePlace       string

	// Integration defaults; the values saved under Settings → Integrations win.
	NotifyURL    string
	NotifyFormat string // ntfy | apprise | json

	OpenAIKey     string
	OpenAIModel   string
	OpenAIBaseURL string

	// The AI day video (POST /memories/{id}/video?engine=ai): a local
	// text-to-video script (LTX-Video 2B + MLX T5) rendered on the box.
	AIVideoScript  string
	AIVideoPython  string
	AIVideoSeconds int

	// The LLM that themes a public post from the day's description:
	// palette, effect, background, typography accents for the post page.
	AIDesignBaseURL string
	AIDesignKey     string
	AIDesignModel   string

	// Outbound lookups identify as this.
	UserAgent string

	DemoData bool

	// Warnings are logged once the logger exists (settings that are ignored).
	Warnings []string
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

// loadDotEnv reads a simple KEY=VALUE .env file and exports any variable that
// is not already present in the real environment. It never overrides an
// existing value, so real environment variables always win. Lines that are
// blank, start with #, or have no '=' are skipped. A missing file is not an
// error.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k == "" {
			continue
		}
		if _, present := os.LookupEnv(k); present {
			continue
		}
		os.Setenv(k, v)
	}
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(env(key, "")) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envInt(key string, def int) int {
	if v := env(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := env(key, ""); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := env(key, ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// Load reads configuration from the environment. A .env file in the working
// directory is loaded first, but it never overrides variables already set
// in the real environment.
func Load() (*Config, error) {
	loadDotEnv(".env")
	dataDir := env("DATA_DIR", "./data")
	c := &Config{
		Host:           env("HOST", "0.0.0.0"),
		Port:           envInt("PORT", 8080),
		BaseURL:        strings.TrimRight(env("BASE_URL", ""), "/"),
		DataDir:        dataDir,
		DBPath:         env("DB_PATH", dataDir+"/zan-day.db"),
		FilesDir:       env("FILES_DIR", dataDir+"/files"),
		LogLevel:       strings.ToLower(env("LOG_LEVEL", "info")),
		LogFormat:      strings.ToLower(env("LOG_FORMAT", "text")),
		TrustProxy:     envBool("TRUST_PROXY", true),
		MetricsEnabled: envBool("METRICS_ENABLED", true),

		AuthMode:             strings.ToLower(env("AUTH_MODE", "local")),
		AuthProxyHeader:      env("AUTH_PROXY_HEADER", "Remote-User"),
		AuthProxyEmailHeader: env("AUTH_PROXY_EMAIL_HEADER", "Remote-Email"),
		SessionTTL:           envDuration("SESSION_TTL", 720*time.Hour),
		SecureCookies:        envBool("SECURE_COOKIES", false),
		AdminEmail:           strings.ToLower(env("ADMIN_EMAIL", "")),
		AdminPassword:        env("ADMIN_PASSWORD", ""),
		AdminName:            env("ADMIN_NAME", "Admin"),

		OIDCIssuer:       strings.TrimRight(env("OIDC_ISSUER", ""), "/"),
		OIDCClientID:     env("OIDC_CLIENT_ID", ""),
		OIDCClientSecret: env("OIDC_CLIENT_SECRET", ""),
		OIDCName:         env("OIDC_NAME", "single sign-on"),
		OIDCAutoCreate:   envBool("OIDC_AUTO_CREATE", true),

		DefaultCurrency: strings.ToUpper(env("DEFAULT_CURRENCY", "EUR")),
		HomeLat:         envFloat("HOME_LAT", 0),
		HomeLon:         envFloat("HOME_LON", 0),
		HomePlace:       env("HOME_PLACE", ""),

		NotifyURL:    env("NOTIFY_URL", ""),
		NotifyFormat: strings.ToLower(env("NOTIFY_FORMAT", "")),

		OpenAIKey:     env("OPENAI_API_KEY", ""),
		OpenAIModel:   env("OPENAI_MODEL", ""),
		OpenAIBaseURL: strings.TrimRight(env("OPENAI_BASE_URL", ""), "/"),

		AIVideoScript:  env("AI_VIDEO_SCRIPT", "/tmp/vidgen/ai_video.py"),
		AIVideoPython:  env("AI_VIDEO_PYTHON", "/tmp/vidgen/venv/bin/python"),
		AIVideoSeconds: envInt("AI_VIDEO_SECONDS", 5),

		AIDesignBaseURL: strings.TrimRight(env("AI_DESIGN_BASE_URL", "https://llm.alem.ai/v1"), "/"),
		AIDesignKey:     env("AI_DESIGN_KEY", ""),
		AIDesignModel:   env("AI_DESIGN_MODEL", "kazllm"),

		UserAgent: env("USER_AGENT", "ZanDay/1.0"),

		DemoData: envBool("DEMO_DATA", false),
	}
	switch c.AuthMode {
	case "local", "proxy", "none":
	default:
		return nil, fmt.Errorf("AUTH_MODE must be one of local, proxy, none (got %q)", c.AuthMode)
	}
	if c.OIDCIssuer != "" && (c.OIDCClientID == "" || c.BaseURL == "") {
		return nil, fmt.Errorf("OIDC_ISSUER needs OIDC_CLIENT_ID and BASE_URL (the callback is BASE_URL/api/v1/auth/oidc/callback)")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return nil, fmt.Errorf("PORT out of range: %d", c.Port)
	}
	return c, nil
}

// Addr returns host:port.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

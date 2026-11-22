// Package server wires the HTTP API, auth and the embedded SPA.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	"example.com/zan-homelab-day/internal/config"
	"example.com/zan-homelab-day/internal/store"
)

// Version is injected at build time.
var Version = "dev"

// Server holds dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	log      *slog.Logger
	webFS    fs.FS
	started  time.Time
	requests atomic.Int64
	errors   atomic.Int64

	notifyMu sync.Mutex
	notified map[string]string // "<user>:evening"/"<user>:morning"/"<user>:weekly" → last day sent

	oidcMu sync.Mutex
	oidc   *oidcProvider
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS) *Server {
	// box-wide defaults every account starts from (no personal values here)
	st.Defaults = func(d *store.Settings) {
		if cfg.DefaultCurrency != "" {
			d.HomeCurrency = cfg.DefaultCurrency
		}
	}
	return &Server{cfg: cfg, store: st, log: log, webFS: webFS, started: time.Now(), notified: map[string]string{}, oidc: &oidcProvider{issuerURL: cfg.OIDCIssuer}}
}

// Bootstrap seeds the first admin (if configured) and the owner's
// integration defaults, then starts the background work.
func (s *Server) Bootstrap(ctx context.Context) error {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 && s.cfg.AuthMode == "local" && s.cfg.AdminEmail != "" && s.cfg.AdminPassword != "" {
		if len(s.cfg.AdminPassword) < 8 {
			return errors.New("ADMIN_PASSWORD must be at least 8 characters")
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), bcrypt.DefaultCost)
		u := &store.User{Email: s.cfg.AdminEmail, Name: s.cfg.AdminName, PasswordHash: string(h), Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		st, _ := s.store.GetSettings(ctx, 0)
		st.SetupComplete = true
		if err := s.store.SaveBoxSettings(ctx, st); err != nil {
			return err
		}
		s.log.Info("admin user created from environment", "email", u.Email)
	}
	if err := s.seedOwner(ctx); err != nil {
		return err
	}
	_ = s.store.PurgeSessions(ctx)
	go s.janitor(ctx)
	return nil
}

// seedOwner gives the owner (the first admin) the AI key, notification route
// and home place from the environment. Those are the owner's own accounts,
// so nobody else gets them. Only empty values are filled.
func (s *Server) seedOwner(ctx context.Context) error {
	owner := s.store.OwnerID(ctx)
	if owner == 0 {
		return nil
	}
	if err := s.store.AdoptLegacySettings(ctx, owner); err != nil {
		return err
	}
	st, err := s.store.GetSettings(ctx, owner)
	if err != nil {
		return err
	}
	changed := !s.store.HasPersonalSettings(ctx, owner)
	set := func(dst *string, v string) {
		if *dst == "" && v != "" {
			*dst = v
			changed = true
		}
	}
	set(&st.NotifyURL, s.cfg.NotifyURL)
	set(&st.AIKey, s.cfg.OpenAIKey)
	set(&st.AIModel, s.cfg.OpenAIModel)
	set(&st.AIBaseURL, s.cfg.OpenAIBaseURL)
	if s.cfg.NotifyFormat != "" && st.NotifyURL == s.cfg.NotifyURL && st.NotifyFormat != s.cfg.NotifyFormat {
		st.NotifyFormat = s.cfg.NotifyFormat
		changed = true
	}
	if st.HomeLat == 0 && st.HomeLon == 0 && (s.cfg.HomeLat != 0 || s.cfg.HomeLon != 0) {
		st.HomeLat, st.HomeLon, st.HomePlace = s.cfg.HomeLat, s.cfg.HomeLon, s.cfg.HomePlace
		changed = true
	}
	if !changed {
		return nil
	}
	return s.store.SaveSettings(ctx, owner, st)
}

// janitor pushes the daily reminders for every account once their hour has passed.
func (s *Server) janitor(ctx context.Context) {
	first := time.NewTimer(10 * time.Second)
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	tick := func() {
		users, _ := s.store.ListUsers(ctx)
		for i := range users {
			s.maybeNotify(ctx, &users[i])
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			tick()
		case <-t.C:
			tick()
		}
	}
}

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(180 * time.Second))

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/setup", s.handleSetup)
		r.Post("/auth/register", s.handleRegister)
		r.Get("/auth/status", s.handleAuthStatus)
		r.Get("/auth/oidc/start", s.handleOIDCStart)
		r.Get("/auth/oidc/callback", s.handleOIDCCallback)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)

			r.Get("/dashboard", s.handleDashboard)
			r.Get("/home", s.handleHome)
			r.Get("/meta", s.handleMeta)
			r.Get("/search", s.handleSearch)

			r.Get("/today", s.handleToday)
			r.Get("/days/{date}", s.handleDay)
			r.Put("/days/{date}", s.handlePutEntry)
			r.Delete("/days/{date}", s.handleDeleteEntry)
			r.Post("/days/{date}/pins", s.handleCreatePin)
			r.Delete("/pins/{id}", s.handleDeletePin)

			r.Get("/entries", s.handleListEntries)
			r.Post("/entries", s.handleQuickEntry)
			r.Get("/tags", s.handleTags)
			r.Get("/calendar", s.handleCalendar)
			r.Get("/years", s.handleYears)
			r.Get("/years/{year}", s.handleYear)

			r.Get("/moments", s.handleListMoments)
			r.Post("/moments", s.handleCreateMoment)
			r.Put("/moments/{id}", s.handleUpdateMoment)
			r.Delete("/moments/{id}", s.handleDeleteMoment)

			r.Post("/ai/days/{date}/draft", s.handleAIDraft)
			r.Get("/ai/days/{date}/question", s.handleAIQuestion)
			r.Post("/ai/days/{date}/suggest", s.handleAISuggest)
			r.Post("/ai/transcribe", s.handleAITranscribe)
			r.Get("/ai/stories", s.handleListStories)
			r.Get("/ai/stories/{kind}/{period}", s.handleGetStory)
			r.Post("/ai/stories/{kind}/{period}", s.handleMakeStory)
			r.Delete("/ai/stories/{kind}/{period}", s.handleDeleteStory)
			r.Get("/ai/asks", s.handleListAsks)
			r.Post("/ai/asks", s.handleAsk)
			r.Delete("/ai/asks/{id}", s.handleDeleteAsk)
			r.Get("/ai/usage", s.handleAIUsage)

			r.Get("/memories", s.handleMemoriesList)
			r.Post("/memories", s.handleMemoryCreate)
			r.Get("/memories/{id}", s.handleMemoryGet)
			r.Put("/memories/{id}", s.handleMemoryUpdate)
			r.Delete("/memories/{id}", s.handleMemoryDelete)
			r.Post("/memories/{id}/photos", s.handleMemoryPhotos)
			r.Delete("/memories/{id}/photos/{pid}", s.handleMemoryPhotoDelete)
			r.Post("/memories/{id}/track", s.handleMemoryTrack)
			r.Put("/memories/{id}/track", s.handleMemoryTrackLink)
			r.Post("/memories/{id}/track/find", s.handleMemoryFindTrack)
			r.Post("/memories/{id}/video", s.handleMemoryVideo)
			r.Post("/memories/{id}/narration", s.handleMemoryNarration)
			r.Post("/memories/{id}/analyze", s.handleMemoryAnalyze)
			r.Post("/memories/{id}/background", s.handleMemoryBackground)
			r.Post("/memories/{id}/poster", s.handleMemoryPoster)
			r.Get("/memories/{id}/asset", s.handleMemoryAsset)
			r.Get("/memories/{id}/theme", s.handleMemoryTheme)
			r.Put("/memories/{id}/public", s.handleMemoryPublic)
			r.Get("/posts", s.handlePostsList)
			r.Get("/posts/{id}", s.handlePostGet)
			r.Get("/posts/{id}/asset", s.handlePostAsset)
			r.Get("/posts/{id}/theme", s.handlePostTheme)
			r.Put("/posts/{id}/reaction/{kind}", s.handlePostReaction)
			r.Post("/posts/{id}/comments", s.handlePostComment)
			r.Delete("/posts/{id}/comments/{cid}", s.handlePostCommentDelete)
			r.Put("/memories/{id}/featured", s.handleMemoryFeatured)
			r.Get("/people", s.handlePeopleList)
			r.Get("/people/{id}", s.handlePerson)
			r.Post("/people/{id}/follow", s.handlePersonFollowOn)
			r.Delete("/people/{id}/follow", s.handlePersonFollowOff)

			r.Post("/integrations/test", s.handleIntegrationTest)
			r.Get("/geo/search", s.handleGeocode)
			r.Post("/notify/test", s.handleNotifyTest)
			r.Post("/notify/run", s.handleNotifyRun)

			r.Get("/integrations/google/start", s.handleGoogleStart)
			r.Get("/integrations/google/callback", s.handleGoogleCallback)
			r.Post("/integrations/google/import", s.handleGoogleImport)
			r.Delete("/integrations/google", s.handleGoogleDisconnect)
			r.Post("/integrations/import/zip", s.handleImportZip)
			r.Get("/integrations/import/assets/{date}", s.handleImportAsset)

			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Put("/users/{id}", s.handleUpdateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/settings/system", s.handleSystemInfo)

			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)

			r.Get("/backup", s.handleBackup)
			r.Get("/export.json", s.handleExportJSON)
			r.Get("/export.md", s.handleExportMarkdown)
			r.Post("/import", s.handleImportJSON)
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// dateParam reads {date}, accepting "today" and "yesterday".
func dateParam(r *http.Request) (string, bool) {
	d := chi.URLParam(r, "date")
	switch d {
	case "today":
		return store.Today(), true
	case "yesterday":
		return store.AddDays(store.Today(), -1), true
	}
	return d, validDate(d)
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP zan_day_info Build info.\n# TYPE zan_day_info gauge\nzan_day_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP zan_day_uptime_seconds Process uptime.\n# TYPE zan_day_uptime_seconds gauge\nzan_day_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP zan_day_http_requests_total HTTP requests served.\n# TYPE zan_day_http_requests_total counter\nzan_day_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP zan_day_http_errors_total HTTP 5xx responses.\n# TYPE zan_day_http_errors_total counter\nzan_day_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP zan_day_rows Rows per table.\n# TYPE zan_day_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "zan_day_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}

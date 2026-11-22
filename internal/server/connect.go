package server

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"example.com/zan-homelab-day/internal/store"
)

// ---- Google Photos (the user's own OAuth client) ----

// The Google endpoints are vars (not consts) so tests can point them at a
// fake server.
var (
	googleAuthURL   = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL  = "https://oauth2.googleapis.com/token"
	googleUserURL   = "https://www.googleapis.com/oauth2/v3/userinfo"
	photosSearchURL = "https://photoslibrary.googleapis.com/v1/mediaItems:search"
)

const (
	photosScope    = "https://www.googleapis.com/auth/photoslibrary.readonly"
	photosPageSize = 100
	photosMaxPages = 25 // hard import cap per run
	zipMaxFiles    = 5000
	zipMaxBytes    = 500 << 20
	zipMaxFile     = 50 << 20
	zipUploadLimit = 32 << 20
	zipMaxArchive  = 2 << 30
	importSource   = "import"
	googleSource   = "google_photos"
)

var connectHTTP = &http.Client{Timeout: 60 * time.Second}

var zipImageExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}

func (s *Server) googleRedirectURI() string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/api/v1/integrations/google/callback"
}

// handleGoogleStart begins the OAuth dance: 302 to Google's consent screen.
// The user must have saved their own Client ID/Secret in settings first.
func (s *Server) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	ctx := r.Context()
	st, err := s.store.GetSettings(ctx, u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	if st.GoogleClientID == "" || st.GoogleClientSecret == "" {
		writeErr(w, http.StatusBadRequest, "save the Google client id and secret first")
		return
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		s.fail(w, err, "state")
		return
	}
	state := hex.EncodeToString(buf)
	if err := s.store.CachePut(ctx, fmt.Sprintf("google:oauth:%d", u.ID), state); err != nil {
		s.fail(w, err, "oauth state")
		return
	}
	q := url.Values{
		"client_id":     {st.GoogleClientID},
		"redirect_uri":  {s.googleRedirectURI()},
		"response_type": {"code"},
		"scope":         {photosScope},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	http.Redirect(w, r, googleAuthURL+"?"+q.Encode(), http.StatusFound)
}

// handleGoogleCallback receives the OAuth redirect from Google.
func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	ctx := r.Context()
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Redirect(w, r, "/settings/connect?google_error="+url.QueryEscape(e), http.StatusFound)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		http.Redirect(w, r, "/settings/connect?google_error=missing_code", http.StatusFound)
		return
	}
	cached, _, ok := s.store.CacheGet(ctx, fmt.Sprintf("google:oauth:%d", u.ID))
	s.store.CacheDelete(ctx, fmt.Sprintf("google:oauth:%d", u.ID))
	if !ok || cached != state {
		http.Redirect(w, r, "/settings/connect?google_error=state_mismatch", http.StatusFound)
		return
	}
	st, err := s.store.GetSettings(ctx, u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	tok, err := googleTokenPost(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {st.GoogleClientID},
		"client_secret": {st.GoogleClientSecret},
		"redirect_uri":  {s.googleRedirectURI()},
	})
	if err != nil {
		http.Redirect(w, r, "/settings/connect?google_error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = tok.AccessToken
	}
	email := googleAccountEmail(ctx, tok.AccessToken)
	st.GoogleRefreshToken = refresh
	st.GoogleAccount = email
	if err := s.store.SaveSettings(ctx, u.ID, st); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	http.Redirect(w, r, "/settings/connect?google_ok=1", http.StatusFound)
}

// handleGoogleDisconnect drops the refresh token (client id/secret stay,
// so reconnect is one click).
func (s *Server) handleGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	ctx := r.Context()
	st, err := s.store.GetSettings(ctx, u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	st.GoogleRefreshToken = ""
	st.GoogleLastSync = ""
	if err := s.store.SaveSettings(ctx, u.ID, st); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleGoogleImport pulls recent photos/videos from Google Photos into pins.
func (s *Server) handleGoogleImport(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	ctx := r.Context()
	st, err := s.store.GetSettings(ctx, u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	if st.GoogleRefreshToken == "" {
		writeErr(w, http.StatusBadRequest, "connect Google Photos first")
		return
	}
	tok, err := googleTokenPost(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {st.GoogleRefreshToken},
		"client_id":     {st.GoogleClientID},
		"client_secret": {st.GoogleClientSecret},
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "google token: "+err.Error())
		return
	}
	accessToken := tok.AccessToken
	before, _ := s.store.CountPins(ctx, u.ID)
	fetched := 0
	pins := []store.Pin{}
	pageToken := ""
	for page := 0; page < photosMaxPages; page++ {
		q := url.Values{"pageSize": {fmt.Sprint(photosPageSize)}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		var out struct {
			MediaItems    []photoItem `json:"mediaItems"`
			NextPageToken string      `json:"nextPageToken"`
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, photosSearchURL+"?"+q.Encode(), nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := connectHTTP.Do(req)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "google photos: "+err.Error())
			return
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			writeErr(w, http.StatusBadGateway, fmt.Sprintf("google photos: http %d", resp.StatusCode))
			return
		}
		if err := json.Unmarshal(body, &out); err != nil {
			s.fail(w, err, "google photos response")
			return
		}
		for _, it := range out.MediaItems {
			fetched++
			date, stamp := photoWhen(it.Basic.CreationTime)
			if date == "" {
				date, stamp = time.Now().Format("2006-01-02"), ""
			}
			title := firstNonEmpty(it.Description, it.ID)
			image := it.BaseURL
			if it.MediaItem != "VIDEO" {
				image += "=w800"
			}
			pins = append(pins, store.Pin{UserID: u.ID, Date: date, Source: googleSource, Ref: it.ID, Title: title, Subtitle: stamp, Image: image})
		}
		if out.NextPageToken == "" {
			break
		}
		pageToken = out.NextPageToken
	}
	if len(pins) > 0 {
		if err := s.store.CreatePins(ctx, pins); err != nil {
			s.fail(w, err, "store pins")
			return
		}
	}
	after, _ := s.store.CountPins(ctx, u.ID)
	newPins := after - before
	if newPins < 0 {
		newPins = 0
	}
	st.GoogleLastSync = store.Now()
	if err := s.store.SaveSettings(ctx, u.ID, st); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "fetched": fetched, "new": newPins})
}

type photoItem struct {
	ID          string `json:"id"`
	MediaItem   string `json:"mediaItemType"`
	BaseURL     string `json:"baseUrl"`
	Description string `json:"description"`
	Basic       struct {
		CreationTime string `json:"creationTime"`
	} `json:"basic"`
}

// photoWhen turns a Google creationTime (RFC3339) into a date + "dd.mm hh:mm".
func photoWhen(creationTime string) (date, stamp string) {
	t, err := time.Parse(time.RFC3339Nano, creationTime)
	if err != nil {
		return "", ""
	}
	return t.Format("2006-01-02"), t.Format("02.01 15:04")
}

type googleToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func googleTokenPost(ctx context.Context, form url.Values) (*googleToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := connectHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tok googleToken
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("bad token response: %w", err)
	}
	if tok.Error != "" {
		return nil, errors.New(tok.Error + ": " + tok.ErrorDesc)
	}
	if tok.AccessToken == "" {
		return nil, errors.New("empty access token")
	}
	return &tok, nil
}

// googleAccountEmail is best-effort: an empty string is fine.
func googleAccountEmail(ctx context.Context, accessToken string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := connectHTTP.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(body, &out)
	return out.Email
}

// ---- ZIP import (Google Takeout, iCloud export, social exports) ----

// handleImportZip takes a .zip upload, copies images into data/days/<date>/
// and adds a pin per image. Google Takeout photos are dated from their
// Photos.json sidecar when present; everything else uses the file mtime.
func (s *Server) handleImportZip(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	ctx := r.Context()
	if err := r.ParseMultipartForm(zipUploadLimit); err != nil {
		writeErr(w, http.StatusBadRequest, "expected a file field named file")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "expected a file field named file")
		return
	}
	defer file.Close()
	if header.Size > zipMaxArchive {
		writeErr(w, http.StatusRequestEntityTooLarge, "archive too large")
		return
	}
	zr, err := zip.NewReader(io.NewSectionReader(file, 0, header.Size), header.Size)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "not a valid zip archive")
		return
	}
	meta := s.takeoutMeta(zr)
	imported, skipped := 0, 0
	perDate := map[string]int{}
	dayRoot := filepath.Join(s.cfg.DataDir, "days")
	for _, ze := range zr.File {
		if ze.FileInfo().IsDir() {
			continue
		}
		if imported+skipped >= zipMaxFiles {
			break
		}
		name := ze.Name
		if !zipImageExt[strings.ToLower(filepath.Ext(name))] {
			skipped++
			continue
		}
		if ze.UncompressedSize64 > zipMaxFile {
			skipped++
			continue
		}
		mtime := ze.Modified
		if mtime.Year() < 2000 {
			mtime = time.Now()
		}
		date := mtime.Format("2006-01-02")
		var desc string
		if p, ok := meta[name]; ok {
			if d := epochDate(p.CreationTime); d != "" {
				date = d
			}
			desc = p.Description
		}
		perDate[date]++
		ext := strings.ToLower(filepath.Ext(name))
		rel := fmt.Sprintf("import-%04d%s", perDate[date], ext)
		dir := filepath.Join(dayRoot, date)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			skipped++
			continue
		}
		rc, err := ze.Open()
		if err != nil {
			skipped++
			continue
		}
		dest := filepath.Join(dir, filepath.Base(rel))
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			skipped++
			continue
		}
		if _, err := io.Copy(out, io.LimitReader(rc, zipMaxFile+1)); err != nil {
			out.Close()
			rc.Close()
			os.Remove(dest)
			skipped++
			continue
		}
		out.Close()
		rc.Close()
		ref := sha1short(name)
		pin := store.Pin{
			UserID:   u.ID,
			Date:     date,
			Source:   importSource,
			Ref:      ref,
			Title:    clipRunes(baseName(name), 120),
			Subtitle: clipRunes(desc, 120),
			Image:    "/api/v1/integrations/import/assets/" + date + "?file=" + url.QueryEscape(rel),
		}
		if err := s.store.CreatePin(ctx, &pin); err != nil {
			os.Remove(dest)
			skipped++
			continue
		}
		imported++
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "imported": imported, "skipped": skipped})
}

// handleImportAsset serves an imported image from data/days/<date>/.
func (s *Server) handleImportAsset(w http.ResponseWriter, r *http.Request) {
	userFrom(r.Context())
	date := chi.URLParam(r, "date")
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "bad date")
		return
	}
	file := r.URL.Query().Get("file")
	if !strings.HasPrefix(file, "import-") || strings.ContainsAny(file, "/\\") || file == ".." {
		writeErr(w, http.StatusBadRequest, "bad file")
		return
	}
	http.ServeFile(w, r, filepath.Join(s.cfg.DataDir, "days", date, filepath.Base(file)))
}

// takeoutMeta parses Google Takeout's Photos.json sidecars (if any) and maps
// photo path -> metadata for dating and titles.
func (s *Server) takeoutMeta(zr *zip.Reader) map[string]takeoutPhoto {
	meta := map[string]takeoutPhoto{}
	for _, ze := range zr.File {
		name := ze.Name
		if !strings.EqualFold(filepath.Ext(name), ".json") || !strings.Contains(name, "Photos.json") {
			continue
		}
		rc, err := ze.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(rc, 32<<20))
		rc.Close()
		if err != nil {
			continue
		}
		var photos []takeoutPhoto
		if json.Unmarshal(body, &photos) != nil {
			continue
		}
		for _, p := range photos {
			if p.URL != "" {
				meta[p.URL] = p
			}
		}
	}
	return meta
}

type takeoutPhoto struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	CreationTime float64 `json:"creationTime"`
	Description  string  `json:"description"`
}

func epochDate(sec float64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(int64(sec), 0).UTC().Format("2006-01-02")
}

func baseName(p string) string {
	return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
}

func sha1short(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:8])
}

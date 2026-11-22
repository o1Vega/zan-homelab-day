package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"example.com/zan-homelab-day/internal/store"
)

// connectApp is the fixture for connect.go tests: a full server on an
// httptest server, an admin user, and an authenticated cookie-jar client.
type connectApp struct {
	t   *testing.T
	dir string
	st  *store.Store
	ts  *httptest.Server
	cl  *http.Client // authenticated, follows redirects
	jar *cookiejar.Jar
	uid int64
}

func newConnectApp(t *testing.T) *connectApp {
	t.Helper()
	dir := t.TempDir()
	cfg := testConfig(dir)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	ts := httptest.NewUnstartedServer(nil)
	cfg.BaseURL = "http://" + ts.Listener.Addr().String()
	s := New(cfg, st, log, nil)
	ts.Config.Handler = s.Router()
	ts.Start()
	t.Cleanup(ts.Close)

	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &store.User{Email: "demo@example.com", Name: "A", Role: "admin", PasswordHash: string(hash)}
	if err := st.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	cl := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"email": "demo@example.com", "password": "test-password"})
	res, err := cl.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}
	return &connectApp{t: t, dir: dir, st: st, ts: ts, cl: cl, jar: jar, uid: u.ID}
}

// do runs an authenticated request and parses a JSON object response.
func (a *connectApp) do(method, path string, body io.Reader, ctype string) (int, map[string]any, error) {
	req, err := http.NewRequest(method, a.ts.URL+path, body)
	if err != nil {
		return 0, nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := a.cl.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, nil, err
	}
	var out map[string]any
	if len(b) > 0 {
		if err := json.Unmarshal(b, &out); err != nil {
			return res.StatusCode, nil, err
		}
	}
	return res.StatusCode, out, nil
}

func (a *connectApp) getJSON(path string) (int, map[string]any) {
	code, out, err := a.do(http.MethodGet, path, nil, "")
	if err != nil {
		a.t.Fatal(err)
	}
	return code, out
}

// getRaw is for non-JSON endpoints (imported assets).
func (a *connectApp) getRaw(path string) (int, []byte) {
	req, err := http.NewRequest(http.MethodGet, a.ts.URL+path, nil)
	if err != nil {
		a.t.Fatal(err)
	}
	res, err := a.cl.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	return res.StatusCode, b
}

func (a *connectApp) putJSON(path string, v any) (int, map[string]any) {
	b, _ := json.Marshal(v)
	code, out, err := a.do(http.MethodPut, path, bytes.NewReader(b), "application/json")
	if err != nil {
		a.t.Fatal(err)
	}
	return code, out
}

func (a *connectApp) postJSON(path string) (int, map[string]any) {
	code, out, err := a.do(http.MethodPost, path, nil, "")
	if err != nil {
		a.t.Fatal(err)
	}
	return code, out
}

func (a *connectApp) deleteJSON(path string) (int, map[string]any) {
	code, out, err := a.do(http.MethodDelete, path, nil, "")
	if err != nil {
		a.t.Fatal(err)
	}
	return code, out
}

func (a *connectApp) getSettings() map[string]any {
	code, m := a.getJSON("/api/v1/settings")
	if code != http.StatusOK {
		a.t.Fatalf("GET /settings: %d", code)
	}
	return m
}

// uploadZip POSTs data as multipart form field "file".
func (a *connectApp) uploadZip(name string, data []byte) (int, map[string]any) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		a.t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		a.t.Fatal(err)
	}
	mw.Close()
	code, out, err := a.do(http.MethodPost, "/api/v1/integrations/import/zip", &buf, mw.FormDataContentType())
	if err != nil {
		a.t.Fatal(err)
	}
	return code, out
}

// ---- fake Google (auth, token, userinfo, photos search) ----

type fakeGoogle struct {
	*httptest.Server
	clientID     string
	clientSecret string
}

func newFakeGoogle(t *testing.T, clientID, clientSecret string) *fakeGoogle {
	t.Helper()
	g := &fakeGoogle{clientID: clientID, clientSecret: clientSecret}
	mux := http.NewServeMux()

	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != g.clientID || q.Get("scope") != photosScope || q.Get("state") == "" {
			http.Error(w, "bad auth request", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, q.Get("redirect_uri")+"?"+url.Values{"code": {"code-1"}, "state": {q.Get("state")}}.Encode(), http.StatusFound)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("client_id") != g.clientID || r.FormValue("client_secret") != g.clientSecret {
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
			return
		}
		switch r.FormValue("grant_type") {
		case "authorization_code":
			if r.FormValue("code") != "code-1" {
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "refresh_token": "rt-1", "token_type": "Bearer", "expires_in": 3600})
		case "refresh_token":
			if r.FormValue("refresh_token") != "rt-1" {
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "bad refresh"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at-2", "token_type": "Bearer", "expires_in": 3600})
		default:
			json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
		}
	})

	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"email": "demo@example.com"})
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-2" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("pageToken") {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"mediaItems": []map[string]any{
					{"id": "m1", "mediaItemType": "PHOTO", "baseUrl": "https://lh3.example/m1", "description": "beach", "basic": map[string]string{"creationTime": "2024-03-15T10:00:00+03:00"}},
					{"id": "m2", "mediaItemType": "VIDEO", "baseUrl": "https://lh3.example/m2", "basic": map[string]string{"creationTime": "2024-04-01T08:30:00Z"}},
				},
				"nextPageToken": "p2",
			})
		case "p2":
			json.NewEncoder(w).Encode(map[string]any{
				"mediaItems": []map[string]any{
					{"id": "m3", "mediaItemType": "PHOTO", "baseUrl": "https://lh3.example/m3", "basic": map[string]string{"creationTime": "garbage"}},
				},
			})
		default:
			http.Error(w, "bad page", http.StatusBadRequest)
		}
	})

	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	return g
}

// pointAt redirects the Google endpoints at the fake server for this test.
func (a *connectApp) pointAt(g *fakeGoogle) {
	a.t.Helper()
	old := [4]string{googleAuthURL, googleTokenURL, googleUserURL, photosSearchURL}
	googleAuthURL = g.URL + "/auth"
	googleTokenURL = g.URL + "/token"
	googleUserURL = g.URL + "/userinfo"
	photosSearchURL = g.URL + "/search"
	a.t.Cleanup(func() { googleAuthURL, googleTokenURL, googleUserURL, photosSearchURL = old[0], old[1], old[2], old[3] })
}

// completeGoogleOAuth drives the full browser dance with a client that shares
// a.jar (so the callback is authenticated) and stops once we are back in the
// app on a non-API path.
func (a *connectApp) completeGoogleOAuth() {
	a.t.Helper()
	var last *url.URL
	dance := &http.Client{
		Jar: a.jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			last = req.URL
			if strings.HasPrefix(req.URL.String(), a.ts.URL) && !strings.HasPrefix(req.URL.Path, "/api/") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	res, err := dance.Get(a.ts.URL + "/api/v1/integrations/google/start")
	if err != nil {
		a.t.Fatal(err)
	}
	res.Body.Close()
	if last == nil || last.Path != "/settings/connect" || last.Query().Get("google_ok") != "1" {
		a.t.Fatalf("oauth dance ended at %v", last)
	}
}

// buildTestZip assembles a Google Takeout-like archive:
//   - IMG_0001.jpg (mtime 2024-05-01, overridden by Photos.json -> 2024-03-15)
//   - IMG_0002.png (mtime 1999 -> today)
//   - notes.txt (skipped: not an image)
//   - Photos.json (Takeout sidecar, skipped as a file)
func buildTestZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, content []byte, mod time.Time) {
		h := &zip.FileHeader{Name: name, Modified: mod}
		h.SetMode(0o644)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	add("Photos/IMG_0001.jpg", []byte("jpg-one"), time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC))
	add("Photos/IMG_0002.png", []byte("png-two"), time.Date(1999, 6, 1, 10, 0, 0, 0, time.UTC))
	add("Photos/notes.txt", []byte("skip me"), time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC))
	add("Photos/Google Photos/Album/Photos.json",
		[]byte(`[{"id":"1","url":"Photos/IMG_0001.jpg","creationTime":1710460800,"description":"beach"}]`),
		time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ---- tests ----

func TestGoogleStartWithoutCreds(t *testing.T) {
	a := newConnectApp(t)
	code, m := a.getJSON("/api/v1/integrations/google/start")
	if code != http.StatusBadRequest {
		t.Fatalf("start without creds: got %d, want 400", code)
	}
	if msg, _ := m["error"].(string); !strings.Contains(msg, "save the Google client id and secret first") {
		t.Fatalf("error = %q, want the save-creds hint", msg)
	}
}

func TestGoogleOAuthFlow(t *testing.T) {
	a := newConnectApp(t)
	g := newFakeGoogle(t, "cid", "csecret")
	a.pointAt(g)

	code, _ := a.putJSON("/api/v1/settings", map[string]any{"google_client_id": "cid", "google_client_secret": "csecret"})
	if code != http.StatusOK {
		t.Fatalf("PUT settings: %d", code)
	}

	a.completeGoogleOAuth()

	m := a.getSettings()
	if m["google_connected"] != true {
		t.Errorf("google_connected = %v, want true", m["google_connected"])
	}
	if m["google_account"] != "demo@example.com" {
		t.Errorf("google_account = %v, want demo@example.com", m["google_account"])
	}
	if m["google_client_id"] != "cid" {
		t.Errorf("google_client_id = %v, want cid", m["google_client_id"])
	}
}

func TestGoogleCallbackStateMismatch(t *testing.T) {
	a := newConnectApp(t)
	g := newFakeGoogle(t, "cid", "csecret")
	a.pointAt(g)
	a.putJSON("/api/v1/settings", map[string]any{"google_client_id": "cid", "google_client_secret": "csecret"})

	// No-follow client, but the same jar so the callback sees the session.
	plain := &http.Client{
		Jar:           a.jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := plain.Get(a.ts.URL + "/api/v1/integrations/google/callback?code=x&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("callback: got %d, want 302", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "google_error=state_mismatch") {
		t.Fatalf("Location = %q, want state_mismatch", loc)
	}
}

func TestGoogleImport(t *testing.T) {
	a := newConnectApp(t)
	g := newFakeGoogle(t, "cid", "csecret")
	a.pointAt(g)
	a.putJSON("/api/v1/settings", map[string]any{"google_client_id": "cid", "google_client_secret": "csecret"})
	a.completeGoogleOAuth()

	code, m := a.postJSON("/api/v1/integrations/google/import")
	if code != http.StatusOK {
		t.Fatalf("import: %d %v", code, m)
	}
	if m["fetched"] != float64(3) {
		t.Errorf("fetched = %v, want 3", m["fetched"])
	}
	if m["new"] != float64(3) {
		t.Errorf("new = %v, want 3", m["new"])
	}

	pins, err := a.st.ListPins(context.Background(), a.uid, "2000-01-01", "2100-01-01")
	if err != nil {
		t.Fatal(err)
	}
	byRef := map[string]store.Pin{}
	for _, p := range pins {
		byRef[p.Ref] = p
	}
	if len(byRef) != 3 {
		t.Fatalf("pins = %d, want 3", len(byRef))
	}

	if p := byRef["m1"]; p.Date != "2024-03-15" || p.Image != "https://lh3.example/m1=w800" || p.Title != "beach" || p.Subtitle != "15.03 10:00" {
		t.Errorf("m1 = %+v", p)
	}
	if p := byRef["m2"]; p.Date != "2024-04-01" || p.Image != "https://lh3.example/m2" || p.Subtitle != "01.04 08:30" {
		t.Errorf("m2 = %+v (VIDEO: no =w800 suffix expected)", p)
	}
	if p := byRef["m3"]; p.Date != store.Today() || p.Subtitle != "" || p.Image != "https://lh3.example/m3=w800" || p.Title != "m3" {
		t.Errorf("m3 = %+v (bad creationTime -> today, no stamp)", p)
	}

	if a.getSettings()["google_last_sync"] == "" {
		t.Error("google_last_sync empty after import")
	}

	// Second import: everything upserts, nothing is new.
	code, m = a.postJSON("/api/v1/integrations/google/import")
	if code != http.StatusOK {
		t.Fatalf("re-import: %d %v", code, m)
	}
	if m["new"] != float64(0) {
		t.Errorf("re-import new = %v, want 0", m["new"])
	}
	n, err := a.st.CountPins(context.Background(), a.uid)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("CountPins = %d, want 3", n)
	}
}

func TestGoogleDisconnect(t *testing.T) {
	a := newConnectApp(t)
	g := newFakeGoogle(t, "cid", "csecret")
	a.pointAt(g)
	a.putJSON("/api/v1/settings", map[string]any{"google_client_id": "cid", "google_client_secret": "csecret"})
	a.completeGoogleOAuth()
	a.postJSON("/api/v1/integrations/google/import")

	code, m := a.deleteJSON("/api/v1/integrations/google")
	if code != http.StatusOK || m["ok"] != true {
		t.Fatalf("disconnect: %d %v", code, m)
	}
	if a.getSettings()["google_connected"] != false {
		t.Error("google_connected still true after disconnect")
	}
	// Client id/secret stay, so reconnect is one click.
	if a.getSettings()["google_client_id"] != "cid" {
		t.Error("google_client_id cleared on disconnect, should stay")
	}
	code, m = a.postJSON("/api/v1/integrations/google/import")
	if code != http.StatusBadRequest {
		t.Fatalf("import after disconnect: got %d, want 400", code)
	}
	if msg, _ := m["error"].(string); !strings.Contains(msg, "connect Google Photos first") {
		t.Errorf("error = %q, want connect-first hint", msg)
	}
}

func TestImportZip(t *testing.T) {
	a := newConnectApp(t)
	data := buildTestZip(t)

	code, m := a.uploadZip("takeout.zip", data)
	if code != http.StatusOK {
		t.Fatalf("zip import: %d %v", code, m)
	}
	if m["imported"] != float64(2) {
		t.Errorf("imported = %v, want 2", m["imported"])
	}
	if m["skipped"] != float64(2) { // notes.txt + Photos.json
		t.Errorf("skipped = %v, want 2", m["skipped"])
	}

	// Files landed in the right day folders.
	b, err := os.ReadFile(filepath.Join(a.dir, "days", "2024-03-15", "import-0001.jpg"))
	if err != nil || string(b) != "jpg-one" {
		t.Fatalf("import-0001.jpg: %v %q", err, b)
	}
	today := store.Today()
	b, err = os.ReadFile(filepath.Join(a.dir, "days", today, "import-0001.png"))
	if err != nil || string(b) != "png-two" {
		t.Fatalf("import-0001.png: %v %q", err, b)
	}

	// Pin for the Takeout-dated photo.
	pins, err := a.st.ListPins(context.Background(), a.uid, "2000-01-01", "2100-01-01")
	if err != nil {
		t.Fatal(err)
	}
	var found *store.Pin
	for i := range pins {
		if pins[i].Date == "2024-03-15" && pins[i].Source == "import" {
			found = &pins[i]
			break
		}
	}
	if found == nil {
		t.Fatal("no import pin for 2024-03-15")
	}
	if found.Subtitle != "beach" || found.Title != "IMG_0001" {
		t.Errorf("pin = %+v, want title IMG_0001 subtitle beach", found)
	}
	if found.Image != "/api/v1/integrations/import/assets/2024-03-15?file=import-0001.jpg" {
		t.Errorf("pin.Image = %q", found.Image)
	}

	// Asset is served back.
	code, b = a.getRaw(found.Image)
	if code != http.StatusOK || string(b) != "jpg-one" {
		t.Fatalf("asset GET: %d %q", code, b)
	}

	// Guards: bad date and path traversal.
	code, m = a.getJSON("/api/v1/integrations/import/assets/bad-date?file=import-0001.jpg")
	if code != http.StatusBadRequest {
		t.Errorf("bad date: got %d, want 400", code)
	}
	code, m = a.getJSON("/api/v1/integrations/import/assets/2024-03-15?file=..%2F..%2Fetc%2Fpasswd")
	if code != http.StatusBadRequest {
		t.Errorf("traversal: got %d, want 400", code)
	}
	if msg, _ := m["error"].(string); msg != "bad file" {
		t.Errorf("traversal error = %q, want bad file", msg)
	}

	// Not a zip.
	code, m = a.uploadZip("junk.zip", []byte("not a zip"))
	if code != http.StatusBadRequest {
		t.Fatalf("junk zip: got %d, want 400", code)
	}
	if msg, _ := m["error"].(string); !strings.Contains(msg, "not a valid zip archive") {
		t.Errorf("junk zip error = %q", msg)
	}

	// Re-import: files overwrite, pins upsert (count unchanged).
	code, m = a.uploadZip("takeout.zip", data)
	if code != http.StatusOK {
		t.Fatalf("re-import: %d %v", code, m)
	}
	if m["imported"] != float64(2) {
		t.Errorf("re-import imported = %v, want 2", m["imported"])
	}
	n, err := a.st.CountPins(context.Background(), a.uid)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("CountPins after re-import = %d, want 2", n)
	}
}

func TestConnectHelpers(t *testing.T) {
	if d := epochDate(1710460800); d != "2024-03-15" {
		t.Errorf("epochDate(1710460800) = %q, want 2024-03-15", d)
	}
	if d := epochDate(0); d != "" {
		t.Errorf("epochDate(0) = %q, want empty", d)
	}
	if d, s := photoWhen("2024-03-15T10:00:00+03:00"); d != "2024-03-15" || s != "15.03 10:00" {
		t.Errorf("photoWhen = (%q,%q), want (2024-03-15, 15.03 10:00)", d, s)
	}
	if d, s := photoWhen("nope"); d != "" || s != "" {
		t.Errorf("photoWhen(nope) = (%q,%q), want empty", d, s)
	}
	if got := sha1short("x"); len(got) != 16 {
		t.Errorf("sha1short len = %d, want 16", len(got))
	}
	if b := baseName("a/b/c.JPG"); b != "c" {
		t.Errorf("baseName = %q, want c", b)
	}
}

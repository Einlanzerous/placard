package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	placard "github.com/Einlanzerous/placard"
	"github.com/Einlanzerous/placard/internal/catalog"
	"github.com/Einlanzerous/placard/internal/config"
)

func testServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	entries, err := catalog.Build(placard.Assets)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicRepo == "" {
		cfg.PublicRepo = "Einlanzerous/placard"
	}
	return New(cfg, nil, entries, placard.Assets)
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// /healthz stays unauthenticated and keeps the SWY-192 shape: version, plus a
// sha key that is present and null when the build carried none.
func TestHealthz(t *testing.T) {
	rec := get(t, testServer(t, config.Config{}), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "dev" {
		t.Errorf("version = %v, want dev for an unstamped build", got["version"])
	}
	if v, present := got["sha"]; !present || v != nil {
		t.Errorf("sha = %v (present=%v), want present and null", v, present)
	}
}

func TestServicesIndex(t *testing.T) {
	rec := get(t, testServer(t, config.Config{}), "/api/services")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got servicesJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.UploadsEnabled {
		t.Error("uploads_enabled = true with no token and no store")
	}
	if !strings.HasPrefix(got.CanonicalBase, "https://cdn.jsdelivr.net/gh/Einlanzerous/placard@main/") {
		t.Errorf("canonical_base = %q", got.CanonicalBase)
	}

	statuses := map[string]string{}
	for _, svc := range got.Services {
		statuses[svc.Slug] = svc.Status
	}
	// No store => no checks: present marks are unverified, absent ones unset.
	if statuses["argosy"] != "unverified" {
		t.Errorf("argosy status = %q, want unverified", statuses["argosy"])
	}
	if statuses["wiki"] != "unset" {
		t.Errorf("wiki status = %q, want unset", statuses["wiki"])
	}

	// No PLACARD_PUBLIC_BASE_URL: the edge block says so rather than vanishing.
	if got.Edge.Configured || got.Edge.OK != nil {
		t.Errorf("edge = %+v, want unconfigured with null ok", got.Edge)
	}
}

// With a public base configured but no pass recorded yet, edge.ok stays null
// (unknown), never a false healthy.
func TestServicesEdgeConfigured(t *testing.T) {
	rec := get(t, testServer(t, config.Config{PublicBaseURL: "https://placard.example"}), "/api/services")
	var got servicesJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Edge.Configured || got.Edge.OK != nil || len(got.Edge.Checks) != 0 {
		t.Errorf("edge = %+v, want configured, null ok, no checks", got.Edge)
	}
}

func TestServiceDetailAndUnknown(t *testing.T) {
	s := testServer(t, config.Config{})
	rec := get(t, s, "/api/services/switchyard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var svc serviceJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &svc); err != nil {
		t.Fatal(err)
	}
	if svc.Color != "#e2623d" {
		t.Errorf("switchyard color = %q, want the BRAND.md coral", svc.Color)
	}
	var lightOK bool
	for _, f := range svc.Files {
		if f.Name == "switchyard-mark-light.png" && f.State == "in_repo" &&
			f.CanonicalURL == "https://cdn.jsdelivr.net/gh/Einlanzerous/placard@main/switchyard/switchyard-mark-light.png" {
			lightOK = true
		}
	}
	if !lightOK {
		t.Error("switchyard light mark missing or its canonical URL is off-contract")
	}

	if rec := get(t, s, "/api/services/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown service status = %d, want 404", rec.Code)
	}
}

// The mirror serves the same repo-relative paths jsDelivr does, with image
// content types, publicly.
func TestAssetMirror(t *testing.T) {
	s := testServer(t, config.Config{})
	rec := get(t, s, "/argosy/argosy-mark-light.png")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec := get(t, s, "/argosy/nope.png"); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset = %d, want 404", rec.Code)
	}
	if rec := get(t, s, "/services.json"); rec.Code != http.StatusOK {
		t.Errorf("services.json = %d", rec.Code)
	}
}

func TestFrontPage(t *testing.T) {
	rec := get(t, testServer(t, config.Config{}), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "service marks at a predictable path") {
		t.Error("front page lost its tagline")
	}
}

// Uploads on a deployment with no token (or no database) answer 503, and a
// wrong token answers 401 — never a silent accept on a public host.
func TestUploadGating(t *testing.T) {
	noToken := testServer(t, config.Config{})
	rec := httptest.NewRecorder()
	noToken.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/services/argosy/upload", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("upload with no token configured = %d, want 503", rec.Code)
	}

	// Token configured but store nil: still disabled (nowhere to stage).
	withToken := testServer(t, config.Config{UploadToken: "sekrit"})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/services/argosy/upload", nil)
	req.Header.Set("X-Placard-Token", "sekrit")
	withToken.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("upload with token but no store = %d, want 503", rec.Code)
	}
}

func TestImageContentType(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if ct, err := imageContentType("m.png", png); err != nil || ct != "image/png" {
		t.Errorf("png: ct=%q err=%v", ct, err)
	}
	if ct, err := imageContentType("m.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); err != nil || ct != "image/svg+xml" {
		t.Errorf("svg: ct=%q err=%v", ct, err)
	}
	if _, err := imageContentType("m.html", []byte("<html><body>login</body></html>")); err == nil {
		t.Error("an HTML body (the Access login page failure) must be refused")
	}
	if _, err := imageContentType("m.png", nil); err == nil {
		t.Error("empty upload must be refused")
	}
}

// Package api is Placard's HTTP surface: the designed front page, the mark
// mirror, the machine-readable /api/services index (what PRSR-29 consumes),
// staged uploads, and /healthz.
//
// Everything a launcher or a browser fetches is deliberately unauthenticated —
// that is the service's whole point (IDEA-22). The single write endpoint
// (upload) and staged reads are token-gated instead, because an open write on
// a public host is a free image-hosting service.
package api

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/Einlanzerous/placard/internal/catalog"
	"github.com/Einlanzerous/placard/internal/config"
	"github.com/Einlanzerous/placard/internal/store"
	"github.com/Einlanzerous/placard/internal/version"
)

//go:embed web/index.html
var indexHTML []byte

type Server struct {
	cfg     config.Config
	st      *store.Store // nil = no database: checks and uploads off
	entries []catalog.Entry
	assets  fs.FS
	mux     *http.ServeMux
}

func New(cfg config.Config, st *store.Store, entries []catalog.Entry, assets fs.FS) *Server {
	s := &Server{cfg: cfg, st: st, entries: entries, assets: assets}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/services", s.handleServices)
	mux.HandleFunc("GET /api/services/{slug}", s.handleService)
	mux.HandleFunc("POST /api/services/{slug}/upload", s.handleUpload)
	mux.HandleFunc("GET /api/staged/{id}", s.handleStagedData)
	mux.HandleFunc("GET /services.json", s.handleManifest)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/placard/placard-mark-dark.png", http.StatusFound)
	})
	mux.HandleFunc("GET /{svc}/{file}", s.handleAsset)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux = mux
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Unauthenticated by contract: the compose HEALTHCHECK, uptime-kuma and
	// the delivery reconciler all read this with no credentials, and the
	// reconciler wants the SWY-192 shape — version, plus sha present-and-null
	// when the build carried none.
	var sha any
	if version.Commit != "" {
		sha = version.Commit
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": version.Resolved(),
		"sha":     sha,
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(indexHTML)
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.assets, "services.json")
	if err != nil {
		http.Error(w, "manifest unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

// handleAsset is the mark mirror: the same repo-relative paths jsDelivr
// serves, from the embedded copy of the repo.
func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	svc, file := r.PathValue("svc"), r.PathValue("file")
	name := path.Join(svc, file)
	if name != svc+"/"+file { // path.Join cleaned something away — refuse
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := mime.TypeByExtension(path.Ext(file))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

// authorized checks the staging token (header, bearer, or query param — the
// query form exists so a browser <img> can fetch staged previews).
func (s *Server) authorized(r *http.Request) bool {
	if s.cfg.UploadToken == "" {
		return false
	}
	got := r.Header.Get("X-Placard-Token")
	if got == "" {
		got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.UploadToken)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// storeOr returns the store or writes the standard disabled error.
func (s *Server) storeOr(w http.ResponseWriter) *store.Store {
	if s.st == nil {
		writeError(w, http.StatusServiceUnavailable, "no database configured on this deployment")
		return nil
	}
	return s.st
}

// latestChecks is tolerant: a DB blip must not take the front page down with
// it — the marks are the product, the checks are commentary.
func (s *Server) latestChecks(ctx context.Context) map[string]store.MarkCheck {
	if s.st == nil {
		return nil
	}
	checks, err := s.st.LatestChecks(ctx)
	if err != nil {
		log.Printf("api: latest checks: %v", err)
		return nil
	}
	return checks
}

func (s *Server) stagedFor(ctx context.Context, slug string) []store.StagedUpload {
	if s.st == nil {
		return nil
	}
	ups, err := s.st.StagedUploads(ctx, slug)
	if err != nil {
		log.Printf("api: staged uploads: %v", err)
		return nil
	}
	return ups
}

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/Einlanzerous/placard/internal/catalog"
	"github.com/Einlanzerous/placard/internal/store"
)

// The wire shapes /api/services serves. This is the contract PRSR-29's Access
// application connector consumes — a service's canonical_url plus its latest
// check is exactly the "verify the URL returns a 200 image before writing it"
// input that connector needs.

type fileJSON struct {
	Name         string           `json:"name"`
	Path         string           `json:"path"`
	Role         string           `json:"role"`
	State        string           `json:"state"` // in_repo | missing
	CanonicalURL string           `json:"canonical_url"`
	MirrorURL    string           `json:"mirror_url"`
	Check        *store.MarkCheck `json:"check"` // null until verified
}

type serviceJSON struct {
	Slug         string               `json:"slug"`
	Name         string               `json:"name"`
	Color        string               `json:"color,omitempty"`
	Note         string               `json:"note,omitempty"`
	LegacySource string               `json:"legacy_source,omitempty"`
	Status       string               `json:"status"` // in_repo | unverified | path_rotted | unset
	Files        []fileJSON           `json:"files"`
	Staged       []store.StagedUpload `json:"staged"`
}

type servicesJSON struct {
	Repo           string        `json:"repo"`
	CanonicalBase  string        `json:"canonical_base"`
	UploadsEnabled bool          `json:"uploads_enabled"`
	Services       []serviceJSON `json:"services"`
}

func (s *Server) buildService(e catalog.Entry, checks map[string]store.MarkCheck, staged []store.StagedUpload) serviceJSON {
	svc := serviceJSON{
		Slug:         e.Slug,
		Name:         e.Name,
		Color:        e.Color,
		Note:         e.Note,
		LegacySource: e.LegacySource,
		Files:        make([]fileJSON, 0, len(e.Files)),
		Staged:       staged,
	}
	if svc.Staged == nil {
		svc.Staged = []store.StagedUpload{}
	}

	anyOK, anyBroken := false, false
	for _, f := range e.Files {
		fj := fileJSON{
			Name:         f.Name,
			Path:         f.Path,
			Role:         f.Role,
			State:        "missing",
			CanonicalURL: s.cfg.CanonicalBase() + f.Path,
			MirrorURL:    "/" + f.Path,
		}
		if f.Present {
			fj.State = "in_repo"
			if c, ok := checks[f.Path]; ok {
				c := c
				fj.Check = &c
				if c.OK {
					anyOK = true
				} else {
					anyBroken = true
				}
			}
		}
		svc.Files = append(svc.Files, fj)
	}

	switch {
	case !e.HasMarks():
		svc.Status = "unset"
	case anyBroken:
		svc.Status = "path_rotted"
	case anyOK:
		svc.Status = "in_repo"
	default:
		svc.Status = "unverified"
	}
	return svc
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	checks := s.latestChecks(r.Context())
	staged := s.stagedFor(r.Context(), "")
	bySvc := map[string][]store.StagedUpload{}
	for _, up := range staged {
		bySvc[up.Service] = append(bySvc[up.Service], up)
	}

	resp := servicesJSON{
		Repo:           s.cfg.PublicRepo,
		CanonicalBase:  s.cfg.CanonicalBase(),
		UploadsEnabled: s.cfg.UploadToken != "" && s.st != nil,
	}
	for _, e := range s.entries {
		resp.Services = append(resp.Services, s.buildService(e, checks, bySvc[e.Slug]))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) findEntry(slug string) (catalog.Entry, bool) {
	for _, e := range s.entries {
		if e.Slug == slug {
			return e, true
		}
	}
	return catalog.Entry{}, false
}

func (s *Server) handleService(w http.ResponseWriter, r *http.Request) {
	e, ok := s.findEntry(r.PathValue("slug"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown service")
		return
	}
	svc := s.buildService(e, s.latestChecks(r.Context()), s.stagedFor(r.Context(), e.Slug))
	writeJSON(w, http.StatusOK, svc)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if s.cfg.UploadToken == "" || s.st == nil {
		writeError(w, http.StatusServiceUnavailable, "uploads are disabled on this deployment")
		return
	}
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "staging token required")
		return
	}
	e, ok := s.findEntry(r.PathValue("slug"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown service")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	filename, data, err := readUpload(r)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds the size limit")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ct, err := imageContentType(filename, data)
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
		return
	}

	sum := sha256.Sum256(data)
	id, err := s.st.StageUpload(r.Context(), store.StagedUpload{
		Service:     e.Slug,
		Filename:    path.Base(filename),
		ContentType: ct,
		SizeBytes:   int64(len(data)),
		SHA256:      hex.EncodeToString(sum[:]),
		Data:        data,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "staging failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           id,
		"service":      e.Slug,
		"filename":     path.Base(filename),
		"content_type": ct,
		"size_bytes":   len(data),
	})
}

// readUpload accepts multipart form-data (field "file") or a raw body with
// ?filename= — the former is what the front page sends, the latter is the
// curl-friendly form.
func readUpload(r *http.Request) (string, []byte, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		f, hdr, err := r.FormFile("file")
		if err != nil {
			return "", nil, errors.New(`multipart field "file" required`)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			return "", nil, err
		}
		return hdr.Filename, data, nil
	}
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		return "", nil, errors.New("filename query parameter required for raw uploads")
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "", nil, err
	}
	return filename, data, nil
}

// imageContentType admits PNG/JPEG/GIF/WebP by sniff, and SVG by extension +
// a look at the bytes (DetectContentType calls SVG "text/xml" or
// "text/plain"). Anything else is refused — this store holds marks, not
// arbitrary files.
func imageContentType(filename string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty upload")
	}
	sniffed := http.DetectContentType(data)
	if strings.HasPrefix(sniffed, "image/") && !strings.HasPrefix(sniffed, "image/svg") {
		return sniffed, nil
	}
	if strings.EqualFold(path.Ext(filename), ".svg") &&
		(strings.HasPrefix(sniffed, "text/xml") || strings.HasPrefix(sniffed, "text/plain")) &&
		strings.Contains(string(data[:min(len(data), 4096)]), "<svg") {
		return "image/svg+xml", nil
	}
	return "", errors.New("not a recognized image (" + sniffed + ")")
}

func (s *Server) handleStagedData(w http.ResponseWriter, r *http.Request) {
	st := s.storeOr(w)
	if st == nil {
		return
	}
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "staging token required")
		return
	}
	up, err := st.StagedData(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "no such staged upload")
		return
	}
	w.Header().Set("Content-Type", up.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+up.Filename+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Write(up.Data)
}

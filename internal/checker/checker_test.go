package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Einlanzerous/placard/internal/catalog"
	"github.com/Einlanzerous/placard/internal/store"
)

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)

func testEntries(t *testing.T) []catalog.Entry {
	t.Helper()
	entries, err := catalog.Build(fstest.MapFS{
		"services.json":               {Data: []byte(`{"services":[{"slug":"alpha","name":"Alpha"},{"slug":"placard","name":"Placard"}]}`)},
		"alpha/alpha-mark-light.png":  {Data: pngBytes},
		"alpha/alpha-mark-dark.png":   {Data: pngBytes},
		"placard/placard-mark-light.png": {Data: pngBytes},
		"placard/placard-mark-dark.png":  {Data: pngBytes},
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// A healthy edge: front page 200 text/html, mirror mark 200 image/png.
func TestCheckEdgeHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".png") {
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!DOCTYPE html><title>Placard</title>"))
	}))
	defer srv.Close()

	results := CheckEdge(context.Background(), testEntries(t), srv.URL, nil)
	if len(results) != 2 {
		t.Fatalf("got %d checks, want front page + one mirror mark", len(results))
	}
	for _, c := range results {
		if c.Kind != store.KindEdge {
			t.Errorf("%s: kind = %q", c.File, c.Kind)
		}
		if !c.OK {
			t.Errorf("%s should be healthy: %v", c.File, *c.Error)
		}
	}
	// The service vouches for itself when it can.
	if results[1].Service != "placard" || !strings.Contains(results[1].URL, "placard-mark-light.png") {
		t.Errorf("mirror check should prefer placard's own mark, got %s", results[1].URL)
	}
}

// The Access-gate signature: everything answers 302 to the team login. The
// checker must NOT follow it into a 200 — the redirect is the finding.
func TestCheckEdgeGated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://team.cloudflareaccess.com/cdn-cgi/access/login/x", http.StatusFound)
	}))
	defer srv.Close()

	for _, c := range CheckEdge(context.Background(), testEntries(t), srv.URL, nil) {
		if c.OK {
			t.Errorf("%s passed against a gated edge", c.File)
		}
		if c.Error == nil || !strings.Contains(*c.Error, "302") || !strings.Contains(*c.Error, "cloudflareaccess.com") {
			t.Errorf("%s: error should name the redirect, got %v", c.File, c.Error)
		}
	}
}

// The other gate shape: a 200 HTML login page where an image belongs.
func TestCheckEdgeHTMLWhereImageBelongs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>sign in</html>"))
	}))
	defer srv.Close()

	results := CheckEdge(context.Background(), testEntries(t), srv.URL, nil)
	if !results[0].OK {
		t.Error("front page IS html — that check should pass")
	}
	if results[1].OK {
		t.Error("html where an image belongs must fail")
	}
}

func TestCheckEdgeDisabled(t *testing.T) {
	if got := CheckEdge(context.Background(), testEntries(t), "", nil); got != nil {
		t.Errorf("no public base => no checks, got %d", len(got))
	}
}

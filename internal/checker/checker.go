// Package checker verifies that every canonical asset URL still serves an
// image. Cloudflare stores a logo_url without validating it and the launcher
// falls back to initials on a 404 — no error surfaces anywhere (that is how
// argosy's icon stayed broken for months). Placard is the thing that notices.
package checker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Einlanzerous/placard/internal/catalog"
	"github.com/Einlanzerous/placard/internal/store"
)

// CheckAll fetches the canonical URL of every present PNG and returns one
// result per file. base is Config.CanonicalBase(). Sequential on purpose —
// a couple dozen small GETs against a CDN do not need a worker pool.
func CheckAll(ctx context.Context, entries []catalog.Entry, base string, client *http.Client) []store.MarkCheck {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	var out []store.MarkCheck
	for _, e := range entries {
		for _, f := range e.Files {
			if !f.PNG || !f.Present {
				continue
			}
			out = append(out, checkOne(ctx, client, e.Slug, f.Path, base+f.Path))
		}
	}
	return out
}

func checkOne(ctx context.Context, client *http.Client, service, file, url string) store.MarkCheck {
	c := store.MarkCheck{Service: service, File: file, URL: url}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.Error = ptr(err.Error())
		return c
	}
	resp, err := client.Do(req)
	if err != nil {
		// One retry, transport errors only: a cold DNS resolver or a dropped
		// connection is not path rot, and a false "path rotted" on the front
		// page until the next tick costs more than two seconds here. An HTTP
		// answer, even a 404, is never retried — that is a real observation.
		select {
		case <-ctx.Done():
			c.Error = ptr(err.Error())
			return c
		case <-time.After(2 * time.Second):
		}
		resp, err = client.Do(req.Clone(ctx))
		if err != nil {
			c.Error = ptr(err.Error())
			return c
		}
	}
	defer resp.Body.Close()

	n, _ := io.Copy(io.Discard, resp.Body)
	ct := resp.Header.Get("Content-Type")
	c.HTTPStatus = ptr(resp.StatusCode)
	c.ContentType = ptr(ct)
	c.ContentLength = ptr(n)

	switch {
	case resp.StatusCode != http.StatusOK:
		c.Error = ptr(fmt.Sprintf("status %d", resp.StatusCode))
	case !strings.HasPrefix(ct, "image/"):
		// The exact failure the Access gate produces: 200 text/html login
		// page where an image belongs. Status alone would call it healthy.
		c.Error = ptr("not an image: " + ct)
	case n == 0:
		c.Error = ptr("empty body")
	default:
		c.OK = true
	}
	return c
}

func ptr[T any](v T) *T { return &v }

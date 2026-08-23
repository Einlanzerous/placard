// Package checker verifies that every URL placard vouches for still serves
// what it should. Cloudflare stores a logo_url without validating it and the
// launcher falls back to initials on a 404 — no error surfaces anywhere (that
// is how argosy's icon stayed broken for months). Placard is the thing that
// notices.
//
// Two kinds of check (store.KindCanonical / store.KindEdge): the canonical
// jsDelivr URLs the launcher is given, and — when PLACARD_PUBLIC_BASE_URL is
// set — placard's OWN public hostname. The second exists because an Access
// gate landing on the hostname is invisible to every check below Cloudflare's
// edge and to every person holding a session; it happened within an hour of
// go-live and only a manual curl saw it (PCAD-10).
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
			out = append(out, checkOne(ctx, client, store.KindCanonical, e.Slug, f.Path, base+f.Path, "image/"))
		}
	}
	return out
}

// CheckEdge fetches THROUGH placard's own public edge: the front page, and
// one mirror mark (preferring placard's own — the service vouching for
// itself). Redirects are deliberately NOT followed: an Access gate answers
// with a 302 to the team login, and following it would turn "gated" into a
// 200 text/html that passes a front-page check. The 302 itself is the
// finding. publicBase empty returns nil.
func CheckEdge(ctx context.Context, entries []catalog.Entry, publicBase string, client *http.Client) []store.MarkCheck {
	if publicBase == "" {
		return nil
	}
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	out := []store.MarkCheck{
		checkOne(ctx, client, store.KindEdge, "placard", "front page", publicBase+"/", "text/html"),
	}
	if f, svc, ok := representativeMark(entries); ok {
		out = append(out, checkOne(ctx, client, store.KindEdge, svc, f, publicBase+"/"+f, "image/"))
	}
	return out
}

// representativeMark picks the mirror path the edge check fetches: placard's
// own light mark when present, else the first present mark PNG. One URL is
// enough — every mirror path exercises the same edge, router and embed.
func representativeMark(entries []catalog.Entry) (path, service string, ok bool) {
	var first *catalog.Entry
	for i := range entries {
		e := &entries[i]
		if !e.HasMarks() {
			continue
		}
		if e.Slug == "placard" {
			return e.Files[0].Path, e.Slug, true
		}
		if first == nil {
			first = e
		}
	}
	if first != nil {
		return first.Files[0].Path, first.Slug, true
	}
	return "", "", false
}

func checkOne(ctx context.Context, client *http.Client, kind, service, file, url, wantCTPrefix string) store.MarkCheck {
	c := store.MarkCheck{Kind: kind, Service: service, File: file, URL: url}
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
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		// The Access-gate signature: content answered with a login redirect.
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		if loc := resp.Header.Get("Location"); loc != "" {
			msg += " → " + loc
		}
		c.Error = ptr(msg)
	case resp.StatusCode != http.StatusOK:
		c.Error = ptr(fmt.Sprintf("status %d", resp.StatusCode))
	case !strings.HasPrefix(ct, wantCTPrefix):
		// The other gate signature: 200 text/html login page where an image
		// belongs. Status alone would call it healthy.
		c.Error = ptr(fmt.Sprintf("want %s*, got %s", wantCTPrefix, ct))
	case n == 0:
		c.Error = ptr("empty body")
	default:
		c.OK = true
	}
	return c
}

func ptr[T any](v T) *T { return &v }

package catalog_test

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	placard "github.com/Einlanzerous/placard"
	"github.com/Einlanzerous/placard/internal/catalog"
)

// Build against the real embedded tree. This is the guard that keeps
// assets.go's static embed list honest: a service dir added to the repo but
// not to the embed directive shows up here as missing files, and a
// raster_from_svg service without generated PNGs fails Build outright.
func TestBuildAgainstEmbeddedAssets(t *testing.T) {
	entries, err := catalog.Build(placard.Assets)
	if err != nil {
		t.Fatal(err)
	}

	byslug := map[string]catalog.Entry{}
	for _, e := range entries {
		byslug[e.Slug] = e
	}

	for _, slug := range []string{"argosy", "switchyard", "lyceum", "placard"} {
		e, ok := byslug[slug]
		if !ok {
			t.Fatalf("manifest lost %s", slug)
		}
		if !e.HasMarks() {
			t.Errorf("%s: contract PNGs missing from the embed — check assets.go and `placard gen`", slug)
		}
		for _, f := range e.Files {
			if f.PNG && !f.Present {
				t.Errorf("%s: %s missing (dev variants are generated; run `placard gen`)", slug, f.Path)
			}
		}
	}

	for _, slug := range []string{"wiki", "app-launcher", "amber"} {
		e, ok := byslug[slug]
		if !ok {
			t.Fatalf("manifest lost %s", slug)
		}
		if e.HasMarks() {
			t.Errorf("%s: unexpectedly has marks — update this test and the manifest note", slug)
		}
	}
}

// Every mark this repo publishes is measurable and survives a square tile
// (PRSR-44).
//
// This is the authoring-end guard, and it is deliberately the one place in this
// change that *fails* rather than reports. The distinction is between surfaces:
//
//   - At runtime nothing fails. `placard check` prints a note and keeps its
//     exit code, /api/services serves the measurement, and the front page shows
//     it — because a badly proportioned mark still serves, every consumer that
//     fits rather than fills shows it correctly, and a letterboxed icon beats
//     two grey initials. Refusing to publish over proportions would be worse
//     than the problem.
//   - Here, the subject is this repo's *own* committed assets, and holding them
//     to a standard is what the repo already does — Build errors outright on a
//     raster_from_svg service with no generated PNGs, and CI regenerates and
//     fails on any drift. A mark arriving in a PR is the last moment it is
//     cheap to fix, which is the whole argument for the check existing on this
//     end at all rather than only in Purser.
//
// Every mark passes today: argosy was squared in #6, and switchyard's 483x512
// is the tightest at 0.94:1. If a service ever needs a deliberately non-square
// mark, this assertion is the thing to revisit — with the band in
// internal/shape, which explains what it is protecting.
func TestEveryEmbeddedMarkSurvivesASquareTile(t *testing.T) {
	entries, err := catalog.Build(placard.Assets)
	if err != nil {
		t.Fatal(err)
	}
	measured := 0
	for _, e := range entries {
		for _, f := range e.Files {
			if !f.Present || !f.PNG {
				continue
			}
			if !f.Shape.Measured() {
				// A committed PNG that cannot be decoded is a broken asset,
				// which is a different fault from a badly shaped one and worth
				// its own sentence.
				t.Errorf("%s: present but not decodable as an image", f.Path)
				continue
			}
			measured++
			if !f.Shape.TileSafe() {
				t.Errorf("%s: %s\n\tsquare the source (argosy's SVG viewBox was padded to 1:1 in #6, rather than cropped tight)", f.Path, f.Shape.Note())
			}
		}
	}
	// Guards the wiring rather than the assets: if Build ever stopped measuring,
	// every assertion above would pass vacuously and this file would report a
	// clean estate having looked at nothing.
	if measured == 0 {
		t.Fatal("no mark PNG was measured at all — catalog.Build is no longer reading dimensions")
	}
}

// pngOf renders a real PNG, because Build sniffs the format from the bytes and
// a hand-built header would test the fixture instead of the reader.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode %dx%d: %v", w, h, err)
	}
	return buf.Bytes()
}

// The reporting path, on a tree built for the purpose — because every mark in
// the real repo is square, so nothing above can show what a finding looks like.
//
// Argosy's real pre-#6 dimensions are used deliberately: this is the asset that
// prompted the whole check, and it went unnoticed for months.
func TestOffShapeReportsTheCroppedMarkAndOnlyThat(t *testing.T) {
	entries, err := catalog.Build(fstest.MapFS{
		"services.json":                {Data: []byte(`{"services":[{"slug":"wide","name":"Wide"},{"slug":"square","name":"Square"}]}`)},
		"wide/wide-mark-light.png":     {Data: pngOf(t, 1169, 512)},
		"wide/wide-mark-dark.png":      {Data: pngOf(t, 512, 512)},
		"wide/wide-mark.svg":           {Data: []byte(`<svg viewBox="0 0 1169 512"/>`)},
		"square/square-mark-light.png": {Data: pngOf(t, 483, 512)},
		"square/square-mark-dark.png":  {Data: pngOf(t, 1024, 1024)},
	})
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string][]catalog.File{}
	for _, e := range entries {
		bySlug[e.Slug] = e.OffShape()
	}

	off := bySlug["wide"]
	if len(off) != 1 {
		t.Fatalf("want exactly the 1169x512 mark reported, got %d: %+v", len(off), off)
	}
	if off[0].Path != "wide/wide-mark-light.png" {
		t.Errorf("wrong file reported: %s", off[0].Path)
	}
	if !strings.Contains(off[0].Shape.Note(), "2.28:1") {
		t.Errorf("the note should name the ratio, got %q", off[0].Shape.Note())
	}
	// The dark mark alongside it is square and must not be swept in with it.
	// Nor must the SVG: it is carried but unmeasured, and an unmeasurable file
	// reports nothing rather than being assumed bad.
	if n := len(bySlug["square"]); n != 0 {
		t.Errorf("switchyard-shaped and 1:1 marks are both fine, got %d findings: %+v", n, bySlug["square"])
	}
}

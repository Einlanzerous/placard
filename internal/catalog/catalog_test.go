package catalog_test

import (
	"bytes"
	"image"
	"image/png"
	"slices"
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
				// Names the file to edit, which for a raster_from_svg service
				// is the SVG and never the PNG in front of you: generated files
				// are never hand-edited.
				t.Errorf("%s: %s\n\tsquare %s — pad the viewBox to 1:1 rather than cropping tight, as argosy's was in #6", f.Path, f.Shape.Note(), f.Source)
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

// One badly-shaped SVG is one finding, not four.
//
// A raster_from_svg service's glyph drives both mark PNGs and both -dev
// siblings, so the naive per-file report says the estate has four problems when
// it has one — and names two generated files, whose only correct fix is
// somewhere else. Grouping on the file a human edits is what makes the report
// legible in the one surface an operator reads.
func TestShapeFindingsGroupOnTheFileAHumanEdits(t *testing.T) {
	entries, err := catalog.Build(fstest.MapFS{
		"services.json": {Data: []byte(`{"services":[
			{"slug":"vector","name":"Vector","raster_from_svg":true},
			{"slug":"raster","name":"Raster"}
		]}`)},
		// Derived: one SVG behind four bad PNGs.
		"vector/vector-mark.svg":           {Data: []byte(`<svg viewBox="0 0 1169 512"/>`)},
		"vector/vector-mark-light.png":     {Data: pngOf(t, 1169, 512)},
		"vector/vector-mark-dark.png":      {Data: pngOf(t, 1169, 512)},
		"vector/vector-mark-light-dev.png": {Data: pngOf(t, 1169, 512)},
		"vector/vector-mark-dark-dev.png":  {Data: pngOf(t, 1169, 512)},
		// Committed directly: two editable files, and one bad.
		"raster/raster-mark-light.png":     {Data: pngOf(t, 1169, 512)},
		"raster/raster-mark-dark.png":      {Data: pngOf(t, 512, 512)},
		"raster/raster-mark-light-dev.png": {Data: pngOf(t, 1169, 512)},
		"raster/raster-mark-dark-dev.png":  {Data: pngOf(t, 512, 512)},
	})
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string][]catalog.ShapeFinding{}
	for _, e := range entries {
		bySlug[e.Slug] = e.ShapeFindings()
	}

	// Four bad files, one thing to fix, and it is the SVG.
	vec := bySlug["vector"]
	if len(vec) != 1 {
		t.Fatalf("one glyph is one finding, got %d: %+v", len(vec), vec)
	}
	if vec[0].Source != "vector/vector-mark.svg" {
		t.Errorf("the report must name the editable source, got %q", vec[0].Source)
	}
	if len(vec[0].Files) != 4 {
		t.Errorf("all four derived PNGs should be attributed to it, got %v", vec[0].Files)
	}
	// The grouping must not become a filter: the affected files are still named.
	if !slices.Contains(vec[0].Files, "vector/vector-mark-light-dev.png") {
		t.Errorf("the dev sibling is still cropped and should be listed, got %v", vec[0].Files)
	}

	// The other direction: a directly-committed PNG is its own source, and the
	// square one beside it is not swept in.
	ras := bySlug["raster"]
	if len(ras) != 1 {
		t.Fatalf("one bad committed mark is one finding, got %d: %+v", len(ras), ras)
	}
	if ras[0].Source != "raster/raster-mark-light.png" {
		t.Errorf("a directly-committed PNG is its own source, got %q", ras[0].Source)
	}
	if len(ras[0].Files) != 2 {
		t.Errorf("the light mark and its dev sibling, got %v", ras[0].Files)
	}
}

// Generated() is the predicate the report leans on, so it is pinned rather than
// inferred from Source's spelling at the callsite.
func TestGeneratedTracksWhoWroteTheFile(t *testing.T) {
	entries, err := catalog.Build(fstest.MapFS{
		"services.json":                    {Data: []byte(`{"services":[{"slug":"vector","name":"Vector","raster_from_svg":true}]}`)},
		"vector/vector-mark.svg":           {Data: []byte(`<svg viewBox="0 0 512 512"/>`)},
		"vector/vector-mark-light.png":     {Data: pngOf(t, 512, 512)},
		"vector/vector-mark-dark.png":      {Data: pngOf(t, 512, 512)},
		"vector/vector-mark-light-dev.png": {Data: pngOf(t, 512, 512)},
		"vector/vector-mark-dark-dev.png":  {Data: pngOf(t, 512, 512)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"vector/vector-mark.svg":           false, // committed by a human
		"vector/vector-mark-light.png":     true,  // rasterized from it
		"vector/vector-mark-light-dev.png": true,  // badged from that
	}
	for _, f := range entries[0].Files {
		if w, checked := want[f.Path]; checked && f.Generated() != w {
			t.Errorf("%s: Generated() = %v, want %v (source %q)", f.Path, f.Generated(), w, f.Source)
		}
	}
}

// A file that does not exist still knows how it would be derived.
//
// Source is set above the presence check, so an absent file carries its rule
// rather than an empty string — which Generated() would otherwise read as
// "derived from something", making the answer depend on whether `placard gen`
// had happened to run yet rather than on how the file is defined.
//
// Note what that does *not* mean: a -dev sibling is generated whether or not it
// exists, because the derivation is a property of the path convention. Only the
// empty-Source bug was about presence.
func TestAbsentFilesStillCarryTheirSource(t *testing.T) {
	entries, err := catalog.Build(fstest.MapFS{
		"services.json": {Data: []byte(`{"services":[{"slug":"empty","name":"Empty"}]}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"empty/empty-mark-light.png":     false, // committed directly by a human
		"empty/empty-mark-dark.png":      false,
		"empty/empty-mark.svg":           false, // carried, its own source
		"empty/empty-mark-light-dev.png": true,  // always badged from the mark
		"empty/empty-mark-dark-dev.png":  true,
	}
	seen := 0
	for _, f := range entries[0].Files {
		if f.Present {
			t.Fatalf("%s should be absent in this tree", f.Path)
		}
		if f.Source == "" {
			t.Errorf("%s: absent, but its derivation rule is still knowable", f.Path)
		}
		w, checked := want[f.Path]
		if !checked {
			t.Errorf("unexpected file %s — this table should cover the convention", f.Path)
			continue
		}
		seen++
		if f.Generated() != w {
			t.Errorf("%s: Generated() = %v, want %v (source %q)", f.Path, f.Generated(), w, f.Source)
		}
	}
	if seen != len(want) {
		t.Errorf("checked %d of %d conventional paths", seen, len(want))
	}
}

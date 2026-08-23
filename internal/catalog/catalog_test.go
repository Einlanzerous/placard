package catalog_test

import (
	"testing"

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

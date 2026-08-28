// Package catalog joins the manifest to the embedded asset tree: for every
// service, which conventional files actually exist. Both the HTTP surface and
// the checker read this one view.
package catalog

import (
	"fmt"
	"io/fs"

	"github.com/Einlanzerous/placard/internal/manifest"
	"github.com/Einlanzerous/placard/internal/shape"
)

type File struct {
	// Name is the basename, Path the repo-relative path (also the URL path).
	Name string
	Path string
	// Role is the contract wording shown on the front page.
	Role string
	// PNG marks the raster files — the ones the checker verifies (SVG is
	// carried but unverified, per README).
	PNG     bool
	Present bool
	// Shape is the file's pixel dimensions, zero when it could not be measured
	// (PRSR-44). Read from the embedded bytes rather than from a fetch, because
	// the aspect ratio is a property of the *asset* and this repo is the source
	// of truth for every mark — so it is known offline, with no database, on a
	// PR before jsDelivr has heard of the commit. Only PNGs are measured; see
	// Build.
	Shape shape.Shape
}

type Entry struct {
	manifest.Service
	Files []File
}

// Build reads services.json from fsys and stats each conventional path.
func Build(fsys fs.FS) ([]Entry, error) {
	data, err := fs.ReadFile(fsys, "services.json")
	if err != nil {
		return nil, err
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(m.Services))
	for _, svc := range m.Services {
		files := []File{
			{Path: svc.PNG("light"), Role: "for light surfaces", PNG: true},
			{Path: svc.PNG("dark"), Role: "for dark surfaces", PNG: true},
			{Path: svc.SVG(), Role: "carried, unverified"},
			{Path: svc.DevPNG("light"), Role: "generated dev variant", PNG: true},
			{Path: svc.DevPNG("dark"), Role: "generated dev variant", PNG: true},
		}
		for i := range files {
			f := &files[i]
			f.Name = f.Path[len(svc.Slug)+1:]
			if _, err := fs.Stat(fsys, f.Path); err != nil {
				continue
			}
			f.Present = true
			if f.PNG {
				f.Shape = measure(fsys, f.Path)
			}
		}
		// A raster_from_svg service whose PNGs are absent from the embed is a
		// build defect (gen not run, or the dir missing from assets.go).
		if svc.RasterFromSVG && (!files[0].Present || !files[1].Present) {
			return nil, fmt.Errorf("catalog: %s: raster_from_svg but mark PNGs are not embedded — run `placard gen` and check assets.go", svc.Slug)
		}
		entries = append(entries, Entry{Service: svc, Files: files})
	}
	return entries, nil
}

// measure reads a file's dimensions, reporting nothing it could not read.
//
// Only PNGs are measured, and the carried SVG deliberately is not. For a
// raster_from_svg service the SVG is the *source* and the PNG's aspect follows
// its viewBox, so measuring the PNG already catches a badly-shaped source and
// points at the file a human would fix; for anything else the SVG is not what
// the launcher fetches. Reading it would also mean parsing a viewBox, which is
// a second measurement path answering the same question, and the README's
// contract has the SVG carried but unverified. If that contract changes, this
// is where the second path goes.
//
// A file that cannot be opened or decoded measures as nothing rather than as a
// fault: Build's job is to say what is here, and Present already answers
// whether the file exists.
func measure(fsys fs.FS, path string) shape.Shape {
	f, err := fsys.Open(path)
	if err != nil {
		return shape.Shape{}
	}
	defer f.Close()
	return shape.Measure(f)
}

// HasMarks reports whether the service's two contract PNGs exist.
func (e Entry) HasMarks() bool {
	return e.Files[0].Present && e.Files[1].Present
}

// OffShape returns the present mark PNGs whose proportions a square launcher
// tile will crop. Empty when every mark is fine, or unmeasurable.
func (e Entry) OffShape() []File {
	var out []File
	for _, f := range e.Files {
		if f.Present && f.PNG && f.Shape.Measured() && !f.Shape.TileSafe() {
			out = append(out, f)
		}
	}
	return out
}

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
	// Source is the repo path a human edits to change this file: itself when it
	// is committed directly, and the SVG or the mark PNG it derives from when
	// `placard gen` writes it.
	//
	// It exists so a report can name a file somebody may actually fix. Generated
	// files are never hand-edited (see CLAUDE.md), so telling an operator that
	// argosy-mark-light-dev.png is badly proportioned points at a file whose
	// only correct change is somewhere else — and one badly shaped SVG produces
	// four such files.
	Source string
}

// Generated reports whether `placard gen` writes this file from a committed
// source rather than a human committing it.
func (f File) Generated() bool { return f.Source != f.Path }

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
		// Both PNGs of a variant — the mark and its -dev sibling — trace back to
		// the same editable file, which is what lets a report collapse four
		// findings about one bad glyph into one. The rule mirrors gen.Run's own
		// source selection: a per-variant SVG when the service commits one, else
		// the shared SVG, else the mark PNG itself.
		//
		// Keyed on path rather than on position in files: the two are built a
		// few lines apart and an index offset between them would be silently
		// wrong the first time somebody adds a file to the slice.
		source := map[string]string{}
		for _, v := range manifest.Variants {
			src := svc.PNG(v)
			if svc.RasterFromSVG {
				src = svc.SVG()
				if _, err := fs.Stat(fsys, svc.VariantSVG(v)); err == nil {
					src = svc.VariantSVG(v)
				}
			}
			source[svc.PNG(v)] = src
			source[svc.DevPNG(v)] = src
		}
		for i := range files {
			f := &files[i]
			f.Name = f.Path[len(svc.Slug)+1:]
			// Above the presence check: how a file would be derived does not
			// depend on whether it exists yet, and leaving Source empty on an
			// absent file would have Generated() answer true for it — "do not
			// hand-edit this" said about a file nobody has written.
			//
			// Anything with no derivation rule — the carried SVG, and any file
			// added later without one — is its own source.
			if src, ok := source[f.Path]; ok {
				f.Source = src
			} else {
				f.Source = f.Path
			}
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

// ShapeFinding is one badly-shaped mark, keyed on the file that has to change.
type ShapeFinding struct {
	// Source is the file a human edits. Always the subject of the report.
	Source string
	// Shape is the proportions that file produces.
	Shape shape.Shape
	// Files are the present files carrying them, Source included when it is
	// itself a PNG. More than one means a single edit fixes all of them.
	Files []string
}

// ShapeFindings groups a service's off-shape marks by the file that has to
// change, so one badly-shaped glyph is one finding.
//
// Without the grouping a raster_from_svg service reports four times over — the
// two mark PNGs and their two -dev siblings — and two of those four name
// generated files, which CLAUDE.md's invariant says are never hand-edited. The
// estate has one problem there, and the operator has one file to open.
//
// A service that commits its PNGs directly still reports twice when both are
// wrong, and that is correct rather than a leak: light and dark are two
// committed files and fixing one does not fix the other.
func (e Entry) ShapeFindings() []ShapeFinding {
	var out []ShapeFinding
	at := map[string]int{}
	for _, f := range e.OffShape() {
		i, seen := at[f.Source]
		if !seen {
			at[f.Source] = len(out)
			out = append(out, ShapeFinding{Source: f.Source, Shape: f.Shape})
			i = len(out) - 1
		}
		out[i].Files = append(out[i].Files, f.Path)
	}
	return out
}

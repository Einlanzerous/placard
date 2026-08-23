// Package catalog joins the manifest to the embedded asset tree: for every
// service, which conventional files actually exist. Both the HTTP surface and
// the checker read this one view.
package catalog

import (
	"fmt"
	"io/fs"

	"github.com/Einlanzerous/placard/internal/manifest"
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
			files[i].Name = files[i].Path[len(svc.Slug)+1:]
			if _, err := fs.Stat(fsys, files[i].Path); err == nil {
				files[i].Present = true
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

// HasMarks reports whether the service's two contract PNGs exist.
func (e Entry) HasMarks() bool {
	return e.Files[0].Present && e.Files[1].Present
}

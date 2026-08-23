// Package manifest parses services.json — the single index of every service
// Placard knows about — and owns the path convention a slug expands to. Both
// the generator and the HTTP service go through these helpers; the convention
// is written down exactly once.
package manifest

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type Service struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Color is only set where BRAND.md establishes a rule (coral =
	// Switchyard, gold = Amber, …) — absent means "no rule", not "pick one".
	Color string `json:"color,omitempty"`
	// RasterFromSVG marks the PNGs as derived: `placard gen` writes them from
	// <slug>-mark.svg and CI fails if a hand edit drifts them.
	RasterFromSVG bool   `json:"raster_from_svg,omitempty"`
	Note          string `json:"note,omitempty"`
	LegacySource  string `json:"legacy_source,omitempty"`
}

type Manifest struct {
	Services []Service `json:"services"`
}

// Variants are the two consuming-surface backgrounds the contract names.
var Variants = []string{"light", "dark"}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("services.json: %w", err)
	}
	if len(m.Services) == 0 {
		return nil, fmt.Errorf("services.json: no services")
	}
	seen := make(map[string]bool, len(m.Services))
	for _, s := range m.Services {
		if !slugRe.MatchString(s.Slug) {
			return nil, fmt.Errorf("services.json: bad slug %q (want lowercase [a-z0-9-], the slug is a URL path segment)", s.Slug)
		}
		if s.Name == "" {
			return nil, fmt.Errorf("services.json: %s: name is required", s.Slug)
		}
		if seen[s.Slug] {
			return nil, fmt.Errorf("services.json: duplicate slug %q", s.Slug)
		}
		seen[s.Slug] = true
	}
	return &m, nil
}

// The path convention (README.md), repo-relative with forward slashes —
// identical as a filesystem path and as the URL path under jsDelivr or the
// service mirror.

func (s Service) SVG() string { return s.Slug + "/" + s.Slug + "-mark.svg" }

func (s Service) PNG(variant string) string {
	return s.Slug + "/" + s.Slug + "-mark-" + variant + ".png"
}

func (s Service) DevPNG(variant string) string {
	return s.Slug + "/" + s.Slug + "-mark-" + variant + "-dev.png"
}

// Package gen writes every derived file in the repo: mark PNGs rasterized
// from svg-sourced services, and the -dev.png siblings dev instances use.
//
// Everything here must stay deterministic — same inputs, same bytes, on every
// run and platform — because CI's drift check is `placard gen` followed by a
// git diff, and a nondeterministic byte would fail every honest build.
// Nothing in this package may read the clock, random state, or map order.
package gen

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/Einlanzerous/placard/internal/manifest"
)

// rasterHeight is the pixel height of svg-derived mark PNGs. 512 comfortably
// covers the launcher's 48px tile and any plausible future surface.
const rasterHeight = 512

// Run regenerates the derived files under root (a checkout of this repo).
// logf receives one line per file written.
func Run(root string, logf func(format string, args ...any)) error {
	data, err := os.ReadFile(filepath.Join(root, "services.json"))
	if err != nil {
		return err
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return err
	}

	for _, svc := range m.Services {
		if svc.RasterFromSVG {
			// Each variant renders from its own SVG when one exists
			// (<svc>-mark-light.svg / -dark.svg), else from the shared
			// <svc>-mark.svg — a mark can adapt per surface without every
			// mark having to (PCAD-9). Rasters are cached per source so the
			// shared case still renders once.
			rendered := map[string]*image.RGBA{}
			for _, v := range manifest.Variants {
				src := svc.SVG()
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(svc.VariantSVG(v)))); err == nil {
					src = svc.VariantSVG(v)
				}
				img, ok := rendered[src]
				if !ok {
					img, err = rasterSVG(filepath.Join(root, filepath.FromSlash(src)), rasterHeight)
					if err != nil {
						// Loud, not skipped: the flag is a claim that the SVG
						// is the source of truth, and a missing/broken source
						// is a repo bug.
						return fmt.Errorf("%s: raster_from_svg: %w", svc.Slug, err)
					}
					rendered[src] = img
				}
				if err := writePNG(filepath.Join(root, filepath.FromSlash(svc.PNG(v))), img); err != nil {
					return err
				}
				logf("gen: wrote %s (from %s)", svc.PNG(v), src)
			}
		}

		for _, v := range manifest.Variants {
			base := filepath.Join(root, filepath.FromSlash(svc.PNG(v)))
			src, err := readPNG(base)
			if os.IsNotExist(err) {
				continue // a service with no marks yet (wiki, amber) is fine
			}
			if err != nil {
				return fmt.Errorf("%s: %w", svc.PNG(v), err)
			}
			if err := writePNG(filepath.Join(root, filepath.FromSlash(svc.DevPNG(v))), Badge(src)); err != nil {
				return err
			}
			logf("gen: wrote %s", svc.DevPNG(v))
		}
	}
	return nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

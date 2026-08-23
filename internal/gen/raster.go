package gen

import (
	"fmt"
	"image"
	"math"
	"os"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// rasterSVG renders an SVG file to an RGBA image of the given pixel height,
// width following the viewBox aspect ratio. oksvg covers the subset these
// marks use (paths, rects, plain fills, linear gradients) — it does not do
// CSS <style> blocks or media queries, which is one more reason the carried
// SVGs stay "unverified" in the contract while PNGs are the baseline.
func rasterSVG(path string, targetH int) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	icon, err := oksvg.ReadIconStream(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	vb := icon.ViewBox
	if vb.W <= 0 || vb.H <= 0 {
		return nil, fmt.Errorf("%s: viewBox has no area", path)
	}
	w := int(math.Round(float64(targetH) * vb.W / vb.H))
	if w < 1 {
		w = 1
	}

	img := image.NewRGBA(image.Rect(0, 0, w, targetH))
	// Not SetTarget: oksvg subtracts the viewBox origin in UNSCALED units and
	// then scales it along with the drawing (svg_icon.go: Translate(x-vb.X,
	// y-vb.Y).Scale(...)), so any non-zero-origin viewBox — a cropped mark
	// like argosy's glyph window — renders shifted by origin×(scale-1) and
	// clips. Build the correct transform directly: scale, with the viewBox
	// origin mapped to 0,0.
	sw, sh := float64(w)/vb.W, float64(targetH)/vb.H
	icon.Transform = rasterx.Matrix2D{A: sw, D: sh, E: -vb.X * sw, F: -vb.Y * sh}
	icon.Draw(rasterx.NewDasher(w, targetH, rasterx.NewScannerGV(w, targetH, img, img.Bounds())), 1.0)
	return img, nil
}

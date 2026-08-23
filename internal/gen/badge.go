package gen

import (
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The dev badge, per BRAND.md: yellow #ffd400 rounded rectangle, the word DEV
// in IBM Plex Mono SemiBold #141414, bottom-right corner, proportional to the
// image so a 630×175 wordmark and a 512×512 tile both read at launcher size.

//go:embed font/IBMPlexMono-SemiBold.ttf
var plexMonoSemiBold []byte

var (
	badgeYellow = color.RGBA{R: 0xff, G: 0xd4, B: 0x00, A: 0xff}
	badgeInk    = color.RGBA{R: 0x14, G: 0x14, B: 0x14, A: 0xff}
)

var parseFontOnce = sync.OnceValues(func() (*opentype.Font, error) {
	return opentype.Parse(plexMonoSemiBold)
})

// Badge returns a copy of src with the DEV badge composited bottom-right.
func Badge(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)

	// Proportioned for the actual consumer: a launcher object-fits the whole
	// image into a ~48px tile, so display scale is set by the LONGEST
	// dimension — size against it, clamped so a wide wordmark's badge still
	// fits its short side (PCAD-8; sizing against min-dim made argosy's badge
	// ~3px tall at launcher scale).
	minDim, maxDim := w, h
	if h < w {
		minDim, maxDim = h, w
	}
	badgeH := int(math.Round(0.22 * float64(maxDim)))
	if limit := int(math.Round(0.45 * float64(minDim))); badgeH > limit {
		badgeH = limit
	}
	if badgeH < 14 {
		badgeH = 14
	}
	margin := int(math.Round(0.045 * float64(minDim)))
	if margin < 3 {
		margin = 3
	}

	otf, err := parseFontOnce()
	if err != nil {
		// The font is embedded; failing to parse it is a build defect, not a
		// runtime condition to soldier through with silently un-badged marks.
		panic("gen: embedded IBM Plex Mono is unparseable: " + err.Error())
	}
	face, err := opentype.NewFace(otf, &opentype.FaceOptions{
		Size:    0.60 * float64(badgeH),
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		panic("gen: face: " + err.Error())
	}
	defer face.Close()

	// 0.08em letter spacing, added between glyphs (not after the last).
	spacing := fixed.Int26_6(math.Round(0.08 * 0.60 * float64(badgeH) * 64))
	drawer := &font.Drawer{Dst: out, Src: image.NewUniform(badgeInk), Face: face}
	textW := fixed.I(0)
	for i, r := range "DEV" {
		adv, _ := face.GlyphAdvance(r)
		if i > 0 {
			textW += spacing
		}
		textW += adv
	}

	padX := int(math.Round(0.35 * float64(badgeH)))
	badgeW := textW.Ceil() + 2*padX
	x0 := w - margin - badgeW
	y0 := h - margin - badgeH
	fillRoundedRect(out, x0, y0, badgeW, badgeH, 0.22*float64(badgeH), badgeYellow)

	met := face.Metrics()
	capH := met.CapHeight
	if capH <= 0 {
		capH = met.Ascent
	}
	drawer.Dot = fixed.Point26_6{
		X: fixed.I(x0) + (fixed.I(badgeW)-textW)/2,
		Y: fixed.I(y0) + (fixed.I(badgeH)+capH)/2,
	}
	for i, r := range "DEV" {
		if i > 0 {
			drawer.Dot.X += spacing
		}
		drawer.DrawString(string(r))
	}
	return out
}

// fillRoundedRect composites a solid rounded rectangle onto dst using a
// signed-distance coverage per pixel (1px feather) — antialiased corners with
// nothing but arithmetic, which keeps the output byte-identical everywhere.
func fillRoundedRect(dst *image.RGBA, x0, y0, w, h int, r float64, c color.RGBA) {
	hw, hh := float64(w)/2, float64(h)/2
	if r > hw {
		r = hw
	}
	if r > hh {
		r = hh
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			qx := math.Abs(float64(x)+0.5-hw) - (hw - r)
			qy := math.Abs(float64(y)+0.5-hh) - (hh - r)
			mx, my := math.Max(qx, 0), math.Max(qy, 0)
			d := math.Sqrt(mx*mx+my*my) + math.Min(math.Max(qx, qy), 0) - r
			cov := 0.5 - d
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			blend(dst, x0+x, y0+y, c, cov)
		}
	}
}

// blend composites src over the pixel at (x, y) with the given coverage.
func blend(dst *image.RGBA, x, y int, src color.RGBA, cov float64) {
	if !(image.Point{X: x, Y: y}).In(dst.Rect) {
		return
	}
	i := dst.PixOffset(x, y)
	p := dst.Pix[i : i+4 : i+4]
	a := cov * float64(src.A) / 255
	p[0] = uint8(math.Round(float64(src.R)*a + float64(p[0])*(1-a)))
	p[1] = uint8(math.Round(float64(src.G)*a + float64(p[1])*(1-a)))
	p[2] = uint8(math.Round(float64(src.B)*a + float64(p[2])*(1-a)))
	p[3] = uint8(math.Round(255*a + float64(p[3])*(1-a)))
}

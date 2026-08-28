package shape

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
)

// pngOf renders a real PNG of the given dimensions.
//
// A real one, encoded by the stdlib, rather than a hand-built IHDR: Measure
// sniffs the format from the bytes and the whole point is that it reads what a
// CDN actually serves. An all-zero gray canvas compresses to a couple of
// kilobytes whatever its dimensions.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode %dx%d: %v", w, h, err)
	}
	return buf.Bytes()
}

// The band, with the reason on every row. The numbers are the marks this repo
// actually publishes, measured 2026-08-28, plus argosy's as it stood before #6
// squared it — the asset this whole package is named after.
func TestBandClearsEveryRealMarkAndCatchesArgosy(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		safe bool
		why  string
	}{
		{512, 512, true, "argosy (after #6), placard: exactly 1:1"},
		{1024, 1024, true, "lyceum: 1:1 at twice the size"},
		{483, 512, true, "switchyard: 0.94:1, the tightest mark in the repo"},
		{1169, 512, false, "argosy before #6: 2.28:1, the outer ships cropped off"},
		{512, 1169, false, "the same mark on its side — a square tile crops a portrait just as badly"},
		{600, 512, true, "1.17:1: a designer letterboxing a glyph slightly, which is fine"},
		{700, 512, false, "1.37:1: not fine"},
		{512, 640, true, "1:1.25 is the boundary and is in"},
		{512, 660, false, "1:1.29 is out"},
	} {
		if got := (Shape{tc.w, tc.h}).TileSafe(); got != tc.safe {
			t.Errorf("Shape{%d, %d}.TileSafe() = %v, want %v — %s", tc.w, tc.h, got, tc.safe, tc.why)
		}
	}
}

// The band is symmetric in the ratio, and that is load-bearing rather than
// tidy: the tile is square and has no preference about which way a mark is
// wrong. Both constants are untyped, so this is evaluated exactly.
func TestBandIsSymmetric(t *testing.T) {
	if MinAspect*MaxAspect != 1 {
		t.Errorf("min %v and max %v must be reciprocals — a square tile crops tall and wide marks alike", MinAspect, MaxAspect)
	}
}

// Never treat unmeasurable as bad. An SVG, a truncated file, a format nothing
// decodes: all report nothing rather than a guess.
func TestUnmeasuredSaysNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"an svg, which is carried but has no raster header", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1169 512"/>`)},
		{"a truncated png signature", []byte("\x89PNG\r\n")},
		{"not an image at all", []byte("this is not an image")},
		{"empty", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Measure(bytes.NewReader(tc.body))
			if s.Measured() {
				t.Fatalf("nothing here can be decoded, got %+v", s)
			}
			if s.TileSafe() {
				t.Error("TileSafe must not answer yes for something it never read")
			}
			if n := s.Note(); n != "" {
				t.Errorf("an unmeasured mark reports nothing, got %q", n)
			}
			if a := s.Aspect(); a != "" {
				t.Errorf("an unmeasured mark has no aspect to render, got %q", a)
			}
		})
	}
}

// The svg case above is the one worth stating twice: the viewBox says 1169x512
// and is deliberately not read. Parsing it would mean a second measurement path
// answering the same question, and the README's contract has the SVG carried
// but unverified.
func TestSVGViewBoxIsNotParsed(t *testing.T) {
	s := Measure(strings.NewReader(`<svg viewBox="0 0 1169 512"/>`))
	if s.W != 0 || s.H != 0 {
		t.Errorf("the viewBox must not be read, got %+v", s)
	}
}

// A tile-safe mark is silent. Without this the note becomes wallpaper on every
// report and stops being read.
func TestTileSafeSaysNothing(t *testing.T) {
	if n := (Shape{512, 512}).Note(); n != "" {
		t.Errorf("a 1:1 mark has nothing to report, got %q", n)
	}
}

// The format is sniffed from the bytes, so measurement is of the file rather
// than of what a filename or a header claims about it.
func TestMeasureReadsRealPNGBytes(t *testing.T) {
	s := Measure(bytes.NewReader(pngOf(t, 1169, 512)))
	if s.W != 1169 || s.H != 512 {
		t.Fatalf("want 1169x512, got %+v", s)
	}
	if s.TileSafe() {
		t.Error("2.28:1 is not tile safe")
	}
	for _, want := range []string{"1169x512", "2.28:1", "cropped"} {
		if !strings.Contains(s.Note(), want) {
			t.Errorf("the note should say %q, got %q", want, s.Note())
		}
	}
}

// The number is always the long side, with the colon saying which way round —
// so "2.28:1" and "1:2.28" are two different marks and neither reads as the
// other.
func TestAspectNamesTheLongSide(t *testing.T) {
	if got := (Shape{1169, 512}).Aspect(); got != "2.28:1" {
		t.Errorf("wide mark = %q, want 2.28:1", got)
	}
	if got := (Shape{512, 1169}).Aspect(); got != "1:2.28" {
		t.Errorf("tall mark = %q, want 1:2.28", got)
	}
}

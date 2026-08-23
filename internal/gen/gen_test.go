package gen

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func encode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func flat(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// Determinism is the property the CI drift check stands on: two runs over the
// same input must be byte-identical.
func TestBadgeDeterministic(t *testing.T) {
	src := flat(200, 120, color.RGBA{R: 30, G: 40, B: 50, A: 255})
	if !bytes.Equal(encode(t, Badge(src)), encode(t, Badge(src))) {
		t.Fatal("two Badge runs over the same image differ")
	}
}

func TestBadgeActuallyMarksTheImage(t *testing.T) {
	src := flat(200, 120, color.RGBA{A: 0})
	badged := Badge(src)
	if bytes.Equal(encode(t, src), encode(t, badged)) {
		t.Fatal("Badge returned the input unchanged")
	}
	// The badge sits bottom-right; a pixel there should be the yellow.
	got := badged.RGBAAt(190-badged.Rect.Dx()/20, 105)
	if got.A == 0 {
		t.Errorf("expected badge coverage bottom-right, got transparent")
	}
}

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50" width="100" height="50">
  <rect x="0" y="0" width="100" height="50" fill="#e2623d"/>
</svg>`

func TestRasterSVGDimensionsAndFill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.svg")
	if err := os.WriteFile(path, []byte(testSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := rasterSVG(path, 512)
	if err != nil {
		t.Fatal(err)
	}
	if img.Rect.Dy() != 512 || img.Rect.Dx() != 1024 {
		t.Fatalf("got %dx%d, want 1024x512 (2:1 viewBox at height 512)", img.Rect.Dx(), img.Rect.Dy())
	}
	if c := img.RGBAAt(512, 256); c.R != 0xe2 || c.G != 0x62 || c.B != 0x3d {
		t.Errorf("centre pixel = %v, want the coral fill", c)
	}
}

// A non-zero-origin viewBox (a cropped mark window) must render its full
// content — oksvg's SetTarget mishandles the origin, which rasterSVG works
// around by building the transform itself (PCAD-11).
func TestRasterCroppedViewBox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.svg")
	// The rect exactly fills the offset viewBox; every corner must be inked.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="40 60 100 50" width="100" height="50">
		<rect x="40" y="60" width="100" height="50" fill="#e2623d"/></svg>`
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := rasterSVG(path, 512)
	if err != nil {
		t.Fatal(err)
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for _, p := range [][2]int{{2, 2}, {w - 3, 2}, {2, h - 3}, {w - 3, h - 3}, {w / 2, h / 2}} {
		if c := img.RGBAAt(p[0], p[1]); c.A == 0 {
			t.Errorf("pixel (%d,%d) empty — offset viewBox content shifted or clipped", p[0], p[1])
		}
	}
}

// A variant-specific SVG drives its own PNG; the shared SVG covers the rest
// (PCAD-9 — placard's plate adapts per surface, switchyard's coral does not).
func TestPerVariantSVGSources(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "gamma"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("services.json", `{"services":[{"slug":"gamma","name":"Gamma","raster_from_svg":true}]}`)
	write("gamma/gamma-mark.svg", testSVG) // coral
	write("gamma/gamma-mark-light.svg", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 50">
		<rect x="0" y="0" width="100" height="50" fill="#101010"/></svg>`)

	if err := Run(root, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	read := func(rel string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	light, dark := read("gamma/gamma-mark-light.png"), read("gamma/gamma-mark-dark.png")
	if bytes.Equal(light, dark) {
		t.Fatal("light PNG should come from gamma-mark-light.svg, not the shared SVG")
	}
	img, err := png.Decode(bytes.NewReader(dark))
	if err != nil {
		t.Fatal(err)
	}
	if c := img.(*image.RGBA).RGBAAt(512, 256); c.R != 0xe2 {
		t.Errorf("dark PNG centre = %v, want the shared SVG's coral", c)
	}
}

// Run over a miniature repo tree: idempotent, and dev variants appear for
// committed PNGs while markless services are skipped without error.
func TestRunIsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeFile := func(rel, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("services.json", `{"services":[
		{"slug":"alpha","name":"Alpha","raster_from_svg":true},
		{"slug":"beta","name":"Beta"}
	]}`)
	writeFile("alpha/alpha-mark.svg", testSVG)

	logf := func(string, ...any) {}
	if err := Run(root, logf); err != nil {
		t.Fatal(err)
	}
	first := map[string][]byte{}
	for _, rel := range []string{
		"alpha/alpha-mark-light.png", "alpha/alpha-mark-dark.png",
		"alpha/alpha-mark-light-dev.png", "alpha/alpha-mark-dark-dev.png",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("expected %s to be generated: %v", rel, err)
		}
		first[rel] = data
	}
	if _, err := os.Stat(filepath.Join(root, "beta")); !os.IsNotExist(err) {
		t.Error("beta has no marks and no dir; gen must not invent one")
	}

	if err := Run(root, logf); err != nil {
		t.Fatal(err)
	}
	for rel, want := range first {
		got, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s changed on the second run — gen is not deterministic", rel)
		}
	}
}

// Package shape answers one question about a mark: will a square tile show it,
// or crop it?
//
// # Why this exists
//
// It is the third variant of a failure this estate has now met three times, and
// each check defeated the one before it:
//
//   - A stored logo_url can be a lie. Cloudflare accepts any URL without
//     validating it and the launcher falls back to two grey initials, so a
//     rotted path is indistinguishable from an unset one. That is what this
//     repo exists for, and what internal/checker verifies.
//   - A working URL is not a correct one. Argosy's old URL answered 200
//     image/png and was the 3.6:1 wordmark, illegible at tile size. No fetch
//     check can tell that from the tile mark; knowing which asset is the tile
//     asset is why Placard is a registry rather than a CDN path.
//   - A correct URL is not a correctly-shaped asset. Argosy's canonical mark
//     was 1169x512 — the only non-square one in this repo, against 0.94:1 for
//     switchyard and 1.00:1 for lyceum and placard — and Cloudflare's App
//     Launcher tile is square and *fills* rather than fits, so it cropped the
//     outer ships off the glyph. Placard's own front page fits, so the same
//     file looked right here and wrong in the launcher, with nothing anywhere
//     reporting a problem. Squared in #6 by padding the SVG viewBox to 1:1.
//
// Purser (PRSR-43) reports this at the moment it writes a logo_url, because it
// knows the *surface* — that the tile is square is a fact about Cloudflare, and
// Purser is the package that talks to Cloudflare. This is the other end of the
// same guard (PRSR-44): Placard knows the *asset*, and sees it once at
// authoring time for every consumer rather than one consumer at a time at the
// point of use.
//
// Neither replaces the other, for the reason Purser already gives about this
// repo's own check field: a periodic monitor carries a checked_at, and a stale
// green is exactly how the silent failure comes back.
package shape

import (
	"fmt"
	"image"
	"io"

	// Registered for image.DecodeConfig, which sniffs the format from the bytes
	// rather than from a filename or a Content-Type. PNG is the format this
	// repo's contract is written in; the other two cost a decoder each and mean
	// a mark committed in the wrong format is measured rather than ignored.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// The band a mark has to sit in for a square tile to show it whole, as
// width / height.
//
// MinAspect is 1/MaxAspect rather than a second judgement: the tile is square,
// so it has no preference about which way a mark is wrong and a portrait mark
// is cropped exactly as badly as a landscape one. TestBandIsSymmetric asserts
// the reciprocal so this cannot drift into two independently chosen numbers.
//
// The width is chosen from the marks that exist. This repo publishes switchyard
// at 0.94:1 and lyceum and placard at 1.00:1, so +/-25% clears every real mark
// with room for a designer to letterbox a glyph slightly rather than crop it
// tight. Argosy's, before #6, was 2.28:1 — not a near miss in anybody's
// reading.
//
// Deliberately generous. A false positive costs one line in a report; a false
// negative costs a cropped tile that nobody notices for months, which is the
// failure this whole package is named after.
//
// It matches Purser's band on purpose, and that is a decision rather than a
// copy. Placard serves more than one surface — its own front page fits rather
// than fills, and a mark that letterboxes there is merely letterboxed — so the
// band could have been Placard's own question. It is not, because the launcher
// is the only consumer that *crops*, and a mark that survives the strictest
// consumer survives the rest. If a future consumer crops harder, this is the
// constant to revisit, and the reason to.
const (
	MinAspect = 0.8
	MaxAspect = 1.25
)

// sniffLimit bounds how much of a file is read to find its dimensions.
//
// A budget, not a proof. A PNG's IHDR is the first ~24 bytes, which is the case
// that matters here. A JPEG's SOF sits behind whatever metadata precedes it and
// that is not bounded by anything — an embedded ICC profile splits across APP2
// segments of up to ~64KB each — so a file whose header lies past this measures
// as unmeasured and reports nothing, which is the safe direction.
const sniffLimit = 64 << 10

// Shape is a mark's pixel dimensions, zero when they could not be read.
type Shape struct {
	W int `json:"width"`
	H int `json:"height"`
}

// Measure reads dimensions from the front of an image.
//
// The format is sniffed from the bytes, not taken from the extension: a .png
// that is not a PNG measures as nothing rather than as something invented. A
// decode error is not returned, because there is exactly one thing any caller
// would do with it — treat the file as unmeasured — and handing back an error
// that must be remembered to be ignored is how a third state becomes a second
// one by accident.
func Measure(r io.Reader) Shape {
	cfg, _, err := image.DecodeConfig(io.LimitReader(r, sniffLimit))
	if err != nil {
		return Shape{}
	}
	return Shape{W: cfg.Width, H: cfg.Height}
}

// Measured reports whether dimensions were actually read. False for an SVG, for
// a format no registered decoder knows, and for a file that ended before its
// header did.
func (s Shape) Measured() bool { return s.W > 0 && s.H > 0 }

// TileSafe reports whether a square tile will show this mark whole.
//
// Ask Measured first. An unmeasured shape is neither safe nor unsafe, and
// reading this alone would report every SVG as badly proportioned — never treat
// unmeasurable as bad.
func (s Shape) TileSafe() bool {
	if !s.Measured() {
		return false
	}
	r := float64(s.W) / float64(s.H)
	return r >= MinAspect && r <= MaxAspect
}

// Aspect renders the ratio the way somebody would say it aloud — "2.28:1" for a
// wide mark, "1:2.28" for a tall one — so the number is always the long side
// and the colon says which way round it is. Empty when unmeasured.
func (s Shape) Aspect() string {
	switch {
	case !s.Measured():
		return ""
	case s.W >= s.H:
		return fmt.Sprintf("%.2f:1", float64(s.W)/float64(s.H))
	default:
		return fmt.Sprintf("1:%.2f", float64(s.H)/float64(s.W))
	}
}

// Note is the sentence a report prints about this mark, empty when there is
// nothing to say.
//
// Empty for a tile-safe mark and empty for an unmeasured one. That is the only
// place in this package where those two collapse, and it collapses safely,
// because saying nothing is the whole of what either outcome does — nothing
// downstream branches on the note, so an SVG can never be mistaken for a square
// PNG by anything reading it.
func (s Shape) Note() string {
	if !s.Measured() || s.TileSafe() {
		return ""
	}
	return fmt.Sprintf("%dx%d (%s) — Cloudflare's launcher tile is square and fills rather than fits, so it will be cropped to its centre",
		s.W, s.H, s.Aspect())
}

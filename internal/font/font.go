// Package font provides a pre-rendered glyph atlas for the PITVIPER terminal.
// Uses golang.org/x/image/font/basicfont (7×13, pure Go, no system deps).
// Milestone 1: monochrome atlas; color compositing done by the renderer.
package font

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// GlyphW and GlyphH are the cell dimensions in pixels.
// basicfont.Face7x13 advances are 7px wide; we pad to 8 for byte-alignment.
const (
	GlyphW = 8
	GlyphH = 13
)

// Atlas holds a pre-rendered 1-bit bitmap for printable ASCII (0x20–0x7E).
// atlas[ch-0x20] is a GlyphW×GlyphH byte slice: 1 = foreground, 0 = background.
var Atlas [95][GlyphH * GlyphW]byte

// extended holds glyphs outside the ASCII range that real terminal programs
// depend on. Founder, real-time, reporting real symptoms of the actual bug:
// "pitviper having a hard time displaying tmux stuff right" -> "lots of
// question marks" -- tmux draws its pane borders/status-bar dividers with
// Unicode box-drawing characters (U+2500 block); every one of them fell
// outside the old 0x20-0x7e-only Atlas and silently rendered as the '?'
// fallback glyph, which is exactly that symptom. vterm's own UTF-8 decoder
// (internal/vterm/vterm.go) was already correct -- it decodes the runes
// fine, GlyphBits just had nowhere to look them up.
var extended = map[rune][GlyphH * GlyphW]byte{}

// boxArms describes which of the four line segments a single-line
// Unicode box-drawing character draws from the glyph cell's center point
// -- the real, systematic definition every one of these characters shares
// (https://en.wikipedia.org/wiki/Box-drawing_characters), used here
// instead of hand-drawing 11 separate bitmap literals.
type boxArms struct {
	up, down, left, right bool
}

var boxDrawingChars = map[rune]boxArms{
	0x2500: {false, false, true, true}, // ─ horizontal
	0x2502: {true, true, false, false}, // │ vertical
	0x250C: {false, true, false, true}, // ┌ down-and-right
	0x2510: {false, true, true, false}, // ┐ down-and-left
	0x2514: {true, false, false, true}, // └ up-and-right
	0x2518: {true, false, true, false}, // ┘ up-and-left
	0x251C: {true, true, false, true},  // ├ vertical-and-right
	0x2524: {true, true, true, false},  // ┤ vertical-and-left
	0x252C: {false, true, true, true},  // ┬ horizontal-and-down
	0x2534: {true, false, true, true},  // ┴ horizontal-and-up
	0x253C: {true, true, true, true},   // ┼ cross
}

const (
	boxCenterCol = GlyphW / 2
	boxCenterRow = GlyphH / 2
)

func drawBoxChar(arms boxArms) [GlyphH * GlyphW]byte {
	var bits [GlyphH * GlyphW]byte
	if arms.up {
		for y := 0; y <= boxCenterRow; y++ {
			bits[y*GlyphW+boxCenterCol] = 1
		}
	}
	if arms.down {
		for y := boxCenterRow; y < GlyphH; y++ {
			bits[y*GlyphW+boxCenterCol] = 1
		}
	}
	if arms.left {
		for x := 0; x <= boxCenterCol; x++ {
			bits[boxCenterRow*GlyphW+x] = 1
		}
	}
	if arms.right {
		for x := boxCenterCol; x < GlyphW; x++ {
			bits[boxCenterRow*GlyphW+x] = 1
		}
	}
	return bits
}

func init() {
	face := basicfont.Face7x13
	for ch := rune(0x20); ch <= 0x7e; ch++ {
		dst := image.NewGray(image.Rect(0, 0, GlyphW, GlyphH))
		draw.Draw(dst, dst.Bounds(), image.Black, image.Point{}, draw.Src)
		d := font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(color.White),
			Face: face,
			Dot:  fixed.P(0, face.Metrics().Ascent.Round()),
		}
		d.DrawString(string(ch))
		idx := ch - 0x20
		for y := 0; y < GlyphH; y++ {
			for x := 0; x < GlyphW; x++ {
				if dst.GrayAt(x, y).Y > 0 {
					Atlas[idx][y*GlyphW+x] = 1
				}
			}
		}
	}

	for ch, arms := range boxDrawingChars {
		extended[ch] = drawBoxChar(arms)
	}
}

// GlyphBits returns the pre-rendered bits for the given rune.
// Returns the '?' glyph for any rune with no real glyph defined.
func GlyphBits(ch rune) *[GlyphH * GlyphW]byte {
	if ch >= 0x20 && ch <= 0x7e {
		return &Atlas[ch-0x20]
	}
	if bits, ok := extended[ch]; ok {
		return &bits
	}
	return &Atlas['?'-0x20]
}

// KnownGlyphs returns every rune this package has a real (non-'?')
// bitmap for -- every printable ASCII character plus every box-drawing
// character extended holds. Used by the renderer to build a real GPU
// glyph-texture atlas once at startup (founder: "can we do more to
// unload rendering for pitviper onto the gpu?") instead of hardcoding
// or duplicating this package's own two glyph ranges in cmd/pitviper.
func KnownGlyphs() []rune {
	glyphs := make([]rune, 0, len(Atlas)+len(extended))
	for ch := rune(0x20); ch <= 0x7e; ch++ {
		glyphs = append(glyphs, ch)
	}
	for ch := range extended {
		glyphs = append(glyphs, ch)
	}
	return glyphs
}

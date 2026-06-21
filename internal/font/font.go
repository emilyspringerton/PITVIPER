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
}

// GlyphBits returns the pre-rendered bits for the given rune.
// Returns the '?' glyph for any rune outside the ASCII printable range.
func GlyphBits(ch rune) *[GlyphH * GlyphW]byte {
	if ch >= 0x20 && ch <= 0x7e {
		return &Atlas[ch-0x20]
	}
	return &Atlas['?'-0x20]
}

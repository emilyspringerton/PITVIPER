package font_test

import (
	"testing"

	"pitviper/internal/font"
)

func TestGlyphDimensions(t *testing.T) {
	if font.GlyphW != 8 {
		t.Errorf("GlyphW = %d, want 8", font.GlyphW)
	}
	if font.GlyphH != 13 {
		t.Errorf("GlyphH = %d, want 13", font.GlyphH)
	}
}

func TestAtlasSize(t *testing.T) {
	// ASCII printable range 0x20–0x7E = 95 characters.
	if len(font.Atlas) != 95 {
		t.Errorf("Atlas len = %d, want 95", len(font.Atlas))
	}
}

func TestGlyphBitsKnownChars(t *testing.T) {
	// 'A' and 'Z' should have at least one non-zero pixel (real glyphs).
	for _, ch := range []rune{'A', 'Z', '0', '9', ' '} {
		bits := font.GlyphBits(ch)
		if bits == nil {
			t.Fatalf("GlyphBits(%q) returned nil", ch)
		}
		if ch == ' ' {
			// Space glyph may be all-zero — just check it doesn't panic.
			continue
		}
		nonzero := false
		for _, b := range bits {
			if b != 0 {
				nonzero = true
				break
			}
		}
		if !nonzero {
			t.Errorf("GlyphBits(%q): all pixels are zero (expected rendered glyph)", ch)
		}
	}
}

func TestGlyphBitsOutOfRange(t *testing.T) {
	// Non-ASCII runes return the '?' fallback (must not panic).
	bits := font.GlyphBits(0x1F600) // emoji
	if bits == nil {
		t.Fatal("GlyphBits(emoji) returned nil")
	}
	// Should equal the '?' glyph.
	qbits := font.GlyphBits('?')
	if *bits != *qbits {
		t.Error("out-of-range rune did not return '?' glyph")
	}
}

func TestGlyphBitsAllPrintable(t *testing.T) {
	for ch := rune(0x20); ch <= 0x7e; ch++ {
		bits := font.GlyphBits(ch)
		if bits == nil {
			t.Errorf("GlyphBits(%q) returned nil for printable ASCII", ch)
		}
	}
}

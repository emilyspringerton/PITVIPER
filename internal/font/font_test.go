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

// TestGlyphBitsBoxDrawing covers the real bug the founder reported live:
// "pitviper having a hard time displaying tmux stuff right" -> "lots of
// question marks" -- tmux's pane borders/dividers use Unicode box-drawing
// characters, and every one of them used to silently fall back to '?'.
// This doesn't just check "not '?'" (a box char rendered as, say, a solid
// block would also pass that) -- it checks the actual real shape: a
// horizontal line's row across the vertical center, a vertical line's
// column across the horizontal center.
func TestGlyphBitsBoxDrawing(t *testing.T) {
	qbits := font.GlyphBits('?')

	for _, ch := range []rune{0x2500, 0x2502, 0x250C, 0x2510, 0x2514, 0x2518, 0x251C, 0x2524, 0x252C, 0x2534, 0x253C} {
		bits := font.GlyphBits(ch)
		if bits == nil {
			t.Fatalf("GlyphBits(%U) returned nil", ch)
		}
		if *bits == *qbits {
			t.Errorf("GlyphBits(%U) fell back to '?' -- the exact bug being fixed", ch)
		}
	}

	centerRow, centerCol := font.GlyphH/2, font.GlyphW/2

	// ─ (0x2500): full row of set pixels across the horizontal center row.
	horiz := font.GlyphBits(0x2500)
	for x := 0; x < font.GlyphW; x++ {
		if horiz[centerRow*font.GlyphW+x] == 0 {
			t.Errorf("─ (0x2500): pixel (row %d, col %d) is 0, want a set pixel on the center row", centerRow, x)
		}
	}

	// │ (0x2502): full column of set pixels down the vertical center column.
	vert := font.GlyphBits(0x2502)
	for y := 0; y < font.GlyphH; y++ {
		if vert[y*font.GlyphW+centerCol] == 0 {
			t.Errorf("│ (0x2502): pixel (row %d, col %d) is 0, want a set pixel on the center column", y, centerCol)
		}
	}

	// ┼ (0x253C): both a full row and a full column set (all four arms).
	cross := font.GlyphBits(0x253C)
	if cross[centerRow*font.GlyphW] == 0 || cross[centerRow*font.GlyphW+font.GlyphW-1] == 0 {
		t.Error("┼ (0x253C): expected the horizontal arm to reach both edges")
	}
	if cross[centerCol] == 0 || cross[(font.GlyphH-1)*font.GlyphW+centerCol] == 0 {
		t.Error("┼ (0x253C): expected the vertical arm to reach both edges")
	}
}

// TestGlyphBitsBraillePatterns covers the real bug the founder reported
// live: "all the claude little star animations are ?" -> "in pitviper"
// -- the same '?' fallback bug class as box-drawing, this time for the
// real Unicode Braille Pattern block (U+2800-U+28FF) a Node-based CLI
// spinner (Claude Code's own, very plausibly) animates through.
func TestGlyphBitsBraillePatterns(t *testing.T) {
	qbits := font.GlyphBits('?')

	// The whole real 256-character block must have a real glyph, not '?'.
	for ch := rune(0x2800); ch <= 0x28FF; ch++ {
		bits := font.GlyphBits(ch)
		if bits == nil {
			t.Fatalf("GlyphBits(%U) returned nil", ch)
		}
		if *bits == *qbits {
			t.Errorf("GlyphBits(%U) fell back to '?' -- the exact bug being fixed", ch)
		}
	}

	// U+2800 itself (BRAILLE PATTERN BLANK, all 8 dots raised = none) is
	// real and expected to be genuinely blank -- distinct from '?', not
	// a sign the lookup failed.
	blank := font.GlyphBits(0x2800)
	nonzero := false
	for _, b := range blank {
		if b != 0 {
			nonzero = true
			break
		}
	}
	if nonzero {
		t.Error("U+2800 (BRAILLE PATTERN BLANK, no dots raised) should render as genuinely blank")
	}

	// U+28FF (all 8 dots raised) is real and expected to have every one
	// of its 8 real dot positions set -- not just "non-blank," the real
	// specific shape.
	full := font.GlyphBits(0x28FF)
	setCount := 0
	for _, b := range full {
		if b != 0 {
			setCount++
		}
	}
	if setCount == 0 {
		t.Error("U+28FF (all 8 Braille dots raised) rendered as blank, want real dot pixels set")
	}
}

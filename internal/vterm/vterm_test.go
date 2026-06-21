package vterm

import (
	"strings"
	"testing"
)

func textAt(cells []Cell, cols, row int) string {
	var b strings.Builder
	for c := 0; c < cols; c++ {
		ch := cells[row*cols+c].Ch
		if ch == 0 {
			ch = ' '
		}
		b.WriteRune(ch)
	}
	return strings.TrimRight(b.String(), " ")
}

func TestBasicWrite(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("hello"))
	cells, cols, _, _, _ := s.Snapshot()
	got := textAt(cells, cols, 0)
	if got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}

func TestNewline(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("ab\r\ncd"))
	cells, cols, _, curRow, curCol := s.Snapshot()
	if textAt(cells, cols, 0) != "ab" {
		t.Fatalf("row 0: expected %q", "ab")
	}
	if textAt(cells, cols, 1) != "cd" {
		t.Fatalf("row 1: expected %q", "cd")
	}
	if curRow != 1 || curCol != 2 {
		t.Fatalf("cursor at (%d,%d), want (1,2)", curRow, curCol)
	}
}

func TestClearScreen(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("hello"))
	s.Write([]byte("\x1b[2J")) // erase entire screen
	cells, cols, _, curRow, curCol := s.Snapshot()
	if textAt(cells, cols, 0) != "" {
		t.Fatalf("expected blank screen after clear, got %q", textAt(cells, cols, 0))
	}
	if curRow != 0 || curCol != 0 {
		t.Fatalf("cursor at (%d,%d), want (0,0)", curRow, curCol)
	}
}

func TestCursorMove(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("\x1b[3;5H")) // move to row 3, col 5 (1-indexed)
	_, _, _, curRow, curCol := s.Snapshot()
	if curRow != 2 || curCol != 4 {
		t.Fatalf("cursor at (%d,%d), want (2,4)", curRow, curCol)
	}
}

func TestSGRColor(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("\x1b[31mX")) // red foreground
	cells, _, _, _, _ := s.Snapshot()
	if cells[0].FG != Color(1) { // red = index 1
		t.Fatalf("expected FG=1 (red), got %d", cells[0].FG)
	}
	if cells[0].Ch != 'X' {
		t.Fatalf("expected Ch='X', got %q", cells[0].Ch)
	}
}

func TestScrollUp(t *testing.T) {
	s := New(5, 3)
	s.Write([]byte("AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD"))
	cells, cols, _, _, _ := s.Snapshot()
	// Should have scrolled: row 0 = BBBBB, row 1 = CCCCC, row 2 = DDDDD
	if textAt(cells, cols, 0) != "BBBBB" {
		t.Fatalf("after scroll, row 0 = %q, want BBBBB", textAt(cells, cols, 0))
	}
	if textAt(cells, cols, 2) != "DDDDD" {
		t.Fatalf("after scroll, row 2 = %q, want DDDDD", textAt(cells, cols, 2))
	}
}

func TestBackspace(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("abc\x7f")) // 0x7f = DEL treated as backspace via PTY line discipline
	// vterm only receives the rendered output; line discipline handles backspace.
	// Test that raw backspace (\b = 0x08) moves cursor left.
	s2 := New(10, 5)
	s2.Write([]byte("ab\bc")) // overwrite b with c
	cells, cols, _, _, _ := s2.Snapshot()
	if textAt(cells, cols, 0) != "ac" {
		t.Fatalf("after \\b: %q, want %q", textAt(cells, cols, 0), "ac")
	}
}

func TestSGRBold(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("\x1b[1mB\x1b[0mn")) // bold B, then reset, then n
	cells, _, _, _, _ := s.Snapshot()
	if !cells[0].Bold {
		t.Fatal("expected Bold=true for first char")
	}
	if cells[1].Bold {
		t.Fatal("expected Bold=false after reset")
	}
}

func TestSGR256Color(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("\x1b[38;5;200mX")) // xterm-256 fg=200
	cells, _, _, _, _ := s.Snapshot()
	if cells[0].FG != 200 {
		t.Fatalf("expected FG=200, got %d", cells[0].FG)
	}
}

func TestCursorUpDown(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("\x1b[3;1H"))  // cursor to row 3, col 1
	s.Write([]byte("\x1b[1A"))    // cursor up 1 → row 2
	_, _, _, curRow, _ := s.Snapshot()
	if curRow != 1 { // 0-indexed
		t.Fatalf("expected row 1, got %d", curRow)
	}
	s.Write([]byte("\x1b[2B")) // cursor down 2 → row 3
	_, _, _, curRow, _ = s.Snapshot()
	if curRow != 3 {
		t.Fatalf("expected row 3, got %d", curRow)
	}
}

func TestEraseInLine(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("hello"))
	s.Write([]byte("\x1b[1G")) // cursor to col 1 (1-indexed)
	s.Write([]byte("\x1b[K"))  // erase from cursor to end of line
	cells, cols, _, _, _ := s.Snapshot()
	if textAt(cells, cols, 0) != "" {
		t.Fatalf("expected blank line after EL, got %q", textAt(cells, cols, 0))
	}
}

func TestSavRestoreCursor(t *testing.T) {
	s := New(20, 5)
	s.Write([]byte("\x1b[2;5H")) // row 2, col 5
	s.Write([]byte("\x1b7"))     // save
	s.Write([]byte("\x1b[1;1H")) // move elsewhere
	s.Write([]byte("\x1b8"))     // restore
	_, _, _, curRow, curCol := s.Snapshot()
	if curRow != 1 || curCol != 4 { // 0-indexed
		t.Fatalf("after restore: (%d,%d), want (1,4)", curRow, curCol)
	}
}

func TestUTF8Rune(t *testing.T) {
	s := New(20, 5)
	// "café" — e followed by 3-byte UTF-8 'é' (U+00E9 = 0xC3 0xA9)
	s.Write([]byte("caf\xc3\xa9"))
	cells, cols, _, _, curCol := s.Snapshot()
	if textAt(cells, cols, 0) != "café" {
		t.Fatalf("utf8: got %q, want %q", textAt(cells, cols, 0), "café")
	}
	if curCol != 4 {
		t.Fatalf("utf8: cursor at col %d, want 4", curCol)
	}
}

func TestScrollbackLineSaved(t *testing.T) {
	s := New(10, 3)
	s.Write([]byte("LINE1\r\nLINE2\r\nLINE3\r\nLINE4")) // LINE1 scrolls off
	if s.ScrollbackLen() < 1 {
		t.Fatalf("expected scrollback to have ≥1 line, got %d", s.ScrollbackLen())
	}
}

func TestScrollbackView(t *testing.T) {
	s := New(10, 2)
	// Write 3 lines into a 2-row screen → LINE1 scrolls to scrollback, visible: LINE2+LINE3
	s.Write([]byte("LINE1     \r\nLINE2     \r\nLINE3     "))

	// Without scrolling: live view shows LINE2 and LINE3
	cells, cols, _, curRow, _ := s.Snapshot()
	if textAt(cells, cols, 0) != "LINE2" {
		t.Fatalf("live row 0: got %q, want LINE2", textAt(cells, cols, 0))
	}
	if curRow < 0 {
		t.Error("live view should show cursor")
	}

	// Scroll back 1 line: should show LINE1 and LINE2
	s.ScrollBy(1)
	cells, cols, _, curRow, _ = s.Snapshot()
	if textAt(cells, cols, 0) != "LINE1" {
		t.Fatalf("scrolled row 0: got %q, want LINE1", textAt(cells, cols, 0))
	}
	if curRow != -1 {
		t.Errorf("scrolled view should hide cursor (curRow=%d)", curRow)
	}

	// Reset scroll — cursor reappears
	s.ScrollReset()
	cells, cols, _, curRow, _ = s.Snapshot()
	if textAt(cells, cols, 0) != "LINE2" {
		t.Fatalf("after reset, row 0: got %q, want LINE2", textAt(cells, cols, 0))
	}
	if curRow < 0 {
		t.Error("after reset, cursor should be visible")
	}
}

func TestScrollByClamp(t *testing.T) {
	s := New(5, 2)
	s.Write([]byte("AAAAA\r\nBBBBB\r\nCCCCC")) // 1 line in scrollback
	s.ScrollBy(MaxScrollback + 100)             // over-scroll
	if s.ScrollLines() != s.ScrollbackLen() {
		t.Errorf("over-scroll: scrollLines=%d, want %d", s.ScrollLines(), s.ScrollbackLen())
	}
	s.ScrollBy(-MaxScrollback) // under-scroll to 0
	if s.ScrollLines() != 0 {
		t.Errorf("under-scroll: scrollLines=%d, want 0", s.ScrollLines())
	}
}

func TestResize(t *testing.T) {
	s := New(10, 5)
	s.Write([]byte("hello"))
	s.Resize(20, 10)
	cells, cols, rows, _, _ := s.Snapshot()
	if cols != 20 || rows != 10 {
		t.Fatalf("after resize: cols=%d rows=%d, want 20 10", cols, rows)
	}
	if textAt(cells, cols, 0) != "hello" {
		t.Fatalf("content after resize: %q, want hello", textAt(cells, cols, 0))
	}
}

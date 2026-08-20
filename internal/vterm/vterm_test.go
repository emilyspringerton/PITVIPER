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
	s.Write([]byte("\x1b[3;1H")) // cursor to row 3, col 1
	s.Write([]byte("\x1b[1A"))   // cursor up 1 → row 2
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

func TestCSICNLCPLECH(t *testing.T) {
	// CNL (CSI E): cursor next line N, col=0
	s := New(20, 5)
	s.Write([]byte("START"))
	s.Write([]byte("\033[2E")) // down 2 rows, col=0
	_, _, _, curRow, curCol := s.Snapshot()
	if curRow != 2 || curCol != 0 {
		t.Errorf("CNL: cursor at (%d,%d), want (2,0)", curRow, curCol)
	}

	// CPL (CSI F): cursor previous line N, col=0
	s.Write([]byte("\033[1F")) // up 1 row, col=0
	_, _, _, curRow, curCol = s.Snapshot()
	if curRow != 1 || curCol != 0 {
		t.Errorf("CPL: cursor at (%d,%d), want (1,0)", curRow, curCol)
	}

	// ECH (CSI X): erase N chars at cursor, no movement
	s2 := New(20, 5)
	s2.Write([]byte("ABCDEFGHIJ"))
	s2.Write([]byte("\033[1;4H")) // cursor to col 3 (0-indexed)
	s2.Write([]byte("\033[3X"))   // erase 3 chars → D,E,F blanked; cursor stays at col 3
	cells, cols, _, _, curCol2 := s2.Snapshot()
	if curCol2 != 3 {
		t.Errorf("ECH: cursor stayed at %d, want 3", curCol2)
	}
	row0 := textAt(cells, cols, 0)
	if !strings.HasPrefix(row0, "ABC") || !strings.Contains(row0, "GHI") {
		t.Errorf("ECH: row0 = %q, want ABC followed by blank then GHI", row0)
	}
}

func TestCSIInsertDeleteLine(t *testing.T) {
	// CSI L: insert line at cursor — rows from cursor down shift down, cursor row blanked.
	s := New(10, 4)
	s.Write([]byte("ROW0\r\nROW1\r\nROW2\r\nROW3"))
	// Move cursor to row 1
	s.Write([]byte("\033[2;1H"))
	// Insert 1 line → ROW1,ROW2,ROW3 shift down; row 1 blanked; ROW3 pushed off
	s.Write([]byte("\033[1L"))
	cells, cols, _, _, _ := s.Snapshot()
	if textAt(cells, cols, 0) != "ROW0" {
		t.Errorf("IL: row0 = %q, want ROW0", textAt(cells, cols, 0))
	}
	if textAt(cells, cols, 1) != "" {
		t.Errorf("IL: row1 = %q, want blank (inserted)", textAt(cells, cols, 1))
	}
	if textAt(cells, cols, 2) != "ROW1" {
		t.Errorf("IL: row2 = %q, want ROW1", textAt(cells, cols, 2))
	}
	if textAt(cells, cols, 3) != "ROW2" {
		t.Errorf("IL: row3 = %q, want ROW2", textAt(cells, cols, 3))
	}

	// CSI M: delete line at cursor — rows below shift up, last row blanked.
	s2 := New(10, 3)
	s2.Write([]byte("ROW0\r\nROW1\r\nROW2"))
	s2.Write([]byte("\033[1;1H")) // cursor to row 0
	s2.Write([]byte("\033[1M"))   // delete 1 line → ROW1,ROW2 shift up; last row blank
	cells2, cols2, _, _, _ := s2.Snapshot()
	if textAt(cells2, cols2, 0) != "ROW1" {
		t.Errorf("DL: row0 = %q, want ROW1", textAt(cells2, cols2, 0))
	}
	if textAt(cells2, cols2, 1) != "ROW2" {
		t.Errorf("DL: row1 = %q, want ROW2", textAt(cells2, cols2, 1))
	}
	if textAt(cells2, cols2, 2) != "" {
		t.Errorf("DL: row2 = %q, want blank", textAt(cells2, cols2, 2))
	}
}

func TestCSIScrollUpDown(t *testing.T) {
	s := New(10, 3)
	s.Write([]byte("ROW0\r\nROW1\r\nROW2"))

	// CSI 1 S — scroll up 1 line within scroll region
	s.Write([]byte("\033[1S"))
	cells, cols, _, _, _ := s.Snapshot()
	if textAt(cells, cols, 0) != "ROW1" {
		t.Errorf("after CSI S: row 0 = %q, want ROW1", textAt(cells, cols, 0))
	}
	if textAt(cells, cols, 2) != "" {
		t.Errorf("after CSI S: row 2 = %q, want blank", textAt(cells, cols, 2))
	}

	// CSI 1 T — scroll down 1 line
	s.Write([]byte("\033[1T"))
	cells, cols, _, _, _ = s.Snapshot()
	if textAt(cells, cols, 0) != "" {
		t.Errorf("after CSI T: row 0 = %q, want blank", textAt(cells, cols, 0))
	}
	if textAt(cells, cols, 1) != "ROW1" {
		t.Errorf("after CSI T: row 1 = %q, want ROW1", textAt(cells, cols, 1))
	}
}

func TestCSIDeleteInsertChar(t *testing.T) {
	// CSI 2 P — delete 2 chars at cursor
	s := New(10, 3)
	s.Write([]byte("ABCDEFGH"))
	s.Write([]byte("\033[1;3H")) // cursor to row 1, col 3 (1-indexed) → col 2
	s.Write([]byte("\033[2P"))   // delete 2 chars at col 2
	cells, cols, _, _, _ := s.Snapshot()
	// "ABCDEFGH" → delete at col 2, n=2 → "ABEFGH  "
	if textAt(cells, cols, 0) != "ABEFGH" {
		t.Errorf("CSI P: row 0 = %q, want ABEFGH", textAt(cells, cols, 0))
	}

	// CSI 2 @ — insert 2 blank chars at cursor
	s2 := New(10, 3)
	s2.Write([]byte("ABCDEFGH"))
	s2.Write([]byte("\033[1;3H")) // cursor to col 2 (0-indexed)
	s2.Write([]byte("\033[2@"))   // insert 2 blanks at col 2
	cells2, cols2, _, _, _ := s2.Snapshot()
	// screen=10; "ABCDEFGH" + 2 trailing spaces; insert 2 at col 2 → "AB  CDEFGH"
	if textAt(cells2, cols2, 0) != "AB  CDEFGH" {
		t.Errorf("CSI @: row 0 = %q, want AB  CDEFGH", textAt(cells2, cols2, 0))
	}
}

func TestTabStop(t *testing.T) {
	s := New(40, 5)
	// Tab from col 0 → col 8
	s.Write([]byte("A\tB"))
	_, _, _, _, curCol := s.Snapshot()
	if curCol != 9 { // A at 0, tab→8, B at 8 → cursor at 9
		t.Errorf("after A tab B: curCol=%d, want 9", curCol)
	}
	// Tab from col 9 → col 16
	s.Write([]byte("\tC"))
	_, _, _, _, curCol = s.Snapshot()
	if curCol != 17 {
		t.Errorf("after second tab: curCol=%d, want 17", curCol)
	}
}

func TestIsAltActive(t *testing.T) {
	s := New(20, 5)
	if s.IsAltActive() {
		t.Error("default: IsAltActive should be false")
	}
	s.Write([]byte("\033[?1049h"))
	if !s.IsAltActive() {
		t.Error("after ?1049h: IsAltActive should be true")
	}
	s.Write([]byte("\033[?1049l"))
	if s.IsAltActive() {
		t.Error("after ?1049l: IsAltActive should be false")
	}
}

func TestSGRDefaultColors(t *testing.T) {
	s := New(10, 5)
	// Set red FG and blue BG, then reset to default via ESC [39;49m
	s.Write([]byte("\033[31;44mX")) // red FG + blue BG
	cells, _, _, _, _ := s.Snapshot()
	if cells[0].FG != Color(1) || cells[0].BG != Color(4) {
		t.Fatalf("initial SGR: FG=%d BG=%d, want 1 4", cells[0].FG, cells[0].BG)
	}
	s.Write([]byte("\033[39;49mY")) // default FG + default BG
	cells, _, _, _, _ = s.Snapshot()
	if cells[1].FG != ColorDefault {
		t.Errorf("after SGR39: FG=%d, want ColorDefault(%d)", cells[1].FG, ColorDefault)
	}
	if cells[1].BG != ColorDefault {
		t.Errorf("after SGR49: BG=%d, want ColorDefault(%d)", cells[1].BG, ColorDefault)
	}
}

func TestEraseInDisplay(t *testing.T) {
	// ESC [0J — erase from cursor to end of screen
	s := New(5, 3)
	s.Write([]byte("AAAAA\r\nBBBBB\r\nCCCCC"))
	s.Write([]byte("\033[2;3H")) // row 2, col 3 (1-indexed) → (1,2) 0-indexed
	s.Write([]byte("\033[0J"))   // erase from cursor to end
	cells, cols, _, _, _ := s.Snapshot()
	if textAt(cells, cols, 0) != "AAAAA" {
		t.Errorf("0J: row 0 = %q, want AAAAA", textAt(cells, cols, 0))
	}
	r1 := textAt(cells, cols, 1)
	if len(r1) >= 3 && r1[2] != ' ' {
		t.Errorf("0J: row 1 col 2 should be blank after 0J, got %q", r1)
	}

	// ESC [1J — erase from beginning to cursor
	s2 := New(5, 3)
	s2.Write([]byte("AAAAA\r\nBBBBB\r\nCCCCC"))
	s2.Write([]byte("\033[2;3H")) // row 2, col 3 (1-indexed) → (1,2) 0-indexed
	s2.Write([]byte("\033[1J"))   // erase from start to cursor
	cells2, cols2, _, _, _ := s2.Snapshot()
	// Row 0 should be blank.
	if textAt(cells2, cols2, 0) != "" {
		t.Errorf("1J: row 0 = %q, want blank", textAt(cells2, cols2, 0))
	}
	// Row 2 (CCCCC) should be untouched.
	if textAt(cells2, cols2, 2) != "CCCCC" {
		t.Errorf("1J: row 2 = %q, want CCCCC", textAt(cells2, cols2, 2))
	}
}

func TestDECSTBM(t *testing.T) {
	// 5 rows; set scrolling region to rows 2-4 (1-indexed = top=2, bot=4 → 0-indexed 1..3)
	s := New(10, 5)
	s.Write([]byte("ROW0\r\nROW1\r\nROW2\r\nROW3\r\nROW4"))
	// Set scroll region rows 2-4 (1-indexed), then write a newline inside to force scroll.
	// DECSTBM resets cursor to (0,0). Move cursor to bottom of region then add newline.
	s.Write([]byte("\033[2;4r"))        // set scroll region rows 2-4
	s.Write([]byte("\033[4;1H"))        // move to row 4, col 1
	s.Write([]byte("\r\nSCROLLED\r\n")) // two newlines — region scrolls, not full screen
	cells, cols, _, _, _ := s.Snapshot()
	// Row 0 (ROW0) should be untouched — it is outside the scroll region.
	if textAt(cells, cols, 0) != "ROW0" {
		t.Errorf("DECSTBM: row 0 = %q, want ROW0", textAt(cells, cols, 0))
	}
}

func TestReverseIndex(t *testing.T) {
	s := New(10, 3)
	// Fill screen with LINE1/LINE2/LINE3, cursor at row 0 (top).
	s.Write([]byte("LINE1\r\nLINE2\r\nLINE3"))
	s.Write([]byte("\033[1;1H")) // cursor to row 1, col 1 (top of screen)
	// ESC M at top of scroll region should scroll down, inserting blank at top.
	s.Write([]byte("\033M"))
	cells, cols, _, _, _ := s.Snapshot()
	// Row 0 should now be blank (inserted by scroll-down).
	if textAt(cells, cols, 0) != "" {
		t.Errorf("after RI: row 0 = %q, want blank", textAt(cells, cols, 0))
	}
	// Previous row 0 (LINE1) pushed to row 1.
	if textAt(cells, cols, 1) != "LINE1" {
		t.Errorf("after RI: row 1 = %q, want LINE1", textAt(cells, cols, 1))
	}
	// Previous row 1 (LINE2) pushed to row 2.
	if textAt(cells, cols, 2) != "LINE2" {
		t.Errorf("after RI: row 2 = %q, want LINE2", textAt(cells, cols, 2))
	}
}

func TestAlternateScreen(t *testing.T) {
	s := New(20, 5)
	s.Write([]byte("PRIMARY"))
	cells, cols, _, _, _ := s.Snapshot()
	if textAt(cells, cols, 0) != "PRIMARY" {
		t.Fatalf("before alt: row 0 = %q, want PRIMARY", textAt(cells, cols, 0))
	}

	// Enter alternate screen — primary content should be saved, screen cleared.
	s.Write([]byte("\033[?1049h"))
	cells, cols, _, curRow, curCol := s.Snapshot()
	if textAt(cells, cols, 0) != "" {
		t.Errorf("alt screen: row 0 = %q, want empty", textAt(cells, cols, 0))
	}
	if curRow != 0 || curCol != 0 {
		t.Errorf("alt screen cursor: (%d,%d), want (0,0)", curRow, curCol)
	}

	s.Write([]byte("ALTERNATE"))

	// Leave alternate screen — primary content restored.
	s.Write([]byte("\033[?1049l"))
	cells, cols, _, _, _ = s.Snapshot()
	if textAt(cells, cols, 0) != "PRIMARY" {
		t.Errorf("after alt: row 0 = %q, want PRIMARY", textAt(cells, cols, 0))
	}
}

// TestAlternateScreenResetsScrollRegion is a real regression test for the
// bug the founder reported live: "if i quit a full screen terminal app
// and i type clear it like doesnt clear." Root cause: a fullscreen app
// (vim/htop/less all routinely do this, e.g. to reserve a status line)
// sets a narrower DECSTBM scroll region while in the alternate screen;
// that region wasn't part of the alt-screen save/restore set at all
// (unlike cursor position and colors, right next to it in the same
// code), so it silently leaked onto the primary screen after the app
// quit -- every scroll from then on (a shell prompt's own linefeeds
// included) stayed confined to that leftover narrow band, which looks
// exactly like "clear doesn't work" once new output only ever touches
// a few rows near the top.
func TestAlternateScreenResetsScrollRegion(t *testing.T) {
	s := New(20, 10)

	// Enter alt screen and set a narrow scroll region, the way vim does
	// for a reserved status line -- rows 1-3 only (1-indexed).
	s.Write([]byte("\033[?1049h"))
	s.Write([]byte("\033[2;4r"))

	// Leave alt screen.
	s.Write([]byte("\033[?1049l"))

	// Real assertion: a scroll-triggering linefeed run from the bottom
	// row must now scroll the *whole* primary screen, not stay confined
	// to the vim app's old rows 1-3 band. Fill every row with a unique
	// marker, then linefeed once from the last row.
	for r := 0; r < 10; r++ {
		s.Write([]byte("\r"))
		s.Write([]byte("row" + string(rune('0'+r))))
		if r < 9 {
			s.Write([]byte("\n"))
		}
	}
	// One more linefeed from the bottom row should scroll row 0 off the
	// top (real full-screen scroll), not silently do nothing / only
	// touch rows 1-3.
	s.Write([]byte("\n"))
	cells, cols, _, _, _ := s.Snapshot()
	if textAt(cells, cols, 0) == "row0" {
		t.Error("scroll region leaked from the alternate screen: row 0 still shows the pre-scroll " +
			"content, meaning the linefeed-triggered scroll stayed confined to the fullscreen app's " +
			"old narrow band instead of scrolling the whole primary screen")
	}
}

func TestCursorVisibility(t *testing.T) {
	s := New(20, 5)
	// Default: cursor visible
	if s.CursorHidden {
		t.Error("expected cursor visible by default")
	}
	// ESC [?25l — hide cursor
	s.Write([]byte("\033[?25l"))
	if !s.CursorHidden {
		t.Error("expected cursor hidden after \\033[?25l")
	}
	_, _, _, curRow, curCol := s.Snapshot()
	if curRow != -1 || curCol != -1 {
		t.Errorf("Snapshot with hidden cursor: (%d,%d), want (-1,-1)", curRow, curCol)
	}
	// ESC [?25h — show cursor again
	s.Write([]byte("\033[?25h"))
	if s.CursorHidden {
		t.Error("expected cursor visible after \\033[?25h")
	}
}

func TestOSCWindowTitle(t *testing.T) {
	s := New(20, 5)
	// BEL-terminated OSC 2 (window title)
	s.Write([]byte("\033]2;myshell\007"))
	if s.GetTitle() != "myshell" {
		t.Errorf("OSC2 BEL: title = %q, want myshell", s.GetTitle())
	}
	// ST-terminated OSC 0 (icon + title)
	s.Write([]byte("\033]0;newtitle\033\\"))
	if s.GetTitle() != "newtitle" {
		t.Errorf("OSC0 ST: title = %q, want newtitle", s.GetTitle())
	}
	// OSC must not corrupt surrounding text
	s.Write([]byte("AB\033]2;vim\007CD"))
	cells, cols, _, _, _ := s.Snapshot()
	if textAt(cells, cols, 0) != "ABCD" {
		t.Errorf("OSC mid-stream: got %q, want ABCD", textAt(cells, cols, 0))
	}
	if s.GetTitle() != "vim" {
		t.Errorf("OSC mid-stream title = %q, want vim", s.GetTitle())
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
	s.ScrollBy(MaxScrollback + 100)            // over-scroll
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

// Package vterm implements a minimal VT100 screen state machine.
// It maintains a fixed grid of cells and processes PTY output bytes.
package vterm

import (
	"sync"
	"unicode/utf8"
)

// Color is an xterm-256 color index (0–255) or one of the two sentinel values below.
type Color uint16

const (
	ColorDefault Color = 256 // use terminal default (white fg, black bg)
	ColorBold    Color = 257 // modifier; not a color
)

// Cell is one character cell in the terminal grid.
type Cell struct {
	Ch   rune
	FG   Color
	BG   Color
	Bold bool
}

// Screen is the VT100 screen state machine. Safe for concurrent use.
type Screen struct {
	mu       sync.Mutex
	cols     int
	rows     int
	cells    []Cell // [row*cols + col]
	curRow   int
	curCol   int
	savedRow int
	savedCol int
	// SGR state
	fg   Color
	bg   Color
	bold bool
	// scrolling region (DECSTBM, inclusive 0-indexed). Default: [0, rows-1].
	scrollRegTop int
	scrollRegBot int
	// escape buffer
	esc []byte
	// UTF-8 accumulator for multi-byte sequences
	utf8Buf [4]byte
	utf8Len int
	// scrollback buffer and view offset
	sb        *scrollbackBuf
	scrollTop int // lines scrolled back from live view (0 = live)
	// window title (from OSC 0/1/2 sequences)
	Title string
	// cursor visibility — false when ?25l is active (vim rendering mode)
	CursorHidden bool
	// alternate screen buffer (saved when ?1049h is received)
	altActive    bool
	altCells     []Cell
	altCurRow    int
	altCurCol    int
	altSavedRow  int
	altSavedCol  int
	altFG, altBG Color
	altBold      bool
}

// New creates a Screen of the given dimensions.
func New(cols, rows int) *Screen {
	s := &Screen{
		cols:         cols,
		rows:         rows,
		fg:           ColorDefault,
		bg:           ColorDefault,
		sb:           newScrollbackBuf(MaxScrollback),
		scrollRegTop: 0,
		scrollRegBot: rows - 1,
	}
	s.cells = make([]Cell, cols*rows)
	s.clear(0, 0, cols*rows)
	return s
}

func (s *Screen) clear(start, end, n int) {
	blank := Cell{Ch: ' ', FG: s.fg, BG: s.bg}
	if end > n {
		end = n
	}
	for i := start; i < end; i++ {
		s.cells[i] = blank
	}
}

// Resize adapts the screen to new dimensions, preserving content where possible.
func (s *Screen) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols == s.cols && rows == s.rows {
		return
	}
	newCells := make([]Cell, cols*rows)
	blank := Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}
	for i := range newCells {
		newCells[i] = blank
	}
	copyRows := s.rows
	if rows < copyRows {
		copyRows = rows
	}
	copyCols := s.cols
	if cols < copyCols {
		copyCols = cols
	}
	for r := 0; r < copyRows; r++ {
		for c := 0; c < copyCols; c++ {
			newCells[r*cols+c] = s.cells[r*s.cols+c]
		}
	}
	s.cells = newCells
	s.cols = cols
	s.rows = rows
	if s.curCol >= cols {
		s.curCol = cols - 1
	}
	if s.curRow >= rows {
		s.curRow = rows - 1
	}
}

// Write processes raw PTY output and updates the screen state.
// Handles UTF-8 multi-byte sequences; invalid byte sequences are replaced with U+FFFD.
func (s *Screen) Write(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < len(data); {
		b := data[i]

		// Flush any pending UTF-8 accumulator if we hit an ASCII control or ESC.
		if s.utf8Len > 0 && (b < 0x80 || b >= 0xC0) {
			s.putChar(utf8.RuneError)
			s.utf8Len = 0
		}

		// Inside a multi-byte UTF-8 sequence — continuation byte.
		if s.utf8Len > 0 && b >= 0x80 && b < 0xC0 {
			s.utf8Buf[s.utf8Len] = b
			s.utf8Len++
			need := utf8SeqLen(s.utf8Buf[0])
			if s.utf8Len >= need {
				r, _ := utf8.DecodeRune(s.utf8Buf[:s.utf8Len])
				s.putChar(r)
				s.utf8Len = 0
			}
			i++
			continue
		}

		// Handle escape sequences (ASCII only — ESC is never part of UTF-8).
		if len(s.esc) > 0 {
			s.esc = append(s.esc, b)
			s.handleEsc()
			i++
			continue
		}

		// Start of a multi-byte UTF-8 sequence.
		if b >= 0xC0 {
			s.utf8Buf[0] = b
			s.utf8Len = 1
			i++
			continue
		}

		switch b {
		case 0x1b: // ESC
			s.esc = []byte{0x1b}
		case '\r':
			s.curCol = 0
		case '\n':
			s.curRow++
			if s.curRow > s.scrollRegBot {
				s.scrollUp(1)
				s.curRow = s.scrollRegBot
			}
		case '\b':
			if s.curCol > 0 {
				s.curCol--
			}
		case 7: // BEL — ignore
		case 0: // NUL — ignore
		default:
			if b >= 0x20 {
				s.putChar(rune(b))
			}
		}
		i++
	}
}

// handleOSC processes an OSC payload (the bytes between ESC ] and the terminator).
// Supported: 0 (icon+title), 1 (icon title), 2 (window title).
func (s *Screen) handleOSC(payload []byte) {
	// payload format: "N;text" where N is the OSC command number.
	semi := -1
	for i, b := range payload {
		if b == ';' {
			semi = i
			break
		}
	}
	if semi < 0 {
		return
	}
	cmd := string(payload[:semi])
	text := string(payload[semi+1:])
	switch cmd {
	case "0", "1", "2": // set icon/window title
		s.Title = text
	}
}

// GetTitle returns the current window title (from OSC sequences).
func (s *Screen) GetTitle() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Title
}

// utf8SeqLen returns the total byte length of a UTF-8 sequence starting with lead byte b.
func utf8SeqLen(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b < 0xE0:
		return 2
	case b < 0xF0:
		return 3
	default:
		return 4
	}
}

func (s *Screen) putChar(ch rune) {
	if s.curCol >= s.cols {
		s.curCol = 0
		s.curRow++
		if s.curRow > s.scrollRegBot {
			s.scrollUp(1)
			s.curRow = s.scrollRegBot
		}
	}
	s.cells[s.curRow*s.cols+s.curCol] = Cell{
		Ch:   ch,
		FG:   s.fg,
		BG:   s.bg,
		Bold: s.bold,
	}
	s.curCol++
}

func (s *Screen) scrollUp(n int) {
	top := s.scrollRegTop
	bot := s.scrollRegBot

	// Full-screen scroll with default region — save to scrollback.
	if top == 0 && bot == s.rows-1 {
		if n >= s.rows {
			for r := 0; r < s.rows; r++ {
				s.sb.push(s.cells[r*s.cols : (r+1)*s.cols])
			}
			s.clear(0, s.cols*s.rows, s.cols*s.rows)
			return
		}
		for r := 0; r < n; r++ {
			s.sb.push(s.cells[r*s.cols : (r+1)*s.cols])
		}
		copy(s.cells, s.cells[n*s.cols:])
		blank := Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}
		start := (s.rows - n) * s.cols
		for i := start; i < s.rows*s.cols; i++ {
			s.cells[i] = blank
		}
		return
	}

	// Scroll within restricted region — no scrollback save.
	height := bot - top + 1
	if n >= height {
		// Clear entire region.
		for r := top; r <= bot; r++ {
			for c := 0; c < s.cols; c++ {
				s.cells[r*s.cols+c] = Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}
			}
		}
		return
	}
	// Shift rows up within [top, bot].
	for r := top; r <= bot-n; r++ {
		copy(s.cells[r*s.cols:(r+1)*s.cols], s.cells[(r+n)*s.cols:(r+n+1)*s.cols])
	}
	for r := bot - n + 1; r <= bot; r++ {
		for c := 0; c < s.cols; c++ {
			s.cells[r*s.cols+c] = Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}
		}
	}
}

// scrollDown scrolls the content in the scroll region down by n lines,
// inserting blank lines at the top. Used by ESC M (RI) reverse index.
func (s *Screen) scrollDown(n int) {
	top := s.scrollRegTop
	bot := s.scrollRegBot
	height := bot - top + 1
	if n >= height {
		for r := top; r <= bot; r++ {
			for c := 0; c < s.cols; c++ {
				s.cells[r*s.cols+c] = Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}
			}
		}
		return
	}
	// Shift rows down within [top, bot].
	for r := bot; r >= top+n; r-- {
		copy(s.cells[r*s.cols:(r+1)*s.cols], s.cells[(r-n)*s.cols:(r-n+1)*s.cols])
	}
	for r := top; r < top+n; r++ {
		for c := 0; c < s.cols; c++ {
			s.cells[r*s.cols+c] = Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}
		}
	}
}

// handleEsc processes escape sequences.
// Sequences supported: CSI cursor movement, SGR colors, clear screen/line, save/restore cursor.
func (s *Screen) handleEsc() {
	e := s.esc
	if len(e) < 2 {
		return
	}
	// OSC sequences: ESC ] N ; text BEL   or   ESC ] N ; text ESC \
	if e[1] == ']' {
		// Accumulate until BEL (0x07) or ST (ESC \) terminates the sequence.
		last := e[len(e)-1]
		if last == 0x07 {
			s.handleOSC(e[2 : len(e)-1])
			s.esc = nil
			return
		}
		if len(e) >= 3 && e[len(e)-2] == 0x1b && last == '\\' {
			s.handleOSC(e[2 : len(e)-2])
			s.esc = nil
			return
		}
		if len(e) > 256 {
			s.esc = nil // safety valve
		}
		return
	}

	if e[1] != '[' && e[1] != '7' && e[1] != '8' && e[1] != 'c' && e[1] != 'M' {
		// Unknown ESC byte — consume and reset if it's a terminator.
		if e[1] >= 0x40 && e[1] <= 0x7e {
			s.esc = nil
		}
		return
	}
	// ESC M — Reverse Index (RI): move cursor up one line, scroll down if at top of scroll region
	if e[1] == 'M' {
		if s.curRow == s.scrollRegTop {
			s.scrollDown(1)
		} else if s.curRow > 0 {
			s.curRow--
		}
		s.esc = nil
		return
	}
	// ESC 7 — save cursor
	if e[1] == '7' {
		s.savedRow, s.savedCol = s.curRow, s.curCol
		s.esc = nil
		return
	}
	// ESC 8 — restore cursor
	if e[1] == '8' {
		s.curRow, s.curCol = s.savedRow, s.savedCol
		s.esc = nil
		return
	}
	// ESC c — full reset
	if e[1] == 'c' {
		s.curRow, s.curCol = 0, 0
		s.fg, s.bg, s.bold = ColorDefault, ColorDefault, false
		s.clear(0, s.cols*s.rows, s.cols*s.rows)
		s.esc = nil
		return
	}
	// CSI sequences — ESC [ ... final
	if e[1] == '[' {
		if len(e) < 3 {
			return // need at least ESC [ <final>
		}
		last := e[len(e)-1]
		if last < 0x40 || last > 0x7e {
			// Not yet complete — accumulate.
			if len(e) > 64 {
				s.esc = nil // safety valve
			}
			return
		}
		params := string(e[2 : len(e)-1])
		s.handleCSI(last, params)
		s.esc = nil
	}
}

func (s *Screen) handleCSI(final byte, params string) {
	nums := parseNums(params)

	switch final {
	case 'A': // cursor up
		n := numOr1(nums, 0)
		s.curRow -= n
		if s.curRow < 0 {
			s.curRow = 0
		}
	case 'B': // cursor down
		n := numOr1(nums, 0)
		s.curRow += n
		if s.curRow >= s.rows {
			s.curRow = s.rows - 1
		}
	case 'C': // cursor right
		n := numOr1(nums, 0)
		s.curCol += n
		if s.curCol >= s.cols {
			s.curCol = s.cols - 1
		}
	case 'D': // cursor left
		n := numOr1(nums, 0)
		s.curCol -= n
		if s.curCol < 0 {
			s.curCol = 0
		}
	case 'G': // CHA — cursor horizontal absolute (1-indexed column)
		col := 0
		if len(nums) >= 1 && nums[0] > 0 {
			col = nums[0] - 1
		}
		s.curCol = clamp(col, 0, s.cols-1)
	case 'd': // VPA — vertical position absolute (1-indexed row)
		row := 0
		if len(nums) >= 1 && nums[0] > 0 {
			row = nums[0] - 1
		}
		s.curRow = clamp(row, 0, s.rows-1)
	case 'H', 'f': // cursor position
		row := 0
		col := 0
		if len(nums) >= 1 && nums[0] > 0 {
			row = nums[0] - 1
		}
		if len(nums) >= 2 && nums[1] > 0 {
			col = nums[1] - 1
		}
		s.curRow = clamp(row, 0, s.rows-1)
		s.curCol = clamp(col, 0, s.cols-1)
	case 'J': // erase in display
		n := 0
		if len(nums) > 0 {
			n = nums[0]
		}
		switch n {
		case 0: // to end
			s.clear(s.curRow*s.cols+s.curCol, s.rows*s.cols, s.rows*s.cols)
		case 1: // to beginning
			s.clear(0, s.curRow*s.cols+s.curCol+1, s.rows*s.cols)
		case 2, 3: // entire screen
			s.clear(0, s.rows*s.cols, s.rows*s.cols)
			s.curRow, s.curCol = 0, 0
		}
	case 'K': // erase in line
		n := 0
		if len(nums) > 0 {
			n = nums[0]
		}
		switch n {
		case 0: // to end of line
			for c := s.curCol; c < s.cols; c++ {
				s.cells[s.curRow*s.cols+c] = Cell{Ch: ' ', FG: s.fg, BG: s.bg}
			}
		case 1: // to start of line
			for c := 0; c <= s.curCol; c++ {
				s.cells[s.curRow*s.cols+c] = Cell{Ch: ' ', FG: s.fg, BG: s.bg}
			}
		case 2: // whole line
			for c := 0; c < s.cols; c++ {
				s.cells[s.curRow*s.cols+c] = Cell{Ch: ' ', FG: s.fg, BG: s.bg}
			}
		}
	case 'm': // SGR
		s.handleSGR(nums)
	case 's': // save cursor
		s.savedRow, s.savedCol = s.curRow, s.curCol
	case 'u': // restore cursor
		s.curRow, s.curCol = s.savedRow, s.savedCol
	case 'r': // DECSTBM: set scrolling region (top;bot, 1-indexed)
		top := 1
		bot := s.rows
		if len(nums) >= 1 && nums[0] > 0 {
			top = nums[0]
		}
		if len(nums) >= 2 && nums[1] > 0 {
			bot = nums[1]
		}
		t := clamp(top-1, 0, s.rows-1)
		b := clamp(bot-1, 0, s.rows-1)
		if t < b {
			s.scrollRegTop, s.scrollRegBot = t, b
			s.curRow, s.curCol = 0, 0 // DECSTBM resets cursor to (0,0)
		}
	case 'h', 'l': // DEC private mode set/reset
		switch params {
		case "?25":
			s.CursorHidden = (final == 'l')
		case "?1049":
			if final == 'h' && !s.altActive {
				// Save primary screen and cursor, enter alternate screen.
				s.altCells = make([]Cell, len(s.cells))
				copy(s.altCells, s.cells)
				s.altCurRow, s.altCurCol = s.curRow, s.curCol
				s.altSavedRow, s.altSavedCol = s.savedRow, s.savedCol
				s.altFG, s.altBG, s.altBold = s.fg, s.bg, s.bold
				s.altActive = true
				s.curRow, s.curCol = 0, 0
				s.savedRow, s.savedCol = 0, 0
				s.fg, s.bg, s.bold = ColorDefault, ColorDefault, false
				s.clear(0, len(s.cells), len(s.cells))
			} else if final == 'l' && s.altActive {
				// Restore primary screen and cursor.
				copy(s.cells, s.altCells)
				s.curRow, s.curCol = s.altCurRow, s.altCurCol
				s.savedRow, s.savedCol = s.altSavedRow, s.altSavedCol
				s.fg, s.bg, s.bold = s.altFG, s.altBG, s.altBold
				s.altActive = false
				s.altCells = nil
			}
		}
		// All other modes ignored.
	}
}

func (s *Screen) handleSGR(nums []int) {
	if len(nums) == 0 {
		s.fg, s.bg, s.bold = ColorDefault, ColorDefault, false
		return
	}
	i := 0
	for i < len(nums) {
		n := nums[i]
		switch {
		case n == 0:
			s.fg, s.bg, s.bold = ColorDefault, ColorDefault, false
		case n == 1:
			s.bold = true
		case n == 22:
			s.bold = false
		case n >= 30 && n <= 37:
			s.fg = Color(n - 30)
		case n == 38:
			if i+2 < len(nums) && nums[i+1] == 5 {
				s.fg = Color(nums[i+2])
				i += 2
			}
		case n == 39:
			s.fg = ColorDefault
		case n >= 40 && n <= 47:
			s.bg = Color(n - 40)
		case n == 48:
			if i+2 < len(nums) && nums[i+1] == 5 {
				s.bg = Color(nums[i+2])
				i += 2
			}
		case n == 49:
			s.bg = ColorDefault
		case n >= 90 && n <= 97:
			s.fg = Color(n - 90 + 8)
		case n >= 100 && n <= 107:
			s.bg = Color(n - 100 + 8)
		}
		i++
	}
}

// Snapshot copies the screen state for rendering.
// When scrollTop > 0, the returned cells show historical lines composited with the live screen.
// curRow/curCol are set to -1/-1 when the cursor is scrolled out of view.
func (s *Screen) Snapshot() (cells []Cell, cols, rows, curRow, curCol int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cols, rows = s.cols, s.rows
	out := make([]Cell, rows*cols)
	blank := Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}

	if s.scrollTop == 0 {
		// Live view — fast path.
		copy(out, s.cells)
		if s.CursorHidden {
			return out, cols, rows, -1, -1
		}
		return out, cols, rows, s.curRow, s.curCol
	}

	// Composite: fill rows from scrollback and live screen.
	// scrollTop lines from scrollback (newest first), then live rows.
	sbLen := s.sb.Len()
	for row := 0; row < rows; row++ {
		// How many lines back from the bottom of the scrollback is this row?
		// Row 0 of the visible window maps to scrollback index (sbLen - scrollTop + row).
		sbIdx := sbLen - s.scrollTop + row
		dstStart := row * cols
		if sbIdx >= 0 && sbIdx < sbLen {
			srcLine := s.sb.line(sbIdx)
			n := copy(out[dstStart:dstStart+cols], srcLine)
			// Pad short lines.
			for i := dstStart + n; i < dstStart+cols; i++ {
				out[i] = blank
			}
		} else if sbIdx >= sbLen {
			// This row is in the live screen.
			liveRow := sbIdx - sbLen
			if liveRow < rows {
				copy(out[dstStart:dstStart+cols], s.cells[liveRow*cols:(liveRow+1)*cols])
			}
		} else {
			// Before the start of scrollback — blank.
			for i := dstStart; i < dstStart+cols; i++ {
				out[i] = blank
			}
		}
	}

	// Cursor is not visible when scrolled back.
	return out, cols, rows, -1, -1
}

// ScrollBy scrolls the view by n lines (positive = scroll up into history, negative = scroll down).
// Clamped to [0, scrollback length].
// Resets to live view automatically when reaching the bottom (scrollTop=0).
func (s *Screen) ScrollBy(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrollTop += n
	if s.scrollTop < 0 {
		s.scrollTop = 0
	}
	if s.scrollTop > s.sb.Len() {
		s.scrollTop = s.sb.Len()
	}
}

// ScrollReset snaps back to the live terminal view (scrollTop=0).
func (s *Screen) ScrollReset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scrollTop = 0
}

// ScrollLines returns the current scroll offset (0 = live view).
func (s *Screen) ScrollLines() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scrollTop
}

// ScrollbackLen returns the number of lines saved in the scrollback buffer.
func (s *Screen) ScrollbackLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sb.Len()
}

func parseNums(s string) []int {
	if s == "" {
		return nil
	}
	var nums []int
	cur := 0
	hasDigit := false
	for _, c := range s {
		if c >= '0' && c <= '9' {
			cur = cur*10 + int(c-'0')
			hasDigit = true
		} else if c == ';' {
			nums = append(nums, cur)
			cur = 0
			hasDigit = false
		}
	}
	if hasDigit || len(s) > 0 {
		nums = append(nums, cur)
	}
	return nums
}

func numOr1(nums []int, idx int) int {
	if idx >= len(nums) || nums[idx] == 0 {
		return 1
	}
	return nums[idx]
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

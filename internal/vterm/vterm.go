// Package vterm implements a minimal VT100 screen state machine.
// It maintains a fixed grid of cells and processes PTY output bytes.
package vterm

import "sync"

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
	// escape buffer
	esc []byte
}

// New creates a Screen of the given dimensions.
func New(cols, rows int) *Screen {
	s := &Screen{cols: cols, rows: rows, fg: ColorDefault, bg: ColorDefault}
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
func (s *Screen) Write(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range data {
		if len(s.esc) > 0 {
			s.esc = append(s.esc, b)
			s.handleEsc()
			continue
		}
		switch b {
		case 0x1b: // ESC
			s.esc = []byte{0x1b}
		case '\r':
			s.curCol = 0
		case '\n':
			s.curRow++
			if s.curRow >= s.rows {
				s.scrollUp(1)
				s.curRow = s.rows - 1
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
	}
}

func (s *Screen) putChar(ch rune) {
	if s.curCol >= s.cols {
		s.curCol = 0
		s.curRow++
		if s.curRow >= s.rows {
			s.scrollUp(1)
			s.curRow = s.rows - 1
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
	if n >= s.rows {
		s.clear(0, s.cols*s.rows, s.cols*s.rows)
		return
	}
	copy(s.cells, s.cells[n*s.cols:])
	blank := Cell{Ch: ' ', FG: ColorDefault, BG: ColorDefault}
	start := (s.rows - n) * s.cols
	for i := start; i < s.rows*s.cols; i++ {
		s.cells[i] = blank
	}
}

// handleEsc processes escape sequences.
// Sequences supported: CSI cursor movement, SGR colors, clear screen/line, save/restore cursor.
func (s *Screen) handleEsc() {
	e := s.esc
	if len(e) < 2 {
		return
	}
	if e[1] != '[' && e[1] != '7' && e[1] != '8' && e[1] != 'c' {
		// Unknown ESC byte — consume and reset if it's a terminator.
		if e[1] >= 0x40 && e[1] <= 0x7e {
			s.esc = nil
		}
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
	case 'r': // set scrolling region — stub (accept but don't implement)
	case 'h', 'l': // mode set/reset — ignore (handles ?1049h etc)
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

// Snapshot copies the screen state for rendering. Caller must not modify the returned slice.
func (s *Screen) Snapshot() (cells []Cell, cols, rows, curRow, curCol int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Cell, len(s.cells))
	copy(out, s.cells)
	return out, s.cols, s.rows, s.curRow, s.curCol
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

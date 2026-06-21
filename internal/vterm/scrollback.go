package vterm

// MaxScrollback is the maximum number of lines kept in the scrollback buffer.
const MaxScrollback = 10_000

// scrollbackBuf is a fixed-capacity ring buffer of saved terminal lines.
type scrollbackBuf struct {
	lines [][]Cell // ring; len=capacity
	head  int      // index of oldest line
	count int      // number of valid lines (≤ cap)
}

func newScrollbackBuf(cap int) *scrollbackBuf {
	return &scrollbackBuf{lines: make([][]Cell, cap)}
}

// push saves a copy of the line slice into the ring buffer.
func (sb *scrollbackBuf) push(line []Cell) {
	cp := make([]Cell, len(line))
	copy(cp, line)
	idx := (sb.head + sb.count) % len(sb.lines)
	if sb.count < len(sb.lines) {
		sb.lines[idx] = cp
		sb.count++
	} else {
		// Ring is full — overwrite oldest.
		sb.lines[sb.head] = cp
		sb.head = (sb.head + 1) % len(sb.lines)
	}
}

// line returns the i-th line from oldest (i=0) to newest (i=count-1).
func (sb *scrollbackBuf) line(i int) []Cell {
	if i < 0 || i >= sb.count {
		return nil
	}
	return sb.lines[(sb.head+i)%len(sb.lines)]
}

// Len returns the number of lines stored.
func (sb *scrollbackBuf) Len() int { return sb.count }

// The Screen embeds scrollback state. scrollTop is the number of lines the
// view is scrolled back from the current display position.
// scrollTop=0 means the live screen is visible.
// scrollTop=N means we are looking N lines into the past.


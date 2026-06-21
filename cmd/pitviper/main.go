//go:build linux && cgo

// PITVIPER — SDL2 terminal emulator with Emily Prime integration.
// Milestone 0+1: PTY shell inside an SDL2 window, 8×13 monochrome glyph atlas.
//
// Build:
//   sudo apt install libsdl2-dev
//   CGO_ENABLED=1 go build ./cmd/pitviper
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync/atomic"
	"time"
	"github.com/veandco/go-sdl2/sdl"

	"pitviper/internal/font"
	"pitviper/internal/pty"
	"pitviper/internal/vterm"
)

const version = "0.1.0-milestone1"

// Default terminal dimensions in characters.
const (
	defaultCols = 220
	defaultRows = 50
)

// Color table: xterm-256 base 8 (low-intensity) + 8 (high-intensity).
// Remaining 240 are synthesized on demand in sdlColor().
var baseColors = [16]sdl.Color{
	{R: 0x00, G: 0x00, B: 0x00, A: 0xff}, // 0 black
	{R: 0xcc, G: 0x00, B: 0x00, A: 0xff}, // 1 red
	{R: 0x4e, G: 0x9a, B: 0x06, A: 0xff}, // 2 green
	{R: 0xc4, G: 0xa0, B: 0x00, A: 0xff}, // 3 yellow
	{R: 0x34, G: 0x65, B: 0xa4, A: 0xff}, // 4 blue
	{R: 0x75, G: 0x50, B: 0x7b, A: 0xff}, // 5 magenta
	{R: 0x06, G: 0x98, B: 0x9a, A: 0xff}, // 6 cyan
	{R: 0xd3, G: 0xd7, B: 0xcf, A: 0xff}, // 7 white
	{R: 0x55, G: 0x57, B: 0x53, A: 0xff}, // 8 bright black
	{R: 0xef, G: 0x29, B: 0x29, A: 0xff}, // 9 bright red
	{R: 0x8a, G: 0xe2, B: 0x34, A: 0xff}, // 10 bright green
	{R: 0xfc, G: 0xe9, B: 0x4f, A: 0xff}, // 11 bright yellow
	{R: 0x72, G: 0x9f, B: 0xcf, A: 0xff}, // 12 bright blue
	{R: 0xad, G: 0x7f, B: 0xa8, A: 0xff}, // 13 bright magenta
	{R: 0x34, G: 0xe2, B: 0xe2, A: 0xff}, // 14 bright cyan
	{R: 0xee, G: 0xee, B: 0xec, A: 0xff}, // 15 bright white
}

var defaultFG = sdl.Color{R: 0xdd, G: 0xdd, B: 0xdd, A: 0xff}
var defaultBG = sdl.Color{R: 0x00, G: 0x00, B: 0x00, A: 0xff}

func sdlColor(c vterm.Color, isBold bool, isBackground bool) sdl.Color {
	if c == vterm.ColorDefault {
		if isBackground {
			return defaultBG
		}
		if isBold {
			return baseColors[15]
		}
		return defaultFG
	}
	if int(c) < 16 {
		if isBold && !isBackground && int(c) < 8 {
			return baseColors[c+8]
		}
		return baseColors[c]
	}
	if int(c) < 232 {
		// 6×6×6 cube
		n := int(c) - 16
		b := n % 6
		g := (n / 6) % 6
		r := n / 36
		toV := func(i int) uint8 {
			if i == 0 {
				return 0
			}
			return uint8(55 + i*40)
		}
		return sdl.Color{R: toV(r), G: toV(g), B: toV(b), A: 0xff}
	}
	// 24 grayscale ramp
	v := uint8(8 + (int(c)-232)*10)
	return sdl.Color{R: v, G: v, B: v, A: 0xff}
}

func main() {
	runtime.LockOSThread()

	ver := flag.Bool("version", false, "print version and exit")
	shellFlag := flag.String("shell", "", "shell to launch (default: $SHELL or /bin/bash)")
	flag.Parse()

	if *ver {
		fmt.Println("pitviper", version)
		os.Exit(0)
	}

	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_EVENTS); err != nil {
		fmt.Fprintln(os.Stderr, "SDL2 init:", err)
		os.Exit(1)
	}
	defer sdl.Quit()

	winW := int32(defaultCols * font.GlyphW)
	winH := int32(defaultRows * font.GlyphH)

	win, err := sdl.CreateWindow(
		"PITVIPER",
		sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		winW, winH,
		sdl.WINDOW_SHOWN|sdl.WINDOW_RESIZABLE,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create window:", err)
		os.Exit(1)
	}
	defer win.Destroy()

	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_SOFTWARE)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create renderer:", err)
		os.Exit(1)
	}
	defer ren.Destroy()

	cols, rows := defaultCols, defaultRows
	screen := vterm.New(cols, rows)

	terminal, err := pty.Open(*shellFlag, cols, rows)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open pty:", err)
		os.Exit(1)
	}
	defer terminal.Close()

	var running atomic.Bool
	running.Store(true)

	// Read PTY master → write to vterm.
	go func() {
		buf := make([]byte, 4096)
		for running.Load() {
			n, err := terminal.Master.Read(buf)
			if n > 0 {
				screen.Write(buf[:n])
			}
			if err != nil {
				if err != io.EOF {
					fmt.Fprintln(os.Stderr, "pty read:", err)
				}
				running.Store(false)
				return
			}
		}
	}()

	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	for running.Load() {
		// Drain SDL events.
		for {
			ev := sdl.PollEvent()
			if ev == nil {
				break
			}
			switch e := ev.(type) {
			case *sdl.QuitEvent:
				running.Store(false)

			case *sdl.WindowEvent:
				if e.Event == sdl.WINDOWEVENT_RESIZED {
					w, h := win.GetSize()
					newCols := int(w) / font.GlyphW
					newRows := int(h) / font.GlyphH
					if newCols < 1 {
						newCols = 1
					}
					if newRows < 1 {
						newRows = 1
					}
					if newCols != cols || newRows != rows {
						cols, rows = newCols, newRows
						screen.Resize(cols, rows)
						_ = terminal.Resize(cols, rows)
					}
				}

			case *sdl.KeyboardEvent:
				if e.Type == sdl.KEYDOWN {
					if scrollHandled := handleScrollKey(screen, e); !scrollHandled {
						writeKey(terminal.Master, e)
					}
				}

			case *sdl.TextInputEvent:
				// Snap back to live view when user types.
				if screen.ScrollLines() > 0 {
					screen.ScrollReset()
				}
				text := e.GetText()
				_, _ = terminal.Master.WriteString(text)
			}
		}

		// Wait for next frame tick.
		select {
		case <-ticker.C:
		default:
		}

		// Update window title from OSC sequences.
		if t := screen.GetTitle(); t != "" {
			win.SetTitle("PITVIPER — " + t)
		}

		renderFrame(ren, screen)
	}
}

// renderFrame draws the current screen state onto the SDL2 renderer.
func renderFrame(ren *sdl.Renderer, screen *vterm.Screen) {
	cells, cols, rows, curRow, curCol := screen.Snapshot()

	// Fill background.
	_ = ren.SetDrawColor(defaultBG.R, defaultBG.G, defaultBG.B, 0xff)
	_ = ren.Clear()

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			cell := cells[row*cols+col]
			if cell.Ch == 0 {
				cell.Ch = ' '
			}

			fg := sdlColor(cell.FG, cell.Bold, false)
			bg := sdlColor(cell.BG, cell.Bold, true)

			// Cursor: invert colors.
			if row == curRow && col == curCol {
				fg, bg = bg, fg
			}

			px := int32(col * font.GlyphW)
			py := int32(row * font.GlyphH)

			// Draw background.
			_ = ren.SetDrawColor(bg.R, bg.G, bg.B, bg.A)
			_ = ren.FillRect(&sdl.Rect{X: px, Y: py, W: int32(font.GlyphW), H: int32(font.GlyphH)})

			// Draw foreground glyph bitmap.
			bits := font.GlyphBits(cell.Ch)
			_ = ren.SetDrawColor(fg.R, fg.G, fg.B, fg.A)
			for y := 0; y < font.GlyphH; y++ {
				for x := 0; x < font.GlyphW; x++ {
					if bits[y*font.GlyphW+x] != 0 {
						_ = ren.DrawPoint(px+int32(x), py+int32(y))
					}
				}
			}
		}
	}

	ren.Present()
}

// handleScrollKey handles Shift+PageUp/Down for scrollback. Returns true if consumed.
func handleScrollKey(screen *vterm.Screen, e *sdl.KeyboardEvent) bool {
	shift := e.Keysym.Mod&sdl.KMOD_SHIFT != 0
	if !shift {
		return false
	}
	switch e.Keysym.Sym {
	case sdl.K_PAGEUP:
		screen.ScrollBy(defaultRows / 2)
		return true
	case sdl.K_PAGEDOWN:
		screen.ScrollBy(-(defaultRows / 2))
		return true
	case sdl.K_HOME:
		screen.ScrollBy(vterm.MaxScrollback)
		return true
	case sdl.K_END:
		screen.ScrollReset()
		return true
	}
	// Any non-scroll keypress snaps back to live view.
	if screen.ScrollLines() > 0 {
		screen.ScrollReset()
	}
	return false
}

// writeKey translates SDL keyboard events into PTY input bytes.
func writeKey(w io.Writer, e *sdl.KeyboardEvent) {
	mod := e.Keysym.Mod
	sym := e.Keysym.Sym

	ctrl := mod&sdl.KMOD_CTRL != 0

	switch sym {
	case sdl.K_RETURN, sdl.K_KP_ENTER:
		_, _ = w.Write([]byte{'\r'})
	case sdl.K_BACKSPACE:
		_, _ = w.Write([]byte{0x7f})
	case sdl.K_TAB:
		_, _ = w.Write([]byte{'\t'})
	case sdl.K_ESCAPE:
		_, _ = w.Write([]byte{0x1b})
	case sdl.K_UP:
		_, _ = w.Write([]byte{0x1b, '[', 'A'})
	case sdl.K_DOWN:
		_, _ = w.Write([]byte{0x1b, '[', 'B'})
	case sdl.K_RIGHT:
		_, _ = w.Write([]byte{0x1b, '[', 'C'})
	case sdl.K_LEFT:
		_, _ = w.Write([]byte{0x1b, '[', 'D'})
	case sdl.K_HOME:
		_, _ = w.Write([]byte{0x1b, '[', 'H'})
	case sdl.K_END:
		_, _ = w.Write([]byte{0x1b, '[', 'F'})
	case sdl.K_PAGEUP:
		_, _ = w.Write([]byte{0x1b, '[', '5', '~'})
	case sdl.K_PAGEDOWN:
		_, _ = w.Write([]byte{0x1b, '[', '6', '~'})
	case sdl.K_DELETE:
		_, _ = w.Write([]byte{0x1b, '[', '3', '~'})
	default:
		if ctrl && sym >= sdl.K_a && sym <= sdl.K_z {
			// Ctrl+A = 0x01, ..., Ctrl+Z = 0x1a
			_, _ = w.Write([]byte{byte(sym - sdl.K_a + 1)})
		}
		// Printable keys are handled by TextInput events.
	}
}

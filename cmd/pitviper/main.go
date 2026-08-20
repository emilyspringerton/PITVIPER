//go:build (linux || windows) && cgo

// PITVIPER — SDL2 terminal emulator with Emily Prime integration.
// Milestone 0+1: PTY shell inside an SDL2 window, 8×13 monochrome glyph atlas.
//
// Build (Linux):
//
//	sudo apt install libsdl2-dev
//	CGO_ENABLED=1 go build ./cmd/pitviper
//
// Build (Windows, via MSYS2/MinGW64 — see .github/workflows/ci.yml's build_windows
// job for the exact toolchain: mingw-w64-x86_64-{gcc,go,SDL2,SDL2_image,SDL2_ttf}):
//
//	CGO_ENABLED=1 go build -o pitviper.exe ./cmd/pitviper
//
// The rest of this file is platform-agnostic (pure SDL2 + the io.Reader/Writer
// abstraction over internal/pty, which has its own per-OS build-tagged files) —
// only internal/pty needed a real Windows-specific implementation (ConPTY).
package main

import (
	"flag"
	"fmt"
	"github.com/veandco/go-sdl2/sdl"
	"io"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"pitviper/internal/font"
	"pitviper/internal/gfdapi"
	"pitviper/internal/mudconn"
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

// GFD Channel 11 color scheme — deep black bg, gold accent, freq cyan.
var gfdPalette = struct {
	bg     sdl.Color
	fg     sdl.Color
	accent sdl.Color // Channel 11 gold #e8c842
	freq   sdl.Color // The Frequency cyan #4ad1d1
	bloc   sdl.Color // The Bloc red #d14a4a
	muted  sdl.Color
	barBG  sdl.Color
}{
	bg:     sdl.Color{R: 0x0a, G: 0x0a, B: 0x0a, A: 0xff},
	fg:     sdl.Color{R: 0xd4, G: 0xd4, B: 0xd4, A: 0xff},
	accent: sdl.Color{R: 0xe8, G: 0xc8, B: 0x42, A: 0xff},
	freq:   sdl.Color{R: 0x4a, G: 0xd1, B: 0xd1, A: 0xff},
	bloc:   sdl.Color{R: 0xd1, G: 0x4a, B: 0x4a, A: 0xff},
	muted:  sdl.Color{R: 0x44, G: 0x44, B: 0x44, A: 0xff},
	barBG:  sdl.Color{R: 0x12, G: 0x12, B: 0x12, A: 0xff},
}

var defaultFG = sdl.Color{R: 0xdd, G: 0xdd, B: 0xdd, A: 0xff}
var defaultBG = sdl.Color{R: 0x00, G: 0x00, B: 0x00, A: 0xff}

// gfdMode is set to true when --gfd flag is active.
var gfdMode bool
var gfdWebmaster bool

// gfdClient is the live GFD API state poller (webmaster mode only).
var gfdClient *gfdapi.Client

// districtPaneOpen tracks whether Ctrl+D district overlay is visible (S127-05).
var districtPaneOpen bool

// glyphAtlasTex holds every known monochrome glyph (font.KnownGlyphs())
// pre-rendered once into a single GPU texture, real GPU offload per
// founder's own real-time ask: "can we do more to unload rendering for
// pitviper onto the gpu?" -> "like as soon as the terminal clears it
// should like instantly... it should be very fast." Before this,
// renderFrame's own per-cell loop issued up to font.GlyphW*font.GlyphH
// (104) individual ren.DrawPoint calls per cell -- at defaultCols x
// defaultRows (220x50 = 11,000 cells) that's up to ~1.1M individual
// draw calls in the worst case, every single frame. With the atlas,
// each cell becomes exactly one ren.Copy (a single GPU blit), colored
// via SetColorMod per cell (the atlas itself is rendered in white, so
// one shared texture serves every foreground color) -- real GPU
// offload, not just RENDERER_ACCELERATED, applied to this repo's own
// actual bottleneck.
var (
	glyphAtlasTex  *sdl.Texture
	glyphAtlasSlot = map[rune]int32{}
)

func buildGlyphAtlas(ren *sdl.Renderer) error {
	glyphs := font.KnownGlyphs()
	atlasW := int32(len(glyphs) * font.GlyphW)
	tex, err := ren.CreateTexture(sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_TARGET, atlasW, font.GlyphH)
	if err != nil {
		return fmt.Errorf("create atlas texture: %w", err)
	}
	if err := tex.SetBlendMode(sdl.BLENDMODE_BLEND); err != nil {
		tex.Destroy()
		return fmt.Errorf("set atlas blend mode: %w", err)
	}

	prevTarget := ren.GetRenderTarget()
	if err := ren.SetRenderTarget(tex); err != nil {
		tex.Destroy()
		return fmt.Errorf("set render target to atlas: %w", err)
	}
	_ = ren.SetDrawColor(0, 0, 0, 0)
	_ = ren.Clear()
	_ = ren.SetDrawColor(255, 255, 255, 255)
	for i, ch := range glyphs {
		bits := font.GlyphBits(ch)
		slotX := int32(i * font.GlyphW)
		glyphAtlasSlot[ch] = slotX
		for y := 0; y < font.GlyphH; y++ {
			for x := 0; x < font.GlyphW; x++ {
				if bits[y*font.GlyphW+x] != 0 {
					_ = ren.DrawPoint(slotX+int32(x), int32(y))
				}
			}
		}
	}
	_ = ren.SetRenderTarget(prevTarget)

	glyphAtlasTex = tex
	return nil
}

// emojiTextureCache caches one *sdl.Texture per rendered emoji codepoint
// -- font.EmojiSurface itself already caches the underlying *sdl.Surface,
// but converting a surface to a texture is a real, separate per-renderer
// GPU upload that's worth caching here too rather than redoing every
// frame a given emoji is on screen.
var emojiTextureCache = map[rune]*sdl.Texture{}

func emojiTexture(ren *sdl.Renderer, ch rune) *sdl.Texture {
	if tex, ok := emojiTextureCache[ch]; ok {
		return tex
	}
	surf := font.EmojiSurface(ch)
	if surf == nil {
		emojiTextureCache[ch] = nil
		return nil
	}
	tex, err := ren.CreateTextureFromSurface(surf)
	if err != nil {
		emojiTextureCache[ch] = nil
		return nil
	}
	emojiTextureCache[ch] = tex
	return tex
}

// zoomScale is a Photoshop-inspired QoL addition (founder, real-time: "give
// pitviper quality of life improvements inspired by photoshop keybindings"
// -> "like for zoom and stuff" -> "i guess a key combo with scroll to zoom
// the terminal text size"). Applied via SDL's own renderer scale
// (ren.SetScale) rather than recomputing every font.GlyphW/GlyphH call
// site by hand — the whole frame (cell grid, status bar, GFD pane, logo)
// scales together for free. Clamped to [zoomMin, zoomMax] the same way
// Photoshop clamps its own zoom percentage.
var zoomScale float32 = 1.0

const (
	zoomMin  float32 = 0.5
	zoomMax  float32 = 3.0
	zoomStep float32 = 0.1
)

// useShinyFont is the F11 toggle's own state (founder: "keep the og font
// for now" -> "and the toggle switches to the new shiny font") -- off by
// default, the OG built-in atlas is what ships on first launch.
var useShinyFont bool

// shinyTextureCache mirrors emojiTextureCache's own real reasoning: font.
// ShinyGlyphSurface already caches the rendered *sdl.Surface, this caches
// the separate, real, per-renderer GPU texture upload on top of that.
var shinyTextureCache = map[rune]*sdl.Texture{}

func shinyTexture(ren *sdl.Renderer, ch rune) *sdl.Texture {
	if tex, ok := shinyTextureCache[ch]; ok {
		return tex
	}
	surf := font.ShinyGlyphSurface(ch)
	if surf == nil {
		shinyTextureCache[ch] = nil
		return nil
	}
	tex, err := ren.CreateTextureFromSurface(surf)
	if err != nil {
		shinyTextureCache[ch] = nil
		return nil
	}
	shinyTextureCache[ch] = tex
	return tex
}

func adjustZoom(delta float32) {
	zoomScale += delta
	if zoomScale < zoomMin {
		zoomScale = zoomMin
	}
	if zoomScale > zoomMax {
		zoomScale = zoomMax
	}
}

// selection tracks a mouse-drag text selection (S187-02). PITVIPER keeps its
// own "primary selection" buffer (lastSelected) rather than relying on the
// OS: X11 primary-selection is Linux-only and has no Windows equivalent, so
// middle-click paste here always pastes PITVIPER's own last selection —
// works identically on both platforms, at the cost of not sharing selection
// with other apps' primary-selection buffer the way native X11 apps do.
// Ctrl+Shift+C/V (not plain Ctrl+C/V) so copy/paste never collides with the
// PTY's own Ctrl+C interrupt / Ctrl+V paste-literal-next-char.
var selection struct {
	active             bool // left button currently held, drag in progress
	startRow, startCol int
	endRow, endCol     int
	haveSelection      bool // true once a drag has produced a non-empty range
}
var lastSelected string

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

// gfdBarRows is the number of character rows reserved for the Channel 11 status bar.
const gfdBarRows = 1

func main() {
	runtime.LockOSThread()

	ver := flag.Bool("version", false, "print version and exit")
	shellFlag := flag.String("shell", "", "shell to launch (default: $SHELL or /bin/bash)")
	gfdFlag := flag.String("gfd", "", "connect to GFD MUD at host:port (e.g. localhost:2323)")
	wmFlag := flag.Bool("gfd-webmaster", false, "webmaster mode — elevated display in GFD client")
	flag.Parse()

	if *ver {
		fmt.Println("pitviper", version)
		os.Exit(0)
	}

	gfdMode = *gfdFlag != ""
	gfdWebmaster = *wmFlag

	// Apply GFD color scheme when in MUD mode.
	if gfdMode {
		defaultBG = gfdPalette.bg
		defaultFG = gfdPalette.fg
	}

	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_EVENTS); err != nil {
		fmt.Fprintln(os.Stderr, "SDL2 init:", err)
		os.Exit(1)
	}
	defer sdl.Quit()

	// Real color emoji (founder: "build all emojis into pitviper") --
	// not fatal if it fails (SDL2_ttf or the emoji font not installed
	// yet, see sudo-queue/19-pitviper-freetype-emoji-fonts.sh): PITVIPER
	// already works without color glyphs (the '?' fallback covers the
	// same case it always has), so this degrades instead of crashing,
	// same choice already made elsewhere (e.g. gfdClient being nil
	// outside --gfd mode).
	if err := font.InitEmoji(); err != nil {
		fmt.Fprintln(os.Stderr, "pitviper: color emoji unavailable:", err)
	}

	// F11 "shiny font" toggle (founder: "add on a toggle like f11 or
	// f12" -> "keep the og font for now" -> "and the toggle switches to
	// the new shiny font"). Same degrade-don't-crash choice as InitEmoji
	// -- F11 just reports "not available" until this succeeds.
	if err := font.InitShinyFont(font.GlyphH); err != nil {
		fmt.Fprintln(os.Stderr, "pitviper: shiny font (JetBrains Mono) unavailable:", err)
	}

	winW := int32(defaultCols * font.GlyphW)
	extraH := 0
	if gfdMode {
		extraH = gfdBarRows * font.GlyphH
	}
	winH := int32(defaultRows*font.GlyphH) + int32(extraH)

	winTitle := "PITVIPER"
	if gfdMode {
		winTitle = "GoblinFoxDragon — Channel 11"
		if gfdWebmaster {
			winTitle += " [WEBMASTER]"
		}
	}

	win, err := sdl.CreateWindow(
		winTitle,
		sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		winW, winH,
		sdl.WINDOW_SHOWN|sdl.WINDOW_RESIZABLE,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create window:", err)
		os.Exit(1)
	}
	defer win.Destroy()

	// Founder real-time: "iits just really slow" (also reported as "weird"
	// display, likely tearing from this). RENDERER_SOFTWARE does every
	// glyph's per-pixel FillRect on the CPU with no vsync -- at
	// defaultCols x defaultRows (220x50 = 11,000 cells, up to 8x13=104
	// FillRect calls each) that's up to ~1.1M draw calls/frame in the
	// worst case. RENDERER_ACCELERATED hands the same FillRect calls to
	// the GPU; PRESENTVSYNC caps/syncs Present() to the display refresh
	// instead of racing the 60Hz ticker uncapped, which is the other real
	// source of visible tearing/"weird" frames.
	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create renderer:", err)
		os.Exit(1)
	}
	defer ren.Destroy()

	if err := buildGlyphAtlas(ren); err != nil {
		fmt.Fprintln(os.Stderr, "pitviper: glyph atlas build failed, falling back to per-pixel "+
			"rendering:", err)
	}
	defer func() {
		if glyphAtlasTex != nil {
			glyphAtlasTex.Destroy()
		}
	}()

	cols, rows := defaultCols, defaultRows
	screen := vterm.New(cols, rows)

	// ── Connection: TCP MUD or PTY shell ──────────────────────────────────────

	// ioReader / ioWriter are the abstract read/write ends of the connection.
	// In PTY mode: pty.Terminal.Master. In GFD mode: mudconn.Conn.Master.
	var (
		ioReader   io.Reader
		ioWriter   io.Writer
		connClose  func() error
		connResize func(cols, rows int) error
	)

	if gfdMode {
		addr := *gfdFlag
		if !strings.Contains(addr, ":") {
			addr = addr + ":2323"
		}
		mc, err := mudconn.Dial(addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GFD connect %s: %v\n", addr, err)
			os.Exit(1)
		}
		ioReader = mc.Master
		ioWriter = mc.Master
		connClose = mc.Close
		connResize = mc.Resize
		fmt.Printf("Connected to GFD MUD at %s\n", addr)
	} else {
		terminal, err := pty.Open(*shellFlag, cols, rows)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open pty:", err)
			os.Exit(1)
		}
		ioReader = terminal.Master
		ioWriter = terminal.Master
		// PTY.Close() returns nothing; connClose's shared type (func() error) matches
		// mudconn.Conn.Close's real signature instead -- wrap here rather than change
		// PTY's own public API for a single caller.
		connClose = func() error { terminal.Close(); return nil }
		connResize = terminal.Resize
		_ = connResize // suppress unused warning
		defer terminal.Close()
	}
	defer connClose()

	// Start GFD API state poller in webmaster mode.
	if gfdMode && gfdWebmaster {
		gfdClient = gfdapi.NewClient(gfdapi.Config{
			IDUNABase: "http://localhost:8080",
			MUDAddr:   *gfdFlag,
			Token:     os.Getenv("GFD_TOKEN"),
		})
		gfdClient.Start()
		defer gfdClient.Stop()
	}

	var running atomic.Bool
	running.Store(true)

	// Auto-login state (S127-01): track whether credentials have been sent.
	gfdUser := os.Getenv("GFD_USER")
	gfdPass := os.Getenv("GFD_PASS")
	var loginState struct {
		nameSent bool
		passSent bool
	}

	// Read from connection (PTY or TCP) → write to vterm.
	go func() {
		// Founder, real-time, real crash report: "when i run the crystal.go
		// program or the main.go of the crystal app it like the connection
		// in pitviper isnt cleaned up right the program crashes when you
		// try to open it with tmux open with the crystal running." An
		// unrecovered panic in ANY goroutine takes down the whole Go
		// process, not just that goroutine -- this loop feeds screen.Write
		// every byte a real subprocess (tmux, and whatever's running
		// inside it) produces, with zero defensive boundary. Whatever the
		// exact triggering byte sequence turns out to be, a malformed/
		// unexpected escape sequence should degrade this one connection,
		// not crash the whole terminal -- real defensive practice for a
		// loop processing an untrusted, complex byte stream, not a
		// band-aid over one specific bug.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintln(os.Stderr, "pitviper: recovered from a panic in the read loop "+
					"(this connection is now closed, but pitviper itself did not crash):", r)
				running.Store(false)
			}
		}()
		buf := make([]byte, 4096)
		for running.Load() {
			n, err := ioReader.Read(buf)
			if n > 0 {
				screen.Write(buf[:n])

				// S127-01: auto-login when GFD_USER + GFD_PASS are set.
				if gfdMode && gfdUser != "" && gfdPass != "" {
					chunk := string(buf[:n])
					if !loginState.nameSent && strings.Contains(chunk, "Enter your name:") {
						ioWriter.Write([]byte(gfdUser + "\r\n"))
						loginState.nameSent = true
					} else if loginState.nameSent && !loginState.passSent &&
						(strings.Contains(chunk, "assword:") || strings.Contains(chunk, "Enter your password:")) {
						ioWriter.Write([]byte(gfdPass + "\r\n"))
						loginState.passSent = true
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					label := "pty"
					if gfdMode {
						label = "gfd"
					}
					fmt.Fprintln(os.Stderr, label+" read:", err)
				}
				running.Store(false)
				return
			}
		}
	}()

	// S127-02: Channel 11 splash screen — show for 2s before MUD MOTD.
	if gfdMode {
		renderChannel11Splash(ren, win, 2*time.Second)
	}

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
						if connResize != nil {
							_ = connResize(cols, rows)
						}
					}
				}

			case *sdl.KeyboardEvent:
				if e.Type == sdl.KEYDOWN {
					// S127-05: Ctrl+D toggles the district overlay pane.
					if gfdMode && e.Keysym.Sym == sdl.K_d &&
						(e.Keysym.Mod&sdl.KMOD_CTRL) != 0 {
						districtPaneOpen = !districtPaneOpen
					} else if (e.Keysym.Mod&sdl.KMOD_CTRL) != 0 &&
						(e.Keysym.Sym == sdl.K_EQUALS || e.Keysym.Sym == sdl.K_KP_PLUS) {
						// Ctrl+= (the unshifted key '+' lives on): zoom in, Photoshop's
						// own Ctrl/Cmd+'+' binding.
						adjustZoom(zoomStep)
					} else if (e.Keysym.Mod&sdl.KMOD_CTRL) != 0 &&
						(e.Keysym.Sym == sdl.K_MINUS || e.Keysym.Sym == sdl.K_KP_MINUS) {
						// Ctrl+-: zoom out, Photoshop's own Ctrl/Cmd+'-'.
						adjustZoom(-zoomStep)
					} else if (e.Keysym.Mod&sdl.KMOD_CTRL) != 0 && e.Keysym.Sym == sdl.K_0 {
						// Ctrl+0: reset zoom, Photoshop's own "Fit on Screen"/100% reset.
						zoomScale = 1.0
					} else if e.Keysym.Sym == sdl.K_F11 {
						// F11: toggle the "shiny font" (real JetBrains Mono, anti-aliased,
						// via SDL2_ttf) on top of the original built-in bitmap atlas.
						// Founder: "can you please find the nicest monospace public domain
						// font you can and add it on a toggle like f11 or f12" -> "keep the
						// og font for now" -> "and the toggle switches to the new shiny
						// font." A no-op (with a one-line stderr note) if the font/SDL2_ttf
						// isn't installed yet -- the OG atlas keeps working regardless.
						if font.ShinyFontAvailable() {
							useShinyFont = !useShinyFont
						} else {
							fmt.Fprintln(os.Stderr, "pitviper: shiny font not available "+
								"(SDL2_ttf/JetBrains Mono not installed yet)")
						}
					} else if !gfdMode && e.Keysym.Sym == sdl.K_i &&
						(e.Keysym.Mod&sdl.KMOD_CTRL) != 0 && (e.Keysym.Mod&sdl.KMOD_ALT) != 0 {
						// Founder real-time ask (2026-08-13): a hotkey that SSHes into
						// iduna.farthq.com using the local ~/.ssh profile. Ctrl+Alt+I
						// (not a plain Ctrl+letter) so it doesn't collide with any
						// standard shell control character. Same technique S127-01's
						// GFD auto-login already uses -- write the command into the
						// live PTY as if typed, so `ssh` picks up the user's normal
						// ~/.ssh/config, keys, and known_hosts with no special-casing.
						ioWriter.Write([]byte("ssh iduna.farthq.com\r\n"))
					} else if (e.Keysym.Mod&sdl.KMOD_CTRL) != 0 && (e.Keysym.Mod&sdl.KMOD_SHIFT) != 0 &&
						e.Keysym.Sym == sdl.K_c {
						// Ctrl+Shift+C: explicit copy-current-selection, redundant with
						// copy-on-select-release below but a familiar terminal-emulator
						// affordance (PuTTY/most Linux terminals bind this identically).
						if lastSelected != "" {
							_ = sdl.SetClipboardText(lastSelected)
						}
					} else if (e.Keysym.Mod&sdl.KMOD_CTRL) != 0 && (e.Keysym.Mod&sdl.KMOD_SHIFT) != 0 &&
						e.Keysym.Sym == sdl.K_v {
						// Ctrl+Shift+V: paste OS clipboard. Deliberately NOT plain Ctrl+V —
						// see the `selection` var's doc comment on why Shift is required.
						if text, err := sdl.GetClipboardText(); err == nil && text != "" {
							pasteText(ioWriter, text)
						}
					} else if scrollHandled := handleScrollKey(screen, e); !scrollHandled {
						writeKey(ioWriter, e)
					}
				}

			case *sdl.MouseButtonEvent:
				col, row := pixelToCell(e.X, e.Y)
				switch {
				case e.Button == sdl.BUTTON_LEFT && e.State == sdl.PRESSED:
					selection.active = true
					selection.startRow, selection.startCol = row, col
					selection.endRow, selection.endCol = row, col
					selection.haveSelection = false
				case e.Button == sdl.BUTTON_LEFT && e.State == sdl.RELEASED:
					if selection.active {
						selection.active = false
						if selection.haveSelection {
							lastSelected = selectedText(screen, selection.startRow, selection.startCol, selection.endRow, selection.endCol)
							if lastSelected != "" {
								_ = sdl.SetClipboardText(lastSelected)
							}
						}
					}
				case e.Button == sdl.BUTTON_MIDDLE && e.State == sdl.PRESSED:
					// Middle-click paste, X11-primary-selection-style. Previously read
					// only PITVIPER's own lastSelected buffer -- real bug (founder,
					// real-time, critical: "i can copy the text out but when i paste the
					// code in notepad first... when i middle click... it effing pastes
					// the url the like previous clipboard"): lastSelected only updates
					// on a completed PITVIPER-internal drag-select; anything copied
					// through the real OS clipboard by another route never touches it,
					// so middle-click could paste a stale PITVIPER-internal selection
					// even though the real OS clipboard (and therefore Ctrl+Shift+V, and
					// pasting into another app like Notepad) already has the correct,
					// current text. Both of PITVIPER's own copy paths (Ctrl+Shift+C and
					// drag-release below) already call SetClipboardText in lockstep with
					// setting lastSelected, so the real OS clipboard is always at least
					// as fresh -- prefer it here, falling back to lastSelected only if
					// the OS clipboard read fails or is empty (keeps working the way it
					// always did if there's genuinely nothing on the real clipboard yet).
					if text, err := sdl.GetClipboardText(); err == nil && text != "" {
						pasteText(ioWriter, text)
					} else if lastSelected != "" {
						pasteText(ioWriter, lastSelected)
					}
				}

			case *sdl.MouseWheelEvent:
				// Ctrl+scroll: zoom, Photoshop's own Ctrl/Cmd+scroll-wheel zoom binding
				// (founder: "i guess a key combo with scroll to zoom the terminal text
				// size"). Plain scroll (no Ctrl) is intentionally a no-op here — PITVIPER
				// has no scrollback buffer at all yet (Page Up/Down already scroll the
				// live screen via handleScrollKey, a real separate feature); wiring wheel
				// events to a real scrollback is flagged as its own follow-up, not faked.
				if (sdl.GetModState() & sdl.KMOD_CTRL) != 0 {
					if e.Y > 0 {
						adjustZoom(zoomStep)
					} else if e.Y < 0 {
						adjustZoom(-zoomStep)
					}
				}

			case *sdl.MouseMotionEvent:
				if selection.active {
					col, row := pixelToCell(e.X, e.Y)
					if row != selection.endRow || col != selection.endCol {
						selection.endRow, selection.endCol = row, col
						if row != selection.startRow || col != selection.startCol {
							selection.haveSelection = true
						}
					}
				}

			case *sdl.TextInputEvent:
				// Snap back to live view when user types.
				if screen.ScrollLines() > 0 {
					screen.ScrollReset()
				}
				text := e.GetText()
				if w, ok := ioWriter.(interface{ WriteString(string) (int, error) }); ok {
					_, _ = w.WriteString(text)
				} else {
					_, _ = ioWriter.Write([]byte(text))
				}
			}
		}

		// Wait for next frame tick.
		select {
		case <-ticker.C:
		default:
		}

		// Update window title from OSC sequences (PTY mode only — GFD title is fixed).
		if !gfdMode {
			if t := screen.GetTitle(); t != "" {
				win.SetTitle("PITVIPER — " + t)
			}
		}

		// zoomScale applied once for the whole frame via SDL's own renderer
		// scale, reset immediately after Present so window-chrome-relative
		// SDL calls elsewhere (event coordinates, etc.) stay in real pixels.
		_ = ren.SetScale(zoomScale, zoomScale)
		renderFrame(ren, screen)
		if gfdMode {
			renderGFDBar(ren)
		}
		// S127-05: district overlay pane (Ctrl+D toggle).
		if gfdMode && districtPaneOpen && gfdClient != nil {
			renderDistrictPane(ren, gfdClient.Snapshot())
		}
		ren.Present()
		_ = ren.SetScale(1, 1)
	}
}

// renderGFDBar draws the Channel 11 status bar at the bottom of the window.
func renderGFDBar(ren *sdl.Renderer) {
	winW, winH, _ := ren.GetOutputSize()
	barH := int32(gfdBarRows * font.GlyphH)
	barY := winH - barH

	_ = ren.SetDrawColor(gfdPalette.barBG.R, gfdPalette.barBG.G, gfdPalette.barBG.B, 0xff)
	_ = ren.FillRect(&sdl.Rect{X: 0, Y: barY, W: winW, H: barH})

	_ = ren.SetDrawColor(gfdPalette.accent.R, gfdPalette.accent.G, gfdPalette.accent.B, 0xff)
	_ = ren.DrawLine(0, barY, winW, barY)

	label := "* LIVE  CHANNEL 11"
	if gfdWebmaster {
		label = "* LIVE  CHANNEL 11  [WEBMASTER]"
	}
	renderBarText(ren, label, 4, barY+2, gfdPalette.accent)

	// Live API state (webmaster mode only).
	if gfdWebmaster && gfdClient != nil {
		state := gfdClient.Snapshot()
		gearColor := gfdPalette.freq
		if state.EmilyGear == "REST" {
			gearColor = gfdPalette.muted
		} else if state.EmilyGear == "UNKNOWN" {
			gearColor = gfdPalette.bloc
		}
		gearLabel := "EMILY:" + state.EmilyGear
		renderBarText(ren, gearLabel, 4+int32((len(label)+2)*font.GlyphW), barY+2, gearColor)

		if state.TierName != "" {
			tierX := 4 + int32((len(label)+len(gearLabel)+4)*font.GlyphW)
			renderBarText(ren, "["+state.TierName+"]", tierX, barY+2, gfdPalette.freq)
		}
	}

	ts := time.Now().Format("15:04:05")
	tsX := winW - int32((len(ts)+2)*font.GlyphW)
	if tsX > 0 {
		renderBarText(ren, ts, tsX, barY+2, gfdPalette.muted)
	}
}

// renderBarText draws a plain ASCII string into the bar using the glyph atlas.
func renderBarText(ren *sdl.Renderer, text string, x, y int32, col sdl.Color) {
	if glyphAtlasTex != nil {
		_ = glyphAtlasTex.SetColorMod(col.R, col.G, col.B)
		_ = glyphAtlasTex.SetAlphaMod(col.A)
		for i, ch := range text {
			px := x + int32(i*font.GlyphW)
			slotX, ok := glyphAtlasSlot[ch]
			if !ok {
				slotX = glyphAtlasSlot['?']
			}
			_ = ren.Copy(glyphAtlasTex,
				&sdl.Rect{X: slotX, Y: 0, W: int32(font.GlyphW), H: int32(font.GlyphH)},
				&sdl.Rect{X: px, Y: y, W: int32(font.GlyphW), H: int32(font.GlyphH)})
		}
		return
	}
	_ = ren.SetDrawColor(col.R, col.G, col.B, col.A)
	for i, ch := range text {
		px := x + int32(i*font.GlyphW)
		bits := font.GlyphBits(rune(ch))
		for row := 0; row < font.GlyphH; row++ {
			for c2 := 0; c2 < font.GlyphW; c2++ {
				if bits[row*font.GlyphW+c2] != 0 {
					_ = ren.DrawPoint(px+int32(c2), y+int32(row))
				}
			}
		}
	}
}

// renderDistrictPane draws a 20-col right-side district overlay pane (S127-05).
// Shows live Field Office state from IDUNA: district name, phase, holder, alertness.
func renderDistrictPane(ren *sdl.Renderer, state gfdapi.State) {
	winW, winH, _ := ren.GetOutputSize()
	paneW := int32(20 * font.GlyphW)
	paneX := winW - paneW

	// Dark semi-opaque background panel.
	_ = ren.SetDrawColor(0x0c, 0x0c, 0x14, 0xe0)
	_ = ren.FillRect(&sdl.Rect{X: paneX, Y: 0, W: paneW, H: winH})

	// Border line.
	gold := gfdPalette.accent
	_ = ren.SetDrawColor(gold.R, gold.G, gold.B, 0xff)
	_ = ren.DrawLine(paneX, 0, paneX, winH)

	y := int32(4)
	lineH := int32(font.GlyphH + 2)

	renderBarText(ren, "DISTRICT STATE", paneX+4, y, gold)
	y += lineH * 2

	muted := gfdPalette.muted
	freq := gfdPalette.freq

	if len(state.Districts) == 0 {
		renderBarText(ren, "(no data)", paneX+4, y, muted)
		return
	}
	for _, d := range state.Districts {
		name := d.DistrictName
		if len(name) > 16 {
			name = name[:16]
		}
		renderBarText(ren, name, paneX+4, y, gold)
		y += lineH

		phaseCol := muted
		switch d.Phase {
		case "Held":
			phaseCol = freq
		case "Contested", "Containment":
			phaseCol = gfdPalette.bloc
		}
		renderBarText(ren, d.Phase, paneX+6, y, phaseCol)
		y += lineH

		holder := d.HolderID
		if holder == "" {
			holder = "---"
		}
		if len(holder) > 14 {
			holder = holder[:14]
		}
		renderBarText(ren, holder, paneX+6, y, muted)
		y += lineH + 4

		if y+lineH > winH-int32(gfdBarRows*font.GlyphH) {
			break
		}
	}
}

// renderChannel11Splash shows the Channel 11 logo full-screen for the given duration.
// Pumps SDL events during the wait so the window doesn't appear frozen.
func renderChannel11Splash(ren *sdl.Renderer, win *sdl.Window, dur time.Duration) {
	winW, winH, _ := ren.GetOutputSize()

	gold := sdl.Color{R: 0xd4, G: 0xa0, B: 0x17, A: 0xff}
	dark := sdl.Color{R: 0x08, G: 0x08, B: 0x0c, A: 0xff}
	muted := sdl.Color{R: 0x44, G: 0x44, B: 0x55, A: 0xff}

	logoLines := []string{
		"  ██████╗██╗  ██╗ █",
		"  ██╔════╝██║  ██║ ▄",
		"  ██║     ███████║ 1",
		"  ██║     ██╔══██║ 1",
		"  ╚██████╗██║  ██║ █",
		"   ╚═════╝╚═╝  ╚═╝  ",
	}
	title := "GOBLIN FOX DRAGON"
	blink := "● CONNECTING..."

	deadline := time.Now().Add(dur)
	blinkTick := time.NewTicker(500 * time.Millisecond)
	defer blinkTick.Stop()
	blinkOn := true
	frameTick := time.NewTicker(time.Second / 30)
	defer frameTick.Stop()

	render := func() {
		_ = ren.SetDrawColor(dark.R, dark.G, dark.B, 0xff)
		_ = ren.Clear()

		// Centered logo block
		logoY := winH/2 - int32(len(logoLines)*font.GlyphH) - int32(font.GlyphH)
		for i, line := range logoLines {
			x := (winW - int32(len(line)*font.GlyphW)) / 2
			renderBarText(ren, line, x, logoY+int32(i*font.GlyphH), gold)
		}
		// Title below logo
		titleY := logoY + int32(len(logoLines)*font.GlyphH) + 4
		titleX := (winW - int32(len(title)*font.GlyphW)) / 2
		renderBarText(ren, title, titleX, titleY, gold)
		// Blink line
		if blinkOn {
			blinkY := titleY + int32(font.GlyphH)*2
			blinkX := (winW - int32(len(blink)*font.GlyphW)) / 2
			renderBarText(ren, blink, blinkX, blinkY, muted)
		}
		ren.Present()
	}

	render()
	for time.Now().Before(deadline) {
		// Pump events so the window stays responsive.
		for {
			ev := sdl.PollEvent()
			if ev == nil {
				break
			}
			if _, quit := ev.(*sdl.QuitEvent); quit {
				return
			}
		}
		select {
		case <-blinkTick.C:
			blinkOn = !blinkOn
			render()
		case <-frameTick.C:
			render()
		}
	}
	_ = win // satisfy compiler
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

			// Real color emoji (founder: "build all emojis into pitviper"),
			// layered on top of the existing monochrome path rather than
			// replacing it -- falls through to the bitmap glyph below if
			// the emoji font isn't loaded (InitEmoji failed/not called) or
			// this specific codepoint has no glyph.
			if font.IsEmoji(cell.Ch) {
				if tex := emojiTexture(ren, cell.Ch); tex != nil {
					_ = ren.Copy(tex, nil, &sdl.Rect{X: px, Y: py, W: int32(font.GlyphW), H: int32(font.GlyphH)})
					continue
				}
			}

			// F11 "shiny font" (real JetBrains Mono via SDL2_ttf), checked
			// before the built-in atlas -- only when the toggle is on and
			// the font actually loaded; falls through to the OG atlas
			// otherwise, same layering as the emoji branch above.
			if useShinyFont {
				if tex := shinyTexture(ren, cell.Ch); tex != nil {
					_ = tex.SetColorMod(fg.R, fg.G, fg.B)
					_ = tex.SetAlphaMod(fg.A)
					_ = ren.Copy(tex, nil, &sdl.Rect{X: px, Y: py, W: int32(font.GlyphW), H: int32(font.GlyphH)})
					continue
				}
			}

			// Draw foreground glyph: one GPU texture blit from the shared
			// atlas (real GPU offload), falling back to the old per-pixel
			// path only if the atlas failed to build at startup.
			if glyphAtlasTex != nil {
				slotX, ok := glyphAtlasSlot[cell.Ch]
				if !ok {
					slotX = glyphAtlasSlot['?']
				}
				_ = glyphAtlasTex.SetColorMod(fg.R, fg.G, fg.B)
				_ = glyphAtlasTex.SetAlphaMod(fg.A)
				_ = ren.Copy(glyphAtlasTex,
					&sdl.Rect{X: slotX, Y: 0, W: int32(font.GlyphW), H: int32(font.GlyphH)},
					&sdl.Rect{X: px, Y: py, W: int32(font.GlyphW), H: int32(font.GlyphH)})
			} else {
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
	}

	// Selection highlight (S187-02): drawn as a semi-transparent overlay so
	// the underlying glyph is still legible, matching how most terminal
	// emulators render an in-progress or just-finished selection.
	if selection.active || selection.haveSelection {
		r1, c1, r2, c2 := selection.startRow, selection.startCol, selection.endRow, selection.endCol
		if r1 > r2 || (r1 == r2 && c1 > c2) {
			r1, c1, r2, c2 = r2, c2, r1, c1
		}
		_ = ren.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
		_ = ren.SetDrawColor(0x60, 0x80, 0xff, 0x60)
		for row := r1; row <= r2 && row < rows; row++ {
			startCol, endCol := 0, cols-1
			if row == r1 {
				startCol = c1
			}
			if row == r2 {
				endCol = c2
			}
			if endCol >= cols {
				endCol = cols - 1
			}
			if startCol > endCol {
				continue
			}
			px := int32(startCol * font.GlyphW)
			py := int32(row * font.GlyphH)
			w := int32((endCol - startCol + 1) * font.GlyphW)
			_ = ren.FillRect(&sdl.Rect{X: px, Y: py, W: w, H: int32(font.GlyphH)})
		}
		_ = ren.SetDrawBlendMode(sdl.BLENDMODE_NONE)
	}
}

// pixelToCell converts window-relative pixel coordinates to a (col, row) cell
// position, clamped to non-negative — mouse events firing slightly outside
// the grid (e.g. in the GFD status bar) shouldn't produce a negative index.
func pixelToCell(x, y int32) (col, row int) {
	col = int(x) / font.GlyphW
	row = int(y) / font.GlyphH
	if col < 0 {
		col = 0
	}
	if row < 0 {
		row = 0
	}
	return col, row
}

// selectedText extracts the text between two cell positions from the current
// screen snapshot, normalizing so the range reads top-to-bottom regardless of
// drag direction. Trailing spaces on each line are trimmed (standard terminal
// copy behavior — cell padding shouldn't end up in the clipboard).
func selectedText(screen *vterm.Screen, r1, c1, r2, c2 int) string {
	cells, cols, rows, _, _ := screen.Snapshot()
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if r1 < 0 {
		r1 = 0
	}
	if r2 >= rows {
		r2 = rows - 1
	}
	if r2 < r1 {
		return ""
	}

	var b strings.Builder
	for row := r1; row <= r2; row++ {
		startCol, endCol := 0, cols-1
		if row == r1 {
			startCol = c1
		}
		if row == r2 {
			endCol = c2
		}
		if startCol < 0 {
			startCol = 0
		}
		if endCol >= cols {
			endCol = cols - 1
		}
		line := make([]rune, 0, endCol-startCol+1)
		for col := startCol; col <= endCol; col++ {
			ch := cells[row*cols+col].Ch
			if ch == 0 {
				ch = ' '
			}
			line = append(line, ch)
		}
		b.WriteString(strings.TrimRight(string(line), " "))
		if row != r2 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// pasteText writes pasted text into the connection, converting newlines to
// carriage returns to match how K_RETURN is sent elsewhere in this file —
// a shell reads \r as Enter, not \n.
func pasteText(w io.Writer, text string) {
	_, _ = w.Write([]byte(strings.ReplaceAll(text, "\n", "\r")))
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

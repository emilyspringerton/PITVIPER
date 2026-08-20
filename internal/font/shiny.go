//go:build linux || windows

// shiny.go — the real "shiny font" F11/F12 toggle. Founder, real-time:
// "can you please find the nicest monospace public domain font you can and
// add it on a toggle like f11 or f12" -> "keep the og font for now" ->
// "and the toggle switches to the new shiny font".
//
// Real, honest note on "public domain": genuinely public-domain, high-
// quality modern monospace fonts are rare -- almost everything real and
// well-regarded (JetBrains Mono, Fira Mono, Cascadia Code, IBM Plex Mono)
// ships under a real, different permissive open license (SIL OFL, Apache
// 2.0, MIT), not literally public domain. This uses JetBrains Mono
// specifically -- SIL OFL 1.1 licensed, free to embed -- because it's
// already PITVIPER/CLAUDE.md's own stated intended font ("FreeType2
// (glyph rendering, embedded JetBrains Mono)" in the Stack section), not
// a fresh pick, and it's a real, well-regarded, purpose-built terminal
// font. The license distinction is stated here honestly rather than
// mislabeling OFL as public domain.
package font

import (
	"fmt"
	"sync"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// jetBrainsMonoPath is the real, standard install location the Debian/
// Ubuntu fonts-jetbrains-mono package (candidate 2.304+ds-4 as of this
// writing) uses.
const jetBrainsMonoPath = "/usr/share/fonts/truetype/jetbrains-mono/JetBrainsMono-Regular.ttf"

var (
	shinyFont  *ttf.Font
	shinyCache = map[rune]*sdl.Surface{}
	shinyMu    sync.Mutex
)

// InitShinyFont loads JetBrains Mono at the given point size. Real,
// honest failure mode matching InitEmoji: returns an error rather than
// panicking if SDL2_ttf or the font package isn't installed yet (see
// sudo-queue/19-pitviper-freetype-emoji-fonts.sh) -- the F11 toggle
// simply has nothing to switch to until this succeeds, the built-in
// atlas keeps working regardless.
func InitShinyFont(pointSize int) error {
	if !ttf.WasInit() {
		if err := ttf.Init(); err != nil {
			return fmt.Errorf("ttf.Init: %w", err)
		}
	}
	f, err := ttf.OpenFont(jetBrainsMonoPath, pointSize)
	if err != nil {
		return fmt.Errorf("open %s: %w", jetBrainsMonoPath, err)
	}
	shinyFont = f
	return nil
}

// ShinyFontAvailable reports whether InitShinyFont has succeeded, so
// callers (the F11 handler) can tell "toggle did nothing, font never
// loaded" apart from "toggle is now off."
func ShinyFontAvailable() bool {
	return shinyFont != nil
}

// ShinyGlyphSurface returns a cached, real anti-aliased glyph surface for
// ch, rendered by JetBrains Mono itself -- or nil if the font isn't
// loaded or has no real glyph for ch (falls back to the existing
// monochrome atlas at the call site, same pattern as EmojiSurface).
func ShinyGlyphSurface(ch rune) *sdl.Surface {
	if shinyFont == nil {
		return nil
	}
	shinyMu.Lock()
	defer shinyMu.Unlock()
	if surf, ok := shinyCache[ch]; ok {
		return surf
	}
	surf, err := shinyFont.RenderUTF8Blended(string(ch), sdl.Color{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		shinyCache[ch] = nil
		return nil
	}
	shinyCache[ch] = surf
	return surf
}

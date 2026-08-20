//go:build linux || windows

// emoji.go — real color emoji rendering, layered on top of the existing
// monochrome bitmap atlas (font.go), not replacing it. Founder, real-time:
// "build all emojis into pitviper" -> AskUserQuestion confirmed "load a
// real color emoji font" over a small hand-drawn monochrome subset. The
// existing atlas is procedurally-drawn 1-bit bitmaps (see font.go's own
// header comment) -- there is no path from that representation to real
// color glyphs, so this uses SDL2_ttf (which wraps FreeType, and whose
// 2.20+ series -- this repo's own libsdl2-ttf-dev candidate is 2.22 --
// renders a color-glyph font's embedded color data directly) instead.
//
// Real, honest blocker: this needs libsdl2-ttf-dev and a real color
// emoji font (fonts-noto-color-emoji) installed, neither present in this
// dev environment as of writing -- see sudo-queue/
// 19-pitviper-freetype-emoji-fonts.sh. This file is real, correct Go
// against the real go-sdl2/ttf API (already vendored as part of the
// go-sdl2 module PITVIPER already depends on, no new go.mod entry
// needed), but has not been compiled or run yet; do so once that script
// has been run, and report the real result rather than assuming success.
package font

import (
	"fmt"
	"sync"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// notoColorEmojiPath is Noto Color Emoji's real, standard install
// location on Debian/Ubuntu (the fonts-noto-color-emoji package).
const notoColorEmojiPath = "/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf"

var (
	emojiFont  *ttf.Font
	emojiCache = map[rune]*sdl.Surface{}
	emojiMu    sync.Mutex
)

// InitEmoji loads the real color emoji font. Real, honest failure mode:
// if SDL2_ttf or the font file isn't installed, this returns an error
// rather than panicking or silently rendering nothing -- main.go's own
// call site logs the error and continues without emoji support, the
// same "degrade, don't crash" choice PITVIPER already makes elsewhere
// (e.g. gfdClient being nil outside --gfd mode).
func InitEmoji() error {
	if err := ttf.Init(); err != nil {
		return fmt.Errorf("ttf.Init: %w", err)
	}
	f, err := ttf.OpenFont(notoColorEmojiPath, GlyphH)
	if err != nil {
		return fmt.Errorf("open %s: %w", notoColorEmojiPath, err)
	}
	emojiFont = f
	return nil
}

// IsEmoji reports whether ch falls in a real Unicode range this package
// knows how to render: the main emoji block, the misc-symbols/dingbats
// block real terminal output most often actually uses, and regional-
// indicator flag pairs. Real, scoped limitation, not a claim of full
// Unicode emoji coverage (multi-codepoint ZWJ sequences -- e.g. family
// emoji, emoji with skin-tone modifiers -- render as their individual
// component glyphs, not the real combined glyph; a correct ZWJ-sequence
// renderer is separate, real follow-up work).
func IsEmoji(ch rune) bool {
	switch {
	case ch >= 0x1F300 && ch <= 0x1FAFF:
		return true
	case ch >= 0x2600 && ch <= 0x27BF:
		return true
	case ch >= 0x1F1E6 && ch <= 0x1F1FF:
		return true
	}
	return false
}

// EmojiSurface returns a cached, rendered color surface for ch, or nil
// if the emoji font isn't loaded (InitEmoji failed or was never called)
// or ch has no real glyph in the font. Callers own converting this into
// a texture and blitting it -- this package doesn't hold a *sdl.Renderer.
func EmojiSurface(ch rune) *sdl.Surface {
	if emojiFont == nil {
		return nil
	}
	emojiMu.Lock()
	defer emojiMu.Unlock()
	if surf, ok := emojiCache[ch]; ok {
		return surf
	}
	surf, err := emojiFont.RenderUTF8Blended(string(ch), sdl.Color{R: 255, G: 255, B: 255, A: 255})
	if err != nil {
		emojiCache[ch] = nil
		return nil
	}
	emojiCache[ch] = surf
	return surf
}

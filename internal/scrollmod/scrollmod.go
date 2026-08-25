// Package scrollmod is PITVIPER's first real PARENA-authored mod: plain
// (non-Ctrl) mouse-wheel scroll, wired through a PARENA-compiled C
// function rather than a direct Go patch. Founder, real-time (2026-08-25):
// "fix with parena" / "mod surface first" / "api first" — the founder
// explicitly chose the mod-surface route over a quick direct Go fix,
// continuing the already-established "mod surface first" policy from
// S189-32/S189-40 (2026-08-20) rather than opening a new precedent.
//
// v0 scope, deliberately minimal: no dispatch table, no plugin discovery,
// no config. One PARENA function (on-wheel-scroll, in
// stdlib/pitviper/vterm_mod.prn) compiled to C and linked directly into
// this cgo package; PITVIPER's SDL event loop calls TriggerWheelScroll on
// every wheel event, which calls the compiled mod's on_wheel_scroll, which
// calls back into pitviper_host_scroll (exported below) to actually move
// the existing vterm scrollback. This is the narrowest real thing that
// makes "the fix is a PARENA mod" true, not a general plugin system --
// see EMILY/BACKLOG.md SECTION 192 for the full writeup and the
// editor/plugin.prn precedent (S189-29/34/40) this borrows its FFI
// pattern from.
//
// vterm_mod.c in this package directory is PARENA-generated (`parena
// build stdlib/pitviper/vterm_mod.prn -o internal/scrollmod/vterm_mod.c`)
// -- do not hand-edit it; regenerate from the .prn source instead.
package scrollmod

/*
#cgo CFLAGS: -include ${SRCDIR}/scrollmod_host.h
extern void on_wheel_scroll(int delta);
*/
import "C"

// ScrollCallback is set once at PITVIPER startup (main.go) to the real
// scrollback hook (screen.ScrollLines/ScrollReset's own wiring, the same
// mechanism Page Up/Down's handleScrollKey already uses) -- dependency
// injection rather than an import cycle, since this package can't import
// cmd/pitviper's main package.
var ScrollCallback func(delta int)

// pitviper_host_scroll is called BY the PARENA-compiled mod (vterm_mod.c's
// on_wheel_scroll), not the other way around -- this is the "host"
// half of the mod-surface ABI declared in scrollmod_host.h.
//
//export pitviper_host_scroll
func pitviper_host_scroll(delta C.int) {
	if ScrollCallback != nil {
		ScrollCallback(int(delta))
	}
}

// TriggerWheelScroll is what main.go's SDL.MouseWheelEvent handler calls.
// It enters the PARENA-compiled mod (on_wheel_scroll), which immediately
// calls back into pitviper_host_scroll above -- a real round-trip through
// compiled PARENA code, not a no-op passthrough.
func TriggerWheelScroll(delta int) {
	C.on_wheel_scroll(C.int(delta))
}

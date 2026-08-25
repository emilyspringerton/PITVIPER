package scrollmod

import "testing"

// TestTriggerWheelScrollRoundTrip proves the actual round trip works:
// Go calls into vterm_mod.c's PARENA-compiled on_wheel_scroll, which
// calls back out to pitviper_host_scroll (exported below), which invokes
// ScrollCallback — not just "it compiles" (S189-40's own compiled-only
// bar), a real, live-executed cgo call through generated C and back.
func TestTriggerWheelScrollRoundTrip(t *testing.T) {
	defer func() { ScrollCallback = nil }()

	var got int
	called := false
	ScrollCallback = func(delta int) {
		called = true
		got = delta
	}

	TriggerWheelScroll(9)

	if !called {
		t.Fatal("ScrollCallback was never invoked — round trip through the PARENA-compiled mod did not reach the host callback")
	}
	if got != 9 {
		t.Errorf("ScrollCallback delta = %d, want 9", got)
	}
}

// TestTriggerWheelScrollNilCallback confirms a nil ScrollCallback (the
// state before main.go registers one) doesn't panic — pitviper_host_scroll
// guards on this explicitly.
func TestTriggerWheelScrollNilCallback(t *testing.T) {
	ScrollCallback = nil
	TriggerWheelScroll(3) // must not panic
}

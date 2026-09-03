package main

import "testing"

// TestShouldTryShinyFallback -- kanban 232131231, "fix unicode in pitviper." Real, genuine bug:
// the OG bitmap atlas only covers ASCII + a curated extended set (box-drawing/braille); any
// other real Unicode character used to silently render as '?' even with the real TTF "shiny"
// font available, since that font was only ever consulted when the F11 toggle was manually on.
func TestShouldTryShinyFallback(t *testing.T) {
	cases := []struct {
		name        string
		useShiny    bool
		inAtlas     bool
		wantAttempt bool
	}{
		{"toggle on, ASCII in atlas -- still tries shiny (user preference)", true, true, true},
		{"toggle on, not in atlas -- tries shiny", true, false, true},
		{"toggle off, ASCII in atlas -- fast path, no shiny attempt", false, true, false},
		{"toggle off, real unicode NOT in atlas -- must still try shiny (the real bug fixed here)", false, false, true},
	}
	for _, c := range cases {
		if got := shouldTryShinyFallback(c.useShiny, c.inAtlas); got != c.wantAttempt {
			t.Errorf("%s: shouldTryShinyFallback(%v, %v) = %v, want %v", c.name, c.useShiny, c.inAtlas, got, c.wantAttempt)
		}
	}
}

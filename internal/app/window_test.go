package app

import "testing"

// The window opens at a size the interface was checked at, and can never be made smaller than
// the minimum.
func TestTheWindowStartsNoSmallerThanItsMinimum(t *testing.T) {
	if MinWindowWidth <= 0 || MinWindowHeight <= 0 {
		t.Fatal("a window needs a minimum size")
	}
	if DefaultWindowWidth < MinWindowWidth || DefaultWindowHeight < MinWindowHeight {
		t.Errorf("the window opens at %dx%d, below its %dx%d minimum", DefaultWindowWidth, DefaultWindowHeight, MinWindowWidth, MinWindowHeight)
	}
}

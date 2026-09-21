package main

import "testing"

// The window opens at a size the interface was checked at, and can never be made smaller than
// the minimum.
func TestTheWindowStartsNoSmallerThanItsMinimum(t *testing.T) {
	if minWindowWidth <= 0 || minWindowHeight <= 0 {
		t.Fatal("a window needs a minimum size")
	}
	if defaultWindowWidth < minWindowWidth || defaultWindowHeight < minWindowHeight {
		t.Errorf("the window opens at %dx%d, below its %dx%d minimum", defaultWindowWidth, defaultWindowHeight, minWindowWidth, minWindowHeight)
	}
}

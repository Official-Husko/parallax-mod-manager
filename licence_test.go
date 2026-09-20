package main

import (
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/about"
)

// The licence must ship inside the binary (it requires every copy to carry it),
// and the About page names it from its own text.
func TestTheLicenceShipsInsideTheBuild(t *testing.T) {
	a := &App{}
	text := a.LicenceText()
	if len(text) < 1000 || !strings.Contains(text, "Non-Commercial") {
		t.Fatalf("LicenceText is %d bytes and does not look like the licence", len(text))
	}
	if !strings.Contains(text, "https://github.com/Official-Husko/parallax-mod-manager") {
		t.Error("the licence text should name the Official Project")
	}
	name, id := about.ParseLicence(text)
	if !strings.Contains(name, "Non-Commercial Source License") || id != "PMM-NCSL-1.0" {
		t.Errorf("parsed %q / %q from the shipped licence", name, id)
	}
}

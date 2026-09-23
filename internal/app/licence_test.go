package app

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/about"
)

// The licence must ship inside the binary (it requires every copy to carry it),
// and the About page names it from its own text. main.go's own //go:embed reaches
// LICENCE.md (gamedata.go, at the repo root); this package, a sibling directory,
// can't embed it too, so this test reads the exact same source file directly and
// builds a real App from it via New, the same way main.go does with the real thing.
func TestTheLicenceShipsInsideTheBuild(t *testing.T) {
	licence, err := os.ReadFile(filepath.Join("..", "..", "LICENCE.md"))
	if err != nil {
		t.Fatalf("reading LICENCE.md: %v", err)
	}
	a := New(nil, string(licence), nil, embed.FS{}, nil)
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

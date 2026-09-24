package modedit

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

func TestTemplatesOnlyStellarisGetsTheFiveExtraOnes(t *testing.T) {
	stellaris := Templates(stellarisGameID)
	if len(stellaris) != 6 {
		t.Fatalf("Templates(stellaris) = %d entries, want 6", len(stellaris))
	}
	if stellaris[0].ID != "blank" {
		t.Fatalf("Templates(stellaris)[0].ID = %q, want %q", stellaris[0].ID, "blank")
	}

	other := Templates("some-other-game-uuid")
	if len(other) != 1 || other[0].ID != "blank" {
		t.Fatalf("Templates(other) = %+v, want just [blank]", other)
	}
}

func TestTemplateByIDFallsBackToBlank(t *testing.T) {
	for _, id := range []string{"", "nonsense", "event_chain"} {
		got := TemplateByID("some-other-game-uuid", id)
		if got.ID != "blank" {
			t.Errorf("TemplateByID(other, %q).ID = %q, want %q", id, got.ID, "blank")
		}
	}
	got := TemplateByID(stellarisGameID, "event_chain")
	if got.ID != "event_chain" {
		t.Errorf("TemplateByID(stellaris, %q).ID = %q, want %q", "event_chain", got.ID, "event_chain")
	}
}

// TestTemplateScriptFilesParseCleanly is the real correctness check for every hand-written
// Clausewitz skeleton this feature ships: every .txt file every Stellaris-only template
// produces must parse with no error through the same internal/script.Parse this app's own
// conflict scanning and Checks tab use - a syntax mistake here would otherwise only surface the
// first time someone actually opened Checks on a freshly templated mod.
func TestTemplateScriptFilesParseCleanly(t *testing.T) {
	fields := Fields{Name: "Precursor Tales"}
	for _, tpl := range Templates(stellarisGameID) {
		if tpl.Build == nil {
			continue
		}
		for _, f := range tpl.Build(fields) {
			if strings.HasSuffix(f.RelPath, ".yml") {
				if _, err := locale.Parse([]byte(f.Content)); err != nil {
					t.Errorf("template %q file %s: locale.Parse: %v", tpl.ID, f.RelPath, err)
				}
				continue
			}
			if _, err := script.Parse([]byte(f.Content)); err != nil {
				t.Errorf("template %q file %s: script.Parse: %v\n---\n%s", tpl.ID, f.RelPath, err, f.Content)
			}
		}
	}
}

func TestTemplateBuildUsesTheGivenName(t *testing.T) {
	fields := Fields{Name: "My Cool Species!"}
	for _, tpl := range Templates(stellarisGameID) {
		if tpl.Build == nil {
			continue
		}
		files := tpl.Build(fields)
		if len(files) == 0 {
			t.Errorf("template %q: Build returned no files", tpl.ID)
		}
		for _, f := range files {
			if f.RelPath == "" {
				t.Errorf("template %q: a file has an empty RelPath", tpl.ID)
			}
			if strings.Contains(f.RelPath, "\\") {
				t.Errorf("template %q: RelPath %q should use forward slashes", tpl.ID, f.RelPath)
			}
		}
	}
}

func TestScriptIDSanitizesToAClausewitzSafeIdentifier(t *testing.T) {
	cases := map[string]string{
		"Precursor Tales":  "precursor_tales",
		"My Cool Species!": "my_cool_species",
		"  leading space":  "leading_space",
		"":                 "mod",
		"___":              "mod",
		"Already_Safe_123": "already_safe_123",
	}
	for in, want := range cases {
		if got := scriptID(in); got != want {
			t.Errorf("scriptID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlaceholderThumbnailIsAValid512x384PNG(t *testing.T) {
	data := PlaceholderThumbnail()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("PlaceholderThumbnail did not decode as PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 512 || b.Dy() != 384 {
		t.Errorf("PlaceholderThumbnail size = %dx%d, want 512x384", b.Dx(), b.Dy())
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// The launch must leave every enabled mod with a stub the game can follow, mend the ones whose
// folder moved, and say so - but never touch a mod the playset does not enable.
func TestEnsureModStubsMendsWhatTheGameCouldNotLoad(t *testing.T) {
	modDir := t.TempDir()
	library := t.TempDir()

	stale := "/run/media/someone/Old Mount/Stellaris/"
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(modDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	folder := func(name string) string {
		t.Helper()
		p := filepath.Join(library, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	lustful := folder("Lustful Void")
	write("Lustful Void.mod", "name=\"Lustful Void\"\npath=\""+stale+"Lustful Void\"\n")
	notEnabled := folder("Not Enabled")
	write("Not Enabled.mod", "name=\"Not Enabled\"\npath=\""+stale+"Not Enabled\"\n")
	extra := folder("Extra")

	scanned := []mod.Mod{
		{ID: "Lustful Void", Source: mod.SourceLocal, ContentPath: lustful, DescriptorPath: filepath.Join(modDir, "Lustful Void.mod"),
			Descriptor: mod.Descriptor{Name: "Lustful Void", Path: stale + "Lustful Void"}},
		{ID: "Not Enabled", Source: mod.SourceLocal, ContentPath: notEnabled, DescriptorPath: filepath.Join(modDir, "Not Enabled.mod"),
			Descriptor: mod.Descriptor{Name: "Not Enabled", Path: stale + "Not Enabled"}},
		{ID: "extra_abc", Source: mod.SourceLocal, ContentPath: extra, DescriptorPath: filepath.Join(extra, "descriptor.mod"),
			Descriptor: mod.Descriptor{Name: "Extra Mod"}},
		{ID: "Gone", Source: mod.SourceLocal, ContentPath: stale + "Gone", ContentMissing: true, DescriptorPath: filepath.Join(modDir, "Gone.mod"),
			Descriptor: mod.Descriptor{Name: "Gone", Path: stale + "Gone"}},
	}

	var got []ModStubsReport
	a := &App{eventSink: func(name string, data ...any) {
		if name == modStubsEvent && len(data) == 1 {
			got = append(got, data[0].(ModStubsReport))
		}
	}}
	a.ensureModStubs("game-1", modDir, []string{"Lustful Void", "extra_abc", "Gone", "not-scanned"}, scanned)

	if len(got) != 1 {
		t.Fatalf("expected one report, got %d", len(got))
	}
	r := got[0]
	if r.GameID != "game-1" || strings.Join(r.Fixed, ",") != "Lustful Void" ||
		strings.Join(r.Created, ",") != "Extra Mod" || strings.Join(r.Missing, ",") != "Gone" || len(r.Failed) != 0 {
		t.Errorf("report = %+v", r)
	}

	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(modDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		return string(b)
	}
	if body := read("Lustful Void.mod"); !strings.Contains(body, "path=\""+lustful+"\"") || strings.Contains(body, stale) {
		t.Errorf("Lustful Void.mod was not repaired:\n%s", body)
	}
	if body := read("extra_abc.mod"); !strings.Contains(body, "path=\""+extra+"\"") {
		t.Errorf("extra_abc.mod does not point at the extra folder:\n%s", body)
	}
	if body := read("Not Enabled.mod"); !strings.Contains(body, stale) {
		t.Errorf("a mod the playset does not enable was touched:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(modDir, "Gone.mod")); !os.IsNotExist(err) {
		t.Errorf("a stub was created for a mod with no folder: %v", err)
	}

	// Nothing left to mend: a second launch is silent.
	got = nil
	a.ensureModStubs("game-1", modDir, []string{"Lustful Void", "extra_abc"}, scanned)
	if len(got) != 0 {
		t.Errorf("second launch reported %+v, want nothing", got)
	}
}

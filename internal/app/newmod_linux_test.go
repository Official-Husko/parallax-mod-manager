package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modedit"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

func (e *editorEnv) scanIDs(t *testing.T) map[string]mod.Mod {
	t.Helper()
	res, err := scan.Scan(context.Background(), scan.Options{Game: e.cfg, ExtraFolders: []string{e.extra}})
	if err != nil {
		t.Fatalf("scan.Scan: %v", err)
	}
	byID := make(map[string]mod.Mod, len(res.Mods))
	for _, m := range res.Mods {
		byID[m.ID] = m
	}
	return byID
}

func historyDirExists(t *testing.T, configAppDir string) bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(configAppDir, "mod_edit_history"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries) > 0
}

func TestNewModLocationsListsTheGamesOwnFolderAndExtraFolders(t *testing.T) {
	env := newEditorEnv(t)
	locations, err := env.a.NewModLocations(env.cfg.ID)
	if err != nil {
		t.Fatalf("NewModLocations: %v", err)
	}
	if len(locations) != 2 {
		t.Fatalf("got %d locations, want 2: %+v", len(locations), locations)
	}
	if locations[0].Path != env.modDir || !locations[0].Default {
		t.Errorf("first location = %+v, want the game's own mod folder, Default true", locations[0])
	}
	if locations[1].Path != env.extra || locations[1].Default {
		t.Errorf("second location = %+v, want the extra folder, Default false", locations[1])
	}
}

func TestCreateModInTheGamesOwnFolderWritesADescriptorAndAStub(t *testing.T) {
	env := newEditorEnv(t)
	req := NewModRequest{
		Fields:   modedit.Fields{Name: "Brand New Mod", Version: "1.0", SupportedVersion: "v4.*", Tags: []string{"Gameplay"}},
		Location: env.modDir,
	}
	res, err := env.a.CreateMod(env.cfg.ID, req)
	if err != nil {
		t.Fatalf("CreateMod: %v", err)
	}
	// descriptor.mod, the stub, and - since the default (Blank) template always includes one
	// now - a placeholder thumbnail.png.
	if len(res.Files) != 3 {
		t.Fatalf("wrote %v, want 3 files", res.Files)
	}

	descPath := filepath.Join(env.modDir, "Brand New Mod", "descriptor.mod")
	stubPath := filepath.Join(env.modDir, "Brand New Mod.mod")
	thumbPath := filepath.Join(env.modDir, "Brand New Mod", "thumbnail.png")
	if _, err := os.Stat(descPath); err != nil {
		t.Errorf("descriptor.mod was not written: %v", err)
	}
	if _, err := os.Stat(stubPath); err != nil {
		t.Errorf("stub was not written: %v", err)
	}
	if _, err := os.Stat(thumbPath); err != nil {
		t.Errorf("placeholder thumbnail was not written: %v", err)
	}
	if !env.sawEvent("mods-changed") {
		t.Error("mods-changed was not emitted")
	}
	if historyDirExists(t, env.a.configAppDir) {
		t.Error("CreateMod wrote a history entry - it should never feed Undo last save")
	}

	byID := env.scanIDs(t)
	if _, ok := byID["Brand New Mod"]; !ok {
		t.Errorf("the new mod was not discovered by a real scan: %+v", byID)
	}

	// It is editable right away, just like any other local mod.
	info, err := env.a.ModEditInfo(env.cfg.ID, "Brand New Mod")
	if err != nil {
		t.Fatalf("ModEditInfo: %v", err)
	}
	if !info.Editable {
		t.Errorf("the created mod is not editable: %+v", info)
	}
}

func TestCreateModInAnExtraFolderWritesNoStub(t *testing.T) {
	env := newEditorEnv(t)
	req := NewModRequest{Fields: modedit.Fields{Name: "Extra New Mod"}, Location: env.extra}
	res, err := env.a.CreateMod(env.cfg.ID, req)
	if err != nil {
		t.Fatalf("CreateMod: %v", err)
	}
	// descriptor.mod and the default template's placeholder thumbnail - never a stub outside
	// the game's own mod folder.
	if len(res.Files) != 2 {
		t.Fatalf("wrote %v, want 2 files (no stub)", res.Files)
	}
	if _, err := os.Stat(filepath.Join(env.extra, "Extra New Mod.mod")); !os.IsNotExist(err) {
		t.Error("a stub was written for an extra-folder mod, want none")
	}

	byID := env.scanIDs(t)
	found := false
	for _, m := range byID {
		if m.Descriptor.Name == "Extra New Mod" {
			found = true
		}
	}
	if !found {
		t.Errorf("the extra-folder mod was not discovered by a real scan: %+v", byID)
	}
}

func TestTemplatesForGameListsAllSixForStellaris(t *testing.T) {
	env := newEditorEnv(t)
	templates, err := env.a.TemplatesForGame(env.cfg.ID)
	if err != nil {
		t.Fatalf("TemplatesForGame: %v", err)
	}
	if len(templates) != 6 {
		t.Fatalf("got %d templates, want 6: %+v", len(templates), templates)
	}
	if templates[0].ID != "blank" {
		t.Errorf("first template = %+v, want Blank first", templates[0])
	}
}

func TestTemplatesForGameRefusesAnUnknownGame(t *testing.T) {
	env := newEditorEnv(t)
	if _, err := env.a.TemplatesForGame("not-a-real-game-id"); err == nil {
		t.Fatal("TemplatesForGame for an unknown game succeeded, want an error")
	}
}

func TestCreateModWithATemplateWritesItsExtraFilesAndAThumbnail(t *testing.T) {
	env := newEditorEnv(t)
	req := NewModRequest{
		Fields:   modedit.Fields{Name: "Precursor Tales", Version: "1.0", SupportedVersion: "v4.*"},
		Location: env.modDir,
		Template: "event_chain",
	}
	res, err := env.a.CreateMod(env.cfg.ID, req)
	if err != nil {
		t.Fatalf("CreateMod: %v", err)
	}
	// descriptor.mod, the stub, thumbnail.png, the event file, the on_actions file, and the
	// localisation file.
	if len(res.Files) != 6 {
		t.Fatalf("wrote %v, want 6 files", res.Files)
	}

	modRoot := filepath.Join(env.modDir, "Precursor Tales")
	for _, rel := range []string{
		"thumbnail.png",
		"events/precursor_tales_events.txt",
		"common/on_actions/precursor_tales_on_actions.txt",
		"localisation/english/precursor_tales_l_english.yml",
	} {
		if _, err := os.Stat(filepath.Join(modRoot, rel)); err != nil {
			t.Errorf("%s was not written: %v", rel, err)
		}
	}
}

func TestPreviewNewModWithATemplateShowsItsExtraFilesAndThumbnail(t *testing.T) {
	env := newEditorEnv(t)
	req := NewModRequest{
		Fields:   modedit.Fields{Name: "Precursor Tales"},
		Location: env.modDir,
		Template: "event_chain",
	}
	preview, err := env.a.PreviewNewMod(env.cfg.ID, req)
	if err != nil {
		t.Fatalf("PreviewNewMod: %v", err)
	}
	if preview.Thumbnail == nil {
		t.Error("preview has no Thumbnail, want the template's placeholder")
	}
	if preview.Nothing {
		t.Error("preview.Nothing is true, want false - a template always writes something")
	}
	wantSuffixes := []string{"events/precursor_tales_events.txt", "common/on_actions/precursor_tales_on_actions.txt", "localisation/english/precursor_tales_l_english.yml"}
	for _, suffix := range wantSuffixes {
		found := false
		for _, f := range preview.Files {
			if strings.HasSuffix(f.Path, suffix) {
				found = true
				if f.Kind != modedit.KindContent {
					t.Errorf("file %s has Kind %q, want %q", suffix, f.Kind, modedit.KindContent)
				}
			}
		}
		if !found {
			t.Errorf("preview.Files has nothing ending in %s: %+v", suffix, preview.Files)
		}
	}
	// Nothing was actually written - only PreviewNewMod ran.
	if _, err := os.Stat(filepath.Join(env.modDir, "Precursor Tales")); !os.IsNotExist(err) {
		t.Error("PreviewNewMod wrote real files, want none")
	}
}

func TestCreateModWithAnUnknownTemplateFallsBackToBlank(t *testing.T) {
	env := newEditorEnv(t)
	req := NewModRequest{Fields: modedit.Fields{Name: "Whatever"}, Location: env.modDir, Template: "not-a-real-template"}
	res, err := env.a.CreateMod(env.cfg.ID, req)
	if err != nil {
		t.Fatalf("CreateMod: %v", err)
	}
	if len(res.Files) != 3 {
		t.Fatalf("wrote %v, want 3 files (descriptor, stub, thumbnail - the Blank fallback)", res.Files)
	}
}

func TestCreateModRefusesACollidingName(t *testing.T) {
	env := newEditorEnv(t)
	// "Stub Only.mod" already exists directly in the game's own mod folder (see newEditorEnv) -
	// a name whose sanitized folder name matches it collides on the stub, even though its
	// content folder lives elsewhere.
	req := NewModRequest{Fields: modedit.Fields{Name: "Stub Only"}, Location: env.modDir}
	if _, err := env.a.CreateMod(env.cfg.ID, req); err == nil {
		t.Fatal("CreateMod with a name already in use succeeded, want an error")
	}
	if _, err := os.Stat(filepath.Join(env.modDir, "Stub Only.mod")); err != nil {
		t.Fatal("the original stub should still be there")
	}
}

func TestDuplicateModCopiesEveryFileAndOnlyRenames(t *testing.T) {
	env := newEditorEnv(t)
	req := DuplicateRequest{Name: "Local A Copy", Location: env.modDir}
	res, err := env.a.DuplicateMod(env.cfg.ID, "local_a", "dup-1", req)
	if err != nil {
		t.Fatalf("DuplicateMod: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("wrote %v, want 2 files", res.Files)
	}

	newDir := filepath.Join(env.modDir, "Local A Copy")
	if got := readText(t, filepath.Join(newDir, "common", "a.txt")); got != "x = 1" {
		t.Errorf("content file = %q, want the source's own content copied over", got)
	}
	desc := readText(t, filepath.Join(newDir, "descriptor.mod"))
	if !strings.Contains(desc, `name="Local A Copy"`) || !strings.Contains(desc, `version="1.0"`) || !strings.Contains(desc, `"Gameplay"`) {
		t.Errorf("duplicated descriptor = %q, want the renamed copy with the source's other fields kept", desc)
	}
	stub := readText(t, filepath.Join(env.modDir, "Local A Copy.mod"))
	if !strings.Contains(stub, `path="`+newDir+`"`) {
		t.Errorf("duplicated stub = %q, want path=%q", stub, newDir)
	}

	// The original is untouched.
	if readText(t, filepath.Join(env.lib, "Local A", "descriptor.mod")) == "" {
		t.Fatal("original descriptor is gone")
	}
	if !strings.Contains(readText(t, filepath.Join(env.lib, "Local A", "descriptor.mod")), `name="Local A"`) {
		t.Error("the original mod's own name was changed by duplicating it")
	}
	if historyDirExists(t, env.a.configAppDir) {
		t.Error("DuplicateMod wrote a history entry - it should never feed Undo last save")
	}
	if !env.sawEvent("mods-changed") {
		t.Error("mods-changed was not emitted")
	}
}

func TestDuplicateModWorksForAWorkshopModWithoutTouchingIt(t *testing.T) {
	env := newEditorEnv(t)
	before := readText(t, filepath.Join(env.modDir, "ugc_999.mod"))

	res, err := env.a.DuplicateMod(env.cfg.ID, "ugc_999", "dup-2", DuplicateRequest{Name: "My Own Copy", Location: env.modDir})
	if err != nil {
		t.Fatalf("DuplicateMod of a Workshop mod: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("wrote %v, want 2 files", res.Files)
	}
	if readText(t, filepath.Join(env.modDir, "ugc_999.mod")) != before {
		t.Error("duplicating a Workshop mod changed its own stub")
	}
	if _, err := os.Stat(filepath.Join(env.lib, "workshop", "999", "common", "w.txt")); err != nil {
		t.Errorf("the Workshop mod's own content is gone: %v", err)
	}
}

func TestDuplicateModRefusesForThePatchAndAPackedArchive(t *testing.T) {
	env := newEditorEnv(t)
	if _, err := env.a.DuplicateMod(env.cfg.ID, "zzzzz_parallax_patch", "dup-3", DuplicateRequest{Name: "Patch Copy", Location: env.modDir}); err == nil {
		t.Error("duplicating the generated patch succeeded, want a refusal")
	}
	if _, err := env.a.DuplicateMod(env.cfg.ID, "packed", "dup-4", DuplicateRequest{Name: "Packed Copy", Location: env.modDir}); err == nil {
		t.Error("duplicating a packed archive succeeded, want a refusal")
	}
}

func TestCancellingADuplicateLeavesNoPartialFolderBehind(t *testing.T) {
	env := newEditorEnv(t)
	local := filepath.Join(env.lib, "Local A")
	for i := 0; i < 40; i++ {
		writeText(t, filepath.Join(local, "common", fmt.Sprintf("extra%d.txt", i)), "padding so the copy takes more than one file")
	}

	const requestID = "cancel-me"
	seen := 0
	env.a.eventSink = func(name string, args ...any) {
		env.eventMu.Lock()
		env.events = append(env.events, name)
		env.eventMu.Unlock()
		if name == "duplicate-progress" {
			seen++
			if seen == 3 {
				env.a.CancelDuplicate(requestID)
			}
			if p, ok := args[0].(DuplicateProgress); ok && p.RequestID != requestID {
				t.Errorf("progress RequestID = %q, want %q", p.RequestID, requestID)
			}
		}
	}

	_, err := env.a.DuplicateMod(env.cfg.ID, "local_a", requestID, DuplicateRequest{Name: "Cancelled Copy", Location: env.modDir})
	if err == nil {
		t.Fatal("DuplicateMod: want an error after cancelling, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(env.modDir, "Cancelled Copy")); !os.IsNotExist(statErr) {
		t.Errorf("a partial folder was left behind: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(env.modDir, "Cancelled Copy.mod")); !os.IsNotExist(statErr) {
		t.Error("a stub was left behind for a cancelled duplicate")
	}
	if seen < 3 {
		t.Fatalf("only saw %d progress events, want at least 3 to trust the cancel happened mid-copy", seen)
	}
}


package library

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func testGameConfig() game.GameConfig {
	return game.GameConfig{
		ID:             "test-game",
		DisplayName:    "Test Game",
		DescriptorType: mod.DescriptorClassic,
		ScanFolders:    []string{"common"},
	}
}

func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// writeMod creates one classic-format mod under modDir: its descriptor (in
// modDir itself, per scan's flat-descriptor convention) plus its content
// under its own subfolder.
func writeMod(t *testing.T, modDir, id, name, script string) {
	t.Helper()
	writeFile(t, modDir, id+".mod", `name = "`+name+`"
path = "`+id+`"
version = "1.0"
`)
	writeFile(t, modDir, filepath.Join(id, "common", "x.txt"), script)
}

func TestLoadGameFlagsAModWhoseContentFolderIsMissing(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { a = 1 }`)
	// A real stub whose own path points nowhere real - a deleted drive, a
	// moved install, or a mod removed by hand outside this app.
	writeFile(t, modDir, "gone.mod", `name = "Gone Mod"
path = "`+filepath.Join(modDir, "does-not-exist")+`"
version = "1.0"
`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a"},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}

	byID := map[string]ModSummary{}
	for _, m := range summary.Mods {
		byID[m.ID] = m
	}
	if byID["mod_a"].ContentMissing {
		t.Error("mod_a has real content, ContentMissing should be false")
	}
	if !byID["gone"].ContentMissing {
		t.Errorf("gone's content folder doesn't exist, ContentMissing should be true, got %+v", byID["gone"])
	}
}

func TestLoadGameCleanScanNoConflicts(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing_b = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 2 {
		t.Fatalf("expected 2 mods, got %d: %+v", len(summary.Mods), summary.Mods)
	}
	if len(summary.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", summary.Conflicts)
	}
	if len(summary.Errors) != 0 {
		t.Errorf("expected no errors, got %+v", summary.Errors)
	}
	if summary.Game.ID != "test-game" {
		t.Errorf("Game.ID = %q, want test-game", summary.Game.ID)
	}
}

// TestLoadGameModWithNoTagsOrDependenciesGetsRealEmptySlices pins a real
// bug: writeMod's descriptor (like most real mods) never declares a
// "tags" or "dependencies" block, leaving mod.Descriptor.Tags/Dependencies
// at Go's nil-slice zero value. encoding/json marshals nil as JSON null,
// and the generated TS binding assigns it straight through - so without
// normalizing at this boundary, the frontend's ModSummary.Tags/Dependencies
// (typed string[]) would actually be null, crashing the mod detail panel's
// first .length/.map call the moment a user selected such a mod.
func TestLoadGameModWithNoTagsOrDependenciesGetsRealEmptySlices(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 1 {
		t.Fatalf("expected 1 mod, got %d: %+v", len(summary.Mods), summary.Mods)
	}
	m := summary.Mods[0]
	if m.Tags == nil {
		t.Error("Tags is nil, want a real empty slice (marshals as JSON null, not [])")
	}
	if m.Dependencies == nil {
		t.Error("Dependencies is nil, want a real empty slice (marshals as JSON null, not [])")
	}
}

func TestLoadGameOnQuickSummaryFiresBeforeParsingWithFullModList(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing_b = { cost = 2 }`)

	var quick *Summary
	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		OnQuickSummary: func(s Summary) {
			if quick != nil {
				t.Fatal("OnQuickSummary called more than once")
			}
			quick = &s
		},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if quick == nil {
		t.Fatal("OnQuickSummary was never called")
	}
	if len(quick.Mods) != 2 {
		t.Fatalf("quick summary: expected 2 mods, got %d: %+v", len(quick.Mods), quick.Mods)
	}
	if len(quick.Conflicts) != 0 {
		t.Errorf("quick summary: expected no conflicts (none resolved yet), got %+v", quick.Conflicts)
	}
	// The names/versions/enabled state should already match the final result.
	if !reflect.DeepEqual(quick.Mods, summary.Mods) {
		t.Errorf("quick.Mods = %+v, want equal to final summary.Mods = %+v", quick.Mods, summary.Mods)
	}
}

func TestLoadGameNilOnQuickSummaryIsSafe(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	if _, err := LoadGame(context.Background(), testGameConfig(), Options{CacheDir: t.TempDir(), ModDir: modDir}); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
}

func TestLoadGameDetectsGenuineConflict(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(summary.Conflicts), summary.Conflicts)
	}
	c := summary.Conflicts[0]
	if c.ID != "shared_thing" {
		t.Errorf("Conflict.ID = %q, want shared_thing", c.ID)
	}
	if len(c.Candidates) != 2 || c.Candidates[0].ModName != "Mod A" || c.Candidates[1].ModName != "Mod B" {
		t.Errorf("Candidates = %+v, want [Mod A, Mod B] (mod display names, not IDs)", c.Candidates)
	}
	if c.Winner != "mod_b" {
		t.Errorf("Winner = %q, want mod_b (LIOS default - last in load order wins)", c.Winner)
	}
}

// A user's own RuleOverrides (internal/priorityrules, converted by the
// caller) must actually flip the winner - proof the override reaches
// conflict.Resolve, not just that it's accepted and ignored.
func TestLoadGameRuleOverrideFlipsTheWinnerToFIOS(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir:      t.TempDir(),
		ModDir:        modDir,
		Order:         conflict.LoadOrder{"mod_a", "mod_b"},
		RuleOverrides: conflict.PriorityRules{"common": conflict.FIOS},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(summary.Conflicts), summary.Conflicts)
	}
	if got := summary.Conflicts[0].Winner; got != "mod_a" {
		t.Errorf("Winner = %q, want mod_a (RuleOverrides forced FIOS - first in load order wins)", got)
	}
}

// A RuleOverride for a Type conflict.DefaultPriorityRules doesn't already
// touch must never affect an unrelated Type's own conflict.
func TestLoadGameRuleOverrideIsScopedToItsOwnType(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir:      t.TempDir(),
		ModDir:        modDir,
		Order:         conflict.LoadOrder{"mod_a", "mod_b"},
		RuleOverrides: conflict.PriorityRules{"some/unrelated/type": conflict.FIOS},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if got := summary.Conflicts[0].Winner; got != "mod_b" {
		t.Errorf("Winner = %q, want mod_b (default LIOS still applies to Type \"common\")", got)
	}
}

func TestEffectiveRulesLayersOverridesOnTopOfTheBuiltInDefaults(t *testing.T) {
	got := effectiveRules(conflict.PriorityRules{"common/buildings": conflict.FIOS})
	if got["common/buildings"] != conflict.FIOS {
		t.Errorf("the override itself is missing: %+v", got)
	}
	if got["common/static_modifiers"] != conflict.FIOS {
		t.Errorf("the confirmed built-in default was dropped: %+v", got)
	}
	// An override for the same Type the built-in default already covers
	// wins - the user's own explicit choice, not the built-in one.
	got = effectiveRules(conflict.PriorityRules{"common/static_modifiers": conflict.LIOS})
	if got["common/static_modifiers"] != conflict.LIOS {
		t.Errorf("the override should win over the built-in default for the same Type: %+v", got)
	}
}

func TestEffectiveRulesWithNoOverridesIsJustTheBuiltInDefaults(t *testing.T) {
	got := effectiveRules(nil)
	if len(got) != len(conflict.DefaultPriorityRules) {
		t.Errorf("got %+v, want exactly the built-in defaults", got)
	}
}

func TestLoadGameMalformedModRecordedAsErrorNotFatal(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "good_mod", "Good Mod", `thing = { cost = 1 }`)
	// A descriptor that fails to parse.
	writeFile(t, modDir, "bad_mod.mod", `name = "unterminated`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 1 || summary.Mods[0].ID != "good_mod" {
		t.Errorf("Mods = %+v, want exactly [good_mod] (the malformed descriptor is a scan error, not a listed mod)", summary.Mods)
	}
	if len(summary.Errors) != 1 {
		t.Errorf("Errors = %v, want exactly 1 (the malformed descriptor)", summary.Errors)
	}
}

func TestLoadGameDefaultOrderEnablesNothing(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing_b = { cost = 2 }`)
	// A mod that fails to parse would show up as a scan/parse error if it
	// were ever actually parsed - it never should be, since nothing is
	// enabled, proving disabled-by-default really does skip parsing too,
	// not just conflict detection.
	writeFile(t, modDir, "mod_c.mod", `name = "Mod C"
path = "mod_c"
version = "1.0"
`)
	writeFile(t, modDir, filepath.Join("mod_c", "common", "x.txt"), `this is not valid { clausewitz syntax`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 3 {
		t.Fatalf("expected all 3 mods still listed, got %d: %+v", len(summary.Mods), summary.Mods)
	}
	for _, m := range summary.Mods {
		if m.Enabled {
			t.Errorf("mod %q Enabled = true, want false (nil Order enables nothing - managing a mod is an explicit choice)", m.ID)
		}
	}
	if len(summary.Errors) != 0 {
		t.Errorf("expected no parse errors (nothing enabled, so mod_c's bad content is never parsed), got %+v", summary.Errors)
	}
}

func TestLoadGameExplicitOrderDisablesAbsentMods(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	// mod_b's content is malformed - if LoadGame parses it anyway, that
	// parse failure would show up in summary.Errors, proving the "disabled
	// mods are never parsed" guarantee actually holds, not just that
	// they're excluded from conflict detection.
	writeMod(t, modDir, "mod_b", "Mod B", `this is not valid { clausewitz syntax`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a"}, // mod_b is absent - disabled
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}

	if len(summary.Mods) != 2 {
		t.Fatalf("expected both mods listed regardless of enabled state, got %+v", summary.Mods)
	}
	byID := map[string]ModSummary{}
	for _, m := range summary.Mods {
		byID[m.ID] = m
	}
	if !byID["mod_a"].Enabled {
		t.Error("mod_a.Enabled = false, want true (present in Order)")
	}
	if byID["mod_b"].Enabled {
		t.Error("mod_b.Enabled = true, want false (absent from Order)")
	}
	if len(summary.Errors) != 0 {
		t.Errorf("Errors = %v, want none - mod_b's malformed content must never be parsed while disabled", summary.Errors)
	}
}

func TestLoadGameDisabledModExcludedFromConflicts(t *testing.T) {
	modDir := t.TempDir()
	// Both mods define the same key with different content - normally a
	// conflict, but mod_b is disabled via Order, so it must not appear.
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a"},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Conflicts) != 0 {
		t.Errorf("expected no conflicts (mod_b disabled), got %+v", summary.Conflicts)
	}
}

func TestLoadGameEmptyModDirNoError(t *testing.T) {
	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 0 {
		t.Errorf("expected no mods, got %+v", summary.Mods)
	}
}

package preferences

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	got := Load(path)
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load(missing) = %+v, want Defaults() = %+v", got, Defaults())
	}
}

func TestLoadCorruptFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.jsonc")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := Load(path)
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load(corrupt) = %+v, want Defaults() = %+v", got, Defaults())
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.jsonc")

	want := Preferences{
		ScanForNewMods:      false,
		CloseAfterLaunch:    true,
		WarnOnPatchMismatch: true,
		LastSelectedGame:    "01a0963a-c214-75a3-908d-1b76b91ea7bf",
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := Load(path)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-tripped = %+v, want %+v", got, want)
	}
}

func TestLoadToleratesJSONCComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.jsonc")
	content := `{
		// user turned this off after a big Workshop update
		"scanForNewMods": false,
		"closeAfterLaunch": true,
		"warnOnPatchMismatch": true,
		"lastSelectedGame": "",
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := Load(path)
	// What the file states wins; every setting it doesn't mention keeps its
	// default (see Load).
	want := Defaults()
	want.ScanForNewMods = false
	want.CloseAfterLaunch = true
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load(jsonc) = %+v, want %+v", got, want)
	}
}

func TestLoadKeepsDefaultsForSettingsAddedAfterTheFileWasSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	// An older file: it has never heard of autosortPatchLast.
	old := `{"scanForNewMods": false, "autosortDependencies": true}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if !got.AutosortPatchLast {
		t.Error("a setting the file doesn't mention must keep its default (on), not silently read as off")
	}
	if got.ScanForNewMods {
		t.Error("a setting the file does state must still win over the default")
	}
}

func TestLoadExplicitFalseOverridesAnOnByDefaultSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	if err := os.WriteFile(path, []byte(`{"autosortPatchLast": false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Load(path).AutosortPatchLast {
		t.Error("an explicit false must stay false")
	}
}

func TestDefaultsKeepTheGeneratedPatchLast(t *testing.T) {
	if !Defaults().AutosortPatchLast {
		t.Error("keeping the generated patch last must be on by default")
	}
}

func TestChangedKeysNamesWhatDiffersByItsJSONName(t *testing.T) {
	before := Defaults()
	after := before
	after.AutosortPatchLast = false
	after.BackgroundIntervalSeconds = 600
	after.ManagedGames = []string{"a"}
	after.GamePaths = map[string]string{"g": "/x"}
	got := ChangedKeys(before, after)
	want := []string{"autosortPatchLast", "managedGames", "gamePaths", "backgroundIntervalSeconds"}
	// Field order, not the order they were changed in above.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChangedKeys = %v, want %v", got, want)
	}
	if got := ChangedKeys(before, before); len(got) != 0 {
		t.Errorf("identical preferences reported changes: %v", got)
	}
	// A nil map or list and an empty one are the same to the user.
	a, b := before, before
	a.GamePaths, b.GamePaths = nil, map[string]string{}
	a.ManagedGames, b.ManagedGames = nil, []string{}
	if got := ChangedKeys(a, b); len(got) != 0 {
		t.Errorf("swapping nil for an empty collection was reported as a change: %v", got)
	}
}

func TestWithPlaysetRenamedFollowsTheNameInSettings(t *testing.T) {
	p := Defaults()
	p.LastActivePlaysets = map[string]string{"g1": "Old", "g2": "Old"}
	p.PlaysetAutoloadCustom = map[string]string{"g1": "Old"}
	orig := p.LastActivePlaysets

	got := p.WithPlaysetRenamed("g1", "Old", "New")
	if got.LastActivePlaysets["g1"] != "New" || got.PlaysetAutoloadCustom["g1"] != "New" {
		t.Errorf("references to the renamed playset weren't updated: %+v %+v", got.LastActivePlaysets, got.PlaysetAutoloadCustom)
	}
	if got.LastActivePlaysets["g2"] != "Old" {
		t.Error("another game's playset that happens to share the name must not be touched")
	}
	if orig["g1"] != "Old" {
		t.Error("the original preferences' map was modified - it may be shared")
	}
	if same := p.WithPlaysetRenamed("g1", "Unrelated", "X"); !reflect.DeepEqual(same, p) {
		t.Error("renaming a playset nothing refers to must change nothing")
	}
}

func TestWithoutPlaysetForgetsADeletedPlayset(t *testing.T) {
	p := Defaults()
	p.LastActivePlaysets = map[string]string{"g": "Gone", "h": "Gone"}
	p.PlaysetAutoloadCustom = map[string]string{"g": "Gone"}
	got := p.WithoutPlayset("g", "Gone")
	if _, ok := got.LastActivePlaysets["g"]; ok {
		t.Error("the deleted playset is still remembered as last active")
	}
	if _, ok := got.PlaysetAutoloadCustom["g"]; ok {
		t.Error("the deleted playset is still pinned for auto-loading")
	}
	if got.LastActivePlaysets["h"] != "Gone" {
		t.Error("another game's entry was removed")
	}
	if p.LastActivePlaysets["g"] != "Gone" {
		t.Error("the original preferences' map was modified")
	}
	if same := p.WithoutPlayset("g", ""); !reflect.DeepEqual(same, p) {
		t.Error("an empty name must never match (an unset entry is not a playset)")
	}
}

func TestObserveGameVersionsReportsOnlyRealChanges(t *testing.T) {
	p := Defaults()

	// First sight: recorded, nothing to report.
	p, changes := p.ObserveGameVersions(map[string]string{"stellaris": "v4.4.5", "hoi4": "v1.14"})
	if len(changes) != 0 {
		t.Fatalf("a game seen for the first time can't have updated: %v", changes)
	}
	if p.LastSeenGameVersions["stellaris"] != "v4.4.5" {
		t.Fatalf("the first version wasn't recorded: %v", p.LastSeenGameVersions)
	}

	// Unchanged: nothing.
	if _, changes := p.ObserveGameVersions(map[string]string{"stellaris": "v4.4.5"}); len(changes) != 0 {
		t.Errorf("an unchanged version reported as a change: %v", changes)
	}

	// Changed, in game ID order, and the new version is remembered.
	next, changes := p.ObserveGameVersions(map[string]string{"stellaris": "v4.4.6", "hoi4": "v1.15"})
	want := []VersionChange{{GameID: "hoi4", From: "v1.14", To: "v1.15"}, {GameID: "stellaris", From: "v4.4.5", To: "v4.4.6"}}
	if !reflect.DeepEqual(changes, want) {
		t.Errorf("changes = %+v, want %+v", changes, want)
	}
	if next.LastSeenGameVersions["stellaris"] != "v4.4.6" {
		t.Error("the new version must be recorded so the same update isn't announced twice")
	}
	if p.LastSeenGameVersions["stellaris"] != "v4.4.5" {
		t.Error("the original preferences' map was modified")
	}
	if _, again := next.ObserveGameVersions(map[string]string{"stellaris": "v4.4.6"}); len(again) != 0 {
		t.Errorf("the same update was announced a second time: %v", again)
	}
}

func TestObserveGameVersionsIgnoresAnUnknownVersion(t *testing.T) {
	p := Defaults()
	p.LastSeenGameVersions = map[string]string{"stellaris": "v4.4.5"}

	got, changes := p.ObserveGameVersions(map[string]string{"stellaris": "", "newgame": ""})
	if len(changes) != 0 {
		t.Errorf("a game that can't report its version isn't an update: %v", changes)
	}
	if got.LastSeenGameVersions["stellaris"] != "v4.4.5" {
		t.Error("an unknown version must not overwrite the remembered one - the game coming back at the same version would look like an update")
	}
	if _, ok := got.LastSeenGameVersions["newgame"]; ok {
		t.Error("an empty version must never be recorded")
	}
	// ...and when it returns at the same version: still no change.
	if _, changes := got.ObserveGameVersions(map[string]string{"stellaris": "v4.4.5"}); len(changes) != 0 {
		t.Errorf("returning at the same version reported a change: %v", changes)
	}
}

func TestBackgroundLookDefaultsToTheOriginalDarkeningAndNoBlur(t *testing.T) {
	d := Defaults()
	if d.BackgroundBlur != 0 || d.BackgroundDarken != DefaultBackgroundDarken || DefaultBackgroundDarken != 84 {
		t.Errorf("Defaults = blur %d, darken %d, want 0 and 84 (the look the app shipped with)", d.BackgroundBlur, d.BackgroundDarken)
	}
	// A file from before these settings existed keeps the shipped look instead of reading darken as 0.
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	if err := os.WriteFile(path, []byte(`{"scanForNewMods": false, "backgroundIntervalSeconds": 60}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if got.BackgroundDarken != 84 || got.BackgroundBlur != 0 || got.ScanForNewMods {
		t.Errorf("older file: blur %d, darken %d, scan %v", got.BackgroundBlur, got.BackgroundDarken, got.ScanForNewMods)
	}
}

func TestBackgroundLookExplicitZeroDarkenStaysZeroAndOutOfRangeIsClamped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	_ = os.WriteFile(path, []byte(`{"backgroundDarken": 0, "backgroundBlur": 55}`), 0o644)
	if got := Load(path); got.BackgroundDarken != 0 || got.BackgroundBlur != 55 {
		t.Errorf("explicit values: blur %d, darken %d, want 55 and 0 (someone may want no darkening)", got.BackgroundBlur, got.BackgroundDarken)
	}
	_ = os.WriteFile(path, []byte(`{"backgroundDarken": 500, "backgroundBlur": -20}`), 0o644)
	if got := Load(path); got.BackgroundDarken != 100 || got.BackgroundBlur != 0 {
		t.Errorf("hand-edited values: blur %d, darken %d, want them clamped to 0 and 100", got.BackgroundBlur, got.BackgroundDarken)
	}
	for in, want := range map[int]int{-5: 0, 0: 0, 42: 42, 100: 100, 101: 100} {
		if got := NormalizedPercent(in); got != want {
			t.Errorf("NormalizedPercent(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestChangedKeysNamesTheBackgroundLookSettings(t *testing.T) {
	a := Defaults()
	b := a
	b.BackgroundBlur, b.BackgroundDarken = 30, 60
	keys := ChangedKeys(a, b)
	if strings.Join(keys, ",") != "backgroundBlur,backgroundDarken" {
		t.Errorf("keys = %v", keys)
	}
}

func TestDeveloperToolsAreOffByDefaultAndSurviveASave(t *testing.T) {
	if Defaults().DeveloperTools {
		t.Fatal("developer tools must be off on a fresh install")
	}
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	if err := os.WriteFile(path, []byte(`{"scanForNewMods": false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Load(path).DeveloperTools {
		t.Error("a file from before the setting existed must read as off")
	}

	p := Defaults()
	p.DeveloperTools = true
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	if !Load(path).DeveloperTools {
		t.Error("the setting was lost by a save and load")
	}
	if keys := ChangedKeys(Defaults(), p); len(keys) != 1 || keys[0] != "developerTools" {
		t.Errorf("ChangedKeys = %v, want [developerTools]", keys)
	}
}

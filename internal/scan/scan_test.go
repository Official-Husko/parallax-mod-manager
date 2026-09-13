package scan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func writeDescriptor(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestScanClassifiesAndParses(t *testing.T) {
	modDir := t.TempDir()

	writeDescriptor(t, modDir, "my_local_mod.mod", `
name = "My Local Mod"
path = "my_local_mod"
`)
	writeDescriptor(t, modDir, "ugc_1830063425.mod", `
name = "AI Species Limit"
path = "ugc_1830063425"
remote_file_id = "1830063425"
`)
	writeDescriptor(t, modDir, "pdx_00001.mod", `
name = "Paradox Launcher Mod"
path = "pdx_00001"
`)
	// Not a descriptor - must be ignored.
	writeDescriptor(t, modDir, "readme.txt", "not a descriptor")

	for _, dir := range []string{"my_local_mod", "ugc_1830063425", "pdx_00001"} {
		if err := os.MkdirAll(filepath.Join(modDir, dir), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected scan errors: %+v", result.Errors)
	}
	if len(result.Mods) != 3 {
		t.Fatalf("expected 3 mods, got %d: %+v", len(result.Mods), result.Mods)
	}

	byID := map[string]mod.Mod{}
	for _, m := range result.Mods {
		byID[m.ID] = m
	}

	local, ok := byID["my_local_mod"]
	if !ok {
		t.Fatalf("missing local mod, got IDs: %v", keysOf(byID))
	}
	if local.Source != mod.SourceLocal {
		t.Errorf("local mod Source = %v, want SourceLocal", local.Source)
	}
	if local.Descriptor.Name != "My Local Mod" {
		t.Errorf("local mod Name = %q", local.Descriptor.Name)
	}

	workshop, ok := byID["ugc_1830063425"]
	if !ok {
		t.Fatalf("missing workshop mod, got IDs: %v", keysOf(byID))
	}
	if workshop.Source != mod.SourceWorkshop {
		t.Errorf("workshop mod Source = %v, want SourceWorkshop", workshop.Source)
	}

	launcher, ok := byID["pdx_00001"]
	if !ok {
		t.Fatalf("missing paradox-launcher mod, got IDs: %v", keysOf(byID))
	}
	if launcher.Source != mod.SourceParadoxLauncher {
		t.Errorf("launcher mod Source = %v, want SourceParadoxLauncher", launcher.Source)
	}
}

func keysOf(m map[string]mod.Mod) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestScanEmptyModDirIsNotAnError(t *testing.T) {
	opts := Options{Game: game.Stellaris, ModDir: t.TempDir()}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 0 {
		t.Errorf("expected no mods, got %+v", result.Mods)
	}
}

func TestScanMissingModDirIsNotAnError(t *testing.T) {
	opts := Options{Game: game.Stellaris, ModDir: filepath.Join(t.TempDir(), "does-not-exist")}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 0 {
		t.Errorf("expected no mods, got %+v", result.Mods)
	}
}

// TestScanFlagsContentMissingWithFriendlyError pins a real bug found via a
// real, genuinely-stale local mod on a real machine: a descriptor whose
// "path" field points at a drive that isn't mounted under that name
// anymore (moved, renamed, or disconnected). Before this, the mod was
// scanned successfully with a ContentPath that didn't exist, and the
// problem only surfaced much later as a raw, unfriendly filesystem error
// (e.g. "lstat ...: no such file or directory") the first time something
// tried to actually read from it - one specific real case being the
// Workspace detail panel's Files tab. Scan now catches this itself, right
// where ContentPath is resolved, with one clear, human-readable message -
// and keeps the mod in Result.Mods (rather than dropping it) so it's still
// visible in the load order instead of silently vanishing.
func TestScanFlagsContentMissingWithFriendlyError(t *testing.T) {
	modDir := t.TempDir()
	writeDescriptor(t, modDir, "stale.mod", `name = "Stale Mod"
path = "/this/path/does/not/exist/stale_mod"`)

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 {
		t.Fatalf("expected the mod to still be listed despite its missing content, got %d mods: %+v", len(result.Mods), result.Mods)
	}
	if !result.Mods[0].ContentMissing {
		t.Error("ContentMissing = false, want true")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly 1 scan error, got %+v", result.Errors)
	}
	msg := result.Errors[0].Err.Error()
	if strings.Contains(msg, "lstat") || strings.Contains(msg, "no such file") {
		t.Errorf("error = %q, want a human-readable message, not a raw filesystem error", msg)
	}
	if strings.Contains(msg, "/this/path/does/not/exist") {
		t.Errorf("error = %q, want it to omit the raw absolute path - not useful to a user, and confusing after a drive gets remounted under a different name", msg)
	}
	if !strings.Contains(msg, "Stale Mod") {
		t.Errorf("error = %q, want it to name the mod", msg)
	}
}

func TestScanMalformedDescriptorIsNonFatal(t *testing.T) {
	modDir := t.TempDir()
	writeDescriptor(t, modDir, "good.mod", `name = "Good Mod"
path = "good"`)
	writeDescriptor(t, modDir, "bad.mod", `name = "unterminated string`)
	if err := os.MkdirAll(filepath.Join(modDir, "good"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 || result.Mods[0].ID != "good" {
		t.Errorf("Mods = %+v, want exactly [good]", result.Mods)
	}
	if len(result.Errors) != 1 || result.Errors[0].Path != filepath.Join(modDir, "bad.mod") {
		t.Errorf("Errors = %+v", result.Errors)
	}
}

func TestScanRelativeContentPathResolvedAgainstModDir(t *testing.T) {
	modDir := t.TempDir()
	writeDescriptor(t, modDir, "relative.mod", `name = "Relative"
path = "relative_content"`)

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(result.Mods))
	}
	want := filepath.Join(modDir, "relative_content")
	if result.Mods[0].ContentPath != want {
		t.Errorf("ContentPath = %q, want %q", result.Mods[0].ContentPath, want)
	}
}

// TestScanTriesEverySteamRootUntilOneResolvesWorkshopContent pins a real
// wiring bug: a user can have more than one Steam installation root (e.g.
// a native install and a Flatpak one), and the game's Workshop content can
// live in a library only one of those roots' own libraryfolders.vdf knows
// about. Scan must try every entry in SteamRoots, not just the first.
func TestScanTriesEverySteamRootUntilOneResolvesWorkshopContent(t *testing.T) {
	modDir := t.TempDir()
	writeDescriptor(t, modDir, "ugc_1830063425.mod", `
name = "AI Species Limit"
path = "ugc_1830063425"
remote_file_id = "1830063425"
`)

	// unrelatedRoot is a real Steam root, but has no library with this
	// game's Workshop content - resolving via it alone must fail.
	unrelatedRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(unrelatedRoot, "steamapps"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// realRoot is the one that actually has it.
	realRoot := t.TempDir()
	steamapps := filepath.Join(realRoot, "steamapps")
	if err := os.MkdirAll(steamapps, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	workshopDir := filepath.Join(steamapps, "workshop", "content", "281990")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	itemDir := filepath.Join(workshopDir, "1830063425")
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	vdf := `"libraryfolders"
{
	"0"
	{
		"path"		"` + realRoot + `"
		"apps"
		{
			"281990"		"1"
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(steamapps, "libraryfolders.vdf"), []byte(vdf), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	opts := Options{Game: game.Stellaris, ModDir: modDir, SteamRoots: []string{unrelatedRoot, realRoot}}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 {
		t.Fatalf("expected 1 mod, got %d: %+v", len(result.Mods), result.Mods)
	}
	if result.Mods[0].ContentPath != itemDir {
		t.Errorf("ContentPath = %q, want %q (resolution must fall through to the second SteamRoot)", result.Mods[0].ContentPath, itemDir)
	}
}

// setUpWorkshopDir creates a Steam root with one library that has appID's
// Workshop content directory, containing one item folder (itemID) with its
// own self-contained descriptor.mod. Returns the Steam root to pass as
// Options.SteamRoots, and the item's content directory.
func setUpWorkshopDir(t *testing.T, appID, itemID, descriptorBody string) (steamRoot, itemDir string) {
	t.Helper()
	steamRoot = t.TempDir()
	steamapps := filepath.Join(steamRoot, "steamapps")
	if err := os.MkdirAll(steamapps, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	workshopDir := filepath.Join(steamapps, "workshop", "content", appID)
	itemDir = filepath.Join(workshopDir, itemID)
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeDescriptor(t, itemDir, "descriptor.mod", descriptorBody)

	vdf := `"libraryfolders"
{
	"0"
	{
		"path"		"` + steamRoot + `"
		"apps"
		{
			"` + appID + `"		"1"
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(steamapps, "libraryfolders.vdf"), []byte(vdf), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return steamRoot, itemDir
}

func TestScanDiscoversUnlinkedWorkshopItem(t *testing.T) {
	modDir := t.TempDir() // no mod/ugc_*.mod stub at all
	steamRoot, itemDir := setUpWorkshopDir(t, "281990", "1121692237", `
name="Gigastructural Engineering & More"
version="3.39.4"
supported_version="v4.4.*"
remote_file_id="1121692237"
tags={
	"Gameplay"
}
`)

	opts := Options{Game: game.Stellaris, ModDir: modDir, SteamRoots: []string{steamRoot}}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 {
		t.Fatalf("expected 1 mod, got %d: %+v", len(result.Mods), result.Mods)
	}

	m := result.Mods[0]
	if m.ID != "ugc_1121692237" {
		t.Errorf("ID = %q, want %q", m.ID, "ugc_1121692237")
	}
	if m.Source != mod.SourceWorkshop {
		t.Errorf("Source = %v, want SourceWorkshop", m.Source)
	}
	if m.ContentPath != itemDir {
		t.Errorf("ContentPath = %q, want %q", m.ContentPath, itemDir)
	}
	if m.Descriptor.Name != "Gigastructural Engineering & More" {
		t.Errorf("Name = %q", m.Descriptor.Name)
	}
}

func TestScanDoesNotDuplicateAlreadyLinkedWorkshopItem(t *testing.T) {
	modDir := t.TempDir()
	steamRoot, itemDir := setUpWorkshopDir(t, "281990", "1830063425", `
name="AI Species Limit"
remote_file_id="1830063425"
`)
	// A stub already links this exact item.
	writeDescriptor(t, modDir, "ugc_1830063425.mod", `
name="AI Species Limit"
path="`+itemDir+`"
remote_file_id="1830063425"
`)

	opts := Options{Game: game.Stellaris, ModDir: modDir, SteamRoots: []string{steamRoot}}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 {
		t.Fatalf("expected exactly 1 mod (no duplicate), got %d: %+v", len(result.Mods), result.Mods)
	}
}

func TestScanIgnoresUnlinkedWorkshopItemsForJSONDescriptorGames(t *testing.T) {
	modDir := t.TempDir()
	steamRoot, _ := setUpWorkshopDir(t, "281990", "1121692237", `
name="Something"
remote_file_id="1121692237"
`)

	jsonGame := game.Stellaris
	jsonGame.DescriptorType = mod.DescriptorJSONv1

	opts := Options{Game: jsonGame, ModDir: modDir, SteamRoots: []string{steamRoot}}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 0 {
		t.Fatalf("expected 0 mods (unlinked-Workshop discovery is classic-only), got %d: %+v", len(result.Mods), result.Mods)
	}
}

func TestScanMissingModDirStillDiscoversUnlinkedWorkshopItems(t *testing.T) {
	modDir := filepath.Join(t.TempDir(), "does-not-exist")
	steamRoot, itemDir := setUpWorkshopDir(t, "281990", "1121692237", `
name="Something"
remote_file_id="1121692237"
`)

	opts := Options{Game: game.Stellaris, ModDir: modDir, SteamRoots: []string{steamRoot}}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 || result.Mods[0].ContentPath != itemDir {
		t.Fatalf("expected 1 mod resolved to %q, got %+v", itemDir, result.Mods)
	}
}

func TestEnsureWorkshopStubWritesMissingStub(t *testing.T) {
	modDir := t.TempDir()
	itemDir := t.TempDir()
	m := mod.Mod{
		ID:          "ugc_1121692237",
		Source:      mod.SourceWorkshop,
		ContentPath: itemDir,
		Descriptor: mod.Descriptor{
			Name:         "Gigastructural Engineering & More",
			RemoteFileID: "1121692237",
			Tags:         []string{"Gameplay"},
		},
	}

	wrote, err := EnsureWorkshopStub(m, modDir)
	if err != nil {
		t.Fatalf("EnsureWorkshopStub: %v", err)
	}
	if !wrote {
		t.Fatal("expected EnsureWorkshopStub to report it wrote a file")
	}

	stubPath := filepath.Join(modDir, "ugc_1121692237.mod")
	data, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	desc, err := mod.ParseDescriptor(data, mod.DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor(written stub): %v", err)
	}
	if desc.Path != itemDir {
		t.Errorf("written stub Path = %q, want %q", desc.Path, itemDir)
	}
	if desc.Name != m.Descriptor.Name {
		t.Errorf("written stub Name = %q, want %q", desc.Name, m.Descriptor.Name)
	}
}

func TestEnsureWorkshopStubNeverOverwritesExisting(t *testing.T) {
	modDir := t.TempDir()
	stubPath := filepath.Join(modDir, "ugc_1121692237.mod")
	writeDescriptor(t, modDir, "ugc_1121692237.mod", `name="Original, written by Steam"`)

	m := mod.Mod{
		ID:          "ugc_1121692237",
		Source:      mod.SourceWorkshop,
		ContentPath: t.TempDir(),
		Descriptor:  mod.Descriptor{Name: "Should not be written", RemoteFileID: "1121692237"},
	}
	wrote, err := EnsureWorkshopStub(m, modDir)
	if err != nil {
		t.Fatalf("EnsureWorkshopStub: %v", err)
	}
	if wrote {
		t.Fatal("expected EnsureWorkshopStub to report it did NOT write (stub already existed)")
	}

	data, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := string(data); got != `name="Original, written by Steam"` {
		t.Errorf("existing stub was modified: %q", got)
	}
}

func TestEnsureWorkshopStubNoopForNonWorkshopMod(t *testing.T) {
	modDir := t.TempDir()
	m := mod.Mod{ID: "my_local_mod", Source: mod.SourceLocal, Descriptor: mod.Descriptor{Name: "Local"}}
	wrote, err := EnsureWorkshopStub(m, modDir)
	if err != nil {
		t.Fatalf("EnsureWorkshopStub: %v", err)
	}
	if wrote {
		t.Fatal("expected no-op for a non-Workshop mod")
	}
	entries, _ := os.ReadDir(modDir)
	if len(entries) != 0 {
		t.Errorf("expected no file written, got %+v", entries)
	}
}

func TestScanContextCancellation(t *testing.T) {
	modDir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeDescriptor(t, modDir, string(rune('a'+i))+".mod", `name = "x"
path = "x"`)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	_, err := Scan(ctx, opts)
	if err == nil {
		t.Fatal("expected Scan to report the cancelled context")
	}
}

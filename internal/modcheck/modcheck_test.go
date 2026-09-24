package modcheck

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

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

func testGame(id string) game.GameConfig {
	return game.GameConfig{ID: id, ScanFolders: []string{"common", "events", "localisation"}}
}

func findingsOf(t *testing.T, findings []Finding, cat Category) []Finding {
	t.Helper()
	var out []Finding
	for _, f := range findings {
		if f.Category == cat {
			out = append(out, f)
		}
	}
	return out
}

func TestCheckFindsSyntaxError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "common/buildings/00_buildings.txt", `some_building = { cost = 100`) // unclosed brace
	m := mod.Mod{ID: "broken_mod", ContentPath: dir}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: t.TempDir()}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	syntax := findingsOf(t, findings, CategorySyntax)
	if len(syntax) != 1 {
		t.Fatalf("expected 1 syntax finding, got %d: %+v", len(syntax), findings)
	}
	if syntax[0].File != filepath.Join("common", "buildings", "00_buildings.txt") {
		t.Errorf("unexpected file: %q", syntax[0].File)
	}
	if syntax[0].Severity != SeverityError {
		t.Errorf("expected error severity, got %q", syntax[0].Severity)
	}
	if syntax[0].Line == 0 {
		t.Error("expected a real line number, got 0")
	}
}

func TestCheckFindsSkippedLocalisationLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "localisation/english/l_test.yml", "l_english:\n GOOD:0 \"fine\"\n not a valid entry\n")
	m := mod.Mod{ID: "loc_mod", ContentPath: dir}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: t.TempDir()}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	syntax := findingsOf(t, findings, CategorySyntax)
	if len(syntax) != 1 {
		t.Fatalf("expected 1 syntax finding for the skipped line, got %d: %+v", len(syntax), findings)
	}
	if syntax[0].Severity != SeverityWarn {
		t.Errorf("expected warn severity for a skipped line, got %q", syntax[0].Severity)
	}
}

func TestCheckCleanModHasNoSyntaxFindings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "common/buildings/00_buildings.txt", `some_building = { cost = 100 }`)
	m := mod.Mod{ID: "clean_mod", ContentPath: dir}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: t.TempDir()}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if syntax := findingsOf(t, findings, CategorySyntax); len(syntax) != 0 {
		t.Errorf("expected no syntax findings, got %+v", syntax)
	}
}

func TestCheckBaseGameConflictOnDifferingContent(t *testing.T) {
	install := t.TempDir()
	writeFile(t, install, "common/buildings/00_buildings.txt", `building_capital = { cost = 100 }`)

	modDir := t.TempDir()
	writeFile(t, modDir, "common/buildings/00_buildings.txt", `building_capital = { cost = 999 }`)

	m := mod.Mod{ID: "overwrite_mod", ContentPath: modDir}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{
		Store:      cache.FileStore{Dir: t.TempDir()},
		InstallDir: install,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	baseGame := findingsOf(t, findings, CategoryBaseGame)
	if len(baseGame) != 1 {
		t.Fatalf("expected 1 base-game finding, got %d: %+v", len(baseGame), findings)
	}
	if baseGame[0].Severity != SeverityWarn {
		t.Errorf("expected warn severity, got %q", baseGame[0].Severity)
	}
}

func TestCheckNoBaseGameConflictOnIdenticalContent(t *testing.T) {
	install := t.TempDir()
	writeFile(t, install, "common/buildings/00_buildings.txt", `building_capital = { cost = 100 }`)

	modDir := t.TempDir()
	writeFile(t, modDir, "common/buildings/00_buildings.txt", `building_capital = { cost = 100 }`)

	m := mod.Mod{ID: "same_mod", ContentPath: modDir}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{
		Store:      cache.FileStore{Dir: t.TempDir()},
		InstallDir: install,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if baseGame := findingsOf(t, findings, CategoryBaseGame); len(baseGame) != 0 {
		t.Errorf("identical content should not be flagged as a conflict, got %+v", baseGame)
	}
}

func TestCheckNoBaseGameConflictOnNewContent(t *testing.T) {
	install := t.TempDir()
	writeFile(t, install, "common/buildings/00_buildings.txt", `building_capital = { cost = 100 }`)

	modDir := t.TempDir()
	writeFile(t, modDir, "common/buildings/01_new_building.txt", `my_new_building = { cost = 50 }`)

	m := mod.Mod{ID: "new_content_mod", ContentPath: modDir}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{
		Store:      cache.FileStore{Dir: t.TempDir()},
		InstallDir: install,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if baseGame := findingsOf(t, findings, CategoryBaseGame); len(baseGame) != 0 {
		t.Errorf("a building the base game doesn't have should not be flagged, got %+v", baseGame)
	}
}

func TestCheckSkipsBaseGameWhenInstallDirEmpty(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "common/buildings/00_buildings.txt", `some_building = { cost = 100 }`)
	m := mod.Mod{ID: "no_install_mod", ContentPath: modDir}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: t.TempDir()}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if baseGame := findingsOf(t, findings, CategoryBaseGame); len(baseGame) != 0 {
		t.Errorf("expected no base-game findings when InstallDir is empty, got %+v", baseGame)
	}
}

func TestCheckReturnsHowManyFilesItExamined(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "common/buildings/00_buildings.txt", `some_building = { cost = 100 }`)
	writeFile(t, dir, "events/some_events.txt", `namespace = some`)
	m := mod.Mod{ID: "two_file_mod", ContentPath: dir}
	cfg := testGame("stellaris")
	store := cache.FileStore{Dir: t.TempDir()}

	_, filesRead, err := Check(context.Background(), m, cfg, Options{Store: store})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if filesRead != 2 {
		t.Errorf("filesRead = %d, want 2", filesRead)
	}

	// A warm second run examines the same two files, even though nothing needed re-parsing.
	_, filesRead, err = Check(context.Background(), m, cfg, Options{Store: store})
	if err != nil {
		t.Fatalf("Check (warm): %v", err)
	}
	if filesRead != 2 {
		t.Errorf("filesRead on a warm run = %d, want 2", filesRead)
	}
}

func TestCheckDescriptorMissingSupportedVersion(t *testing.T) {
	dir := t.TempDir()
	m := mod.Mod{ID: "m", ContentPath: dir, DescriptorPath: filepath.Join(dir, "descriptor.mod")}
	findings := checkDescriptor(m)
	if len(findings) != 1 || findings[0].Severity != SeverityWarn {
		t.Fatalf("expected 1 warn finding for a missing supported_version, got %+v", findings)
	}
}

func TestCheckDescriptorBadlyShapedSupportedVersion(t *testing.T) {
	dir := t.TempDir()
	m := mod.Mod{
		ID: "m", ContentPath: dir, DescriptorPath: filepath.Join(dir, "descriptor.mod"),
		Descriptor: mod.Descriptor{SupportedVersion: "not-a-version"},
	}
	findings := checkDescriptor(m)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for a badly shaped supported_version, got %+v", findings)
	}
}

func TestCheckDescriptorMissingPicture(t *testing.T) {
	dir := t.TempDir()
	m := mod.Mod{
		ID: "m", ContentPath: dir, DescriptorPath: filepath.Join(dir, "descriptor.mod"),
		Descriptor: mod.Descriptor{SupportedVersion: "v4.*", Picture: "thumbnail.png"},
	}
	findings := checkDescriptor(m)
	if len(findings) != 1 || findings[0].Severity != SeverityError {
		t.Fatalf("expected 1 error finding for a missing picture file, got %+v", findings)
	}
}

func TestCheckDescriptorCleanHasNoFindings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "thumbnail.png", "not a real png, existence is all that's checked")
	m := mod.Mod{
		ID: "m", ContentPath: dir, DescriptorPath: filepath.Join(dir, "descriptor.mod"),
		Descriptor: mod.Descriptor{SupportedVersion: "v4.*", Picture: "thumbnail.png"},
	}
	if findings := checkDescriptor(m); len(findings) != 0 {
		t.Errorf("expected no findings for a clean descriptor, got %+v", findings)
	}
}

func TestCheckDependenciesFlagsUnknown(t *testing.T) {
	m := mod.Mod{
		ID:             "m",
		DescriptorPath: "/mods/m/descriptor.mod",
		Descriptor:     mod.Descriptor{Dependencies: []string{"Installed Mod", "Missing Mod"}},
	}
	findings := checkDependencies(m, []string{"Installed Mod"})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for the missing dependency, got %+v", findings)
	}
	if findings[0].Severity != SeverityInfo {
		t.Errorf("expected info severity, got %q", findings[0].Severity)
	}
}

func TestCheckDependenciesNoneMissing(t *testing.T) {
	m := mod.Mod{
		ID:             "m",
		DescriptorPath: "/mods/m/descriptor.mod",
		Descriptor:     mod.Descriptor{Dependencies: []string{"Installed Mod"}},
	}
	if findings := checkDependencies(m, []string{"Installed Mod"}); len(findings) != 0 {
		t.Errorf("expected no findings, got %+v", findings)
	}
}

func TestCheckCategoryOrder(t *testing.T) {
	install := t.TempDir()
	writeFile(t, install, "common/buildings/00_buildings.txt", `building_capital = { cost = 100 }`)

	modDir := t.TempDir()
	writeFile(t, modDir, "common/buildings/00_buildings.txt", `building_capital = { cost = 999`) // conflicts AND fails to parse
	m := mod.Mod{
		ID: "messy_mod", ContentPath: modDir, DescriptorPath: filepath.Join(modDir, "descriptor.mod"),
		Descriptor: mod.Descriptor{Dependencies: []string{"Missing Mod"}},
	}
	cfg := testGame("stellaris")

	findings, _, err := Check(context.Background(), m, cfg, Options{
		Store:          cache.FileStore{Dir: t.TempDir()},
		InstallDir:     install,
		InstalledNames: nil,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// The malformed file fails to parse, so it never contributes a
	// Definition - no base-game conflict is possible for it, only a syntax
	// finding. What matters here is that descriptor and dependency findings
	// still appear, in the fixed category order (syntax findings can't
	// out-order later categories, whatever order sortFindings puts them in).
	wantOrder := []Category{CategorySyntax, CategoryDescriptor, CategoryDependency}
	var gotOrder []Category
	for _, f := range findings {
		if len(gotOrder) == 0 || gotOrder[len(gotOrder)-1] != f.Category {
			gotOrder = append(gotOrder, f.Category)
		}
	}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("expected categories in order %v, got %v (findings: %+v)", wantOrder, gotOrder, findings)
	}
	for i, c := range wantOrder {
		if gotOrder[i] != c {
			t.Errorf("expected categories in order %v, got %v", wantOrder, gotOrder)
			break
		}
	}
}

func TestCheckWritesAReusableCache(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "common/buildings/00_buildings.txt", `some_building = { cost = 100 }`)
	m := mod.Mod{ID: "cached_mod", ContentPath: dir}
	cfg := testGame("stellaris")
	storeDir := t.TempDir()

	if _, _, err := Check(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: storeDir}}); err != nil {
		t.Fatalf("Check: %v", err)
	}

	cacheFile := filepath.Join(storeDir, "stellaris", "cached_mod.gobcache")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("expected Check to leave a reusable parse cache at %s: %v", cacheFile, err)
	}
}

// TestCheckReusesCacheOnSecondRun confirms Check goes through the same
// incremental cache a routine mod scan uses (see pipeline.LoadMod), not a
// bespoke always-fresh parse: a second Check of an unchanged mod must not
// need to re-read its files at all, proven here by revoking read permission
// on the one file this mod has between the two calls - a Check that tried to
// re-read it would fail, one that reused the cached (mtime, size) hit
// wouldn't.
func TestCheckReusesCacheOnSecondRun(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores file permissions")
	}
	dir := t.TempDir()
	writeFile(t, dir, "common/buildings/00_buildings.txt", `some_building = { cost = 100 }`)
	filePath := filepath.Join(dir, "common", "buildings", "00_buildings.txt")

	m := mod.Mod{ID: "reuse_mod", ContentPath: dir}
	cfg := testGame("stellaris")
	store := cache.FileStore{Dir: t.TempDir()}

	if _, _, err := Check(context.Background(), m, cfg, Options{Store: store}); err != nil {
		t.Fatalf("first Check: %v", err)
	}

	if err := os.Chmod(filePath, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(filePath, 0o644)

	if _, _, err := Check(context.Background(), m, cfg, Options{Store: store}); err != nil {
		t.Fatalf("second Check should have reused the cache without re-reading the file, got: %v", err)
	}
}

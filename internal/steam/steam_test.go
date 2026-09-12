package steam

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const twoLibraryVDF = `"libraryfolders"
{
	"0"
	{
		"path"		"/home/user/.steam/steam"
		"label"		""
		"apps"
		{
			"281990"		"12345678"
			"394360"		"1111111"
		}
	}
	"1"
	{
		"path"		"/mnt/games/SteamLibrary"
		"apps"
		{
			"281990"		"12345678"
		}
	}
}
`

func TestParseLibraryFolders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libraryfolders.vdf")
	if err := os.WriteFile(path, []byte(twoLibraryVDF), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	libs, err := ParseLibraryFolders(path)
	if err != nil {
		t.Fatalf("ParseLibraryFolders: %v", err)
	}
	if len(libs) != 2 {
		t.Fatalf("expected 2 libraries, got %d", len(libs))
	}

	if libs[0].Path != "/home/user/.steam/steam" {
		t.Errorf("libs[0].Path = %q", libs[0].Path)
	}
	if _, ok := libs[0].Apps["281990"]; !ok {
		t.Errorf("libs[0] missing app 281990: %v", libs[0].Apps)
	}
	if _, ok := libs[0].Apps["394360"]; !ok {
		t.Errorf("libs[0] missing app 394360: %v", libs[0].Apps)
	}

	if libs[1].Path != "/mnt/games/SteamLibrary" {
		t.Errorf("libs[1].Path = %q", libs[1].Path)
	}
	if _, ok := libs[1].Apps["281990"]; !ok {
		t.Errorf("libs[1] missing app 281990: %v", libs[1].Apps)
	}
	if _, ok := libs[1].Apps["394360"]; ok {
		t.Errorf("libs[1] should not have app 394360: %v", libs[1].Apps)
	}
}

func TestParseLibraryFoldersMissingFile(t *testing.T) {
	_, err := ParseLibraryFolders(filepath.Join(t.TempDir(), "does-not-exist.vdf"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestParseLibraryFoldersMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libraryfolders.vdf")
	if err := os.WriteFile(path, []byte(`"libraryfolders" { "0" { "path" `), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ParseLibraryFolders(path)
	if err == nil {
		t.Fatal("expected an error for malformed VDF")
	}
}

func TestFindWorkshopContentDir(t *testing.T) {
	steamRoot := t.TempDir()
	steamapps := filepath.Join(steamRoot, "steamapps")
	if err := os.MkdirAll(steamapps, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// The library that actually has the app's workshop content on disk is
	// the second one - resolution must not just take the first match.
	otherLibRoot := t.TempDir()
	workshopDir := filepath.Join(otherLibRoot, "steamapps", "workshop", "content", "281990")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	vdf := `"libraryfolders"
{
	"0"
	{
		"path"		"` + steamRoot + `"
		"apps"
		{
			"281990"		"1"
		}
	}
	"1"
	{
		"path"		"` + otherLibRoot + `"
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

	got, err := FindWorkshopContentDir(steamRoot, "281990")
	if err != nil {
		t.Fatalf("FindWorkshopContentDir: %v", err)
	}
	if got != workshopDir {
		t.Errorf("FindWorkshopContentDir = %q, want %q", got, workshopDir)
	}
}

func TestFindWorkshopContentDirNoMatchingApp(t *testing.T) {
	steamRoot := t.TempDir()
	steamapps := filepath.Join(steamRoot, "steamapps")
	if err := os.MkdirAll(steamapps, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	vdf := `"libraryfolders" { "0" { "path" "` + steamRoot + `" "apps" { "999999" "1" } } }`
	if err := os.WriteFile(filepath.Join(steamapps, "libraryfolders.vdf"), []byte(vdf), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := FindWorkshopContentDir(steamRoot, "281990")
	if err == nil {
		t.Fatal("expected an error when no library has the requested app")
	}
}

func TestDefaultRootsOnlyReturnsExistingCandidates(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if roots := DefaultRoots(); len(roots) != 0 {
		t.Errorf("DefaultRoots on an empty home = %v, want none", roots)
	}

	steamRoot := filepath.Join(home, ".steam", "steam")
	if err := os.MkdirAll(steamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	roots := DefaultRoots()
	if len(roots) != 1 || roots[0] != steamRoot {
		t.Errorf("DefaultRoots = %v, want [%q]", roots, steamRoot)
	}
}

func writeAppManifest(t *testing.T, steamappsDir, appID, installDirName string) {
	t.Helper()
	if err := os.MkdirAll(steamappsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Real Steam appmanifest format (VDF despite the .acf extension) -
	// confirmed against an actual appmanifest_281990.acf on a real
	// Stellaris install.
	acf := `"AppState"
{
	"appid"		"` + appID + `"
	"name"		"Test Game"
	"installdir"		"` + installDirName + `"
}
`
	path := filepath.Join(steamappsDir, "appmanifest_"+appID+".acf")
	if err := os.WriteFile(path, []byte(acf), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestFindGameInstallDirChecksRootsOwnCommonFolder(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	steamRoot := filepath.Join(home, ".steam", "steam")
	steamapps := filepath.Join(steamRoot, "steamapps")
	writeAppManifest(t, steamapps, "281990", "Stellaris")
	installDir := filepath.Join(steamapps, "common", "Stellaris")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := FindGameInstallDir("281990")
	if err != nil {
		t.Fatalf("FindGameInstallDir: %v", err)
	}
	if got != installDir {
		t.Errorf("FindGameInstallDir = %q, want %q", got, installDir)
	}
}

func TestFindGameInstallDirUsesManifestInstallDirNotAGuess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	// The manifest's installdir ("TotallyDifferentFolder") deliberately
	// doesn't match anything a caller might guess from a display name or a
	// Paradox user-data folder name - FindGameInstallDir must trust only
	// the manifest, never a name-based guess.
	steamRoot := filepath.Join(home, ".steam", "steam")
	steamapps := filepath.Join(steamRoot, "steamapps")
	writeAppManifest(t, steamapps, "859580", "TotallyDifferentFolder")
	installDir := filepath.Join(steamapps, "common", "TotallyDifferentFolder")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := FindGameInstallDir("859580")
	if err != nil {
		t.Fatalf("FindGameInstallDir: %v", err)
	}
	if got != installDir {
		t.Errorf("FindGameInstallDir = %q, want %q", got, installDir)
	}
}

func TestFindGameInstallDirChecksSecondaryLibraries(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	steamRoot := filepath.Join(home, ".steam", "steam")
	steamapps := filepath.Join(steamRoot, "steamapps")
	if err := os.MkdirAll(steamapps, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	secondaryLib := t.TempDir()
	secondarySteamapps := filepath.Join(secondaryLib, "steamapps")
	writeAppManifest(t, secondarySteamapps, "281990", "Stellaris")
	installDir := filepath.Join(secondarySteamapps, "common", "Stellaris")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	vdf := `"libraryfolders" { "0" { "path" "` + secondaryLib + `" "apps" { "281990" "1" } } }`
	if err := os.WriteFile(filepath.Join(steamapps, "libraryfolders.vdf"), []byte(vdf), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := FindGameInstallDir("281990")
	if err != nil {
		t.Fatalf("FindGameInstallDir: %v", err)
	}
	if got != installDir {
		t.Errorf("FindGameInstallDir = %q, want %q", got, installDir)
	}
}

func TestFindGameInstallDirErrorsWhenNotFound(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := FindGameInstallDir("281990"); err == nil {
		t.Fatal("expected an error when no default root exists at all")
	}
}

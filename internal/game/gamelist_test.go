package game

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func validGameListJSON(id string) string {
	return `{
		"version": "1.0.0",
		"required_parallax_version": "1.0.0",
		"source": "https://example.com/games.jsonc",
		"games": [
			{
				"id": "` + id + `",
				"name": "Test Game",
				"app_id": "12345",
				"folder_name": "Test Game",
				"descriptor_type": "classic",
				"launcher_settings_path": "launcher-settings.json",
				"signature_files": ["launcher-settings.json"],
				"scan_folders": ["common"],
				"dlc": []
			}
		]
	}`
}

const testGameID = "01a0968a-14d2-7000-8000-000000000001"

func TestParseGameListValidFile(t *testing.T) {
	f, err := parseGameList([]byte(validGameListJSON(testGameID)))
	if err != nil {
		t.Fatalf("parseGameList: %v", err)
	}
	if len(f.Games) != 1 || f.Games[0].ID != testGameID {
		t.Errorf("Games = %+v", f.Games)
	}
}

func TestParseGameListRejectsInvalidUUID(t *testing.T) {
	_, err := parseGameList([]byte(validGameListJSON("not-a-uuid")))
	if err == nil {
		t.Fatal("expected an error for an invalid id")
	}
}

func TestParseGameListRejectsDuplicateIDs(t *testing.T) {
	data := `{
		"version": "1.0.0", "required_parallax_version": "1.0.0", "source": "",
		"games": [
			{"id": "` + testGameID + `", "name": "A", "app_id": "1", "folder_name": "A",
			 "descriptor_type": "classic", "launcher_settings_path": "launcher-settings.json",
			 "signature_files": ["launcher-settings.json"], "scan_folders": ["common"]},
			{"id": "` + testGameID + `", "name": "B", "app_id": "2", "folder_name": "B",
			 "descriptor_type": "classic", "launcher_settings_path": "launcher-settings.json",
			 "signature_files": ["launcher-settings.json"], "scan_folders": ["common"]}
		]
	}`
	_, err := parseGameList([]byte(data))
	if err == nil {
		t.Fatal("expected an error for duplicate ids")
	}
}

func TestParseGameListRejectsDuplicateAppIDs(t *testing.T) {
	data := `{
		"version": "1.0.0", "required_parallax_version": "1.0.0", "source": "",
		"games": [
			{"id": "01a0968a-14d2-7000-8000-000000000005", "name": "A", "app_id": "999", "folder_name": "A",
			 "descriptor_type": "classic", "launcher_settings_path": "launcher-settings.json",
			 "signature_files": ["launcher-settings.json"], "scan_folders": ["common"]},
			{"id": "01a0968a-14d2-7000-8000-000000000006", "name": "B", "app_id": "999", "folder_name": "B",
			 "descriptor_type": "classic", "launcher_settings_path": "launcher-settings.json",
			 "signature_files": ["launcher-settings.json"], "scan_folders": ["common"]}
		]
	}`
	_, err := parseGameList([]byte(data))
	if err == nil {
		t.Fatal("expected an error for duplicate app_ids")
	}
}

func TestParseGameListRejectsUnknownDescriptorType(t *testing.T) {
	data := `{
		"version": "1.0.0", "required_parallax_version": "1.0.0", "source": "",
		"games": [{"id": "` + testGameID + `", "name": "A", "app_id": "1", "folder_name": "A",
			"descriptor_type": "made_up", "launcher_settings_path": "launcher-settings.json",
			"signature_files": ["launcher-settings.json"], "scan_folders": ["common"]}]
	}`
	_, err := parseGameList([]byte(data))
	if err == nil {
		t.Fatal("expected an error for an unknown descriptor_type")
	}
}

func TestParseGameListRejectsMissingRequiredField(t *testing.T) {
	data := `{
		"version": "1.0.0", "required_parallax_version": "1.0.0", "source": "",
		"games": [{"id": "` + testGameID + `", "name": "A", "app_id": "",  "folder_name": "A",
			"descriptor_type": "classic", "launcher_settings_path": "launcher-settings.json",
			"signature_files": ["launcher-settings.json"], "scan_folders": ["common"]}]
	}`
	_, err := parseGameList([]byte(data))
	if err == nil {
		t.Fatal("expected an error for an empty app_id")
	}
}

func TestParseGameListRejectsEmptyList(t *testing.T) {
	_, err := parseGameList([]byte(`{"version":"1.0.0","required_parallax_version":"1.0.0","source":"","games":[]}`))
	if err == nil {
		t.Fatal("expected an error for a games list with no entries")
	}
}

func TestVersionSatisfies(t *testing.T) {
	cases := []struct {
		have, want string
		want_      bool
	}{
		{"1.0.0", "1.0.0", true},
		{"1.2.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"2.0.0", "1.9.9", true},
		{"0.9.0", "1.0.0", false},
	}
	for _, c := range cases {
		if got := versionSatisfies(c.have, c.want); got != c.want_ {
			t.Errorf("versionSatisfies(%q, %q) = %v, want %v", c.have, c.want, got, c.want_)
		}
	}
}

func TestLoadRegistryUsesEmbeddedDefaultWhenNoDiskOverride(t *testing.T) {
	registry, notice, err := LoadRegistry("1.0.0", []byte(validGameListJSON(testGameID)), "")
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
	if _, ok := registry.Get(testGameID); !ok {
		t.Error("expected the embedded default's game to be registered")
	}
}

func TestLoadRegistryUsesEmbeddedDefaultWhenDiskFileMissing(t *testing.T) {
	registry, notice, err := LoadRegistry("1.0.0", []byte(validGameListJSON(testGameID)), filepath.Join(t.TempDir(), "does-not-exist.jsonc"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty (a missing override file is normal)", notice)
	}
	if _, ok := registry.Get(testGameID); !ok {
		t.Error("expected the embedded default's game to be registered")
	}
}

func TestLoadRegistryPrefersValidDiskOverride(t *testing.T) {
	overrideID := "01a0968a-14d2-7000-8000-000000000002"
	path := filepath.Join(t.TempDir(), "games.jsonc")
	if err := os.WriteFile(path, []byte(validGameListJSON(overrideID)), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registry, notice, err := LoadRegistry("1.0.0", []byte(validGameListJSON(testGameID)), path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
	if _, ok := registry.Get(overrideID); !ok {
		t.Error("expected the override file's game to be registered")
	}
	if _, ok := registry.Get(testGameID); ok {
		t.Error("expected the embedded default's game NOT to be registered when a valid override is used")
	}
}

func TestLoadRegistryFallsBackWhenDiskOverrideRequiresNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.jsonc")
	data := `{
		"version": "1.0.0", "required_parallax_version": "9.9.9", "source": "",
		"games": [{"id": "` + testGameID + `", "name": "A", "app_id": "1", "folder_name": "A",
			"descriptor_type": "classic", "launcher_settings_path": "launcher-settings.json",
			"signature_files": ["launcher-settings.json"], "scan_folders": ["common"]}]
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	defaultID := "01a0968a-14d2-7000-8000-000000000003"
	registry, notice, err := LoadRegistry("1.0.0", []byte(validGameListJSON(defaultID)), path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if notice == "" {
		t.Error("expected a non-empty notice when the override requires a newer app version")
	}
	if _, ok := registry.Get(defaultID); !ok {
		t.Error("expected the embedded default to be used as the fallback")
	}
}

func TestLoadRegistryFallsBackWhenDiskOverrideIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.jsonc")
	if err := os.WriteFile(path, []byte("{not valid jsonc"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	defaultID := "01a0968a-14d2-7000-8000-000000000004"
	registry, notice, err := LoadRegistry("1.0.0", []byte(validGameListJSON(defaultID)), path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if notice == "" {
		t.Error("expected a non-empty notice when the override file is corrupt")
	}
	if _, ok := registry.Get(defaultID); !ok {
		t.Error("expected the embedded default to be used as the fallback")
	}
}

func TestLoadRegistryErrorsWhenEmbeddedDefaultIsInvalid(t *testing.T) {
	_, _, err := LoadRegistry("1.0.0", []byte("not even json"), "")
	if err == nil {
		t.Fatal("expected an error when the embedded default itself is invalid")
	}
}

// TestRealGamesListFileIsValidAndMatchesTheFixture parses the actual
// data/games.jsonc this app ships (not a fixture) and checks its Stellaris
// entry matches the Stellaris test fixture above - the fixture exists only
// because internal/game can't go:embed a path outside its own directory,
// so nothing else keeps it honest against the real file.
func TestRealGamesListFileIsValidAndMatchesTheFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "data", "games.jsonc"))
	if err != nil {
		t.Fatalf("reading the real data/games.jsonc: %v", err)
	}
	registry, err := func() (*Registry, error) {
		f, err := parseGameList(data)
		if err != nil {
			return nil, err
		}
		return f.toRegistry()
	}()
	if err != nil {
		t.Fatalf("data/games.jsonc is invalid: %v", err)
	}

	got, ok := registry.Get(Stellaris.ID)
	if !ok {
		t.Fatalf("data/games.jsonc has no entry for the Stellaris fixture's id %s", Stellaris.ID)
	}
	if !reflect.DeepEqual(got, Stellaris) {
		t.Errorf("data/games.jsonc's Stellaris entry = %+v,\nwant (fixture) %+v", got, Stellaris)
	}
}

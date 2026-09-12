package game

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// gameListFile mirrors data/games.jsonc's shape - see docs/game-configuration.md.
// Snake_case JSON field names match the user-editable file's own style,
// deliberately not this project's usual camelCase JSON convention.
type gameListFile struct {
	Version                 string          `json:"version"`
	RequiredParallaxVersion string          `json:"required_parallax_version"`
	Source                  string          `json:"source"`
	Games                   []gameListEntry `json:"games"`
}

type gameListEntry struct {
	ID                   string                  `json:"id"`
	Name                 string                  `json:"name"`
	AppID                string                  `json:"app_id"`
	FolderName           string                  `json:"folder_name"`
	DescriptorType       string                  `json:"descriptor_type"`
	LauncherSettingsPath string                  `json:"launcher_settings_path"`
	SignatureFiles       []string                `json:"signature_files"`
	ScanFolders          []string                `json:"scan_folders"`
	DLC                  []dlcEntryJSON          `json:"dlc,omitempty"`
	ExecutableFallback   *executableFallbackJSON `json:"executable_fallback,omitempty"`
}

type dlcEntryJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type executableFallbackJSON struct {
	Path string   `json:"path"`
	Args []string `json:"args,omitempty"`
}

func parseDescriptorType(s string) (mod.DescriptorType, error) {
	switch s {
	case "classic":
		return mod.DescriptorClassic, nil
	case "json_v1":
		return mod.DescriptorJSONv1, nil
	case "json_v2":
		return mod.DescriptorJSONv2, nil
	default:
		return 0, fmt.Errorf("unknown descriptor_type %q", s)
	}
}

// parseGameList unmarshals and validates a JSONC games list. It fails
// closed: any single malformed or incomplete entry invalidates the whole
// list, rather than silently dropping or half-loading one game.
func parseGameList(data []byte) (gameListFile, error) {
	var f gameListFile
	if err := jsonc.Unmarshal(data, &f); err != nil {
		return gameListFile{}, fmt.Errorf("game: parsing games list: %w", err)
	}
	if len(f.Games) == 0 {
		return gameListFile{}, fmt.Errorf("game: games list has no entries")
	}

	seenIDs := make(map[string]string, len(f.Games))    // id -> name, for a useful duplicate error
	seenAppIDs := make(map[string]string, len(f.Games)) // app_id -> name
	for _, e := range f.Games {
		if _, err := uuid.Parse(e.ID); err != nil {
			return gameListFile{}, fmt.Errorf("game: entry %q has an invalid id %q: %w", e.Name, e.ID, err)
		}
		if other, ok := seenIDs[e.ID]; ok {
			return gameListFile{}, fmt.Errorf("game: id %q is used by both %q and %q", e.ID, other, e.Name)
		}
		seenIDs[e.ID] = e.Name

		if other, ok := seenAppIDs[e.AppID]; ok && e.AppID != "" {
			return gameListFile{}, fmt.Errorf("game: app_id %q is used by both %q and %q", e.AppID, other, e.Name)
		}
		seenAppIDs[e.AppID] = e.Name

		if e.Name == "" {
			return gameListFile{}, fmt.Errorf("game: entry %q has an empty name", e.ID)
		}
		if e.AppID == "" {
			return gameListFile{}, fmt.Errorf("game: entry %q (%s) has an empty app_id", e.Name, e.ID)
		}
		if e.FolderName == "" {
			return gameListFile{}, fmt.Errorf("game: entry %q (%s) has an empty folder_name", e.Name, e.ID)
		}
		if e.LauncherSettingsPath == "" {
			return gameListFile{}, fmt.Errorf("game: entry %q (%s) has an empty launcher_settings_path", e.Name, e.ID)
		}
		if len(e.SignatureFiles) == 0 {
			return gameListFile{}, fmt.Errorf("game: entry %q (%s) has no signature_files", e.Name, e.ID)
		}
		if len(e.ScanFolders) == 0 {
			return gameListFile{}, fmt.Errorf("game: entry %q (%s) has no scan_folders", e.Name, e.ID)
		}
		if _, err := parseDescriptorType(e.DescriptorType); err != nil {
			return gameListFile{}, fmt.Errorf("game: entry %q (%s): %w", e.Name, e.ID, err)
		}
	}

	return f, nil
}

// toRegistry converts an already-validated gameListFile into a Registry.
func (f gameListFile) toRegistry() (*Registry, error) {
	games := make([]GameConfig, 0, len(f.Games))
	for _, e := range f.Games {
		descriptorType, err := parseDescriptorType(e.DescriptorType)
		if err != nil {
			return nil, err // parseGameList already validated this; a defensive check
		}

		dlc := make([]DLCEntry, 0, len(e.DLC))
		for _, d := range e.DLC {
			dlc = append(dlc, DLCEntry{ID: d.ID, Name: d.Name})
		}

		var fallback ExecutableInfo
		if e.ExecutableFallback != nil {
			fallback = ExecutableInfo{Path: e.ExecutableFallback.Path, Args: e.ExecutableFallback.Args}
		}

		games = append(games, GameConfig{
			ID:                   e.ID,
			DisplayName:          e.Name,
			SteamAppID:           e.AppID,
			FolderName:           e.FolderName,
			DescriptorType:       descriptorType,
			LauncherSettingsPath: e.LauncherSettingsPath,
			SignatureFiles:       e.SignatureFiles,
			ScanFolders:          e.ScanFolders,
			DLC:                  dlc,
			ExecutableFallback:   fallback,
		})
	}
	return NewRegistry(games), nil
}

// versionSatisfies reports whether have (this app's own version) is the
// same as or newer than want (a file's required_parallax_version) - plain
// X.Y.Z numeric comparison, since this project's own versions are simple
// three-part numbers, not full semver with pre-release/build metadata.
func versionSatisfies(have, want string) bool {
	h, hErr := parseVersion(have)
	w, wErr := parseVersion(want)
	if hErr != nil || wErr != nil {
		return false // an unparseable version can never be satisfied - fail closed
	}
	for i := 0; i < 3; i++ {
		if h[i] != w[i] {
			return h[i] > w[i]
		}
	}
	return true
}

func parseVersion(v string) ([3]int, error) {
	var out [3]int
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return out, fmt.Errorf("game: %q is not an X.Y.Z version", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("game: %q is not an X.Y.Z version: %w", v, err)
		}
		out[i] = n
	}
	return out, nil
}

// LoadRegistry builds the real, active Registry. embeddedDefault (this
// binary's own compiled-in data/games.jsonc) must always parse - a failure
// there is a build-time bug, not a condition this function recovers from.
// If diskPath is non-empty and a readable, valid file exists there whose
// required_parallax_version appVersion satisfies, it's used INSTEAD of the
// default. Any other outcome for the disk file (missing is fine and
// silent; unreadable, corrupt, or requiring a newer app version is not
// fine but still not fatal) falls back to the embedded default and returns
// a non-empty, user-facing notice explaining why.
func LoadRegistry(appVersion string, embeddedDefault []byte, diskPath string) (*Registry, string, error) {
	defaultList, err := parseGameList(embeddedDefault)
	if err != nil {
		return nil, "", fmt.Errorf("game: embedded default games list is invalid: %w", err)
	}
	defaultRegistry, err := defaultList.toRegistry()
	if err != nil {
		return nil, "", fmt.Errorf("game: embedded default games list is invalid: %w", err)
	}

	if diskPath == "" {
		return defaultRegistry, "", nil
	}

	data, err := os.ReadFile(diskPath)
	if os.IsNotExist(err) {
		return defaultRegistry, "", nil
	}
	if err != nil {
		return defaultRegistry, "Couldn't read the custom games list - using the built-in list instead.", nil
	}

	diskList, err := parseGameList(data)
	if err != nil {
		return defaultRegistry, "The custom games list is invalid - using the built-in list instead.", nil
	}
	if !versionSatisfies(appVersion, diskList.RequiredParallaxVersion) {
		return defaultRegistry, fmt.Sprintf(
			"The custom games list needs Parallax Mod Manager %s or newer - using the built-in list instead.",
			diskList.RequiredParallaxVersion,
		), nil
	}

	diskRegistry, err := diskList.toRegistry()
	if err != nil {
		return defaultRegistry, "The custom games list is invalid - using the built-in list instead.", nil
	}
	return diskRegistry, "", nil
}

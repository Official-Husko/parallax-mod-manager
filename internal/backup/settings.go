package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Mode is which mods get backed up automatically.
type Mode string

const (
	// ModeAtRisk backs a Workshop mod up once it is found to be deleted from the
	// Workshop or private, while its files are still on disk. The default: it costs
	// space only for mods that are actually in danger.
	ModeAtRisk Mode = "atrisk"
	// ModeAll keeps a copy of every installed Workshop mod, refreshed when the mod
	// changes. The only way to be certain, and the most space.
	ModeAll Mode = "all"
	// ModeOff makes no automatic backups. A mod can still be backed up by hand.
	ModeOff Mode = "off"
)

// ParseMode reads a mode name; anything unknown or empty is the default, ModeAtRisk.
func ParseMode(s string) Mode {
	switch Mode(s) {
	case ModeAll:
		return ModeAll
	case ModeOff:
		return ModeOff
	}
	return ModeAtRisk
}

// Settings is what the user chose.
type Settings struct {
	Mode Mode `json:"mode"`
	// Path is a backup folder the user picked; empty means the default (DefaultRoot).
	Path string `json:"path"`
}

// Defaults is a fresh install: back up what is at risk, in the default folder.
func Defaults() Settings { return Settings{Mode: ModeAtRisk} }

// FolderName is the name of the default backup folder.
const FolderName = "Parallax Mod Backups"

// DefaultRoot is the backup folder used until the user picks one: Parallax Mod
// Backups in their home folder. Empty when the home folder cannot be found.
func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, FolderName)
}

// Root is the folder backups go in for these settings.
func (s Settings) Root() string {
	if p := strings.TrimSpace(s.Path); p != "" {
		return filepath.Clean(p)
	}
	return DefaultRoot()
}

// ValidateRoot checks that a folder can be backed up into: it is an absolute path,
// it can be created, and a file can be written in it. It leaves the folder in place
// (created) when it works.
func ValidateRoot(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("the backup folder must be an absolute path")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("the backup folder cannot be created: %w", err)
	}
	probe, err := os.CreateTemp(path, ".write-test-*")
	if err != nil {
		return fmt.Errorf("the backup folder cannot be written to: %w", err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// SettingsFile is the settings file's name in the app's config folder.
const SettingsFile = "backup.jsonc"

// Store reads and writes the settings file; Path empty means there is nowhere to
// keep it.
type Store struct {
	Path string
}

// Load reads the file. A missing file is Defaults; one that cannot be read or parsed is
// Defaults with the error, so the caller can log it and carry on.
func (s Store) Load() (Settings, error) {
	if s.Path == "" {
		return Defaults(), nil
	}
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), fmt.Errorf("backup: reading %s: %w", s.Path, err)
	}
	var raw struct {
		Mode string `json:"mode"`
		Path string `json:"path"`
	}
	if err := jsonc.Unmarshal(data, &raw); err != nil {
		return Defaults(), fmt.Errorf("backup: %s is not valid: %w", s.Path, err)
	}
	return Settings{Mode: ParseMode(raw.Mode), Path: strings.TrimSpace(raw.Path)}, nil
}

// Save writes the settings atomically.
func (s Store) Save(st Settings) error {
	if s.Path == "" {
		return errors.New("backup: there is no settings folder to save into")
	}
	st.Mode = ParseMode(string(st.Mode))
	q := func(v string) string { b, _ := json.Marshal(v); return string(b) }
	text := `// Parallax Mod Manager - mod backup settings.
//
// Steam removes a Workshop mod's files when the mod is deleted, and cannot be asked to
// wait, so the app copies such a mod first. You normally change this in Settings > Backup.
{
  // Which mods are backed up on their own:
  //   "atrisk" - a Workshop mod that turns out to be deleted or private (the default)
  //   "all"    - every installed Workshop mod, kept up to date (uses the most space)
  //   "off"    - nothing automatically
  "mode": ` + q(string(st.Mode)) + `,

  // Where backups are kept. Empty means the default, "` + FolderName + `" in your home
  // folder. Each game gets its own folder inside, named by its id: <path>/<game id>/mods/<item id>.
  "path": ` + q(st.Path) + `
}
`
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if _, err := atomicfile.Write(filepath.Dir(s.Path), filepath.Base(s.Path), []byte(text)); err != nil {
		return fmt.Errorf("backup: saving: %w", err)
	}
	return nil
}

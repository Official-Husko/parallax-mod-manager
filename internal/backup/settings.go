package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// Path is a backup folder the user picked; empty means the default (see DefaultRoot).
	Path string `json:"path"`
	// LimitEnabled caps everything the backups take together at LimitBytes: once that
	// is reached no more mods are backed up until the limit is raised or backups are
	// deleted.
	LimitEnabled bool  `json:"limitEnabled"`
	LimitBytes   int64 `json:"limitBytes"`
	// KeepFreeEnabled stops a backup that would leave the drive holding the backup
	// folder with less than KeepFreeBytes free.
	KeepFreeEnabled bool  `json:"keepFreeEnabled"`
	KeepFreeBytes   int64 `json:"keepFreeBytes"`
}

// The values the limits start with.
const (
	// DefaultLimitBytes is the size cap offered when the limit is first switched on.
	DefaultLimitBytes int64 = 50 << 30
	// DefaultKeepFreeBytes is the free space that is left alone, on by default.
	DefaultKeepFreeBytes int64 = 1 << 30
	// MinLimitBytes is the smallest size cap that can be set.
	MinLimitBytes int64 = 1 << 20
)

// Defaults is a fresh install: back up what is at risk, in the default folder, with
// no size cap and 1 GB of free space left alone.
func Defaults() Settings {
	return Settings{
		Mode:            ModeAtRisk,
		LimitBytes:      DefaultLimitBytes,
		KeepFreeEnabled: true,
		KeepFreeBytes:   DefaultKeepFreeBytes,
	}
}

// Limits is what Copy holds a backup to.
func (s Settings) Limits() Limits {
	var l Limits
	if s.LimitEnabled && s.LimitBytes > 0 {
		l.MaxTotal = s.LimitBytes
	}
	if s.KeepFreeEnabled && s.KeepFreeBytes > 0 {
		l.MinFree = s.KeepFreeBytes
	}
	return l
}

// FolderName is the name of the default backup folder.
const FolderName = "Parallax Mod Backups"

// DefaultRoot is the backup folder used until the user picks one: a Parallax Mod
// Backups folder inside the app's own settings folder (configDir). Empty when there
// is no settings folder.
func DefaultRoot(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, FolderName)
}

// Root is the folder backups go in for these settings: the one the user picked, or
// defaultRoot (see DefaultRoot).
func (s Settings) Root(defaultRoot string) string {
	if p := strings.TrimSpace(s.Path); p != "" {
		return filepath.Clean(p)
	}
	return defaultRoot
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
	// Start from the defaults so a file written before a setting existed keeps that
	// setting's default (the free-space guard is on unless the file says otherwise).
	var raw struct {
		Mode            string `json:"mode"`
		Path            string `json:"path"`
		LimitEnabled    *bool  `json:"limitEnabled"`
		LimitBytes      *int64 `json:"limitBytes"`
		KeepFreeEnabled *bool  `json:"keepFreeEnabled"`
		KeepFreeBytes   *int64 `json:"keepFreeBytes"`
	}
	if err := jsonc.Unmarshal(data, &raw); err != nil {
		return Defaults(), fmt.Errorf("backup: %s is not valid: %w", s.Path, err)
	}
	st := Defaults()
	st.Mode = ParseMode(raw.Mode)
	st.Path = strings.TrimSpace(raw.Path)
	if raw.LimitEnabled != nil {
		st.LimitEnabled = *raw.LimitEnabled
	}
	if raw.LimitBytes != nil {
		st.LimitBytes = *raw.LimitBytes
	}
	if raw.KeepFreeEnabled != nil {
		st.KeepFreeEnabled = *raw.KeepFreeEnabled
	}
	if raw.KeepFreeBytes != nil {
		st.KeepFreeBytes = *raw.KeepFreeBytes
	}
	return normalise(st), nil
}

// normalise keeps the sizes sane: a size cap is at least MinLimitBytes and free space
// to keep is not negative.
func normalise(st Settings) Settings {
	st.Mode = ParseMode(string(st.Mode))
	if st.LimitBytes < MinLimitBytes {
		st.LimitBytes = DefaultLimitBytes
	}
	if st.KeepFreeBytes < 0 {
		st.KeepFreeBytes = 0
	}
	return st
}

// Save writes the settings atomically.
func (s Store) Save(st Settings) error {
	if s.Path == "" {
		return errors.New("backup: there is no settings folder to save into")
	}
	st = normalise(st)
	q := func(v string) string { b, _ := json.Marshal(v); return string(b) }
	yn := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
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

  // Where backups are kept. Empty means the default, a "` + FolderName + `" folder inside this
  // settings folder. Each game gets its own folder inside, named by its id: <path>/<game id>/mods/<item id>.
  "path": ` + q(st.Path) + `,

  // A cap on everything the backups take together, in bytes. When it is reached no more mods
  // are backed up until you raise it or delete backups, and the app tells you.
  "limitEnabled": ` + yn(st.LimitEnabled) + `,
  "limitBytes": ` + strconv.FormatInt(st.LimitBytes, 10) + `,

  // Do not back up when that would leave the drive with less than this much free, in bytes.
  "keepFreeEnabled": ` + yn(st.KeepFreeEnabled) + `,
  "keepFreeBytes": ` + strconv.FormatInt(st.KeepFreeBytes, 10) + `
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

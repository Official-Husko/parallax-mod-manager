package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseModeDefaultsToAtRisk(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeAtRisk, "junk": ModeAtRisk, "atrisk": ModeAtRisk, "all": ModeAll, "off": ModeOff} {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultRootIsParallaxModBackupsInsideTheSettingsFolder(t *testing.T) {
	config := filepath.Join(t.TempDir(), "parallax-mod-manager")
	def := DefaultRoot(config)
	if want := filepath.Join(config, "Parallax Mod Backups"); def != want {
		t.Errorf("DefaultRoot = %q, want %q", def, want)
	}
	if DefaultRoot("") != "" {
		t.Error("with no settings folder there is no default")
	}
	if got := (Settings{}).Root(def); got != def {
		t.Errorf("empty path Root = %q, want the default", got)
	}
	if got := (Settings{Path: "  /somewhere/else/  "}).Root(def); got != filepath.Clean("/somewhere/else") {
		t.Errorf("custom path Root = %q", got)
	}
}

func TestSettingsRoundTripAndComments(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), SettingsFile)}
	if st, err := s.Load(); err != nil || st != Defaults() {
		t.Fatalf("missing file: %+v, %v", st, err)
	}
	want := Settings{Mode: ModeAll, Path: "/mnt/big drive/Mod Backups", LimitBytes: DefaultLimitBytes}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got != want {
		t.Errorf("Load = %+v, %v, want %+v", got, err, want)
	}
	data, _ := os.ReadFile(s.Path)
	text := string(data)
	if !strings.Contains(text, "// Which mods are backed up on their own") || !strings.Contains(text, "// Where backups are kept") || !strings.Contains(text, "inside this") || strings.Contains(text, "—") {
		t.Errorf("the file has no explanation beside its fields:\n%s", text)
	}
}

func TestCorruptSettingsFallBackToDefaultsWithAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), SettingsFile)
	_ = os.WriteFile(path, []byte("{ nope"), 0o644)
	st, err := Store{Path: path}.Load()
	if err == nil || st != Defaults() {
		t.Errorf("Load = %+v, %v", st, err)
	}
	if err := (Store{}).Save(Defaults()); err == nil {
		t.Error("Save with no path succeeded")
	}
}

func TestValidateRoot(t *testing.T) {
	if err := ValidateRoot("relative/path"); err == nil {
		t.Error("a relative path was accepted")
	}
	dir := filepath.Join(t.TempDir(), "new", "nested")
	if err := ValidateRoot(dir); err != nil {
		t.Fatalf("a creatable folder was refused: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("the folder was not created")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the write test left files behind: %v", entries)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		ro := filepath.Join(t.TempDir(), "ro")
		_ = os.MkdirAll(ro, 0o755)
		_ = os.Chmod(ro, 0o555)
		defer os.Chmod(ro, 0o755)
		if err := ValidateRoot(ro); err == nil {
			t.Error("a read-only folder was accepted")
		}
	}
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if err := ValidateRoot(file); err == nil {
		t.Error("a file was accepted as the backup folder")
	}
}

func TestNewSettingsDefaultAndOlderFilesKeepThem(t *testing.T) {
	d := Defaults()
	if d.LimitEnabled || d.LimitBytes != 50<<30 || !d.KeepFreeEnabled || d.KeepFreeBytes != 1<<30 {
		t.Errorf("Defaults = %+v, want no size cap (50 GB when switched on) and 1 GB kept free", d)
	}
	// A file written before the limits existed has only mode and path.
	path := filepath.Join(t.TempDir(), SettingsFile)
	_ = os.WriteFile(path, []byte("{\n \"mode\": \"all\",\n \"path\": \"/x/y\"\n}\n"), 0o644)
	got, err := Store{Path: path}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeAll || got.Path != "/x/y" || got.LimitEnabled || !got.KeepFreeEnabled || got.KeepFreeBytes != 1<<30 || got.LimitBytes != 50<<30 {
		t.Errorf("older file = %+v, want its own mode and path with the defaults for the rest", got)
	}
}

func TestLimitSettingsRoundTripAndAreSane(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), SettingsFile)}
	want := Settings{Mode: ModeAtRisk, LimitEnabled: true, LimitBytes: 3 << 40, KeepFreeEnabled: false, KeepFreeBytes: 5 << 30}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got != want {
		t.Errorf("Load = %+v, %v, want %+v", got, err, want)
	}
	data, _ := os.ReadFile(s.Path)
	text := string(data)
	if !strings.Contains(text, "// A cap on everything the backups take") || !strings.Contains(text, "// Do not back up when that would leave") || !strings.Contains(text, `"limitBytes": 3298534883328`) {
		t.Errorf("the new fields have no explanation or the wrong value:\n%s", text)
	}
	// Nonsense sizes fall back to sane ones.
	_ = s.Save(Settings{Mode: ModeAtRisk, LimitEnabled: true, LimitBytes: 5, KeepFreeBytes: -1})
	got, _ = s.Load()
	if got.LimitBytes != DefaultLimitBytes || got.KeepFreeBytes != 0 {
		t.Errorf("sizes = %d / %d, want the default cap and 0", got.LimitBytes, got.KeepFreeBytes)
	}
}

func TestLimitsFollowTheToggles(t *testing.T) {
	st := Settings{LimitEnabled: true, LimitBytes: 10 << 30, KeepFreeEnabled: true, KeepFreeBytes: 1 << 30}
	if l := st.Limits(); l.MaxTotal != 10<<30 || l.MinFree != 1<<30 {
		t.Errorf("Limits = %+v", l)
	}
	st.LimitEnabled, st.KeepFreeEnabled = false, false
	if l := st.Limits(); l != (Limits{}) {
		t.Errorf("switched off, Limits = %+v, want none", l)
	}
}

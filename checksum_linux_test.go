package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/playset"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// checksumApp is an app whose "Stellaris" is the small tree in internal/checksum/testdata, its
// user data folder and playsets in temp dirs.
func checksumApp(t *testing.T, algorithm string) (*App, game.GameConfig, string) {
	t.Helper()
	tree, err := filepath.Abs("internal/checksum/testdata/tree")
	if err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)

	cfg := game.Stellaris
	cfg.ChecksumAlgorithm = algorithm
	modDir := filepath.Join(data, "Paradox Interactive", "Stellaris", "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"modA", "modB"} {
		stub := "name=\"" + strings.ToUpper(m[:1]) + m[1:] + "\"\npath=\"" + filepath.Join(tree, m) + "\"\n"
		if m == "modB" {
			stub += "dependencies={\n\t\"ModA\"\n}\n"
		}
		if err := os.WriteFile(filepath.Join(modDir, m+".mod"), []byte(stub), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The stub of a mod whose folder is gone.
	if err := os.WriteFile(filepath.Join(modDir, "gone.mod"), []byte("name=\"Gone\"\npath=\"/nowhere/at/all\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prefs := preferences.Defaults()
	prefs.GamePaths = map[string]string{cfg.ID: filepath.Join(tree, "game")}
	a := &App{
		ctx:         context.Background(),
		registry:    game.NewRegistry([]game.GameConfig{cfg}),
		preferences: prefs,
		playsets:    playset.FileStore{Dir: t.TempDir()},
	}
	return a, cfg, modDir
}

func savePlayset(t *testing.T, a *App, cfg game.GameConfig, name string, ids ...string) {
	t.Helper()
	if err := a.playsets.Save(context.Background(), playset.Playset{Name: name, GameKey: cfg.ID, ModIDs: ids}); err != nil {
		t.Fatal(err)
	}
}

func TestPlaysetChecksumCalculatesTheSavedPlayset(t *testing.T) {
	a, cfg, _ := checksumApp(t, "stellaris")

	// The dependency (modB needs ModA) is listed first: the load order the game uses puts ModA
	// first. The value is the one the reference implementation gives for this tree.
	savePlayset(t, a, cfg, "mp", "modB", "modA")
	res, err := a.PlaysetChecksum(cfg.ID, "mp")
	if err != nil {
		t.Fatalf("PlaysetChecksum: %v", err)
	}
	if res.Status != checksumReady || res.Checksum != "E859" || res.Files != 14 || res.Mods != 2 {
		t.Errorf("result = %+v, want ready E859 with 14 files and 2 mods", res)
	}

	savePlayset(t, a, cfg, "vanilla")
	res, err = a.PlaysetChecksum(cfg.ID, "vanilla")
	if err != nil || res.Status != checksumReady || res.Checksum != "1E86" || res.Mods != 0 {
		t.Errorf("empty playset = %+v, %v; want the vanilla checksum 1E86", res, err)
	}
}

func TestPlaysetChecksumSaysWhyThereIsNone(t *testing.T) {
	a, cfg, _ := checksumApp(t, "stellaris")

	if res, err := a.PlaysetChecksum(cfg.ID, ""); err != nil || res.Status != checksumUnavailable || !strings.Contains(res.Reason, "No playset") {
		t.Errorf("no playset: %+v, %v", res, err)
	}

	savePlayset(t, a, cfg, "missing-mod", "modA", "never-installed")
	if res, err := a.PlaysetChecksum(cfg.ID, "missing-mod"); err != nil || res.Status != checksumUnavailable || !strings.Contains(res.Reason, "never-installed") {
		t.Errorf("mod not installed: %+v, %v", res, err)
	}

	savePlayset(t, a, cfg, "gone", "modA", "gone")
	if res, err := a.PlaysetChecksum(cfg.ID, "gone"); err != nil || res.Status != checksumUnavailable || !strings.Contains(res.Reason, "Gone") || res.Checksum != "" {
		t.Errorf("mod folder missing: %+v, %v", res, err)
	}

	if _, err := a.PlaysetChecksum(cfg.ID, "no-such-playset"); err == nil {
		t.Error("expected an error for a playset that does not exist")
	}
	if _, err := a.PlaysetChecksum("unknown-game", "mp"); err == nil {
		t.Error("expected an error for an unknown game")
	}

	// An install folder that is not there.
	a.preferences.GamePaths = map[string]string{}
	t.Setenv("HOME", t.TempDir())
	savePlayset(t, a, cfg, "mp", "modA")
	if res, err := a.PlaysetChecksum(cfg.ID, "mp"); err != nil || res.Status != checksumUnavailable {
		t.Errorf("no install folder: %+v, %v", res, err)
	}
}

func TestPlaysetChecksumIsNotOfferedForAGameWithoutAScheme(t *testing.T) {
	a, cfg, _ := checksumApp(t, "")
	savePlayset(t, a, cfg, "mp", "modA")
	res, err := a.PlaysetChecksum(cfg.ID, "mp")
	if err != nil || res.Status != checksumUnsupported || res.Checksum != "" {
		t.Errorf("result = %+v, %v; want unsupported", res, err)
	}
}

func TestANewerRequestStopsTheOlderOne(t *testing.T) {
	a := &App{ctx: context.Background()}
	first, doneFirst := a.startChecksum("g")
	defer doneFirst()
	other, doneOther := a.startChecksum("h")
	defer doneOther()
	second, doneSecond := a.startChecksum("g")
	defer doneSecond()

	if first.Err() == nil {
		t.Error("the older request for the same game should have been stopped")
	}
	if second.Err() != nil || other.Err() != nil {
		t.Error("the newer request, and another game's, must keep running")
	}
}

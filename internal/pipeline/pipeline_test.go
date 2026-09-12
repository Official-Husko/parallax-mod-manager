package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

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

func TestLoadModParsesAllScannedFiles(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, contentDir, "common/buildings/00_buildings.txt", `some_building = { cost = 100 }`)
	writeFile(t, contentDir, "events/00_events.txt", `country_event = { id = m.1 }`)
	writeFile(t, contentDir, "localisation/english/l_test.yml", "l_english:\n KEY:0 \"Value\"\n")
	// Lives inside a scanned folder but has an unrecognized extension - must
	// be skipped by enumeration, not handed to the script parser (which
	// would fail on binary/non-script content).
	writeFile(t, contentDir, "common/models/ignored.dds", "not parsable, must be skipped")

	m := mod.Mod{ID: "test_mod", ContentPath: contentDir}
	cfg := testGame("stellaris")
	opts := Options{Store: cache.FileStore{Dir: t.TempDir()}}

	defs, err := LoadMod(context.Background(), m, cfg, opts)
	if err != nil {
		t.Fatalf("LoadMod: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("expected 3 definitions, got %d: %+v", len(defs), defs)
	}

	ids := map[string]bool{}
	for _, d := range defs {
		ids[d.ID] = true
	}
	for _, want := range []string{"some_building", "country_event", "KEY"} {
		if !ids[want] {
			t.Errorf("missing definition with ID %q, got %v", want, ids)
		}
	}
}

func TestLoadModDeterministicOrderAcrossRuns(t *testing.T) {
	contentDir := t.TempDir()
	for i := 0; i < 20; i++ {
		writeFile(t, contentDir, filepath.Join("common", "buildings", string(rune('a'+i))+".txt"),
			`building = { cost = `+string(rune('0'+i%10))+` }`)
	}
	m := mod.Mod{ID: "test_mod", ContentPath: contentDir}
	cfg := testGame("stellaris")

	var first []string
	for run := 0; run < 5; run++ {
		// Fresh store each run so caching can't be why order looks stable.
		opts := Options{Store: cache.FileStore{Dir: t.TempDir()}, Workers: runtime.NumCPU()}
		defs, err := LoadMod(context.Background(), m, cfg, opts)
		if err != nil {
			t.Fatalf("run %d: LoadMod: %v", run, err)
		}
		var paths []string
		for _, d := range defs {
			paths = append(paths, d.FilePath)
		}
		if run == 0 {
			first = paths
			continue
		}
		if !reflect.DeepEqual(paths, first) {
			t.Fatalf("run %d produced a different file order:\n got  %v\n want %v", run, paths, first)
		}
	}
}

func TestLoadModWorkersOneVsNumCPUIdenticalOutput(t *testing.T) {
	contentDir := t.TempDir()
	for i := 0; i < 12; i++ {
		writeFile(t, contentDir, filepath.Join("common", string(rune('a'+i))+".txt"),
			`x = { a = `+string(rune('0'+i%10))+` }`)
	}
	m := mod.Mod{ID: "test_mod", ContentPath: contentDir}
	cfg := testGame("stellaris")

	single, err := LoadMod(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: t.TempDir()}, Workers: 1})
	if err != nil {
		t.Fatalf("Workers=1: %v", err)
	}
	parallel, err := LoadMod(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: t.TempDir()}, Workers: runtime.NumCPU()})
	if err != nil {
		t.Fatalf("Workers=NumCPU: %v", err)
	}
	if !reflect.DeepEqual(single, parallel) {
		t.Errorf("Workers=1 and Workers=NumCPU produced different output:\n1:   %+v\nCPU: %+v", single, parallel)
	}
}

// TestLoadModSecondRunReusesCacheWithoutReparsing proves caching actually
// short-circuits parsing (not just that output looks the same) by corrupting
// a file's *content* between runs while forcing its (mtime,size) to stay
// identical. If the second run still returns the original, valid
// definitions - rather than failing on or reflecting the now-garbage content
// - it can only have done so via the cache, since actually re-reading and
// re-parsing the corrupted bytes would fail script.Parse.
func TestLoadModSecondRunReusesCacheWithoutReparsing(t *testing.T) {
	contentDir := t.TempDir()
	relPath := filepath.Join("common", "building.txt")
	original := `some_building = { cost = 100 }`
	writeFile(t, contentDir, relPath, original)

	fullPath := filepath.Join(contentDir, relPath)
	infoBefore, err := os.Stat(fullPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	m := mod.Mod{ID: "test_mod", ContentPath: contentDir}
	cfg := testGame("stellaris")
	storeDir := t.TempDir()

	first, err := LoadMod(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: storeDir}})
	if err != nil {
		t.Fatalf("first LoadMod: %v", err)
	}
	if len(first) != 1 || first[0].ID != "some_building" {
		t.Fatalf("first run = %+v, want one definition \"some_building\"", first)
	}

	// Corrupt the content, same byte length as the original, then force the
	// exact same mtime/size so the cache's stat check still hits.
	corrupted := `xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` // same length as `original`
	if len(corrupted) != len(original) {
		t.Fatalf("test fixture bug: corrupted length %d != original length %d", len(corrupted), len(original))
	}
	if err := os.WriteFile(fullPath, []byte(corrupted), 0o644); err != nil {
		t.Fatalf("WriteFile (corrupt): %v", err)
	}
	if err := os.Chtimes(fullPath, infoBefore.ModTime(), infoBefore.ModTime()); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	infoAfter, err := os.Stat(fullPath)
	if err != nil {
		t.Fatalf("Stat after corruption: %v", err)
	}
	if infoAfter.ModTime() != infoBefore.ModTime() || infoAfter.Size() != infoBefore.Size() {
		t.Fatalf("test fixture bug: stat changed after corrupting content (before=%v/%d after=%v/%d)",
			infoBefore.ModTime(), infoBefore.Size(), infoAfter.ModTime(), infoAfter.Size())
	}

	second, err := LoadMod(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: storeDir}})
	if err != nil {
		t.Fatalf("second LoadMod: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("second run did not reuse the cache:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestLoadModTouchedMtimeUnchangedContentStillReusesResult(t *testing.T) {
	contentDir := t.TempDir()
	relPath := filepath.Join("common", "building.txt")
	writeFile(t, contentDir, relPath, `some_building = { cost = 100 }`)

	m := mod.Mod{ID: "test_mod", ContentPath: contentDir}
	cfg := testGame("stellaris")
	storeDir := t.TempDir()

	first, err := LoadMod(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: storeDir}})
	if err != nil {
		t.Fatalf("first LoadMod: %v", err)
	}

	// Touch the file (new mtime) without changing its content.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(contentDir, relPath), future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	second, err := LoadMod(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: storeDir}})
	if err != nil {
		t.Fatalf("second LoadMod: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("touched-but-unchanged content produced different output:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestLoadModReportsProgress(t *testing.T) {
	contentDir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeFile(t, contentDir, filepath.Join("common", string(rune('a'+i))+".txt"), `x = { a = 1 }`)
	}
	m := mod.Mod{ID: "test_mod", ContentPath: contentDir}
	cfg := testGame("stellaris")

	var mu sync.Mutex
	var maxDone, lastTotal int
	opts := Options{
		Store: cache.FileStore{Dir: t.TempDir()},
		OnProgress: func(p Progress) {
			mu.Lock()
			defer mu.Unlock()
			if p.FilesDone > maxDone {
				maxDone = p.FilesDone
			}
			lastTotal = p.FilesTotal
			if p.ModID != "test_mod" {
				t.Errorf("Progress.ModID = %q, want test_mod", p.ModID)
			}
		},
	}
	if _, err := LoadMod(context.Background(), m, cfg, opts); err != nil {
		t.Fatalf("LoadMod: %v", err)
	}
	if maxDone != 5 || lastTotal != 5 {
		t.Errorf("progress reporting: maxDone=%d lastTotal=%d, want 5/5", maxDone, lastTotal)
	}
}

func TestLoadModEmptyModProducesNoDefinitions(t *testing.T) {
	m := mod.Mod{ID: "empty_mod", ContentPath: t.TempDir()}
	cfg := testGame("stellaris")
	defs, err := LoadMod(context.Background(), m, cfg, Options{Store: cache.FileStore{Dir: t.TempDir()}})
	if err != nil {
		t.Fatalf("LoadMod: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected no definitions, got %+v", defs)
	}
}

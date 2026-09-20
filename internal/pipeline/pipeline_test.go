package pipeline

import (
	"context"
	"fmt"
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

// countingStore wraps a real store and counts how often each mod cache is
// written back.
type countingStore struct {
	cache.FileStore
	mu    sync.Mutex
	saves int
}

func (s *countingStore) Save(ctx context.Context, c *cache.ModCache) error {
	s.mu.Lock()
	s.saves++
	s.mu.Unlock()
	return s.FileStore.Save(ctx, c)
}

func TestLoadModOnlyWritesTheCacheWhenSomethingChanged(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, contentDir, "common/buildings/a.txt", `thing_a = { cost = 1 }`)
	writeFile(t, contentDir, "common/buildings/b.txt", `thing_b = { cost = 2 }`)
	m := mod.Mod{ID: "m", ContentPath: contentDir}
	store := &countingStore{FileStore: cache.FileStore{Dir: t.TempDir()}}
	stats := &Stats{}
	opts := Options{Store: store, Stats: stats}
	ctx := context.Background()

	first, err := LoadMod(ctx, m, testGame("g"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if store.saves != 1 || stats.Parsed.Load() != 2 || stats.Saved.Load() != 1 {
		t.Fatalf("first run: saves=%d parsed=%d, want 1 and 2 (everything is new)", store.saves, stats.Parsed.Load())
	}

	second, err := LoadMod(ctx, m, testGame("g"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if store.saves != 1 {
		t.Errorf("an unchanged second run wrote the cache again (saves=%d) - it has nothing new to write", store.saves)
	}
	if !reflect.DeepEqual(first, second) {
		t.Error("skipping the save must not change what LoadMod returns")
	}
	if stats.Cached.Load() != 2 {
		t.Errorf("cached = %d, want both files reused on stat alone", stats.Cached.Load())
	}

	// A real edit (different size, so the stat check can't miss it) is written.
	writeFile(t, contentDir, "common/buildings/a.txt", `thing_a = { cost = 100 extra = yes }`)
	third, err := LoadMod(ctx, m, testGame("g"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if store.saves != 2 {
		t.Errorf("saves = %d after an edit, want 2", store.saves)
	}
	if len(third) != 2 || stats.Parsed.Load() != 3 {
		t.Errorf("after the edit: defs=%d parsed=%d, want 2 and 3", len(third), stats.Parsed.Load())
	}
	if stats.Files.Load() != 6 || stats.Cached.Load() != 3 {
		t.Errorf("files=%d cached=%d, want 6 examined and 3 cached in total", stats.Files.Load(), stats.Cached.Load())
	}

	// And the edit really reached the disk: a fresh run after it saves nothing.
	if _, err := LoadMod(ctx, m, testGame("g"), opts); err != nil {
		t.Fatal(err)
	}
	if store.saves != 2 {
		t.Errorf("saves = %d, want the edited cache to have been persisted (no fourth write)", store.saves)
	}
}

func TestLoadModTouchedFileRefreshesTheCacheOnce(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, contentDir, "common/buildings/a.txt", `thing_a = { cost = 1 }`)
	m := mod.Mod{ID: "m", ContentPath: contentDir}
	store := &countingStore{FileStore: cache.FileStore{Dir: t.TempDir()}}
	stats := &Stats{}
	opts := Options{Store: store, Stats: stats}
	ctx := context.Background()
	if _, err := LoadMod(ctx, m, testGame("g"), opts); err != nil {
		t.Fatal(err)
	}

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(contentDir, "common/buildings/a.txt"), later, later); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := LoadMod(ctx, m, testGame("g"), opts); err != nil {
			t.Fatal(err)
		}
	}
	if stats.Touched.Load() != 1 {
		t.Errorf("touched = %d, want 1 (same content, new mtime, seen once)", stats.Touched.Load())
	}
	if store.saves != 2 {
		t.Errorf("saves = %d, want 2: the first parse, and one to record the new mtime - after which it's a plain hit", store.saves)
	}
}

func TestStatsKeepCountingAFileThatFailsToParseOnWarmRuns(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, contentDir, "common/buildings/good.txt", `thing = { cost = 1 }`)
	writeFile(t, contentDir, "common/buildings/bad.txt", `thing = { cost = `)
	m := mod.Mod{ID: "m", ContentPath: contentDir}
	opts := Options{Store: cache.FileStore{Dir: t.TempDir()}}
	for run := 1; run <= 2; run++ {
		stats := &Stats{}
		opts.Stats = stats
		if _, err := LoadMod(context.Background(), m, testGame("g"), opts); err != nil {
			t.Fatal(err)
		}
		if stats.ParseErrors.Load() != 1 {
			t.Errorf("run %d: ParseErrors = %d, want 1 - the broken file is still broken on a warm run", run, stats.ParseErrors.Load())
		}
	}
}

func TestStatsNameAFileThatFailsToParseOnlyOnTheRunThatFirstHitIt(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, contentDir, "common/buildings/good.txt", `thing = { cost = 1 }`)
	writeFile(t, contentDir, "common/buildings/bad.txt", `thing = { cost = `)
	m := mod.Mod{ID: "m", ContentPath: contentDir}
	opts := Options{Store: cache.FileStore{Dir: t.TempDir()}}

	first := &Stats{}
	opts.Stats = first
	if _, err := LoadMod(context.Background(), m, testGame("g"), opts); err != nil {
		t.Fatal(err)
	}
	problems, more := first.Problems()
	if more != 0 || len(problems) != 1 {
		t.Fatalf("first run: problems = %+v (+%d more), want exactly the broken file", problems, more)
	}
	if p := problems[0]; p.ModID != "m" || p.Path != "common/buildings/bad.txt" || p.Err == "" {
		t.Errorf("first run: problem = %+v, want the mod, the file's relative path and a reason", p)
	}

	// Warm: still counted, but not named again - it isn't news.
	second := &Stats{}
	opts.Stats = second
	if _, err := LoadMod(context.Background(), m, testGame("g"), opts); err != nil {
		t.Fatal(err)
	}
	if problems, _ := second.Problems(); len(problems) != 0 {
		t.Errorf("warm run named %+v again, want nothing new", problems)
	}
	if second.ParseErrors.Load() != 1 {
		t.Errorf("warm run ParseErrors = %d, want it still counted", second.ParseErrors.Load())
	}

	// Editing the file makes it a new parse - and a new report if still broken.
	writeFile(t, contentDir, "common/buildings/bad.txt", `thing = { cost = 2 `)
	third := &Stats{}
	opts.Stats = third
	if _, err := LoadMod(context.Background(), m, testGame("g"), opts); err != nil {
		t.Fatal(err)
	}
	if problems, _ := third.Problems(); len(problems) != 1 {
		t.Errorf("a changed, still-broken file should be named again, got %+v", problems)
	}
}

func TestStatsKeepOnlyTheFirstFewProblemsAndCountTheRest(t *testing.T) {
	contentDir := t.TempDir()
	total := maxProblems + 7
	for i := 0; i < total; i++ {
		writeFile(t, contentDir, fmt.Sprintf("common/buildings/bad%03d.txt", i), `thing = { cost = `)
	}
	m := mod.Mod{ID: "m", ContentPath: contentDir}
	stats := &Stats{}
	if _, err := LoadMod(context.Background(), m, testGame("g"), Options{Store: cache.FileStore{Dir: t.TempDir()}, Stats: stats}); err != nil {
		t.Fatal(err)
	}
	problems, more := stats.Problems()
	if len(problems) != maxProblems || more != total-maxProblems {
		t.Errorf("kept %d (+%d more), want %d (+%d more)", len(problems), more, maxProblems, total-maxProblems)
	}
	if got := stats.ParseErrors.Load(); got != int64(total) {
		t.Errorf("ParseErrors = %d, want every one of the %d counted", got, total)
	}
}

func TestLoadModKeepsTheGoodEntriesOfALocaleFileWithABadLine(t *testing.T) {
	contentDir := t.TempDir()
	writeFile(t, contentDir, "localisation/english/l_test.yml",
		"l_english:\n KEY_A:0 \"one\"\n BROKEN:0 \"runs over\nonto another line\"\n KEY_B:0 \"two\" # with a comment\n")
	writeFile(t, contentDir, "localisation/english/l_clean.yml", "l_english:\n KEY_C:0 \"three\"\n")
	m := mod.Mod{ID: "m", ContentPath: contentDir}
	opts := Options{Store: cache.FileStore{Dir: t.TempDir()}}

	for run := 1; run <= 2; run++ {
		stats := &Stats{}
		opts.Stats = stats
		defs, err := LoadMod(context.Background(), m, testGame("g"), opts)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, d := range defs {
			ids[d.ID] = true
		}
		for _, want := range []string{"KEY_A", "KEY_B", "KEY_C"} {
			if !ids[want] {
				t.Errorf("run %d: %s missing from %v - one bad line must not cost the file", run, want, ids)
			}
		}
		if ids["BROKEN"] {
			t.Errorf("run %d: the unreadable line must not become a definition", run)
		}
		if stats.ParseErrors.Load() != 1 {
			t.Errorf("run %d: files with problems = %d, want 1 (also on the warm run)", run, stats.ParseErrors.Load())
		}
	}
}

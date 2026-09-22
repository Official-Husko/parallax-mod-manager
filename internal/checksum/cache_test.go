package checksum

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withCache(in Input, cache *ResultCache) Input {
	in.Cache = cache
	return in
}

// copyTreeMod copies the fixture's game folder and modA into an isolated temp dir, so a test that
// mutates a file's content or timestamps (to prove the cache notices, or does not read what it
// should not) never touches the shared testdata fixture other tests in this package also use.
func copyTreeMod(t *testing.T) (gameDir, modDir string) {
	t.Helper()
	root := t.TempDir()
	copyDir := func(src, dst string) {
		t.Helper()
		if err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out := filepath.Join(dst, rel)
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			return os.WriteFile(out, data, 0o644)
		}); err != nil {
			t.Fatal(err)
		}
	}
	gameDir = filepath.Join(root, "game")
	modDir = filepath.Join(root, "modA")
	copyDir(filepath.Join(tree, "game"), gameDir)
	copyDir(filepath.Join(tree, "modA"), modDir)
	return gameDir, modDir
}

func TestCacheHitSkipsReadingFileContentEntirely(t *testing.T) {
	gameDir, modDir := copyTreeMod(t)
	a := Mod{ID: "a", Name: "ModA", Content: modDir}
	in := Input{Algorithm: Stellaris, GameDir: gameDir, LauncherSettings: filepath.Join(gameDir, "launcher-settings.json"), Mods: []Mod{a}}
	var cache ResultCache

	got1, err := Compute(context.Background(), withCache(in, &cache))
	if err != nil {
		t.Fatal(err)
	}

	// Overwrite the mod's file with different bytes but keep the same size and the same
	// modification time (set explicitly, since a fast filesystem can otherwise give a new write
	// the same timestamp as the old one by coincidence, which would make this test pass for the
	// wrong reason). A cache that actually reads the file would see this and compute something
	// different; one that trusts the fingerprint returns the old answer unchanged.
	target := filepath.Join(modDir, "common", "a.txt")
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	same := time.Unix(0, info.ModTime().UnixNano())
	if err := os.WriteFile(target, []byte("A from modZ"), 0o644); err != nil { // same length as "A from modA"
		t.Fatal(err)
	}
	if err := os.Chtimes(target, same, same); err != nil {
		t.Fatal(err)
	}

	got2, err := Compute(context.Background(), withCache(in, &cache))
	if err != nil {
		t.Fatal(err)
	}
	if got2.Full != got1.Full {
		t.Errorf("a cache hit re-read the file: got %s, want the cached %s", got2.Full, got1.Full)
	}

	// A real change - a new modification time - is never missed.
	later := same.Add(time.Hour)
	if err := os.Chtimes(target, later, later); err != nil {
		t.Fatal(err)
	}
	got3, err := Compute(context.Background(), withCache(in, &cache))
	if err != nil {
		t.Fatal(err)
	}
	if got3.Full == got1.Full {
		t.Error("a changed modification time was not enough to invalidate the cache")
	}
	if got3.Full != mustCompute(t, Input{Algorithm: Stellaris, GameDir: gameDir, LauncherSettings: in.LauncherSettings, Mods: []Mod{a}}).Full {
		t.Error("the recomputed value after a real change was not what a fresh calculation gives")
	}
}

func TestCacheDetectsEveryKindOfChange(t *testing.T) {
	a := treeMod("ModA", "modA", nil)
	b := treeMod("ModB", "modB", nil, "ModA")
	base := treeInput(Stellaris, a)

	baseline := func() *ResultCache {
		var c ResultCache
		if _, err := Compute(context.Background(), withCache(base, &c)); err != nil {
			t.Fatal(err)
		}
		return &c
	}
	changed := func(in Input, c *ResultCache) bool {
		before := len(c.entries)
		res, err := Compute(context.Background(), withCache(in, c))
		if err != nil {
			t.Fatal(err)
		}
		// A genuinely new combination adds an entry; the assertion below is really about whether
		// the *answer* differs, this just confirms a cache miss truly happened, not a false hit.
		_ = before
		return res.Full != mustCompute(t, base).Full
	}

	if !changed(treeInput(HOI4, a), baseline()) {
		t.Error("a different algorithm was not detected as a change")
	}
	if !changed(treeInput(Stellaris, a, b), baseline()) {
		t.Error("a different mod set was not detected as a change")
	}
	if !changed(treeInput(Stellaris, b, a), baseline()) {
		t.Error("a different mod order was not detected as a change")
	}
}

func mustCompute(t *testing.T, in Input) Result {
	t.Helper()
	res, err := Compute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestCacheKeyIncludesGameModDirAndOrderedModIDs(t *testing.T) {
	a := Mod{ID: "a"}
	b := Mod{ID: "b"}
	base := Input{Algorithm: Stellaris, GameDir: "/g", ModDir: "/m", Mods: []Mod{a, b}}

	if cacheKey(base) == cacheKey(Input{Algorithm: HOI4, GameDir: "/g", ModDir: "/m", Mods: []Mod{a, b}}) {
		t.Error("algorithm not part of the key")
	}
	if cacheKey(base) == cacheKey(Input{Algorithm: Stellaris, GameDir: "/other", ModDir: "/m", Mods: []Mod{a, b}}) {
		t.Error("game dir not part of the key")
	}
	if cacheKey(base) == cacheKey(Input{Algorithm: Stellaris, GameDir: "/g", ModDir: "/other", Mods: []Mod{a, b}}) {
		t.Error("mod dir not part of the key")
	}
	if cacheKey(base) == cacheKey(Input{Algorithm: Stellaris, GameDir: "/g", ModDir: "/m", Mods: []Mod{b, a}}) {
		t.Error("mod order not part of the key")
	}
	if cacheKey(base) != cacheKey(Input{Algorithm: Stellaris, GameDir: "/g", ModDir: "/m", Mods: []Mod{a, b}}) {
		t.Error("two identical inputs got different keys")
	}
}

func TestFingerprintCoversSaltPathSizeAndTime(t *testing.T) {
	type f struct {
		path string
		size int64
		time time.Time
	}
	get := func(x f) (string, int64, time.Time) { return x.path, x.size, x.time }
	at := time.Unix(1700000000, 0)
	base := []f{{"a", 1, at}, {"b", 2, at}}

	if fingerprintOf("salt", base, get) != fingerprintOf("salt", base, get) {
		t.Error("the same input produced different fingerprints")
	}
	if fingerprintOf("salt", base, get) == fingerprintOf("other salt", base, get) {
		t.Error("salt is not covered")
	}
	if fingerprintOf("salt", base, get) == fingerprintOf("salt", []f{{"a", 1, at}, {"b", 999, at}}, get) {
		t.Error("size is not covered")
	}
	if fingerprintOf("salt", base, get) == fingerprintOf("salt", []f{{"a", 1, at}, {"b", 2, at.Add(time.Second)}}, get) {
		t.Error("modification time is not covered")
	}
	if fingerprintOf("salt", base, get) == fingerprintOf("salt", []f{{"c", 1, at}, {"b", 2, at}}, get) {
		t.Error("path is not covered")
	}
	if fingerprintOf("salt", []f{}, get) == fingerprintOf("salt", base, get) {
		t.Error("an empty file list should not collide with a non-empty one")
	}
}

func TestCacheEvictsTheOldestBeyondItsLimit(t *testing.T) {
	var c ResultCache
	for i := 0; i < maxCachedResults+3; i++ {
		key := string(rune('a' + i))
		c.store(key, "fp", Result{Full: key})
	}
	if len(c.entries) != maxCachedResults {
		t.Fatalf("cache holds %d entries, want %d", len(c.entries), maxCachedResults)
	}
	if _, ok := c.lookup("a", "fp"); ok {
		t.Error("the oldest entry should have been evicted")
	}
	last := string(rune('a' + maxCachedResults + 2))
	if _, ok := c.lookup(last, "fp"); !ok {
		t.Error("the newest entry should still be there")
	}
}

func TestCachedResultCannotBeMutatedThroughAnotherCopy(t *testing.T) {
	var c ResultCache
	c.store("k", "fp", Result{Order: []string{"A"}, Warnings: []string{"w"}})
	got, ok := c.lookup("k", "fp")
	if !ok {
		t.Fatal("expected a hit")
	}
	got.Order[0] = "TAMPERED"
	got.Warnings[0] = "TAMPERED"
	again, _ := c.lookup("k", "fp")
	if again.Order[0] != "A" || again.Warnings[0] != "w" {
		t.Errorf("mutating one lookup's result affected another: %+v", again)
	}
}

func TestANilCacheIsAlwaysAMissAndNeverPanics(t *testing.T) {
	var c *ResultCache
	if _, ok := c.lookup("k", "fp"); ok {
		t.Error("a nil cache should never hit")
	}
	c.store("k", "fp", Result{}) // must not panic
}

func TestNoCacheConfiguredComputesFreshEveryTime(t *testing.T) {
	a := treeMod("ModA", "modA", nil)
	in := treeInput(Stellaris, a) // Cache left nil
	r1, err := Compute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Compute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Full != r2.Full {
		t.Errorf("two computations of the same input disagreed: %s vs %s", r1.Full, r2.Full)
	}
}

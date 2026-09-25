package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// buildSyntheticMod writes a synthetic content tree with n files spread across
// common/, events/ and localisation/english/ - the same three scan folders
// testGame already uses - with real (if repetitive) Clausewitz-script and
// locale content, so LoadMod exercises its real script.Parse/locale.Parse
// paths rather than short-circuiting on empty files. Built fresh under
// b.TempDir() every call: never a checked-in fixture (see
// docs/performance-strategy.md's note on a prior test that corrupted one by
// mutating it in place).
func buildSyntheticMod(b *testing.B, n int) string {
	b.Helper()
	dir := b.TempDir()
	folders := []string{"common/buildings", "events", "localisation/english"}
	for i := 0; i < n; i++ {
		folder := folders[i%len(folders)]
		if folder == "localisation/english" {
			relPath := filepath.Join(folder, fmt.Sprintf("l_bench_%04d.yml", i))
			content := fmt.Sprintf("l_english:\n KEY_%04d:0 \"Benchmark value number %d\"\n", i, i)
			writeBenchFile(b, dir, relPath, content)
			continue
		}
		relPath := filepath.Join(folder, fmt.Sprintf("bench_%04d.txt", i))
		content := fmt.Sprintf(`thing_%04d = {
	cost = %d
	category = "bench"
	modifier = {
		factor = %d
	}
}
`, i, i%500, i%10)
		writeBenchFile(b, dir, relPath, content)
	}
	return dir
}

// writeBenchFile is pipeline_test.go's writeFile, taken as a *testing.B
// instead - kept as its own copy rather than widening writeFile itself to
// testing.TB, since this file's benchmarks are the only thing here that ever
// runs against a *testing.B.
func writeBenchFile(b *testing.B, dir, relPath, content string) {
	b.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		b.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		b.Fatalf("WriteFile: %v", err)
	}
}

// BenchmarkEnumerateFiles isolates the stat/walk stage on its own, since it
// runs on every LoadMod call regardless of cache state.
func BenchmarkEnumerateFiles(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			dir := buildSyntheticMod(b, n)
			cfg := testGame("bench")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := EnumerateFiles(dir, cfg.ScanFolders); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLoadMod_Cold measures a full stat->hash->parse pass against an
// empty cache - every file must be read and parsed.
func BenchmarkLoadMod_Cold(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			dir := buildSyntheticMod(b, n)
			m := mod.Mod{ID: "bench_mod", ContentPath: dir}
			cfg := testGame("bench")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// A fresh store every iteration: this benchmark is about the cold
				// (nothing cached yet) path specifically, not the warm one below.
				opts := Options{Store: cache.FileStore{Dir: b.TempDir()}}
				if _, err := LoadMod(context.Background(), m, cfg, opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLoadMod_Warm measures the cache-hit path: every file's (mtime,
// size) already matches, so LoadMod should do no reading or parsing at all,
// just EnumerateFiles + a Lookup per file.
func BenchmarkLoadMod_Warm(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			dir := buildSyntheticMod(b, n)
			m := mod.Mod{ID: "bench_mod", ContentPath: dir}
			cfg := testGame("bench")
			opts := Options{Store: cache.FileStore{Dir: b.TempDir()}}

			// Prime the cache once, outside the timed loop.
			if _, err := LoadMod(context.Background(), m, cfg, opts); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := LoadMod(context.Background(), m, cfg, opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

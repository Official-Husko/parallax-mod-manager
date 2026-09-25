package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
)

// buildSyntheticModSet writes modCount classic-format mods, filesPerMod
// content files each, under a fresh b.TempDir() - the "whole pipeline"
// shape docs/performance-strategy.md's own real-world case study measured
// (86 real mods), scaled down to something CI can run in a fraction of a
// second while still exercising scan -> per-mod parse -> conflict-resolve
// end to end. Every file's content differs (by mod and file index), so
// BuildIndex/Resolve sees a real, non-degenerate key spread rather than one
// giant duplicate.
func buildSyntheticModSet(b *testing.B, modCount, filesPerMod int) (modDir string, order conflict.LoadOrder) {
	b.Helper()
	modDir = b.TempDir()
	order = make(conflict.LoadOrder, modCount)
	for i := 0; i < modCount; i++ {
		id := fmt.Sprintf("mod_%03d", i)
		order[i] = id
		writeBenchFile(b, modDir, id+".mod", fmt.Sprintf(`name = "Mod %d"
path = "%s"
version = "1.0"
`, i, id))
		for j := 0; j < filesPerMod; j++ {
			// Every third file's first key collides with the previous mod's,
			// so conflict resolution has real contested keys to resolve
			// instead of every key belonging to exactly one mod.
			key := fmt.Sprintf("thing_%03d_%03d", i, j)
			if j%3 == 0 && i > 0 {
				key = fmt.Sprintf("thing_%03d_%03d", i-1, j)
			}
			content := fmt.Sprintf("%s = { cost = %d category = \"bench\" }\n", key, (i+j)%500)
			writeBenchFile(b, modDir, filepath.Join(id, "common", fmt.Sprintf("f%03d.txt", j)), content)
		}
	}
	return modDir, order
}

// writeBenchFile is loadgame_test.go's writeFile, taken as a *testing.B
// instead - kept as its own copy rather than widening writeFile itself,
// since this file's benchmarks are the only thing here that ever runs
// against a *testing.B.
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

// BenchmarkLoadGame is the end-to-end number: scan -> per-mod parse (cache
// stat/hash/parse) -> conflict resolve, cold vs warm cache, on a synthetic
// modlist sized similarly to this project's own real-world case studies.
func BenchmarkLoadGame(b *testing.B) {
	modDir, order := buildSyntheticModSet(b, 20, 100)
	cfg := testGameConfig()
	ctx := context.Background()

	b.Run("cold", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			opts := Options{CacheDir: b.TempDir(), ModDir: modDir, Order: order}
			if _, err := LoadGame(ctx, cfg, opts); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("warm", func(b *testing.B) {
		opts := Options{CacheDir: b.TempDir(), ModDir: modDir, Order: order}
		if _, err := LoadGame(ctx, cfg, opts); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := LoadGame(ctx, cfg, opts); err != nil {
				b.Fatal(err)
			}
		}
	})
}

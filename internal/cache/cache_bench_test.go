package cache

import (
	"context"
	"fmt"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// syntheticModCache builds a ModCache with n files, each carrying defsPerFile
// Definitions - the shape docs/performance-strategy.md's own cache-format
// case study was measured against (a real 1,800-file mod whose JSON cache
// reached 165MB). Deterministic content, no randomness: a benchmark's numbers
// should only move because the code changed, never because the fixture did.
func syntheticModCache(n, defsPerFile int) *ModCache {
	c := newModCache("bench-game", "bench-mod")
	for i := 0; i < n; i++ {
		relPath := fmt.Sprintf("common/buildings/bench_%04d.txt", i)
		defs := make([]definition.Definition, defsPerFile)
		for j := 0; j < defsPerFile; j++ {
			defs[j] = definition.Definition{
				Type:     "common/buildings",
				ID:       fmt.Sprintf("thing_%04d_%02d", i, j),
				ModID:    "bench-mod",
				FilePath: relPath,
				Hash:     uint64(i*1000 + j),
				Span:     definition.Span{StartOffset: j * 80, EndOffset: j*80 + 79, StartLine: j * 4, EndLine: j*4 + 3},
				Order:    j,
			}
		}
		c.Put(relPath, FileRecord{
			Path:            relPath,
			ModTimeUnixNano: 1_700_000_000_000_000_000 + int64(i),
			Size:            int64(200 + defsPerFile*40),
			Hash:            uint64(i),
			Definitions:     defs,
		})
	}
	return c
}

// BenchmarkFileStore_SaveLoad is the whole gob round trip FileStore actually
// does on disk - the stage docs/performance-strategy.md's "cache format"
// case study measured switching from JSON to gob (165MB file, 386ms/585ms
// JSON encode/decode vs 120ms/124ms gob). Sub-benchmarks at a few sizes so a
// regression at one scale doesn't hide behind an average across all of them.
func BenchmarkFileStore_SaveLoad(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			c := syntheticModCache(n, 4)
			store := FileStore{Dir: b.TempDir()}
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := store.Save(ctx, c); err != nil {
					b.Fatal(err)
				}
				if _, err := store.Load(ctx, c.GameKey, c.ModID); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkFileRecord_Codec isolates the compact FileRecord codec
// (record_codec.go) from FileStore's own I/O, so the codec's own cost (the
// thing the "compact cache format" case study actually changed - shared
// strings once per file instead of once per definition) is visible on its
// own.
func BenchmarkFileRecord_Codec(b *testing.B) {
	for _, n := range []int{10, 100} {
		b.Run(fmt.Sprintf("defs=%d", n), func(b *testing.B) {
			rec := syntheticModCache(1, n).Files["common/buildings/bench_0000.txt"]
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, err := rec.GobEncode()
				if err != nil {
					b.Fatal(err)
				}
				var back FileRecord
				if err := back.GobDecode(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

package conflict

import (
	"fmt"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// mostlyUncontestedInputs builds mods mods of keysPerMod definitions each,
// shaped like a real large modlist rather than conflict_test.go's own
// randomInputs (whose deliberately tiny key alphabet is built to stress
// duplicate/collision handling exhaustively, not to model realistic
// sparsity - reusing it here would make nearly every key contested, hiding
// exactly the saving this benchmark exists to show). Instead, 90% of each
// mod's keys are globally unique to that mod (never contested by
// construction) and 10% are drawn from a small shared pool that every mod
// also writes to (always contested) - roughly the shape of the real 86-mod
// case study in docs/performance-strategy.md, where only about 0.5% of
// ~900k definitions were ever actually contested.
func mostlyUncontestedInputs(mods, keysPerMod int) ([]Input, LoadOrder) {
	const sharedPoolSize = 50
	inputs := make([]Input, mods)
	order := make(LoadOrder, mods)
	for i := 0; i < mods; i++ {
		id := fmt.Sprintf("mod_%03d", i)
		order[i] = id
		defs := make([]definition.Definition, keysPerMod)
		for j := 0; j < keysPerMod; j++ {
			key := fmt.Sprintf("uniq_%03d_%04d", i, j)
			if j%10 == 0 {
				key = fmt.Sprintf("shared_%02d", j%sharedPoolSize)
			}
			defs[j] = definition.Definition{Type: "common/bench", ID: key, ModID: id, FilePath: "f.txt", Hash: uint64(j), Order: j}
		}
		inputs[i] = Input{Mod: mod.Mod{ID: id, Descriptor: mod.Descriptor{Name: id}}, Defs: defs}
	}
	return inputs, order
}

// BenchmarkBuildIndex compares the sequential path against the
// parallel-shard path on the exact same, deliberately small input (well
// under the default shardThreshold, then forced into sharding by lowering
// it to 1) - the fairest possible side-by-side of the sharding fix
// docs/performance-strategy.md's "warm rescan" case study describes, since
// both runs see identical data and only the threshold differs.
func BenchmarkBuildIndex(b *testing.B) {
	inputs, order := mostlyUncontestedInputs(10, 2000) // ~20,000 defs: comfortably under shardThreshold (50,000)

	b.Run("sequential", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			BuildIndex(order, inputs)
		}
	})

	b.Run("sharded", func(b *testing.B) {
		old := shardThreshold
		shardThreshold = 1
		defer func() { shardThreshold = old }()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			BuildIndex(order, inputs)
		}
	})
}

// BenchmarkResolve_ConflictsOnly vs BenchmarkResolve_Full puts a real number
// behind Options.ConflictsOnly's own doc comment: skipping
// Result.Resolutions (every key, not just contested ones) is meant to save
// real work on a large, mostly-uncontested modlist - the shape
// mostlyUncontestedInputs builds, unlike a small-alphabet fixture where
// almost everything collides.
func BenchmarkResolve_ConflictsOnly(b *testing.B) {
	for _, keysPerMod := range []int{1000, 8000} {
		b.Run(fmt.Sprintf("keysPerMod=%d", keysPerMod), func(b *testing.B) {
			inputs, order := mostlyUncontestedInputs(20, keysPerMod)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				Resolve(order, inputs, Options{ConflictsOnly: true})
			}
		})
	}
}

func BenchmarkResolve_Full(b *testing.B) {
	for _, keysPerMod := range []int{1000, 8000} {
		b.Run(fmt.Sprintf("keysPerMod=%d", keysPerMod), func(b *testing.B) {
			inputs, order := mostlyUncontestedInputs(20, keysPerMod)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				Resolve(order, inputs, Options{})
			}
		})
	}
}

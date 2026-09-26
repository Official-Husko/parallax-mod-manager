package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// withMergeSafeType registers typ as merge-safe (and, if given, its
// repeatable keys) for the duration of one test - writeMod's fixture always
// uses the "common" Type, so every test in this file uses that. Restored on
// cleanup so tests never leak state into each other - conflict.MergeSafeTypes
// otherwise ships empty on purpose (see its own doc comment).
func withMergeSafeType(t *testing.T, typ string, repeatable map[string]bool) {
	t.Helper()
	dt := definition.Type(typ)
	conflict.MergeSafeTypes[dt] = true
	if repeatable != nil {
		conflict.RepeatableMergeKeys[dt] = repeatable
	}
	t.Cleanup(func() {
		delete(conflict.MergeSafeTypes, dt)
		delete(conflict.RepeatableMergeKeys, dt)
	})
}

func TestGeneratePatchMergesSafeAdditiveContent(t *testing.T) {
	withMergeSafeType(t, "common", nil)

	modDir := t.TempDir()
	// mod_a (loses on LIOS) adds @extra, which mod_b's winning version lacks;
	// both agree on @a, so nothing collides.
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { @a = 1 @extra = 99 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing = { @a = 1 }`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if result.MergedKeys != 1 {
		t.Errorf("MergedKeys = %d, want 1", result.MergedKeys)
	}
	if result.PatchedKeys != 1 {
		t.Errorf("PatchedKeys = %d, want 1", result.PatchedKeys)
	}

	contentPath := filepath.Join(modDir, patchModID, "common", patchModID+".txt")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "@a = 1") {
		t.Errorf("content = %q, want the winner's own @a entry preserved", content)
	}
	if !strings.Contains(content, "@extra = 99") {
		t.Errorf("content = %q, want mod_a's safely-additive @extra entry merged in", content)
	}
}

func TestGeneratePatchMergeSkipsWholeModOnAnyBatchFailure(t *testing.T) {
	withMergeSafeType(t, "common", nil)

	modDir := t.TempDir()
	// mod_a is a non-winning candidate on two Keys in the same file: one
	// where it could safely add something new (thing_x), and one where it
	// genuinely disagrees with the winner (thing_y). Mod-level atomicity
	// means NEITHER gets mod_a's contribution.
	writeMod(t, modDir, "mod_a", "Mod A", `thing_x = { @a = 1 @extra = 1 } thing_y = { @c = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing_x = { @a = 1 } thing_y = { @c = 2 }`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if result.MergedKeys != 0 {
		t.Errorf("MergedKeys = %d, want 0 - the whole mod must be discarded, including thing_x's own safe addition", result.MergedKeys)
	}

	contentPath := filepath.Join(modDir, patchModID, "common", patchModID+".txt")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "@extra") {
		t.Errorf("content = %q, must not contain mod_a's @extra - its whole batch should have been discarded", data)
	}
}

func TestGeneratePatchMergeLaterModCollidesWithEarlierModsAddition(t *testing.T) {
	withMergeSafeType(t, "common", nil)

	modDir := t.TempDir()
	// mod_c wins. mod_a (processed first, lowest priority) safely adds
	// @new = 1. mod_b (processed second) proposes its own, different
	// @new = 2 - which must be checked against the state mod_a already left,
	// not just against mod_c's original winner, and refused.
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { @a = 1 @new = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing = { @a = 1 @new = 2 }`)
	writeMod(t, modDir, "mod_c", "Mod C", `thing = { @a = 1 }`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b", "mod_c"},
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if result.MergedKeys != 1 {
		t.Errorf("MergedKeys = %d, want 1 (mod_a's addition still applies)", result.MergedKeys)
	}

	contentPath := filepath.Join(modDir, patchModID, "common", patchModID+".txt")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "@new = 1") {
		t.Errorf("content = %q, want mod_a's @new = 1 (processed first, load-order ascending)", content)
	}
	if strings.Contains(content, "@new = 2") {
		t.Errorf("content = %q, must not contain mod_b's colliding @new = 2", content)
	}
}

// writeModAt is writeMod but for a Type other than the fixed "common" folder
// writeMod always uses - needed to exercise the one real, confirmed
// MergeSafeTypes entry (common/governments/authorities), which writeMod's
// fixture can't reach.
func writeModAt(t *testing.T, modDir, id, name, relDir, script string) {
	t.Helper()
	writeFile(t, modDir, id+".mod", `name = "`+name+`"
path = "`+id+`"
version = "1.0"
`)
	writeFile(t, modDir, filepath.Join(id, relDir, "x.txt"), script)
}

func TestGeneratePatchMergesRealAuthoritySwapAgainstAMixedBody(t *testing.T) {
	// No withMergeSafeType call - common/governments/authorities is a real,
	// already-enabled entry. Content shape matches the real vanilla file
	// (see conflict.RepeatableMergeKeys' own doc comment): several ordinary
	// fields plus a repeated advanced_authority_swap key, not a block
	// consisting solely of swaps.
	modDir := t.TempDir()
	const relDir = "common/governments/authorities"
	writeModAt(t, modDir, "mod_a", "Mod A", relDir, `auth_democratic = {
	election_term_years = 10
	color = { 81 140 44 255 }
	advanced_authority_swap = { name = "vanilla_swap" }
	advanced_authority_swap = { name = "mod_a_new_swap" }
}`)
	writeModAt(t, modDir, "mod_b", "Mod B", relDir, `auth_democratic = {
	election_term_years = 10
	color = { 81 140 44 255 }
	advanced_authority_swap = { name = "vanilla_swap" }
}`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"}, // mod_b wins (LIOS)
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if result.MergedKeys != 1 {
		t.Fatalf("MergedKeys = %d, want 1", result.MergedKeys)
	}

	contentPath := filepath.Join(modDir, patchModID, relDir, patchModID+".txt")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `election_term_years = 10`) {
		t.Errorf("content = %q, want the winner's own ordinary fields preserved", content)
	}
	if !strings.Contains(content, `mod_a_new_swap`) {
		t.Errorf("content = %q, want mod_a's own new swap merged in", content)
	}
	if strings.Count(content, "vanilla_swap") != 1 {
		t.Errorf("content = %q, want the shared vanilla_swap entry exactly once, not duplicated", content)
	}
}

func TestGeneratePatchMergeSafeTypesEmptyByDefaultIsNoOp(t *testing.T) {
	// No withMergeSafeType call - the plain "common" Type used by writeMod's
	// fixture isn't on conflict.MergeSafeTypes (only
	// common/governments/authorities is). A candidate that *could*
	// additively merge must not be merged when its own Type isn't opted in.
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { @a = 1 @extra = 99 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing = { @a = 1 }`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if result.MergedKeys != 0 {
		t.Errorf("MergedKeys = %d, want 0 - MergeSafeTypes is empty by default", result.MergedKeys)
	}
	contentPath := filepath.Join(modDir, patchModID, "common", patchModID+".txt")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "@extra") {
		t.Errorf("content = %q, must not contain mod_a's @extra with no Type opted in", data)
	}
}

package conflict

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

// blockOf parses src as one top-level "key = { ... }" definition and returns
// its own inner Block - the real parser, not hand-built structs, so these
// tests exercise real Offset/Key/Value shapes.
func blockOf(t *testing.T, src string) script.Block {
	t.Helper()
	f, err := script.Parse([]byte(src))
	if err != nil {
		t.Fatalf("script.Parse: %v", err)
	}
	if len(f.Root.Entries) != 1 || f.Root.Entries[0].Value.Kind != script.KindBlock {
		t.Fatalf("expected exactly one top-level block entry, got %+v", f.Root.Entries)
	}
	return *f.Root.Entries[0].Value.Block
}

func TestAdditiveEntriesNewKeyIsSafeToAdd(t *testing.T) {
	winner := blockOf(t, `thing = { @a = 1 }`)
	candidate := blockOf(t, `thing = { @a = 1 @b = 2 }`)

	added, ok := AdditiveEntries(winner, candidate, nil)
	if !ok {
		t.Fatal("expected ok = true")
	}
	if len(added) != 1 || added[0].Key != "@b" {
		t.Errorf("added = %+v, want exactly one entry keyed @b", added)
	}
}

func TestAdditiveEntriesExactDuplicateIsNoOp(t *testing.T) {
	winner := blockOf(t, `thing = { @a = 1 }`)
	candidate := blockOf(t, `thing = { @a = 1 }`) // byte-identical entry, different mod's file

	added, ok := AdditiveEntries(winner, candidate, nil)
	if !ok {
		t.Fatal("expected ok = true")
	}
	if len(added) != 0 {
		t.Errorf("added = %+v, want none - already effectively present", added)
	}
}

func TestAdditiveEntriesSameKeyDifferentContentIsRefused(t *testing.T) {
	winner := blockOf(t, `thing = { @a = 1 }`)
	candidate := blockOf(t, `thing = { @a = 2 }`) // same key, genuinely different value

	_, ok := AdditiveEntries(winner, candidate, nil)
	if ok {
		t.Fatal("expected ok = false - a real disagreement over the same key")
	}
}

func TestAdditiveEntriesRepeatableKeyAllowsNewInstance(t *testing.T) {
	winner := blockOf(t, `thing = { swap = { name = "a" } }`)
	candidate := blockOf(t, `thing = { swap = { name = "a" } swap = { name = "b" } }`)
	repeatable := map[string]bool{"swap": true}

	added, ok := AdditiveEntries(winner, candidate, repeatable)
	if !ok {
		t.Fatal("expected ok = true for a confirmed repeatable key")
	}
	if len(added) != 1 || added[0].Key != "swap" {
		t.Fatalf("added = %+v, want exactly one new swap entry", added)
	}
	// The already-present swap (identical content) must not be duplicated.
	for _, a := range added {
		if a.Value.Block != nil {
			for _, e := range a.Value.Block.Entries {
				if e.Key == "name" && e.Value.Raw == "a" {
					t.Error("the already-present swap entry was added again")
				}
			}
		}
	}
}

func TestAdditiveEntriesRepeatableKeyStillDedupesExactRepeat(t *testing.T) {
	winner := blockOf(t, `thing = { swap = { name = "a" } }`)
	candidate := blockOf(t, `thing = { swap = { name = "a" } }`) // same repeatable key, byte-identical
	repeatable := map[string]bool{"swap": true}

	added, ok := AdditiveEntries(winner, candidate, repeatable)
	if !ok {
		t.Fatal("expected ok = true")
	}
	if len(added) != 0 {
		t.Errorf("added = %+v, want none - exact repeat of what's already there", added)
	}
}

func TestAdditiveEntriesBareListEntriesMatchByContentOnly(t *testing.T) {
	// A nested bare list: two mods each contribute their own event refs.
	winnerList := blockOf(t, `events = { a.1 }`)
	candidateList := blockOf(t, `events = { a.1 b.2 }`)

	added, ok := AdditiveEntries(winnerList, candidateList, nil)
	if !ok {
		t.Fatal("expected ok = true")
	}
	if len(added) != 1 || added[0].Key != "" || added[0].Value.Raw != "b.2" {
		t.Errorf("added = %+v, want exactly one bare entry b.2", added)
	}
}

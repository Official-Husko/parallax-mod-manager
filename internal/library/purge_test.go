package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFindEmptyModsFindsMissingAndEmptyLocalMods(t *testing.T) {
	modDir := t.TempDir()

	// A real, normal mod - must never be flagged.
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)

	// A local mod whose declared content path doesn't exist.
	writeFile(t, modDir, "stale_mod.mod", `name = "Stale Mod"
path = "/this/path/does/not/exist"
`)

	// A local mod whose content folder exists but has no files at all.
	writeFile(t, modDir, "empty_mod.mod", `name = "Empty Mod"
path = "empty_mod"
`)
	if err := os.MkdirAll(filepath.Join(modDir, "empty_mod"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	candidates, err := FindEmptyMods(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("FindEmptyMods: %v", err)
	}

	byID := map[string]EmptyModCandidate{}
	for _, c := range candidates {
		byID[c.ID] = c
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %+v", len(candidates), candidates)
	}
	if _, ok := byID["mod_a"]; ok {
		t.Error("mod_a is a real mod with real content, should not be flagged")
	}
	if c, ok := byID["stale_mod"]; !ok || c.Reason != "Content folder doesn't exist" {
		t.Errorf("stale_mod = %+v (found=%v), want Reason %q", c, ok, "Content folder doesn't exist")
	}
	if c, ok := byID["empty_mod"]; !ok || c.Reason != "Content folder is empty" {
		t.Errorf("empty_mod = %+v (found=%v), want Reason %q", c, ok, "Content folder is empty")
	}
}

// TestFindEmptyModsReturnsRealEmptySliceWhenNoneFound pins the same class
// of bug already found once in this project (see library.go's package doc
// comment): a Go nil slice marshals as JSON null, and the frontend's
// loading state is exactly "still null" - so a genuinely empty result
// (zero candidates) would be indistinguishable from "hasn't loaded yet",
// leaving the purge dialog stuck on its loading message forever.
func TestFindEmptyModsReturnsRealEmptySliceWhenNoneFound(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)

	candidates, err := FindEmptyMods(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("FindEmptyMods: %v", err)
	}
	if candidates == nil {
		t.Error("candidates is nil, want a real empty slice (marshals as JSON null, not [])")
	}
}

func TestFindEmptyModsNeverFlagsWorkshopMods(t *testing.T) {
	// A Workshop item with no content yet is far more likely still
	// downloading than genuinely broken - must never be a purge candidate,
	// regardless of how empty/missing its content currently looks.
	modDir := t.TempDir()
	writeFile(t, modDir, "ugc_123.mod", `name = "Not Downloaded Yet"
path = "/this/path/does/not/exist"
remote_file_id = "123"
`)

	candidates, err := FindEmptyMods(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("FindEmptyMods: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected no candidates, got %+v", candidates)
	}
}

func TestPurgeModsDeletesOnlyTheDescriptorFile(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "empty_mod.mod", `name = "Empty Mod"
path = "empty_mod"
`)
	if err := os.MkdirAll(filepath.Join(modDir, "empty_mod"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	result, err := PurgeMods(context.Background(), testGameConfig(), Options{ModDir: modDir}, []string{"empty_mod"})
	if err != nil {
		t.Fatalf("PurgeMods: %v", err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != "empty_mod" {
		t.Errorf("Deleted = %+v, want [empty_mod]", result.Deleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %+v, want none", result.Errors)
	}

	if _, err := os.Stat(filepath.Join(modDir, "empty_mod.mod")); !os.IsNotExist(err) {
		t.Errorf("expected the descriptor file to be gone, stat err = %v", err)
	}
	// The (empty) content folder itself must be left alone - only the
	// descriptor stub is ever deleted.
	if _, err := os.Stat(filepath.Join(modDir, "empty_mod")); err != nil {
		t.Errorf("expected the content folder to still exist, stat err = %v", err)
	}
}

func TestPurgeModsExcludesUnselectedMods(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "empty_a.mod", `name = "Empty A"
path = "empty_a"
`)
	writeFile(t, modDir, "empty_b.mod", `name = "Empty B"
path = "empty_b"
`)
	for _, dir := range []string{"empty_a", "empty_b"} {
		if err := os.MkdirAll(filepath.Join(modDir, dir), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	// Only empty_a is selected for deletion - empty_b must be untouched,
	// exactly like a user unchecking it in the confirmation dialog.
	result, err := PurgeMods(context.Background(), testGameConfig(), Options{ModDir: modDir}, []string{"empty_a"})
	if err != nil {
		t.Fatalf("PurgeMods: %v", err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != "empty_a" {
		t.Errorf("Deleted = %+v, want [empty_a]", result.Deleted)
	}
	if _, err := os.Stat(filepath.Join(modDir, "empty_b.mod")); err != nil {
		t.Errorf("expected empty_b's descriptor to be untouched, stat err = %v", err)
	}
}

func TestPurgeModsRejectsRealContentEvenIfRequested(t *testing.T) {
	// A safety net against a stale selection: even if the caller asks to
	// delete a mod that no longer qualifies (its content reappeared since
	// the candidate list was shown), PurgeMods must refuse rather than
	// trust the request blindly.
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)

	result, err := PurgeMods(context.Background(), testGameConfig(), Options{ModDir: modDir}, []string{"mod_a"})
	if err != nil {
		t.Fatalf("PurgeMods: %v", err)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %+v, want none - mod_a has real content", result.Deleted)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %+v, want exactly 1 explaining the refusal", result.Errors)
	}
	if _, err := os.Stat(filepath.Join(modDir, "mod_a.mod")); err != nil {
		t.Errorf("expected mod_a's descriptor to be untouched, stat err = %v", err)
	}
}

// TestPurgeModsReturnsRealEmptySlicesWhenNothingHappened pins the same
// nil-slice-crosses-as-JSON-null class of bug: an empty modIDs call (or
// one where every entry is rejected) must still hand the frontend real
// empty arrays for Deleted/Errors, not null.
func TestPurgeModsReturnsRealEmptySlicesWhenNothingHappened(t *testing.T) {
	modDir := t.TempDir()

	result, err := PurgeMods(context.Background(), testGameConfig(), Options{ModDir: modDir}, nil)
	if err != nil {
		t.Fatalf("PurgeMods: %v", err)
	}
	if result.Deleted == nil {
		t.Error("Deleted is nil, want a real empty slice (marshals as JSON null, not [])")
	}
	if result.Errors == nil {
		t.Error("Errors is nil, want a real empty slice (marshals as JSON null, not [])")
	}
}

func TestPurgeModsRejectsWorkshopMods(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "ugc_123.mod", `name = "Not Downloaded Yet"
path = "/this/path/does/not/exist"
remote_file_id = "123"
`)

	result, err := PurgeMods(context.Background(), testGameConfig(), Options{ModDir: modDir}, []string{"ugc_123"})
	if err != nil {
		t.Fatalf("PurgeMods: %v", err)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %+v, want none - Workshop mods are never purged", result.Deleted)
	}
	if _, err := os.Stat(filepath.Join(modDir, "ugc_123.mod")); err != nil {
		t.Errorf("expected the Workshop stub to be untouched, stat err = %v", err)
	}
}

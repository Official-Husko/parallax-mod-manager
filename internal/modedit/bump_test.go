package modedit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveDirMatchesApplysOwnNamingRule(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	at := time.Date(2026, 1, 2, 3, 4, 5, 6000, time.UTC)
	got := s.SaveDir(at)
	want := filepath.Join(s.Dir, "20260102T030405.000006000Z")
	if got != want {
		t.Errorf("SaveDir(%v) = %q, want %q", at, got, want)
	}
}

func TestVersionSnapshotRoundTrips(t *testing.T) {
	root := t.TempDir()
	s := Store{Dir: t.TempDir(), Roots: []string{root}}
	writes := []Write{{Path: filepath.Join(root, "descriptor.mod"), Data: []byte("name=\"A\"\n")}}
	at, err := s.Apply(writes)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if _, ok := s.LatestVersionSnapshot(); ok {
		t.Fatal("LatestVersionSnapshot found one before any was written")
	}

	snap := VersionSnapshot{Fields: Fields{Name: "A", Version: "1.0", Tags: []string{"X"}}, ContentFileCount: 5, SavedAt: at}
	if err := WriteVersionSnapshot(s.SaveDir(at), snap); err != nil {
		t.Fatalf("WriteVersionSnapshot: %v", err)
	}

	got, ok := s.LatestVersionSnapshot()
	if !ok {
		t.Fatal("LatestVersionSnapshot found nothing after writing one")
	}
	if got.Fields.Name != "A" || got.ContentFileCount != 5 || len(got.Fields.Tags) != 1 || got.Fields.Tags[0] != "X" {
		t.Errorf("LatestVersionSnapshot = %+v, want a match for %+v", got, snap)
	}
}

func TestSuggestBumpNoSuggestionWhenNothingChanged(t *testing.T) {
	fields := Fields{Name: "A", Version: "3.2", Tags: []string{"X"}}
	last := VersionSnapshot{Fields: fields, ContentFileCount: 10}
	if _, ok := SuggestBump(last, fields, 10, "3.2"); ok {
		t.Error("SuggestBump found a suggestion when nothing changed, want none")
	}
}

func TestSuggestBumpNoSuggestionWhenVersionDoesNotParse(t *testing.T) {
	last := VersionSnapshot{Fields: Fields{Tags: []string{"X"}}, ContentFileCount: 1}
	current := Fields{Tags: []string{"X", "Y"}}
	if _, ok := SuggestBump(last, current, 5, "latest"); ok {
		t.Error("SuggestBump found a suggestion for an unparseable version, want none")
	}
}

// TestSuggestBumpMatchesTheMockupsOwnWorkedExample: files added and tags changed since 3.2 was
// saved -> Minor, exactly as the redesign's own reference screen shows.
func TestSuggestBumpMatchesTheMockupsOwnWorkedExample(t *testing.T) {
	last := VersionSnapshot{Fields: Fields{Name: "A", Version: "3.2", Tags: []string{"Graphics"}}, ContentFileCount: 10}
	current := Fields{Name: "A", Version: "3.2", Tags: []string{"Graphics", "Species", "Portraits"}}
	got, ok := SuggestBump(last, current, 48, "3.2")
	if !ok {
		t.Fatal("SuggestBump found nothing, want a Minor suggestion")
	}
	if got.Suggested != BumpMinor {
		t.Errorf("Suggested = %q, want %q", got.Suggested, BumpMinor)
	}
	if got.Options[BumpMinor].String() != "3.3" {
		t.Errorf("Options[minor] = %q, want %q", got.Options[BumpMinor].String(), "3.3")
	}
	want := "38 files added and tags changed since 3.2 was saved."
	if got.Reason != want {
		t.Errorf("Reason = %q, want %q", got.Reason, want)
	}
}

func TestSuggestBumpSuggestsMajorForAReplacePathsChange(t *testing.T) {
	last := VersionSnapshot{Fields: Fields{Version: "1.0", ReplacePaths: []string{"common/buildings"}}, ContentFileCount: 3}
	current := Fields{Version: "1.0", ReplacePaths: []string{"common/buildings", "common/species_classes"}}
	got, ok := SuggestBump(last, current, 3, "1.0")
	if !ok || got.Suggested != BumpMajor {
		t.Fatalf("SuggestBump = %+v, ok=%v, want Major", got, ok)
	}
}

func TestSuggestBumpSuggestsMajorForAMadeForGameMajorChange(t *testing.T) {
	last := VersionSnapshot{Fields: Fields{Version: "1.0", SupportedVersion: "v3.*"}, ContentFileCount: 3}
	current := Fields{Version: "1.0", SupportedVersion: "v4.*"}
	got, ok := SuggestBump(last, current, 3, "1.0")
	if !ok || got.Suggested != BumpMajor {
		t.Fatalf("SuggestBump = %+v, ok=%v, want Major", got, ok)
	}
}

func TestSuggestBumpSuggestsPatchForASmallChange(t *testing.T) {
	last := VersionSnapshot{Fields: Fields{Name: "Old Name", Version: "1.0"}, ContentFileCount: 3}
	current := Fields{Name: "New Name", Version: "1.0"}
	got, ok := SuggestBump(last, current, 3, "1.0")
	if !ok || got.Suggested != BumpPatch {
		t.Fatalf("SuggestBump = %+v, ok=%v, want Patch", got, ok)
	}
}

func TestSuggestBumpSuggestsMinorForARemovedFile(t *testing.T) {
	last := VersionSnapshot{Fields: Fields{Version: "1.0"}, ContentFileCount: 10}
	current := Fields{Version: "1.0"}
	got, ok := SuggestBump(last, current, 7, "1.0")
	if !ok || got.Suggested != BumpMinor {
		t.Fatalf("SuggestBump = %+v, ok=%v, want Minor", got, ok)
	}
	want := "3 files removed since 1.0 was saved."
	if got.Reason != want {
		t.Errorf("Reason = %q, want %q", got.Reason, want)
	}
}

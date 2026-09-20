package modupdates

import (
	"testing"
)

func fp(files int, size, newest int64) Fingerprint {
	return Fingerprint{Known: true, Files: files, Size: size, Newest: newest}
}

func snap(at int64, recs map[string]Record) Snapshot {
	return Snapshot{TakenAt: at, Mods: recs}
}

func findChange(t *testing.T, changes []Change, id string) Change {
	t.Helper()
	for _, c := range changes {
		if c.ModID == id {
			return c
		}
	}
	t.Fatalf("no change for %q in %+v", id, changes)
	return Change{}
}

func hasChange(changes []Change, id string) bool {
	for _, c := range changes {
		if c.ModID == id {
			return true
		}
	}
	return false
}

func TestDiffUpdatedByVersion(t *testing.T) {
	base := snap(1, map[string]Record{"a": {Name: "A", Version: "1.4", Content: fp(10, 100, 5)}})
	cur := snap(2, map[string]Record{"a": {Name: "A", Version: "1.5", Content: fp(12, 150, 9)}})
	c := findChange(t, Diff(&base, cur), "a")
	if c.Kind != KindUpdated || !c.New {
		t.Errorf("kind/new = %v/%v, want updated/true", c.Kind, c.New)
	}
	if c.FromVersion != "1.4" || c.ToVersion != "1.5" {
		t.Errorf("versions = %q -> %q", c.FromVersion, c.ToVersion)
	}
	if !c.FilesChanged || c.FilesDelta != 2 || c.SizeDelta != 50 {
		t.Errorf("files = %v delta %d size %d, want true/2/50", c.FilesChanged, c.FilesDelta, c.SizeDelta)
	}
}

func TestDiffUpdatedByWorkshopTime(t *testing.T) {
	base := snap(1, map[string]Record{"a": {Name: "A", Version: "1.0", WorkshopUpdated: 100, Content: fp(1, 1, 1)}})
	cur := snap(2, map[string]Record{"a": {Name: "A", Version: "1.0", WorkshopUpdated: 200, Content: fp(1, 1, 1)}})
	c := findChange(t, Diff(&base, cur), "a")
	if c.Kind != KindUpdated || c.WorkshopUpdated != 200 {
		t.Errorf("got %+v, want updated with workshop time 200", c)
	}
	if c.FilesChanged {
		t.Error("files unchanged, but FilesChanged is set")
	}
	if c.FromVersion != "" || c.ToVersion != "" {
		t.Errorf("version did not change, got %q -> %q", c.FromVersion, c.ToVersion)
	}
}

func TestDiffWorkshopTimeNeedsAKnownBefore(t *testing.T) {
	// Steam could not be asked last time (0): a date now is not an update.
	base := snap(1, map[string]Record{"a": {Name: "A", WorkshopUpdated: 0, Content: fp(1, 1, 1)}})
	cur := snap(2, map[string]Record{"a": {Name: "A", WorkshopUpdated: 500, Content: fp(1, 1, 1)}})
	if changes := Diff(&base, cur); len(changes) != 0 {
		t.Errorf("changes = %+v, want none", changes)
	}
}

func TestDiffChangedWhenOnlyFilesDiffer(t *testing.T) {
	base := snap(1, map[string]Record{"a": {Name: "A", Version: "1.0", Content: fp(3, 30, 5)}})
	cur := snap(2, map[string]Record{"a": {Name: "A", Version: "1.0", Content: fp(3, 30, 9)}})
	c := findChange(t, Diff(&base, cur), "a")
	if c.Kind != KindChanged || !c.FilesChanged || c.FilesDelta != 0 || c.SizeDelta != 0 {
		t.Errorf("got %+v, want changed by a rewrite in place (deltas zero)", c)
	}
}

func TestDiffUnknownFingerprintNeverReportsAChange(t *testing.T) {
	base := snap(1, map[string]Record{"a": {Name: "A", Content: Fingerprint{}}, "b": {Name: "B", Content: fp(1, 1, 1)}})
	cur := snap(2, map[string]Record{"a": {Name: "A", Content: fp(9, 9, 9)}, "b": {Name: "B", Content: Fingerprint{}}})
	if changes := Diff(&base, cur); len(changes) != 0 {
		t.Errorf("changes = %+v, want none - an unreadable side can't be compared", changes)
	}
}

func TestDiffNothingChanged(t *testing.T) {
	recs := map[string]Record{"a": {Name: "A", Version: "1", WorkshopUpdated: 5, Content: fp(1, 1, 1)}}
	base, cur := snap(1, recs), snap(2, recs)
	if changes := Diff(&base, cur); len(changes) != 0 {
		t.Errorf("changes = %+v, want none", changes)
	}
}

func TestDiffRemoved(t *testing.T) {
	base := snap(1, map[string]Record{"a": {Name: "Gone Mod", Source: "workshop", RemoteFileID: "9"}, "b": {Name: "B"}})
	cur := snap(2, map[string]Record{"b": {Name: "B"}})
	c := findChange(t, Diff(&base, cur), "a")
	if c.Kind != KindRemoved || !c.New || c.Name != "Gone Mod" || c.RemoteFileID != "9" {
		t.Errorf("got %+v", c)
	}
}

func TestDiffDeletedFromWorkshopIsNewOnlyTheFirstTime(t *testing.T) {
	alive := Record{Name: "A", Source: "workshop", RemoteFileID: "1"}
	gone := Record{Name: "A", Source: "workshop", RemoteFileID: "1", WorkshopGone: true, GoneSince: 77}

	base := snap(1, map[string]Record{"a": alive})
	c := findChange(t, Diff(&base, snap(2, map[string]Record{"a": gone})), "a")
	if c.Kind != KindDeleted || !c.New || c.GoneSince != 77 {
		t.Errorf("first time: got %+v, want new deleted since 77", c)
	}

	base = snap(1, map[string]Record{"a": gone})
	c = findChange(t, Diff(&base, snap(2, map[string]Record{"a": gone})), "a")
	if c.Kind != KindDeleted || c.New {
		t.Errorf("already gone before: got %+v, want a standing (not new) entry", c)
	}
}

func TestDiffWithoutABaselineOnlyListsStandingDeletions(t *testing.T) {
	cur := snap(2, map[string]Record{
		"ok":   {Name: "OK", Version: "1", Content: fp(1, 1, 1)},
		"gone": {Name: "Gone", Source: "workshop", WorkshopGone: true, GoneSince: 2},
	})
	changes := Diff(nil, cur)
	if len(changes) != 1 || changes[0].ModID != "gone" || changes[0].Kind != KindDeleted || changes[0].New {
		t.Errorf("changes = %+v, want just the standing deletion", changes)
	}
}

func TestDiffIgnoresModsAddedSinceTheBaseline(t *testing.T) {
	base := snap(1, map[string]Record{"a": {Name: "A"}})
	cur := snap(2, map[string]Record{"a": {Name: "A"}, "new": {Name: "Brand New", Version: "1"}})
	if hasChange(Diff(&base, cur), "new") {
		t.Error("a newly installed mod was reported as changed")
	}
}

func TestDiffIsSortedByKindThenName(t *testing.T) {
	base := snap(1, map[string]Record{
		"u2": {Name: "beta", Version: "1"}, "u1": {Name: "Alpha", Version: "1"},
		"c1": {Name: "Chg", Content: fp(1, 1, 1)}, "r1": {Name: "Removed"}, "d1": {Name: "Del", Source: "workshop"},
	})
	cur := snap(2, map[string]Record{
		"u2": {Name: "beta", Version: "2"}, "u1": {Name: "Alpha", Version: "2"},
		"c1": {Name: "Chg", Content: fp(2, 1, 1)}, "d1": {Name: "Del", Source: "workshop", WorkshopGone: true},
	})
	var order []string
	for _, c := range Diff(&base, cur) {
		order = append(order, c.ModID)
	}
	want := []string{"d1", "r1", "u1", "u2", "c1"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

package modupdates

import "testing"

func TestTrackerFirstRunHasNoBaselineAndSavesForNext(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	tr := &Tracker{Store: store}
	cur := snap(10, map[string]Record{"a": {Name: "A", Version: "1"}})

	r, err := tr.Commit("g", cur, true, "")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if r.BaselineAt != 0 || len(r.Changes) != 0 || r.Changes == nil || r.ModsChecked != 1 || !r.WorkshopChecked {
		t.Errorf("report = %+v, want no baseline, an empty non-nil change list", r)
	}
	if saved := store.Load("g"); saved == nil || saved.TakenAt != 10 {
		t.Errorf("saved = %+v, want the snapshot written for the next run", saved)
	}
}

func TestTrackerComparesAgainstThePreviousRunNotTheLastCheck(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	// The previous run left this behind.
	if err := store.Save("g", snap(1, map[string]Record{"a": {Name: "A", Version: "1"}})); err != nil {
		t.Fatal(err)
	}

	tr := &Tracker{Store: store}
	r, _ := tr.Commit("g", snap(2, map[string]Record{"a": {Name: "A", Version: "2"}}), true, "")
	if r.BaselineAt != 1 || len(r.Changes) != 1 || r.Changes[0].Kind != KindUpdated {
		t.Fatalf("first check = %+v, want the update since the previous run", r)
	}

	// A later check in the same run still reports it: the baseline does not
	// move to the newest snapshot.
	r, _ = tr.Commit("g", snap(3, map[string]Record{"a": {Name: "A", Version: "2"}}), true, "")
	if len(r.Changes) != 1 {
		t.Errorf("second check = %+v, want the same update still reported", r.Changes)
	}
}

func TestTrackerNextRunStartsFromWhatWasSaved(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	first := &Tracker{Store: store}
	first.Commit("g", snap(1, map[string]Record{"a": {Name: "A", Version: "1"}}), true, "")
	first.Commit("g", snap(2, map[string]Record{"a": {Name: "A", Version: "2"}}), true, "")

	// A restart: the new run's baseline is the newest thing the last run saved,
	// so the update it already showed is not reported again.
	next := &Tracker{Store: store}
	r, _ := next.Commit("g", snap(3, map[string]Record{"a": {Name: "A", Version: "2"}}), true, "")
	if r.BaselineAt != 2 || len(r.Changes) != 0 {
		t.Errorf("report = %+v, want baseline 2 and no changes", r)
	}
}

func TestTrackerMarkSeenClearsWhatWasReported(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	store.Save("g", snap(1, map[string]Record{"a": {Name: "A", Version: "1"}}))
	tr := &Tracker{Store: store}
	tr.Commit("g", snap(2, map[string]Record{"a": {Name: "A", Version: "2"}}), true, "")

	if marked := tr.MarkSeen("g"); len(marked.Changes) != 0 || marked.BaselineAt != 2 || marked.Changes == nil {
		t.Errorf("MarkSeen report = %+v, want an empty list against baseline 2", marked)
	}
	r, _ := tr.Commit("g", snap(3, map[string]Record{"a": {Name: "A", Version: "2"}}), true, "")
	if len(r.Changes) != 0 || r.BaselineAt != 2 {
		t.Errorf("after MarkSeen = %+v, want nothing and baseline 2", r)
	}
	r, _ = tr.Commit("g", snap(4, map[string]Record{"a": {Name: "A", Version: "3"}}), true, "")
	if len(r.Changes) != 1 {
		t.Errorf("a later update should report again, got %+v", r.Changes)
	}
}

func TestTrackerUnreadableScanIsNotEveryModRemoved(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	store.Save("g", snap(1, map[string]Record{"a": {Name: "A"}, "b": {Name: "B"}}))
	tr := &Tracker{Store: store}

	r, err := tr.Commit("g", snap(2, map[string]Record{}), true, "")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !r.Unreadable || len(r.Changes) != 0 {
		t.Errorf("report = %+v, want Unreadable with nothing reported", r)
	}
	if saved := store.Load("g"); saved == nil || saved.TakenAt != 1 || len(saved.Mods) != 2 {
		t.Errorf("saved = %+v, want the earlier snapshot left alone", saved)
	}
	if prev := tr.Previous("g"); prev == nil || len(prev.Mods) != 2 {
		t.Errorf("Previous = %+v, want the earlier snapshot kept", prev)
	}
}

func TestTrackerGamesAreIndependent(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	store.Save("stellaris", snap(1, map[string]Record{"a": {Name: "A", Version: "1"}}))
	tr := &Tracker{Store: store}
	r, _ := tr.Commit("hoi4", snap(2, map[string]Record{"a": {Name: "A", Version: "9"}}), true, "")
	if r.BaselineAt != 0 || len(r.Changes) != 0 {
		t.Errorf("hoi4 = %+v, want no baseline of its own", r)
	}
}

func TestTrackerSaveFailureStillReports(t *testing.T) {
	tr := &Tracker{} // no Store dir: nothing can be saved
	r, err := tr.Commit("g", snap(1, map[string]Record{"a": {Name: "A"}}), false, "offline")
	if err == nil {
		t.Error("expected the save error to be returned")
	}
	if r.ModsChecked != 1 || r.WorkshopChecked || r.WorkshopError != "offline" {
		t.Errorf("report = %+v, want it filled in regardless", r)
	}
}

func TestTrackerMarkSeenKeepsStandingDeletions(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	tr := &Tracker{Store: store}
	gone := Record{Name: "Gone", Source: SourceWorkshop, RemoteFileID: "1", WorkshopGone: true, GoneSince: 5}
	tr.Commit("g", snap(5, map[string]Record{"a": gone}), true, "")
	marked := tr.MarkSeen("g")
	if len(marked.Changes) != 1 || marked.Changes[0].Kind != KindDeleted || marked.Changes[0].New {
		t.Errorf("MarkSeen = %+v, want the standing deletion kept, not new", marked.Changes)
	}
}

func TestTrackerMarkSeenBeforeAnyCheck(t *testing.T) {
	tr := &Tracker{Store: Store{Dir: t.TempDir()}}
	if r := tr.MarkSeen("g"); r.Changes == nil || len(r.Changes) != 0 {
		t.Errorf("report = %+v, want an empty non-nil list", r)
	}
}

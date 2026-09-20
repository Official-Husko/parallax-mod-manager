package modupdates

import "sync"

// Report is the result of one check, handed to the frontend.
type Report struct {
	GameID string
	// CheckedAt is when the check ran, in unix seconds.
	CheckedAt int64
	// BaselineAt is when the snapshot compared against was taken (unix
	// seconds), or 0 when there was none: this is the first run with this
	// feature, so there is nothing to compare yet and tracking starts now.
	BaselineAt int64
	Changes    []Change
	// ModsChecked is how many mods were looked at.
	ModsChecked int
	// WorkshopChecked is true when Steam was asked and answered; when false,
	// WorkshopError says why, and updates and deletions on the Workshop could
	// not be seen (changes to the files still can).
	WorkshopChecked bool
	WorkshopError   string
	// Unreadable is true when the scan found no mods although the last one
	// found some - a drive that is not connected, say. Nothing is compared or
	// saved then, so a missing drive never reads as every mod being removed.
	Unreadable bool
}

// Tracker holds, per game, what each check compares against. The baseline is the
// snapshot from before this run (loaded on first use) and stays fixed for the
// whole run, so the report keeps saying what changed since the last startup even
// as checks repeat; last is the newest snapshot, which is what gets saved for
// the next run to compare with. The zero value works.
type Tracker struct {
	Store Store

	mu    sync.Mutex
	games map[string]*tracked
}

type tracked struct {
	baseline *Snapshot
	last     *Snapshot
	// report is the newest report handed out, which MarkSeen re-derives.
	report Report
}

func (t *Tracker) game(gameID string) *tracked {
	if t.games == nil {
		t.games = map[string]*tracked{}
	}
	g, ok := t.games[gameID]
	if !ok {
		saved := t.Store.Load(gameID)
		g = &tracked{baseline: saved, last: saved}
		t.games[gameID] = g
	}
	return g
}

// Previous returns the newest snapshot known for gameID (or nil), for Build to
// carry forward what it knew about the Workshop.
func (t *Tracker) Previous(gameID string) *Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.game(gameID).last
}

// Commit compares cur with the baseline, saves cur for the next run and returns
// the report. The error is only that saving failed - the report is still valid.
func (t *Tracker) Commit(gameID string, cur Snapshot, workshopChecked bool, workshopError string) (Report, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	g := t.game(gameID)

	report := Report{
		GameID:          gameID,
		CheckedAt:       cur.TakenAt,
		ModsChecked:     len(cur.Mods),
		WorkshopChecked: workshopChecked,
		WorkshopError:   workshopError,
		// Never nil: a nil slice crosses to the frontend as JSON null, which
		// its generated types do not allow for.
		Changes: []Change{},
	}
	if g.baseline != nil {
		report.BaselineAt = g.baseline.TakenAt
	}
	if len(cur.Mods) == 0 && g.last != nil && len(g.last.Mods) > 0 {
		report.Unreadable = true
		return report, nil
	}
	if changes := Diff(g.baseline, cur); changes != nil {
		report.Changes = changes
	}
	g.last = &cur
	g.report = report
	return report, t.Store.Save(gameID, cur)
}

// MarkSeen makes what is installed now the baseline, so what was reported so
// far stops being reported (until something else changes), and returns the
// report as it now reads: only what is still standing, such as mods already
// deleted from the Workshop. Before any check has run there is nothing to mark
// and the report is empty.
func (t *Tracker) MarkSeen(gameID string) Report {
	t.mu.Lock()
	defer t.mu.Unlock()
	g := t.game(gameID)
	if g.last == nil {
		return Report{GameID: gameID, Changes: []Change{}}
	}
	g.baseline = g.last
	report := g.report
	report.GameID = gameID
	report.BaselineAt = g.baseline.TakenAt
	report.Changes = []Change{}
	if changes := Diff(g.baseline, *g.last); changes != nil {
		report.Changes = changes
	}
	g.report = report
	return report
}

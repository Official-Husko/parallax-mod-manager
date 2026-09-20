import {library} from '../../wailsjs/go/models';

// The conflicts the backend reports are those of the last scan, which resolves
// only the mods of the playset it was given (see App.ScanGame): saving or
// loading a playset rescans, but editing the load order does not. So after
// clearing the list, starting a new one, or removing mods, the reported
// conflicts still describe mods that are no longer active. This narrows them to
// what the load order holds right now:
//
// - a conflict whose candidates are all still active is kept as it was;
// - one that lost some candidates is kept, trimmed to the active ones, while
//   two or more remain and the scanned winner is still one of them;
// - anything else is dropped. Its winner would have to be recomputed under the
//   game's own merge rule, and showing a stale one is worse than showing
//   nothing until the next save re-checks it.
//
// Mods added since the last scan have no conflicts in the data at all, so those
// only appear after a save - see listEditedSinceScan.
export function liveConflicts(conflicts: library.ConflictSummary[], active: Set<string>): library.ConflictSummary[] {
    const live: library.ConflictSummary[] = [];
    for (const c of conflicts) {
        const candidates = c.Candidates.filter((cand) => active.has(cand.ModID));
        if (candidates.length === c.Candidates.length) {
            live.push(c);
        } else if (candidates.length >= 2 && candidates.some((cand) => cand.ModID === c.Winner)) {
            live.push(new library.ConflictSummary({...c, Candidates: candidates}));
        }
    }
    return live;
}

// Whether the load order holds different mods than the last scan resolved -
// mods were added or removed since, so the conflict data is out of date until a
// save re-checks it.
export function listEditedSinceScan(order: string[], mods: library.ModSummary[]): boolean {
    const scanned = new Set<string>();
    for (const m of mods) if (m.Enabled) scanned.add(m.ID);
    return order.length !== scanned.size || order.some((id) => !scanned.has(id));
}

// A tiny, module-level running total of Checks tab activity this session - never cleared by a
// mod switch or a "mods-changed" event (unlike Editor.tsx's own per-mod checkResults cache,
// which is exactly that kind of snapshot). Deliberately a standalone singleton, the same
// lightweight pattern data/notifications.ts already uses, rather than threading a prop through
// Editor.tsx/EditorChecks.tsx for one small stat: any tab that runs a check calls recordCheck
// once, and the Checks tab's own Result cache card reads the running total back.

interface SessionStats {
    totalBytes: number;
    modsChecked: Set<string>;
    // The first run this session that also built a game's base index from scratch, and how
    // long that one run took - recorded once and never overwritten, so a much faster run later
    // doesn't erase it. Lets the Result cache card's "Last run" stat tell a slow first run
    // apart from every fast one after it, the way app.CheckResult.IndexBuilt marks it server-side.
    firstIndexDurationMs: number | null;
}

const stats: SessionStats = {totalBytes: 0, modsChecked: new Set(), firstIndexDurationMs: null};

export function recordCheck(modId: string, resultBytes: number, durationMS?: number, indexBuilt?: boolean): void {
    stats.totalBytes += Math.max(0, resultBytes);
    stats.modsChecked.add(modId);
    if (indexBuilt && stats.firstIndexDurationMs === null && durationMS !== undefined) {
        stats.firstIndexDurationMs = durationMS;
    }
}

export function getSessionStats(): {totalBytes: number; modsChecked: number; firstIndexDurationMs: number | null} {
    return {totalBytes: stats.totalBytes, modsChecked: stats.modsChecked.size, firstIndexDurationMs: stats.firstIndexDurationMs};
}

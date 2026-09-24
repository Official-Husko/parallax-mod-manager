// A tiny, module-level running total of Checks tab activity this session - never cleared by a
// mod switch or a "mods-changed" event (unlike Editor.tsx's own per-mod checkResults cache,
// which is exactly that kind of snapshot). Deliberately a standalone singleton, the same
// lightweight pattern data/notifications.ts already uses, rather than threading a prop through
// Editor.tsx/EditorChecks.tsx for one small stat: any tab that runs a check calls recordCheck
// once, and the Checks tab's own Result cache card reads the running total back.

interface SessionStats {
    totalBytes: number;
    modsChecked: Set<string>;
}

const stats: SessionStats = {totalBytes: 0, modsChecked: new Set()};

export function recordCheck(modId: string, resultBytes: number): void {
    stats.totalBytes += Math.max(0, resultBytes);
    stats.modsChecked.add(modId);
}

export function getSessionStats(): {totalBytes: number; modsChecked: number} {
    return {totalBytes: stats.totalBytes, modsChecked: stats.modsChecked.size};
}

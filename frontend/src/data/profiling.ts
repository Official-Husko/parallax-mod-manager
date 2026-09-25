// A small, framework-free frontend profiler - the same plain module-level-state shape as
// data/tooltip.ts and data/checksSessionStats.ts. Wraps real work with performance.mark/measure
// (so it also shows up as named entries in the browser's own Performance panel, not just this
// module's own table) and keeps a running per-label total, for Settings > Debug's Profiling
// panel (see DebugPanel.tsx).
//
// Off by default, and a near-free passthrough when disabled (one boolean check): nothing outside
// DebugPanel ever calls setEnabled(true), and DebugPanel only exists in a development build (see
// data/developerTools.ts) - so it's safe to leave time()/timeAsync() calls in real views like
// Browse.tsx permanently, rather than needing a build-tag-style strip for a release build.

interface Aggregate {
    count: number;
    totalMs: number;
    maxMs: number;
}

export interface Measurement {
    label: string;
    count: number;
    totalMs: number;
    avgMs: number;
    maxMs: number;
}

let enabled = false;
let seq = 0;
const aggregates = new Map<string, Aggregate>();

export function setEnabled(v: boolean): void {
    enabled = v;
}

export function isEnabled(): boolean {
    return enabled;
}

function record(label: string, durationMs: number): void {
    const agg = aggregates.get(label) ?? {count: 0, totalMs: 0, maxMs: 0};
    agg.count += 1;
    agg.totalMs += durationMs;
    agg.maxMs = Math.max(agg.maxMs, durationMs);
    aggregates.set(label, agg);
}

// finish turns one start mark into a measure, records it, then clears both marks and the
// measure - the browser's own performance entry buffer would otherwise grow for as long as
// profiling stays on, across an entire session.
function finish(label: string, startMark: string): void {
    const endMark = `${startMark}-end`;
    performance.mark(endMark);
    const duration = performance.measure(label, startMark, endMark).duration;
    performance.clearMarks(startMark);
    performance.clearMarks(endMark);
    performance.clearMeasures(label);
    record(label, duration);
}

// time wraps a synchronous computation - use it around a useMemo body, not around the memo hook
// itself, so a measurement is only recorded when the computation actually reruns.
export function time<T>(label: string, fn: () => T): T {
    if (!enabled) return fn();
    const startMark = `parallax-prof-${label}-${++seq}`;
    performance.mark(startMark);
    try {
        return fn();
    } finally {
        finish(label, startMark);
    }
}

// timeAsync is time's counterpart for an awaited call - e.g. a view's mount-time fetch sequence.
export async function timeAsync<T>(label: string, fn: () => Promise<T>): Promise<T> {
    if (!enabled) return fn();
    const startMark = `parallax-prof-${label}-${++seq}`;
    performance.mark(startMark);
    try {
        return await fn();
    } finally {
        finish(label, startMark);
    }
}

// getMeasurements returns every recorded label's stats, slowest total first - the sort order
// the Profiling panel's list renders in directly.
export function getMeasurements(): Measurement[] {
    return Array.from(aggregates.entries())
        .map(([label, agg]) => ({label, count: agg.count, totalMs: agg.totalMs, avgMs: agg.totalMs / agg.count, maxMs: agg.maxMs}))
        .sort((a, b) => b.totalMs - a.totalMs);
}

export function clearMeasurements(): void {
    aggregates.clear();
}

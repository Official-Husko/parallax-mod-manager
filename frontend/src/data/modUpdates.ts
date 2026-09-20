import {useEffect, useState} from 'preact/hooks';
import {CheckModUpdates, MarkModUpdatesSeen} from '../../wailsjs/go/main/App';
import type {modupdates} from '../../wailsjs/go/models';

// What happened to a game's mods since the app last started - the frontend half
// of internal/modupdates. The backend does the comparing (and remembers what the
// mods looked like last time); this keeps its latest report per game in one place
// so the sidebar card and the Updates window show the same thing without each
// asking for it. A plain module-level store, like data/notifications.ts.

export type ModUpdatesStatus = 'idle' | 'checking' | 'ready' | 'error';

export interface ModUpdatesState {
    status: ModUpdatesStatus;
    // The newest report, kept while a later check runs (and after one fails), so
    // the list doesn't blank out under a refresh.
    report: modupdates.Report | null;
    error: string;
}

const IDLE: ModUpdatesState = {status: 'idle', report: null, error: ''};

const states = new Map<string, ModUpdatesState>();
// Games already checked this run - see ensureModUpdates.
const started = new Set<string>();
const listeners = new Set<() => void>();

function setState(gameId: string, next: ModUpdatesState) {
    states.set(gameId, next);
    for (const listener of listeners) listener();
}

// checkModUpdates runs a check now. refresh asks Steam again for what the
// Workshop says instead of trusting what it said earlier this run. A check
// already under way is left to finish.
export async function checkModUpdates(gameId: string, refresh = false): Promise<void> {
    const current = states.get(gameId) ?? IDLE;
    if (current.status === 'checking') return;
    setState(gameId, {...current, status: 'checking', error: ''});
    try {
        const report = await CheckModUpdates(gameId, refresh);
        setState(gameId, {status: 'ready', report, error: ''});
    } catch (err) {
        setState(gameId, {status: 'error', report: current.report, error: String(err)});
    }
}

// ensureModUpdates makes the once-per-run check for a game: the first time it is
// asked about, and never again (later checks are checkModUpdates' job).
export function ensureModUpdates(gameId: string): void {
    if (!gameId || started.has(gameId)) return;
    started.add(gameId);
    void checkModUpdates(gameId);
}

// markModUpdatesSeen makes what is installed now the point later checks compare
// with, so what was listed stops being listed.
export async function markModUpdatesSeen(gameId: string): Promise<void> {
    const current = states.get(gameId) ?? IDLE;
    try {
        const report = await MarkModUpdatesSeen(gameId);
        setState(gameId, {status: 'ready', report, error: ''});
    } catch (err) {
        setState(gameId, {...current, status: 'error', error: String(err)});
    }
}

export function useModUpdates(gameId: string): ModUpdatesState {
    const [, rerender] = useState(0);
    useEffect(() => {
        const listener = () => rerender((n) => n + 1);
        listeners.add(listener);
        // Effects run after the first paint, so a change that landed between that
        // render and this subscription (a check failing at once) would otherwise
        // never be shown: draw once more from the current state.
        listener();
        return () => { listeners.delete(listener); };
    }, []);
    return states.get(gameId) ?? IDLE;
}

export type ChangeKind = 'deleted' | 'removed' | 'updated' | 'changed';

// How each kind of change is drawn, everywhere it appears. Amber for the
// Workshop deleting a mod (worth knowing, not a failure), green for an update,
// blue for files that changed, quiet grey for a mod that is simply gone.
export const CHANGE_KINDS: Record<ChangeKind, {icon: string; color: string; label: string; plural: string}> = {
    deleted: {icon: 'fa-cloud-slash', color: 'var(--amber)', label: 'Deleted from the Workshop', plural: 'deleted from the Workshop'},
    removed: {icon: 'fa-circle-minus', color: 'var(--text-muted)', label: 'Removed', plural: 'removed'},
    updated: {icon: 'fa-circle-arrow-up', color: 'var(--green)', label: 'Updated', plural: 'updated'},
    changed: {icon: 'fa-pen-to-square', color: 'var(--blue)', label: 'Files changed', plural: 'changed'},
};

// The order kinds are listed in: what most needs a look first.
export const CHANGE_KIND_ORDER: ChangeKind[] = ['deleted', 'removed', 'updated', 'changed'];

export interface ChangeSummary {
    // How many mods each kind happened to since the last startup.
    counts: Record<ChangeKind, number>;
    // How many of those are new since then (the card's headline number).
    fresh: number;
    // Mods already deleted from the Workshop before the last startup - still
    // listed, but not news.
    standing: number;
}

export function summarizeChanges(report: modupdates.Report | null): ChangeSummary {
    const counts: Record<ChangeKind, number> = {deleted: 0, removed: 0, updated: 0, changed: 0};
    let fresh = 0;
    let standing = 0;
    for (const c of report?.Changes ?? []) {
        if (c.New) {
            counts[c.Kind as ChangeKind]++;
            fresh++;
        } else {
            standing++;
        }
    }
    return {counts, fresh, standing};
}

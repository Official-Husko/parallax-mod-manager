import {useEffect, useState} from 'preact/hooks';
import {CheckLoversLabUpdates, CheckModUpdates, MarkModUpdatesSeen} from '../../wailsjs/go/main/App';
import {modupdates} from '../../wailsjs/go/models';
import {FLAG} from './flags';

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

// The latest LoversLab-sourced changes known per game, kept separate from states so
// that whichever of checkModUpdates/checkLoversLabUpdates happens to resolve last
// never silently drops the other's contribution - withLoversLabChanges re-attaches
// this every time either one sets a fresh report, regardless of which finished first.
const loversLabChanges = new Map<string, modupdates.Change[]>();

function withLoversLabChanges(gameId: string, report: modupdates.Report): modupdates.Report {
    const ll = loversLabChanges.get(gameId) ?? [];
    // Every existing loverslab-sourced entry is dropped and the current list re-appended in
    // full, rather than only replacing the ones that still match by ModID - a mod resolved
    // since the last merge (installed the update, so this game's own next LoversLab check no
    // longer lists it) has to actually disappear here too, not just fail to be duplicated.
    const withoutLoversLab = report.Changes.filter((c) => c.Source !== 'loverslab');
    if (withoutLoversLab.length === report.Changes.length && ll.length === 0) return report;
    return modupdates.Report.createFrom({...report, Changes: [...withoutLoversLab, ...ll]});
}

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
        setState(gameId, {status: 'ready', report: withLoversLabChanges(gameId, report), error: ''});
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

// checkLoversLabUpdates asks which of gameId's LoversLab-installed mods have a newer
// version, and merges the result into the same report checkModUpdates already keeps
// (Source "loverslab", same Change shape - see loverslabupdates.go) rather than a
// second, separate updates display. A mod already listed under the same id is
// replaced, not duplicated, so repeat checks stay idempotent. A failure here (not
// signed in to LoversLab, most commonly - an entirely normal state, not a bug) is
// swallowed rather than surfaced as this whole report's own error: the Workshop
// side's own report, if any, keeps showing exactly as it was.
export async function checkLoversLabUpdates(gameId: string): Promise<void> {
    try {
        const llChanges = await CheckLoversLabUpdates(gameId);
        loversLabChanges.set(gameId, llChanges ?? []);
        const current = states.get(gameId) ?? IDLE;
        // Nothing to add now, and nothing stale from an earlier merge to remove either -
        // a real early-out, not just an empty-list shortcut (an empty llChanges still has to
        // reach withLoversLabChanges below when the displayed report already carries a
        // loverslab-sourced entry, so a resolved update actually clears instead of sticking
        // around until an unrelated Workshop-side refresh happens to replace the whole report).
        const hadLoversLabChanges = current.report?.Changes.some((c) => c.Source === 'loverslab') ?? false;
        if ((!llChanges || llChanges.length === 0) && !hadLoversLabChanges) return;
        const base = current.report ?? modupdates.Report.createFrom({
            GameID: gameId,
            CheckedAt: Math.floor(Date.now() / 1000),
            BaselineAt: 0,
            Changes: [],
            ModsChecked: 0,
            WorkshopChecked: false,
            WorkshopError: '',
            Unreadable: false,
        });
        setState(gameId, {status: 'ready', report: withLoversLabChanges(gameId, base), error: current.error});
    } catch {
        // Not signed in to LoversLab, or a network hiccup - nothing to show yet,
        // and not a reason to blank out or error whatever's already displayed.
    }
}

// ensureLoversLabUpdates starts (or restarts, if enabled/intervalHours changed since
// the last call) a periodic LoversLab update check for gameId: once immediately, then
// every intervalHours while the app stays open - the same "on startup" moment
// ensureModUpdates already covers, just repeating, since (unlike Steam Workshop) this
// app has no other way to learn a LoversLab mod updated. enabled false stops it
// (Settings > Browse's own toggle - see BrowseSettingsPanel.tsx).
const loversLabTimers = new Map<string, ReturnType<typeof setInterval>>();
const loversLabConfig = new Map<string, string>();

export function ensureLoversLabUpdates(gameId: string, enabled: boolean, intervalHours: number): void {
    if (!gameId) return;
    const key = `${enabled}:${intervalHours}`;
    if (loversLabConfig.get(gameId) === key) return; // nothing about this game's own check has changed
    loversLabConfig.set(gameId, key);

    const existing = loversLabTimers.get(gameId);
    if (existing !== undefined) {
        clearInterval(existing);
        loversLabTimers.delete(gameId);
    }
    if (!enabled) return;

    void checkLoversLabUpdates(gameId);
    const ms = Math.max(1, intervalHours) * 60 * 60 * 1000;
    loversLabTimers.set(gameId, setInterval(() => void checkLoversLabUpdates(gameId), ms));
}

// markModUpdatesSeen makes what is installed now the point later checks compare
// with, so what was listed stops being listed - the Workshop side only: a LoversLab
// update has no equivalent "acknowledge and stop nagging" action, since the only real
// way to resolve one is to actually install it (which is what makes it disappear on
// its own - see loverslabinstall.go), so it keeps showing here even after this.
export async function markModUpdatesSeen(gameId: string): Promise<void> {
    const current = states.get(gameId) ?? IDLE;
    try {
        const report = await MarkModUpdatesSeen(gameId);
        setState(gameId, {status: 'ready', report: withLoversLabChanges(gameId, report), error: ''});
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

// How each kind of change is drawn, everywhere it appears. The Workshop
// deleting a mod is drawn as the mod's own "deleted" flag is (see data/flags.ts),
// green for an update,
// blue for files that changed, quiet grey for a mod that is simply gone.
export const CHANGE_KINDS: Record<ChangeKind, {icon: string; color: string; label: string; plural: string}> = {
    deleted: {icon: FLAG.deleted.icon, color: FLAG.deleted.color, label: 'Deleted from the Workshop', plural: 'deleted from the Workshop'},
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

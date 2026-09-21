import {EventsOn} from '../../wailsjs/runtime/runtime';
import {notify} from './notifications';
import type {NotificationKind} from './notifications';

// Launch-time mod repair, the interface half: before a launch the backend makes sure the game
// can find every enabled mod (see modstubs.go and internal/scan/stubs.go) and reports what it
// had to do, so a mod that was silently not loading is not a surprise in the game.

export interface ModStubsReport {
    GameID: string;
    Fixed: string[] | null;
    Created: string[] | null;
    Missing: string[] | null;
    Failed: string[] | null;
}

// "A", "A and B", "A, B and C" - and "A, B, C and 4 more" past a handful.
function nameList(names: string[]): string {
    const shown = names.slice(0, 3).map((n) => `'${n}'`);
    if (names.length > 3) return `${shown.join(', ')} and ${names.length - 3} more`;
    if (shown.length > 1) return `${shown.slice(0, -1).join(', ')} and ${shown[shown.length - 1]}`;
    return shown[0];
}

function mods(n: number): string {
    return n === 1 ? 'mod' : 'mods';
}

// describeModStubs turns a report into the messages to show: one for what was repaired (good
// news), one for what could not be (the person has to act).
export function describeModStubs(r: ModStubsReport): {kind: NotificationKind; message: string}[] {
    const fixed = [...(r.Fixed ?? []), ...(r.Created ?? [])];
    const missing = r.Missing ?? [];
    const failed = r.Failed ?? [];
    const out: {kind: NotificationKind; message: string}[] = [];
    if (fixed.length > 0) {
        out.push({
            kind: 'info',
            message: `Repaired the link the game uses to find ${fixed.length} ${mods(fixed.length)} (${nameList(fixed)}) - they pointed at a folder that no longer exists, or had no link at all, so the game would not have loaded them.`,
        });
    }
    if (missing.length > 0) {
        out.push({
            kind: 'warning',
            message: `${missing.length} enabled ${mods(missing.length)} could not be found on disk and will not load in the game: ${nameList(missing)}. Add the folder they moved to under Settings > Paths & folders > Extra mod folders.`,
        });
    }
    if (failed.length > 0) {
        out.push({
            kind: 'error',
            message: `Couldn't repair the link for ${nameList(failed)} - check the activity log for why.`,
        });
    }
    return out;
}

// installModStubNotifications shows those messages as they come in; returns its cleanup.
export function installModStubNotifications(): () => void {
    return EventsOn('mod-stubs-changed', (r: ModStubsReport) => {
        for (const m of describeModStubs(r)) notify(m.kind, m.message);
    });
}

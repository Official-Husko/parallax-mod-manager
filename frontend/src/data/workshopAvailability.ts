import type {main} from '../../wailsjs/go/models';
import {timeAgo} from './format';
import {FLAG} from './flags';

// What became of a Workshop mod on Steam - unlisted, private or deleted - and how
// the app knows. The backend (WorkshopAvailability, internal/steamapi.Classify)
// decides; this is the wording and the lookup. An ordinary listed mod, or one nothing
// can be said about, has no entry at all.

export type WorkshopState = 'unlisted' | 'private' | 'deleted';

export interface WorkshopFlag {
    state: WorkshopState;
    // How it was worked out: record, friends, denied, banned, page-up or page-gone.
    reason: string;
    // Where the mod stands for preservation (see backups.go): done, incomplete, pending,
    // gone, failed, off or '' when it is not at risk; and when a copy was saved (Unix seconds).
    backupState: string;
    backedUpAt: number;
}

// workshopFlags indexes the backend's list by the Workshop item id a mod's
// descriptor carries.
export function workshopFlags(list: main.WorkshopAvailability[]): Map<string, WorkshopFlag> {
    const byId = new Map<string, WorkshopFlag>();
    for (const a of list) {
        if (a.State === 'unlisted' || a.State === 'private' || a.State === 'deleted') {
            byId.set(a.RemoteFileID, {state: a.State, reason: a.Reason, backupState: a.BackupState, backedUpAt: a.BackedUpAt});
        }
    }
    return byId;
}

// workshopFlagText is the short label and the one-sentence explanation for a flag.
// The sentence says how sure the app is: Steam's own record, or an inference from
// what the free API and the item's page said.
export function workshopFlagText(flag: WorkshopFlag): {title: string; detail: string} {
    switch (flag.state) {
        case 'unlisted':
            return flag.reason === 'page-up'
                ? {
                    title: 'Unlisted',
                    detail: 'Steam\'s free API does not return it, but its Workshop page is up: an unlisted item, reachable only by its link. It works in the game. A Steam API key makes this certain.',
                }
                : {
                    title: 'Unlisted',
                    detail: 'Reachable only by its link: it is not listed on the Workshop. It works in the game and its author can still update it.',
                };
        case 'private':
            if (flag.reason === 'friends') {
                return {title: 'Friends only', detail: 'Its author made it visible to friends only, so it is hidden from the Workshop. Your installed copy keeps working.'};
            }
            if (flag.reason === 'denied') {
                return {title: 'Private', detail: 'Steam refuses access to it: its author made it private or friends only. Your installed copy keeps working.'};
            }
            return {title: 'Private', detail: 'Its author made it private, so it is hidden from the Workshop. Your installed copy keeps working.'};
        case 'deleted':
            if (flag.reason === 'banned') {
                return {title: 'Removed by Steam', detail: 'Steam\'s moderators removed it from the Workshop. Your installed copy keeps working but will never update.'};
            }
            if (flag.reason === 'page-gone') {
                return {
                    title: 'Deleted or private',
                    detail: 'Its Workshop page is gone: its author deleted it, or made it private (a page seen without a Steam account cannot tell which). Your installed copy keeps working but will not update.',
                };
            }
            return {title: 'Deleted', detail: 'Deleted from the Workshop. Your installed copy keeps working but will never update.'};
    }
}

// backupNote says what became of a copy of the mod, for a flag that is about a mod
// Steam is about to remove (deleted or private). undefined when it does not apply.
export function backupNote(flag: WorkshopFlag): string | undefined {
    switch (flag.backupState) {
        case 'done':
            return `A copy of this mod was saved ${timeAgo(flag.backedUpAt)}, in your backup folder.`;
        case 'incomplete':
            return 'Only part of this mod could be saved: Steam removed some of its files while it was being copied.';
        case 'pending':
            return 'A copy is being made in your backup folder.';
        case 'gone':
            return 'Steam had already removed this mod\'s files, so no copy could be made.';
        case 'failed':
            return 'The backup failed, so no copy exists yet. The activity log says why.';
        case 'off':
            return 'Backups are off, so this mod is not being saved. Turn them on under Settings > Backup.';
        default:
            return undefined;
    }
}

export function workshopFlagStyle(flag: WorkshopFlag): {icon: string; color: string} {
    return FLAG[flag.state];
}

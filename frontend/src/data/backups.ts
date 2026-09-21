import {EventsOn} from '../../wailsjs/runtime/runtime';
import {formatBytes} from './format';
import {dismiss, notify, updateNotification} from './notifications';

// Mod preservation, the interface half: what the backend says as it copies a Workshop
// mod that Steam is about to remove (see internal/backup and backups.go). Copies happen
// in the background, so this is how the person hears about them.

// Why a mod was copied, as a phrase that follows "backed up".
export function backupReasonPhrase(reason: string): string {
    switch (reason) {
        case 'deleted': return 'it was deleted from the Workshop';
        case 'private': return 'it was made private';
        case 'all': return 'every-mod backups are on';
        default: return 'on request';
    }
}

// A short label for the reason chip in the backups list.
export function backupReasonLabel(reason: string): string {
    switch (reason) {
        case 'deleted': return 'Deleted';
        case 'private': return 'Private';
        case 'all': return 'Every mod';
        default: return 'Manual';
    }
}

// The payload of the backend's "backup-done" event.
interface BackupResult {
    GameID: string;
    ModID: string;
    RemoteFileID: string;
    Name: string;
    Reason: string;
    Status: 'ok' | 'incomplete' | 'failed';
    Message: string;
    Size: number;
}

// installBackupNotifications turns the backend's backup events into notifications: one
// per finished copy, and a single progress toast while several mods are being copied
// (the first "every mod" run can be dozens). Returns the unsubscribe.
export function installBackupNotifications(): () => void {
    let progressId: string | null = null;

    const offDone = EventsOn('backup-done', (r: BackupResult) => {
        if (r.Status === 'ok') {
            notify('success', `Backed up '${r.Name}' (${formatBytes(r.Size)}) - ${backupReasonPhrase(r.Reason)}.`);
        } else if (r.Status === 'incomplete') {
            notify('warning', `Only part of '${r.Name}' could be backed up - ${r.Message}.`);
        } else {
            notify('error', `Couldn't back up '${r.Name}': ${r.Message}`);
        }
    });

    const offProgress = EventsOn('backup-progress', (p: {GameID: string; Done: number; Total: number; Name: string}) => {
        // A single copy is announced by its own result; a progress bar is for a batch.
        if (p.Total < 2) return;
        const message = `Backing up Workshop mods (${p.Done + 1} of ${p.Total}): ${p.Name}`;
        const progress = Math.round((p.Done / p.Total) * 100);
        if (progressId) updateNotification(progressId, {message, progress});
        else progressId = notify('progress', message, {progress});
    });

    const offChanged = EventsOn('backups-changed', () => {
        if (!progressId) return;
        const id = progressId;
        progressId = null;
        updateNotification(id, {kind: 'success', message: 'Mod backups are up to date.', progress: 100});
    });

    return () => {
        offDone();
        offProgress();
        offChanged();
        if (progressId) dismiss(progressId);
    };
}

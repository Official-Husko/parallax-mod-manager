import {EventsOn} from '../../wailsjs/runtime/runtime';
import {notify} from './notifications';

// The Steam Direct shim's own live, best-effort ping (see internal/launchershim,
// companions/launcher-shim) - the moment Steam actually starts the shim in place of
// a game's own dowser/dowser.exe, ahead of internal/gameproc's own polling ever
// noticing the game's process. Not a Wails-bound method's own parameter or return
// type, so it never gets a generated model - mirrors the shim's own Ping payload.
export interface LauncherShimPing {
    resolvedExe: string;
    pid: number;
}

// installLauncherShimNotifications shows a toast for each ping; returns its cleanup.
export function installLauncherShimNotifications(): () => void {
    return EventsOn('launcher-shim-ping', (p: LauncherShimPing) => {
        const name = p.resolvedExe.split(/[/\\]/).pop() || p.resolvedExe;
        notify('info', `Launching ${name} via Steam Direct...`);
    });
}

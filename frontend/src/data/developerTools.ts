import type {app} from '../../wailsjs/go/models';

// The developer tools setting (Settings > Debug), the interface half - see devtools.go for what
// the framework decides and what the setting controls.

// Whether Shift+right-click should show the browser's own menu (with Inspect Element) instead of
// the app's. Read by the app-wide right-click handler, so it lives here rather than in a view.
let shiftRightClickNative = false;

export function setShiftRightClickNative(on: boolean) {
    shiftRightClickNative = on;
}

export function wantsNativeContextMenu(e: MouseEvent): boolean {
    return shiftRightClickNative && e.shiftKey;
}

// The message that opens the inspector, per platform. Wails answers one on Linux and one on
// macOS; on Windows only the keyboard shortcut works.
export function inspectorMessage(os: string): string | null {
    if (os === 'linux') return 'wails:showInspector';
    if (os === 'darwin') return 'wails:openInspector';
    return null;
}

// openInspector asks the window to open the web inspector; false when this platform has no way
// to do it from here.
export function openInspector(os: string): boolean {
    const message = inspectorMessage(os);
    const invoke = (window as unknown as {WailsInvoke?: (m: string) => void}).WailsInvoke;
    if (!message || !invoke) return false;
    invoke(message);
    return true;
}

// The keyboard shortcut that opens it, as a person would press it.
export function inspectorShortcut(os: string): string {
    if (os === 'windows') return 'F12';
    if (os === 'darwin') return 'right-click, then Inspect Element';
    return 'Ctrl+Shift+F12';
}

export type DeveloperToolsStatus = app.DeveloperToolsStatus;

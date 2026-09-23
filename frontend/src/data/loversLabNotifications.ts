import {useEffect, useState} from 'preact/hooks';
import {LoversLabUnreadNotifications} from '../../wailsjs/go/main/App';

// The signed-in LoversLab account's own real unread-notification count, polled
// periodically while the app is open - see loverslab.go's LoversLabUnreadNotifications
// and docs/loverslab.md's Notifications section: the site's own live-updating bell has
// its AJAX polling disabled server-side (confirmed live), so a periodic real page fetch
// is the honest equivalent, not an invented endpoint. Unlike mod updates, this belongs
// to the account itself, not a specific game - a plain module-level store, like
// data/notifications.ts, with no per-game key.

let unreadCount = 0;
const listeners = new Set<() => void>();

function setUnreadCount(next: number) {
    if (unreadCount === next) return;
    unreadCount = next;
    for (const listener of listeners) listener();
}

// checkLoversLabNotifications asks for the real current count now. A failure (most
// commonly: not signed in to LoversLab) is swallowed rather than surfaced - it leaves
// whatever count is already shown instead of blanking it out over a transient miss.
export async function checkLoversLabNotifications(): Promise<void> {
    try {
        setUnreadCount(await LoversLabUnreadNotifications());
    } catch {
        // Not signed in, or a network hiccup - nothing to update.
    }
}

const NOTIFICATIONS_URL = 'https://www.loverslab.com/notifications/';

// openLoversLabNotifications opens the real notifications page in the system browser -
// there is no in-app list to show instead: a populated notification's own markup was
// never observed in research (the test account has had zero notifications through
// every pass so far), so this app only ever shows the real, confirmed unread count,
// never a guessed-at rendering of the notifications themselves. See
// docs/loverslab.md's Notifications section.
export function loversLabNotificationsURL(): string {
    return NOTIFICATIONS_URL;
}

let timer: ReturnType<typeof setInterval> | undefined;
let config = '';

// ensureLoversLabNotifications starts (or restarts, if enabled/intervalMinutes changed
// since the last call) a periodic notification check: once immediately, then every
// intervalMinutes while the app stays open. enabled false stops it and clears the
// badge (Settings > Browse's own toggle - see BrowseSettingsPanel.tsx).
export function ensureLoversLabNotifications(enabled: boolean, intervalMinutes: number): void {
    const key = `${enabled}:${intervalMinutes}`;
    if (config === key) return;
    config = key;

    if (timer !== undefined) {
        clearInterval(timer);
        timer = undefined;
    }
    if (!enabled) {
        setUnreadCount(0);
        return;
    }

    void checkLoversLabNotifications();
    const ms = Math.max(1, intervalMinutes) * 60 * 1000;
    timer = setInterval(() => void checkLoversLabNotifications(), ms);
}

export function useLoversLabUnreadCount(): number {
    const [, rerender] = useState(0);
    useEffect(() => {
        const listener = () => rerender((n) => n + 1);
        listeners.add(listener);
        // Effects run after the first paint - draw once more from the current count in
        // case it changed between that render and this subscription.
        listener();
        return () => { listeners.delete(listener); };
    }, []);
    return unreadCount;
}

// A small global notification/toast store - no Context/Provider wiring
// needed, since any component can import notify()/updateNotification()/
// dismiss() directly and any component rendering <NotificationStack/>
// (just one, in app.tsx) subscribes via useNotifications(). Kept as a
// plain module-level store rather than reaching for a state-management
// library - this app has no other global state need beyond this one.
import {useEffect, useState} from 'preact/hooks';

// 'warning' is for something the user should act on but that isn't a failure
// (a patch that's gone stale, say): like an error it stays until dismissed, and
// unlike an error it isn't red.
export type NotificationKind = 'info' | 'success' | 'warning' | 'error' | 'progress';

export interface Notification {
    id: string;
    kind: NotificationKind;
    message: string;
    // 0-100 for a determinate progress bar; undefined means an
    // indeterminate bar (still working, real length unknown) - only
    // meaningful for kind: 'progress'.
    progress?: number;
    // An optional real recovery action (e.g. "Retry") shown next to the
    // message - most useful on an 'error' notification that isn't
    // auto-dismissed, so there's actually time to click it.
    action?: {label: string; onClick: () => void};
    // When set, a second control with this label (an "OK") that just dismisses the
    // notification - for one that stays until the person has seen it, next to `action`.
    dismissLabel?: string;
}

// Messages/infos auto-dismiss after this long - warnings, errors and in-progress
// notifications stay until the code driving them resolves them (progress
// -> success/error) or the user dismisses them by hand.
const AUTO_DISMISS_MS = 5000;

let notifications: Notification[] = [];
const listeners = new Set<(n: Notification[]) => void>();
let nextId = 1;

function emit() {
    const snapshot = notifications;
    for (const listener of listeners) listener(snapshot);
}

function scheduleAutoDismiss(id: string, kind: NotificationKind) {
    if (kind !== 'info' && kind !== 'success') {
        return;
    }
    setTimeout(() => dismiss(id), AUTO_DISMISS_MS);
}

// notify adds a new notification and returns its id - hang onto that id
// to later updateNotification() it in place (e.g. a 'progress' toast
// that becomes 'success' or 'error' once the work it describes finishes)
// or dismiss() it early.
export function notify(kind: NotificationKind, message: string, extra?: Pick<Notification, 'progress' | 'action' | 'dismissLabel'>): string {
    const id = `n${nextId++}`;
    notifications = [...notifications, {id, kind, message, ...extra}];
    emit();
    scheduleAutoDismiss(id, kind);
    return id;
}

// updateNotification changes an existing notification in place (e.g.
// advancing a progress bar, or turning a 'progress' toast into its final
// 'success'/'error' outcome) rather than replacing it with a new one -
// so it doesn't jump position in the stack or restart mid-read for
// someone already looking at it. A stale id (already dismissed, or never
// existed) is silently ignored. Passing action: undefined explicitly
// clears a previous action (e.g. once "Retry" has been clicked and a
// fresh attempt is under way); omitting the key entirely leaves whatever
// action was already there untouched.
export function updateNotification(id: string, patch: Partial<Pick<Notification, 'kind' | 'message' | 'progress' | 'action'>>) {
    let changedKind: NotificationKind | undefined;
    notifications = notifications.map((n) => {
        if (n.id !== id) return n;
        const next = {...n, ...patch};
        if (patch.kind && patch.kind !== n.kind) changedKind = patch.kind;
        return next;
    });
    emit();
    if (changedKind) scheduleAutoDismiss(id, changedKind);
}

// trackTask runs task under one 'progress' notification and settles it in
// place when the task ends: turned into a 'success' toast with `success`, or
// dismissed quietly when there is nothing worth confirming; an error becomes
// an 'error' notification carrying the failure (prefixed with `failure` when
// given). Resolves true when the task succeeded and false when it failed -
// the failure is already reported, so callers only branch on the result.
export async function trackTask(
    message: string,
    task: () => Promise<unknown>,
    outcome: {success?: string; failure?: string} = {},
): Promise<boolean> {
    const id = notify('progress', message);
    try {
        await task();
    } catch (err) {
        updateNotification(id, {kind: 'error', message: outcome.failure ? `${outcome.failure}: ${String(err)}` : String(err)});
        return false;
    }
    if (outcome.success) updateNotification(id, {kind: 'success', message: outcome.success});
    else dismiss(id);
    return true;
}

// hasNotification says whether a notification is still showing - for code that raises
// one at most once at a time.
export function hasNotification(id: string | null): boolean {
    return id !== null && notifications.some((n) => n.id === id);
}

export function dismiss(id: string) {
    if (!notifications.some((n) => n.id === id)) return;
    notifications = notifications.filter((n) => n.id !== id);
    emit();
}

export function useNotifications(): Notification[] {
    const [state, setState] = useState<Notification[]>(notifications);
    useEffect(() => {
        listeners.add(setState);
        return () => { listeners.delete(setState); };
    }, []);
    return state;
}

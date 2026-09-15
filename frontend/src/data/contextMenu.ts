// A small global right-click context menu store, mirroring
// data/notifications.ts's own plain module-level pub/sub pattern (no
// Context/Provider wiring, no new dependency) - any component calls
// openContextMenu() directly from its own onContextMenu handler; one
// <ContextMenu/> in app.tsx renders whatever's currently open.
import {useEffect, useState} from 'preact/hooks';

export interface ContextMenuItem {
    label: string;
    onClick: () => void;
    // A real destructive/irreversible action (removing a mod from the
    // load order, say) gets a visually distinct treatment, matching this
    // app's own red-for-destructive convention elsewhere - not disabled,
    // just not styled the same as a plain navigation/utility action.
    danger?: boolean;
    disabled?: boolean;
    // Renders a thin rule above this item, for grouping related actions
    // (e.g. reordering separate from removing) without a whole submenu.
    separatorBefore?: boolean;
}

export interface ContextMenuState {
    x: number;
    y: number;
    items: ContextMenuItem[];
}

let state: ContextMenuState | null = null;
const listeners = new Set<(s: ContextMenuState | null) => void>();

function emit() {
    for (const listener of listeners) listener(state);
}

// openContextMenu opens (or replaces) the single global context menu at
// a real mouse event's own position, clamped on-screen by the rendering
// component itself. Call from an element's onContextMenu handler; be sure
// to call event.preventDefault() there too, same as everywhere else in
// this app - see the app-wide default-suppression in app.tsx for
// anywhere that doesn't open a custom menu of its own.
export function openContextMenu(event: MouseEvent, items: ContextMenuItem[]) {
    event.preventDefault();
    event.stopPropagation();
    state = {x: event.clientX, y: event.clientY, items};
    emit();
}

export function closeContextMenu() {
    if (!state) return;
    state = null;
    emit();
}

export function useContextMenu(): ContextMenuState | null {
    const [local, setLocal] = useState(state);
    useEffect(() => {
        listeners.add(setLocal);
        return () => { listeners.delete(setLocal); };
    }, []);
    return local;
}

import {useEffect, useState} from 'preact/hooks';

// What the background is showing right now, and a way to ask it for another picture. The
// background (AppBackground, in app.tsx) and the buttons that steer it (Settings > Appearance)
// share no parent, so - like data/notifications.ts - this is a plain module-level store.

export interface CurrentBackground {
    gameId: string;
    // The image's file name.
    name: string;
    // Where it loaded from: "from disk" or "from <host>".
    origin: string;
}

let current: CurrentBackground | null = null;
let random: (() => void) | null = null;
const listeners = new Set<() => void>();

function changed() {
    for (const listener of listeners) listener();
}

// publishBackground is called by the background whenever an image goes on screen, and with
// null when there is none.
export function publishBackground(next: CurrentBackground | null) {
    current = next;
    changed();
}

// registerRandomBackground is how the running background offers "show another": pass the
// function while it can (rotation running, more than one image), null when it cannot.
export function registerRandomBackground(fn: (() => void) | null) {
    random = fn;
    changed();
}

// requestRandomBackground asks for another picture. False when the background cannot supply one.
export function requestRandomBackground(): boolean {
    if (!random) return false;
    random();
    return true;
}

export function useCurrentBackground(): {current: CurrentBackground | null; canRandom: boolean} {
    const [, rerender] = useState(0);
    useEffect(() => {
        const listener = () => rerender((n) => n + 1);
        listeners.add(listener);
        listener();
        return () => { listeners.delete(listener); };
    }, []);
    return {current, canRandom: random !== null};
}

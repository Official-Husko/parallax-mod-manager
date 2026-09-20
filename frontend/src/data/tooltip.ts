// A small global rich-tooltip store, the same plain module-level pub/sub
// shape as data/contextMenu.ts - one <Tooltip/> in app.tsx renders whatever
// is currently showing. Unlike a native `title`, the content is real JSX, so
// a tooltip can show a flag or a domain segment exactly as it looks in the
// list (colored icon, mask-cut letter bar) next to its explanation.
//
// Callers spread tip(() => <content/>) onto an element. The content is built
// lazily, only once the hover delay has passed, so a list row that offers a
// tooltip costs nothing until someone actually rests the pointer on it.
import type {ComponentChild} from 'preact';
import {useEffect, useState} from 'preact/hooks';

export interface TooltipState {
    // Bumped per showing, so the renderer remounts (and re-measures) when
    // the tooltip moves from one element straight to the next.
    id: number;
    el: Element;
    content: ComponentChild;
}

// Roughly a native tooltip's own pause - long enough that sweeping the
// pointer across a list doesn't flash a tooltip per row.
const SHOW_DELAY_MS = 350;
// Moving to a neighbouring element this soon after a tooltip closed skips the
// delay - see showTooltip.
const CHAIN_WINDOW_MS = 200;

let state: TooltipState | null = null;
let nextId = 1;
let timer: number | undefined;
let hiddenAt = -Infinity;
const listeners = new Set<(s: TooltipState | null) => void>();

function emit() {
    for (const listener of listeners) listener(state);
}

// showTooltip shows what build() returns for el after the hover delay - or
// at once when a tooltip is up or only just closed, so moving along a row of
// adjacent segments reads as one continuous hover instead of a pause per
// segment (the pointer leaving one segment closes its tooltip before it
// enters the next).
// build() returning null shows nothing (e.g. a mod that has no flags).
export function showTooltip(el: Element, build: () => ComponentChild | null) {
    window.clearTimeout(timer);
    const show = () => {
        if (!el.isConnected) return;
        const content = build();
        if (content == null) {
            hideTooltip();
            return;
        }
        state = {id: nextId++, el, content};
        emit();
    };
    if (state || performance.now() - hiddenAt < CHAIN_WINDOW_MS) show();
    else timer = window.setTimeout(show, SHOW_DELAY_MS);
}

export function hideTooltip() {
    window.clearTimeout(timer);
    if (!state) return;
    state = null;
    hiddenAt = performance.now();
    emit();
}

// tip returns the two hover handlers that make an element show a tooltip.
export function tip(build: () => ComponentChild | null) {
    return {
        onMouseEnter: (e: MouseEvent) => showTooltip(e.currentTarget as Element, build),
        onMouseLeave: hideTooltip,
    };
}

export function useTooltip(): TooltipState | null {
    const [local, setLocal] = useState(state);
    useEffect(() => {
        listeners.add(setLocal);
        return () => { listeners.delete(setLocal); };
    }, []);
    return local;
}

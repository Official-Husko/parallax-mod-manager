import {useEffect, useState} from 'preact/hooks';

// How the rotating background image is drawn: blurred, and darkened by a layer over it.
// Both are strengths out of 100 in the settings (preferences.backgroundBlur and
// backgroundDarken), mapped here to what CSS wants.

export const DEFAULT_BACKGROUND_BLUR = 0;
// The darkening the app shipped with, as a strength: the layer fades from 80% to 88% black at
// exactly this setting, so nothing looks different until the slider is moved.
export const DEFAULT_BACKGROUND_DARKEN = 84;

// The strongest blur, in CSS pixels, at a strength of 100.
export const MAX_BLUR_PX = 24;

const SCRIM_TOP_ALPHA = 0.8;
const SCRIM_BOTTOM_ALPHA = 0.88;

const clampPercent = (v: number) => Math.min(100, Math.max(0, Number.isFinite(v) ? v : 0));

// blurPixels maps a blur strength (0-100) to CSS pixels.
export function blurPixels(strength: number): number {
    return Math.round((clampPercent(strength) / 100) * MAX_BLUR_PX * 10) / 10;
}

// scrimAlphas maps a darken strength (0-100) to the opacity of the layer's top and bottom
// edges. Scaled from the shipped look, so DEFAULT_BACKGROUND_DARKEN gives exactly 0.8 and
// 0.88, 0 gives no layer at all, and 100 always reaches fully opaque - two straight segments
// (0 to the default, then the default to 100) meeting at the shipped look's own value, rather
// than one straight line through 0 and the default extended to 100: a single line through
// (0, 0) and (84, 0.8) only reaches 0.952 by strength 100, never the full 1.0 the top edge
// needs to actually hide the image - 100 on the slider read as "fully dark" while a genuinely
// bright part of the picture (a ship's hull, say) stayed faintly visible right through it,
// nowhere near "no layer at all"'s counterpart at the other end.
function scrimAlpha(base: number, strength: number): number {
    const s = clampPercent(strength);
    const raw = s <= DEFAULT_BACKGROUND_DARKEN
        ? base * (s / DEFAULT_BACKGROUND_DARKEN)
        : base + (1 - base) * ((s - DEFAULT_BACKGROUND_DARKEN) / (100 - DEFAULT_BACKGROUND_DARKEN));
    return Math.round(Math.min(1, Math.max(0, raw)) * 1000) / 1000;
}

export function scrimAlphas(strength: number): {top: number; bottom: number} {
    return {top: scrimAlpha(SCRIM_TOP_ALPHA, strength), bottom: scrimAlpha(SCRIM_BOTTOM_ALPHA, strength)};
}

export interface BackgroundLook {
    blur: number;
    darken: number;
}

// A live preview while a slider is being dragged: the background follows the thumb at once,
// and the setting is only saved when it is let go. A plain module-level store, like
// data/notifications.ts, so the sliders (in Settings) and the background (in app.tsx) need no
// shared parent. null means no preview: the saved settings apply.
let preview: BackgroundLook | null = null;
const listeners = new Set<(look: BackgroundLook | null) => void>();

export function previewBackgroundLook(look: BackgroundLook | null) {
    preview = look;
    for (const listener of listeners) listener(look);
}

export function useBackgroundLookPreview(): BackgroundLook | null {
    const [state, setState] = useState<BackgroundLook | null>(preview);
    useEffect(() => {
        listeners.add(setState);
        setState(preview);
        return () => { listeners.delete(setState); };
    }, []);
    return state;
}

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
// 0.88, 0 gives no layer at all, and it saturates at fully opaque.
export function scrimAlphas(strength: number): {top: number; bottom: number} {
    const k = clampPercent(strength) / DEFAULT_BACKGROUND_DARKEN;
    const round = (v: number) => Math.round(Math.min(1, v) * 1000) / 1000;
    return {top: round(SCRIM_TOP_ALPHA * k), bottom: round(SCRIM_BOTTOM_ALPHA * k)};
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

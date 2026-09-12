import {getSwatchesSync} from 'colorthief';

// An accent color pulled from a game's own logo art, plus the text color
// that stays readable on top of it - colorthief's semantic swatches
// already solve the "readable text on an arbitrary extracted color"
// problem, so this is preferred over just taking the flat dominant color.
export interface Accent {
    color: string;
    textColor: string;
}

const cache = new Map<string, Accent | null>();

function loadImage(src: string): Promise<HTMLImageElement> {
    return new Promise((resolve, reject) => {
        const img = new Image();
        img.onload = () => resolve(img);
        img.onerror = () => reject(new Error('image failed to load'));
        img.src = src;
    });
}

// extractAccent picks the most useful swatch for a UI accent (a vibrant
// tone reads far better than colorthief's flat "most common pixel" color,
// which is often just a logo's plain background). Falls back through
// progressively less ideal swatches, and finally to null - a game with no
// usable color just keeps this app's default rust accent.
export async function extractAccent(imageSrc: string): Promise<Accent | null> {
    const cached = cache.get(imageSrc);
    if (cached !== undefined) {
        return cached;
    }

    let accent: Accent | null = null;
    try {
        const img = await loadImage(imageSrc);
        const swatches = getSwatchesSync(img);
        const swatch = swatches.Vibrant ?? swatches.LightVibrant ?? swatches.DarkVibrant
            ?? swatches.Muted ?? swatches.LightMuted ?? swatches.DarkMuted ?? null;
        if (swatch) {
            accent = {color: swatch.color.hex(), textColor: swatch.titleTextColor.hex()};
        }
    } catch {
        accent = null;
    }

    cache.set(imageSrc, accent);
    return accent;
}

function hexToRgb(hex: string): [number, number, number] {
    const clean = hex.replace('#', '');
    const n = parseInt(clean, 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function rgbToHex(r: number, g: number, b: number): string {
    const clamp = (n: number) => Math.max(0, Math.min(255, Math.round(n)));
    return '#' + [r, g, b].map((n) => clamp(n).toString(16).padStart(2, '0')).join('');
}

function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
    r /= 255; g /= 255; b /= 255;
    const max = Math.max(r, g, b), min = Math.min(r, g, b);
    const l = (max + min) / 2;
    if (max === min) {
        return [0, 0, l * 100];
    }
    const d = max - min;
    const s = d / (1 - Math.abs(2 * l - 1));
    let h: number;
    switch (max) {
        case r: h = ((g - b) / d) % 6; break;
        case g: h = (b - r) / d + 2; break;
        default: h = (r - g) / d + 4; break;
    }
    h *= 60;
    if (h < 0) h += 360;
    return [h, s * 100, l * 100];
}

function hslToRgb(h: number, s: number, l: number): [number, number, number] {
    s /= 100; l /= 100;
    const c = (1 - Math.abs(2 * l - 1)) * s;
    const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
    const m = l - c / 2;
    let rgb: [number, number, number];
    if (h < 60) rgb = [c, x, 0];
    else if (h < 120) rgb = [x, c, 0];
    else if (h < 180) rgb = [0, c, x];
    else if (h < 240) rgb = [0, x, c];
    else if (h < 300) rgb = [x, 0, c];
    else rgb = [c, 0, x];
    return [(rgb[0] + m) * 255, (rgb[1] + m) * 255, (rgb[2] + m) * 255];
}

// adjustLightness shifts a hex color's HSL lightness by deltaPercent
// (negative darkens, positive brightens), clamped so it never crushes to
// pure black/white - used to derive "found"/"selected"/"not found" row
// tones from one extracted accent instead of needing three separate
// extractions.
function adjustLightness(hex: string, deltaPercent: number): string {
    const [h, s, l] = rgbToHsl(...hexToRgb(hex));
    const newL = Math.max(6, Math.min(94, l + deltaPercent));
    return rgbToHex(...hslToRgb(h, s, newL));
}

// AccentTiers are the two tones a found game's row/checkbox can show, both
// derived from the one extracted accent color: base for one found but not
// yet chosen to manage, and bright for one found and selected. A game that
// wasn't found gets no accent color at all - just this app's plain
// default styling.
export interface AccentTiers {
    base: string;
    bright: string;
}

export function accentTiers(hex: string): AccentTiers {
    return {
        base: adjustLightness(hex, -12),
        bright: adjustLightness(hex, 10),
    };
}

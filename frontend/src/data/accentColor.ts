import {getSwatchesSync} from 'colorthief';
import {distinctColors, hexToRgb, hslToRgb, rgbToHex, rgbToHsl} from './accentPick';
import type {Accent, PaletteColor} from './accentPick';

// An accent color pulled from a game's own logo art, plus the text color
// that stays readable on top of it - colorthief's semantic swatches
// already solve the "readable text on an arbitrary extracted color"
// problem, so this is preferred over just taking the flat dominant color.
export type {Accent} from './accentPick';

const cache = new Map<string, Accent | null>();
const paletteCache = new Map<string, PaletteColor[]>();

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

// The swatches colorthief names, best accent candidates first.
const SWATCH_ORDER = ['Vibrant', 'LightVibrant', 'DarkVibrant', 'Muted', 'LightMuted', 'DarkMuted'] as const;

// extractPalette lists the main colours of a game's logo - colorthief's named swatches, the vibrant
// ones first, without near-duplicates - for picking one as a custom accent. Empty when the image
// cannot be read.
export async function extractPalette(imageSrc: string): Promise<PaletteColor[]> {
    const cached = paletteCache.get(imageSrc);
    if (cached) return cached;
    let colors: PaletteColor[] = [];
    try {
        const img = await loadImage(imageSrc);
        const swatches = getSwatchesSync(img);
        colors = distinctColors(SWATCH_ORDER.flatMap((role) => {
            const s = swatches[role];
            return s ? [{hex: s.color.hex().toLowerCase(), role}] : [];
        }));
    } catch {
        colors = [];
    }
    paletteCache.set(imageSrc, colors);
    return colors;
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

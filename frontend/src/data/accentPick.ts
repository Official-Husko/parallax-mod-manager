// Choosing the interface's accent colour, kept free of the DOM and of colorthief so the rules can
// be tested on their own: what a hex colour means, which text stays readable on it, and which of
// the three modes (the game's own colour, a custom one, the app's default) wins.

// An accent colour and the text colour that stays readable on top of it.
export interface Accent {
    color: string;
    textColor: string;
}

export type AccentMode = 'game' | 'custom' | 'default';

// What an unknown or missing mode means, as in the backend (preferences.NormalizedAccentMode).
export function normalizeAccentMode(mode: string | undefined): AccentMode {
    return mode === 'custom' || mode === 'default' ? mode : 'game';
}

// normalizeHex returns "#rrggbb" in lower case for #rgb or #rrggbb (the # optional), or '' when the
// text is not a hex colour - the same rule the backend saves by (preferences.NormalizedHexColor).
export function normalizeHex(value: string): string {
    let v = value.trim().replace(/^#/, '');
    if (/^[0-9a-fA-F]{3}$/.test(v)) v = v.split('').map((c) => c + c).join('');
    return /^[0-9a-fA-F]{6}$/.test(v) ? `#${v.toLowerCase()}` : '';
}

export function hexToRgb(hex: string): [number, number, number] {
    const n = parseInt(hex.replace('#', ''), 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

export function rgbToHex(r: number, g: number, b: number): string {
    const clamp = (n: number) => Math.max(0, Math.min(255, Math.round(n)));
    return '#' + [r, g, b].map((n) => clamp(n).toString(16).padStart(2, '0')).join('');
}

export function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
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

export function hslToRgb(h: number, s: number, l: number): [number, number, number] {
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

// WCAG relative luminance of a colour.
function luminance(hex: string): number {
    const [r, g, b] = hexToRgb(hex).map((c) => {
        const s = c / 255;
        return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
    const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
    return (hi + 0.05) / (lo + 0.05);
}

// The two text colours the app puts on an accent: the near-black it has always used on its rust
// (App.css --rust-text) and a near-white for accents too dark for that.
const DARK_TEXT = '#120d0a';
const LIGHT_TEXT = '#fff4f2';

// readableTextOn picks whichever of the two reads better on the given colour.
export function readableTextOn(hex: string): string {
    return contrast(hex, DARK_TEXT) >= contrast(hex, LIGHT_TEXT) ? DARK_TEXT : LIGHT_TEXT;
}

// The app's own background (App.css --bg-app), which the accent is drawn on as text and lines.
const APP_BACKGROUND = '#0a0d12';

// The contrast an accent must have against that background to be used as it is: the 4.5:1 WCAG asks
// of ordinary text, since the accent colours small labels, active tabs and the game's name.
export const MIN_ACCENT_CONTRAST = 4.5;

// ensureVisible is the colour the interface should actually use for a chosen one: the colour itself
// when it stands out enough on the app's dark background, otherwise a lighter shade of the same hue
// (just as much lighter as it takes). A near-black accent would make active tabs and accent-coloured
// text vanish, so no source of an accent - a custom colour, a game's icon - reaches the interface
// without passing this. '' when the text is not a colour.
export function ensureVisible(hex: string): string {
    const color = normalizeHex(hex);
    if (!color) return '';
    if (contrast(color, APP_BACKGROUND) >= MIN_ACCENT_CONTRAST) return color;
    const [h, s, l] = rgbToHsl(...hexToRgb(color));
    for (let lightness = Math.ceil(l) + 1; lightness <= 100; lightness++) {
        const lighter = rgbToHex(...hslToRgb(h, s, lightness));
        if (contrast(lighter, APP_BACKGROUND) >= MIN_ACCENT_CONTRAST) return lighter;
    }
    return '#ffffff';
}

// customAccent is the accent to use for a chosen colour - made visible (see ensureVisible), with
// readable text for it - or null when it is not a colour.
export function customAccent(hex: string): Accent | null {
    const color = ensureVisible(hex);
    return color ? {color, textColor: readableTextOn(color)} : null;
}

// visibleGameAccent is a game's icon colour made visible the same way. One that already stands out
// is kept exactly, with the text colour colorthief chose for it; a lightened one gets text picked
// for the new colour.
function visibleGameAccent(game: Accent): Accent {
    const color = ensureVisible(game.color);
    if (!color || color === normalizeHex(game.color)) return game;
    return {color, textColor: readableTextOn(color)};
}

// resolveAccent decides the accent to draw the interface with:
//   - a colour being previewed while it is picked wins over everything;
//   - 'default' is the app's own colour (null: leave the stylesheet's);
//   - 'custom' is the chosen colour; if there is none yet the game's own colour stands in;
//   - 'game' is the game's colour, or null (the app's own) for a game with no usable icon.
// Whatever the source, a colour too dark to read on the app's background is lightened first.
export function resolveAccent(mode: AccentMode, customHex: string, game: Accent | null, previewHex: string | null = null): Accent | null {
    const preview = previewHex ? customAccent(previewHex) : null;
    if (preview) return preview;
    if (mode === 'default') return null;
    const fromGame = game ? visibleGameAccent(game) : null;
    if (mode === 'custom') return customAccent(customHex) ?? fromGame;
    return fromGame;
}

export interface PaletteColor {
    hex: string;
    // Where in the picture it came from (colorthief's swatch role), for a tooltip.
    role: string;
}

function distance(a: string, b: string): number {
    const [r1, g1, b1] = hexToRgb(a);
    const [r2, g2, b2] = hexToRgb(b);
    return Math.hypot(r1 - r2, g1 - g2, b1 - b2);
}

// distinctColors drops a colour that is nearly the same as one already kept (colorthief often finds
// the same tone under two roles), keeping the first of each.
export function distinctColors(colors: PaletteColor[], minDistance = 28): PaletteColor[] {
    const kept: PaletteColor[] = [];
    for (const c of colors) {
        if (!kept.some((k) => distance(k.hex, c.hex) < minDistance)) kept.push(c);
    }
    return kept;
}

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

// hardToSee says whether a colour would be hard to make out drawn on the app's dark background
// (below the 3:1 contrast WCAG asks of large text and interface parts): a near-black accent would
// make the active tab and other accent-coloured text vanish.
export function hardToSee(hex: string): boolean {
    const color = normalizeHex(hex);
    return !!color && contrast(color, APP_BACKGROUND) < 3;
}

// customAccent is the accent for a chosen colour, or null when it is not a colour.
export function customAccent(hex: string): Accent | null {
    const color = normalizeHex(hex);
    return color ? {color, textColor: readableTextOn(color)} : null;
}

// resolveAccent decides the accent to draw the interface with:
//   - a colour being previewed while it is picked wins over everything;
//   - 'default' is the app's own colour (null: leave the stylesheet's);
//   - 'custom' is the chosen colour; if there is none yet the game's own colour stands in;
//   - 'game' is the game's colour, or null (the app's own) for a game with no usable icon.
export function resolveAccent(mode: AccentMode, customHex: string, game: Accent | null, previewHex: string | null = null): Accent | null {
    const preview = previewHex ? customAccent(previewHex) : null;
    if (preview) return preview;
    if (mode === 'default') return null;
    if (mode === 'custom') return customAccent(customHex) ?? game;
    return game;
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

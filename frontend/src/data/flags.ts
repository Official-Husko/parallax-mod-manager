// The three per-mod problem flags (version mismatch, hard conflict,
// dependency issue) and the one icon + color each is drawn with, everywhere
// it shows up - Active row FLAGS column, detail panel, pre-flight list,
// Autosort modals. Icons are Font Awesome Pro solid glyphs; colors are the
// --flag-* tokens in App.css. Each flag differs in both shape and hue so
// they stay tellable apart at 10px and for anyone who can't rely on color.
export const FLAG = {
    // Built for a different game version than the one installed - it may
    // well still work, so it is a warning (amber), not an error.
    version: {icon: 'fa-code-compare', color: 'var(--flag-version)'},
    // Two active mods define the same key. The one thing red is reserved for.
    conflict: {icon: 'fa-burst', color: 'var(--flag-conflict)'},
    // A declared requirement isn't active, isn't installed, or loads too late.
    dependency: {icon: 'fa-link-slash', color: 'var(--flag-dependency)'},
} as const;

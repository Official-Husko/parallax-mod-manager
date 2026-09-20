import type {library} from '../../wailsjs/go/models';

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

export interface ModConflictInfo {
    // How many contested keys this mod is a candidate in.
    keys: number;
    // The other mods it contests them with, by name, in first-seen order.
    others: string[];
}

// conflictsByMod turns the flat conflict list into what the conflict flag's
// tooltip needs per mod: how many keys it contests and with whom.
export function conflictsByMod(conflicts: library.ConflictSummary[]): Map<string, ModConflictInfo> {
    const info = new Map<string, ModConflictInfo>();
    for (const c of conflicts) {
        for (const cand of c.Candidates) {
            let entry = info.get(cand.ModID);
            if (!entry) {
                entry = {keys: 0, others: []};
                info.set(cand.ModID, entry);
            }
            entry.keys++;
            for (const other of c.Candidates) {
                if (other.ModID !== cand.ModID && !entry.others.includes(other.ModName)) entry.others.push(other.ModName);
            }
        }
    }
    return info;
}

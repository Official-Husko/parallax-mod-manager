import type {library} from '../../wailsjs/go/models';

// The per-mod flags - three problems (version mismatch, hard conflict,
// dependency issue) and three Steam Workshop states (unlisted, private,
// deleted) - and the one icon + color each is drawn with, everywhere
// it shows up - Active row FLAGS column, Available list, detail panel,
// pre-flight list, Autosort modals, the Updates window. Icons are Font Awesome Pro solid glyphs; colors are the
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
    // What became of the mod on the Steam Workshop (see data/workshopAvailability.ts).
    // Not problems with the mod as installed, so none of them is red or amber:
    // unlisted is only a fact worth knowing (teal), private and deleted mean the
    // mod may never update (pink, orange).
    unlisted: {icon: 'fa-eye-slash', color: 'var(--flag-unlisted)'},
    private: {icon: 'fa-lock', color: 'var(--flag-private)'},
    deleted: {icon: 'fa-cloud-slash', color: 'var(--flag-deleted)'},
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

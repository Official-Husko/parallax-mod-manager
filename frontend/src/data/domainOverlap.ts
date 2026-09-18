import type {library} from '../../wailsjs/go/models';

// The Active list's per-row colored segments (see data/mockData.ts's own
// `domains`) reduce every genuine conflict a mod is a candidate in, within
// one top-level content folder, to one of three states - mirroring the
// legend under the list (.active-legend):
export type DomainState = 'overwritten' | 'partial' | 'clean';

// A conflict's own Key.Type is the definition's containing folder path,
// relative to the mod's content root (see internal/pipeline's own
// `definition.Type(filepath.Dir(relPath))`) - e.g. "common/buildings" or
// "localisation/english". Only the first path segment matters here; it's
// checked against Stellaris's own top-level content folders, which is
// what the fixed six-letter CEGILM legend was always built around (see
// data/mockData.ts) - a conflict whose Type starts with some other folder
// (a different game's own layout) simply isn't represented in any segment
// yet, the same known limitation the legend itself already has.
const DOMAIN_LETTERS: Record<string, string> = {
    common: 'C',
    events: 'E',
    gfx: 'G',
    interface: 'I',
    localisation: 'L',
    map: 'M',
};

export const DOMAIN_NAMES: Record<string, string> = {
    C: 'common',
    E: 'events',
    G: 'gfx',
    I: 'interface',
    L: 'localisation',
    M: 'map',
};

function domainLetterForType(type: string): string | undefined {
    return DOMAIN_LETTERS[type.split('/')[0]];
}

// computeDomainOverlap derives, for every mod that's a candidate in at
// least one genuine conflict, which domain letters its content is
// contested in and whether it wins, loses, or splits within each one:
//
// - "overwritten": every contested key this mod touches in that domain,
//   it loses - none of its own content there actually takes effect.
// - "partial": a mix - it wins some contested keys in that domain and
//   loses others.
// - "clean": no contested keys there at all, or it wins every one it has
//   (nothing of its own is actually overwritten).
//
// A mod absent from the returned map has no conflicts anywhere and should
// be treated as "clean" in every domain - callers default to that rather
// than this function filling in every mod's every domain explicitly.
export function computeDomainOverlap(conflicts: library.ConflictSummary[]): Map<string, Partial<Record<string, DomainState>>> {
    const tally = new Map<string, Map<string, { won: boolean; lost: boolean }>>();
    for (const c of conflicts) {
        const letter = domainLetterForType(c.Type);
        if (!letter) continue;
        for (const cand of c.Candidates) {
            let byDomain = tally.get(cand.ModID);
            if (!byDomain) {
                byDomain = new Map();
                tally.set(cand.ModID, byDomain);
            }
            let entry = byDomain.get(letter);
            if (!entry) {
                entry = {won: false, lost: false};
                byDomain.set(letter, entry);
            }
            if (cand.ModID === c.Winner) {
                entry.won = true;
            } else {
                entry.lost = true;
            }
        }
    }

    const result = new Map<string, Partial<Record<string, DomainState>>>();
    for (const [modId, byDomain] of tally) {
        const states: Partial<Record<string, DomainState>> = {};
        for (const [letter, {won, lost}] of byDomain) {
            states[letter] = won && lost ? 'partial' : lost ? 'overwritten' : 'clean';
        }
        result.set(modId, states);
    }
    return result;
}

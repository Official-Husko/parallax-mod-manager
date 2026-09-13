import type {library} from '../../wailsjs/go/models';

// Autosort rearranges a load order using two real, derivable rules - never
// anything fabricated:
//
// 1. Fixes/Utilities/Patch tag rule: a mod tagged "Fixes", "Utilities", or
//    "Patch" moves to the very end - the same convention this app's own
//    generated patch mod already follows (see internal/library.
//    GeneratePatch's descriptor tags) and real-world modding practice
//    (compatibility patches load last).
// 2. Declared-dependency rule: a mod with a Dependencies entry (from its
//    own descriptor - see docs/paradox-mod-format.md) moves to load right
//    after the last of its dependencies it can find among the other
//    currently enabled mods, matched best-effort by display name (the
//    same approach the mod detail panel's Requires section already uses -
//    a dependency name isn't guaranteed to resolve to another mod's ID).
//
// Dependencies are applied last, as a final correcting pass, so a
// tag-based move can never leave a declared dependency violated -
// mirroring the order a proven, real Stellaris mod-sorting tool
// (stellaris-mod-sorter) already uses: tag/special-case rules first,
// dependency topology last, since load-order correctness matters more
// than any other rule.

export interface AutosortMove {
    modId: string;
    modName: string;
    fromIndex: number; // 0-based, in the order passed in
    toIndex: number; // 0-based, in the returned order
    reasons: string[];
}

export interface AutosortOptions {
    dependencies: boolean;
    fixesLast: boolean;
}

export interface AutosortResult {
    order: string[];
    moves: AutosortMove[];
    // cycleMods lists mods caught in a circular dependency (A needs B
    // needs A, however many mods long) - real, honest reporting rather
    // than silently guessing at an order for them. Left in their original
    // relative position, at the end, past everything the topological sort
    // could actually resolve.
    cycleMods: string[];
}

const lateTags = new Set(['fixes', 'utilities', 'patch']);

function isLateTagged(mod: library.ModSummary | undefined): boolean {
    return !!mod && mod.Tags.some((t) => lateTags.has(t.toLowerCase()));
}

// moveTaggedToEnd stable-partitions order into [everything else, late-
// tagged mods], preserving each group's own relative order.
function moveTaggedToEnd(order: string[], modsById: Map<string, library.ModSummary>): string[] {
    const normal: string[] = [];
    const late: string[] = [];
    for (const id of order) {
        (isLateTagged(modsById.get(id)) ? late : normal).push(id);
    }
    return [...normal, ...late];
}

// moveAfterDependencies topologically sorts order so every mod loads after
// every dependency it declares (best-effort name match against the other
// currently enabled mods). Uses a *stable* Kahn's algorithm - the same
// class of algorithm a proven, real Stellaris mod-sorting tool
// (stellaris-mod-sorter) already uses for exactly this - rather than
// repeatedly re-scanning for single violations: each round moves every
// mod with no remaining unresolved dependency in one pass, in its
// original relative order, so a mod with no ordering constraint at all
// never moves just because something elsewhere in the list did. This is
// both more efficient (guaranteed to finish in at most len(order) rounds,
// versus a repeated-single-move approach's worst-case cubic blowup on a
// long dependency chain) and, unlike a repeated-move heuristic with a
// fixed iteration cap, can actually detect a genuine dependency cycle
// instead of silently giving up on it.
function moveAfterDependencies(order: string[], modsById: Map<string, library.ModSummary>): { order: string[]; cycleMods: string[] } {
    const idByName = new Map<string, string>();
    for (const id of order) {
        const mod = modsById.get(id);
        if (mod) idByName.set(mod.Name, id);
    }

    // dependents[x] = mods that must load after x. inDegree[m] = how many
    // of m's own declared dependencies haven't been placed yet.
    const dependents = new Map<string, string[]>();
    const inDegree = new Map<string, number>();
    for (const id of order) inDegree.set(id, 0);

    for (const id of order) {
        const mod = modsById.get(id);
        if (!mod) continue;
        for (const depName of mod.Dependencies) {
            const depId = idByName.get(depName);
            if (!depId || depId === id) continue;
            inDegree.set(id, (inDegree.get(id) ?? 0) + 1);
            const list = dependents.get(depId);
            if (list) {
                list.push(id);
            } else {
                dependents.set(depId, [id]);
            }
        }
    }

    const remaining = new Set(order);
    const result: string[] = [];
    while (remaining.size > 0) {
        let placedThisRound = false;
        for (const id of order) {
            if (!remaining.has(id) || (inDegree.get(id) ?? 0) > 0) continue;
            result.push(id);
            remaining.delete(id);
            placedThisRound = true;
            for (const dependent of dependents.get(id) ?? []) {
                inDegree.set(dependent, (inDegree.get(dependent) ?? 0) - 1);
            }
        }
        if (!placedThisRound) {
            // Every remaining mod still has an unresolved dependency on
            // another remaining mod - a genuine cycle. Append them in
            // their original relative order rather than guessing, and
            // report exactly which mods are affected.
            const cycleMods = order.filter((id) => remaining.has(id));
            return {order: [...result, ...cycleMods], cycleMods};
        }
    }
    return {order: result, cycleMods: []};
}

export function autosort(order: string[], modsById: Map<string, library.ModSummary>, opts: AutosortOptions): AutosortResult {
    const knownNames = new Set([...modsById.values()].map((m) => m.Name));

    let working = [...order];
    let cycleMods: string[] = [];
    if (opts.fixesLast) {
        working = moveTaggedToEnd(working, modsById);
    }
    if (opts.dependencies) {
        const result = moveAfterDependencies(working, modsById);
        working = result.order;
        cycleMods = result.cycleMods;
    }

    const moves: AutosortMove[] = [];
    for (let toIndex = 0; toIndex < working.length; toIndex++) {
        const id = working[toIndex];
        const fromIndex = order.indexOf(id);
        if (fromIndex === toIndex) continue;

        const mod = modsById.get(id);
        const reasons: string[] = [];
        if (opts.fixesLast && isLateTagged(mod)) {
            reasons.push('Tagged Fixes/Utilities/Patch - moved to the end');
        }
        if (opts.dependencies && cycleMods.includes(id)) {
            reasons.push("Part of a circular dependency - couldn't be fully ordered");
        } else if (opts.dependencies && mod) {
            const resolvedDeps = mod.Dependencies.filter((name) => knownNames.has(name));
            if (resolvedDeps.length > 0) {
                reasons.push(`Depends on ${resolvedDeps.join(', ')}`);
            }
        }
        if (reasons.length === 0) {
            reasons.push('Reordered by autosort');
        }

        moves.push({modId: id, modName: mod?.Name ?? id, fromIndex, toIndex, reasons});
    }

    return {order: working, moves, cycleMods};
}

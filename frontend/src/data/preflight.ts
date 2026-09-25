import {FLAG} from './flags';
import type {library} from '../../wailsjs/go/models';
import {checksumShown} from './checksum';
import type {ChecksumState} from './checksum';

// Real pre-flight checks for the Workspace's own actions rail and the
// full "Ready to launch?" dialog - both used to show the exact same
// hardcoded mockup rows (fake mod names like "Road to 56", a fake
// checksum-matches-your-friends line this project has no way to actually
// know) regardless of what was really active or really conflicting. Every
// item here is instead computed from data this project already has in
// memory - no new fetch, no invented numbers. Two of the mockup's five
// original rows don't have an honest replacement and are dropped rather
// than faked: "N mods target an older patch" would need this project to
// detect the game's own currently-installed version, which it doesn't do
// anywhere yet; "load order matches your friends" would need an online
// checksum-sharing feature that doesn't exist at all.
export interface PreflightItem {
    // Set for the rows a screen may treat specially (the launch dialog shows the checksum in a
    // box of its own, so it leaves this row out while there is nothing to add to it).
    id?: 'checksum';
    icon: string;
    color: string;
    title: string;
    detail: string;
}

export interface DependencyIssues {
    missing: number;
    misordered: number;
    // Every mod, by ID, that's either missing one of its own declared
    // dependencies from the active load order or sits ahead of one it
    // needs - what the Active list's own per-row warning shield reflects,
    // so it never has to recompute this same matching itself.
    affectedIds: Set<string>;
    // Per affected mod, which declared dependencies are the problem: names
    // not in the active load order at all, and names that are active but
    // load after this mod. What the flag's own hover tooltip lists.
    byMod: Map<string, {missing: string[]; misordered: string[]}>;
}

// findDependencyIssues is the real dependency-matching both
// buildPreflightItems (aggregate counts) and the Active list's own
// per-row warning shield need - best-effort, by declared dependency name
// against the other currently active mods, the same real matching
// Autosort's own dependency rule already uses (see data/autosort.ts).
export function findDependencyIssues(active: library.ModSummary[]): DependencyIssues {
    const activeNames = new Set(active.map((m) => m.Name));
    const indexById = new Map(active.map((m, i) => [m.ID, i]));
    // A Map lookup instead of active.find(...) inside the loop below - the same best-effort
    // by-name matching data/autosort.ts's own idByName already uses (a duplicate active name,
    // already an inherent ambiguity in this "match by display name, not a real reference"
    // approach - see that file's own doc comment - resolves the same last-one-wins way there
    // too). Matters for a large, realistic Stellaris load order: this is the O(n) scan that
    // made the active list's own referential-stability bug upstream in Workspace.tsx worth
    // fixing in the first place.
    const byName = new Map(active.map((m) => [m.Name, m]));
    let missing = 0;
    let misordered = 0;
    const affectedIds = new Set<string>();
    const byMod = new Map<string, {missing: string[]; misordered: string[]}>();
    const issuesOf = (id: string) => {
        let issues = byMod.get(id);
        if (!issues) {
            issues = {missing: [], misordered: []};
            byMod.set(id, issues);
        }
        return issues;
    };
    for (const m of active) {
        // The generated patch lists every mod it was built from as a
        // dependency, as a snapshot for the launcher. Whether it's still
        // accurate is the patch's own staleness check, and where it sits is
        // governed by the "keep the generated patch last" setting - neither
        // belongs in the dependency chain check.
        if (m.GeneratedPatch) continue;
        for (const depName of m.Dependencies) {
            if (!activeNames.has(depName)) {
                missing++;
                affectedIds.add(m.ID);
                issuesOf(m.ID).missing.push(depName);
                continue;
            }
            const depMod = byName.get(depName);
            if (depMod && (indexById.get(depMod.ID) ?? 0) > (indexById.get(m.ID) ?? 0)) {
                misordered++;
                affectedIds.add(m.ID);
                issuesOf(m.ID).misordered.push(depName);
            }
        }
    }
    return {missing, misordered, affectedIds, byMod};
}

export function buildPreflightItems(
    active: library.ModSummary[],
    conflicts: library.ConflictSummary[],
    scanErrors: string[],
    dependencyIssues: DependencyIssues,
    checksum?: {state: ChecksumState; playsetName: string; unsaved: boolean},
): PreflightItem[] {
    const items: PreflightItem[] = [];

    if (scanErrors.length > 0) {
        items.push({
            icon: 'fa-triangle-exclamation',
            color: 'var(--amber)',
            title: `${scanErrors.length} scan issue${scanErrors.length === 1 ? '' : 's'}`,
            detail: scanErrors[0],
        });
    } else {
        items.push({
            icon: 'fa-check',
            color: 'var(--green)',
            title: `All ${active.length} mod${active.length === 1 ? '' : 's'} present`,
            detail: 'Nothing missing from disk or Workshop.',
        });
    }

    if (conflicts.length > 0) {
        items.push({
            icon: FLAG.conflict.icon,
            color: FLAG.conflict.color,
            title: `${conflicts.length} hard conflict${conflicts.length === 1 ? '' : 's'} unresolved`,
            detail: 'Two or more active mods define the same key - see the Conflict Resolver.',
        });
    } else {
        items.push({
            icon: 'fa-check',
            color: 'var(--green)',
            title: 'No conflicts detected',
            detail: 'No two active mods define the same key.',
        });
    }

    const {missing, misordered} = dependencyIssues;
    if (missing > 0 || misordered > 0) {
        const parts: string[] = [];
        if (missing > 0) parts.push(`${missing} not active`);
        if (misordered > 0) parts.push(`${misordered} loading in the wrong order`);
        const total = missing + misordered;
        items.push({
            icon: FLAG.dependency.icon,
            color: FLAG.dependency.color,
            title: `${total} dependency issue${total === 1 ? '' : 's'} found`,
            detail: `${parts.join(', ')} - try Autosort.`,
        });
    } else {
        items.push({
            icon: 'fa-check',
            color: 'var(--green)',
            title: 'Dependency chain complete',
            detail: 'Every required mod is active and correctly ordered.',
        });
    }

    if (checksum && checksumShown(checksum.state, checksum.playsetName)) {
        items.push(checksumPreflightItem(checksum.state, checksum.unsaved));
    }

    return items;
}

// The multiplayer checksum's row. The code describes the playset as saved - what launching
// loads - so while the list on screen has edits not saved yet it says so instead of vouching
// for a number that is about to change.
export function checksumPreflightItem(state: ChecksumState, unsaved: boolean): PreflightItem {
    if (state.kind === 'calculating') {
        return {id: 'checksum', icon: 'fa-spinner fa-spin', color: 'var(--text-muted)', title: 'Calculating the checksum...', detail: 'Working out the code the game will show on its main menu.'};
    }
    if (state.kind === 'unavailable') {
        return {id: 'checksum', icon: 'fa-triangle-exclamation', color: 'var(--amber)', title: 'Checksum unavailable', detail: state.reason};
    }
    const detail = `Every player needs this same code to join your multiplayer game - it is the one the game shows on its main menu. Worked out from ${state.files.toLocaleString()} files.`;
    if (unsaved) {
        return {id: 'checksum', icon: 'fa-triangle-exclamation', color: 'var(--amber)', title: `Checksum ${state.value} · save to update`, detail: `${detail} It describes the saved playset; the changes not saved yet will change it.`};
    }
    return {id: 'checksum', icon: 'fa-check', color: 'var(--green)', title: `Checksum ${state.value} · MP ready`, detail};
}

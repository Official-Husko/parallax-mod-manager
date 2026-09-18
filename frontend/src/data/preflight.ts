import type {library} from '../../wailsjs/go/models';

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
}

// findDependencyIssues is the real dependency-matching both
// buildPreflightItems (aggregate counts) and the Active list's own
// per-row warning shield need - best-effort, by declared dependency name
// against the other currently active mods, the same real matching
// Autosort's own dependency rule already uses (see data/autosort.ts).
export function findDependencyIssues(active: library.ModSummary[]): DependencyIssues {
    const activeNames = new Set(active.map((m) => m.Name));
    const indexById = new Map(active.map((m, i) => [m.ID, i]));
    let missing = 0;
    let misordered = 0;
    const affectedIds = new Set<string>();
    for (const m of active) {
        for (const depName of m.Dependencies) {
            if (!activeNames.has(depName)) {
                missing++;
                affectedIds.add(m.ID);
                continue;
            }
            const depMod = active.find((x) => x.Name === depName);
            if (depMod && (indexById.get(depMod.ID) ?? 0) > (indexById.get(m.ID) ?? 0)) {
                misordered++;
                affectedIds.add(m.ID);
            }
        }
    }
    return {missing, misordered, affectedIds};
}

export function buildPreflightItems(
    active: library.ModSummary[],
    conflicts: library.ConflictSummary[],
    scanErrors: string[],
    dependencyIssues: DependencyIssues,
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
            icon: 'fa-xmark',
            color: 'var(--red)',
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
            icon: 'fa-triangle-exclamation',
            color: 'var(--amber)',
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

    return items;
}

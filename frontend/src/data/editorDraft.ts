// The Editor's working copy of one mod's descriptor values, and the rules for telling whether it
// differs from what is saved - kept free of the DOM so they can be tested on their own.

export interface DraftFields {
    name: string;
    version: string;
    supportedVersion: string;
    tags: string[];
    dependencies: string[];
    replacePaths: string[];
}

// What the backend reports for a mod (app.EditInfo's Fields), by name.
export interface SavedFields {
    Name: string;
    Version: string;
    SupportedVersion: string;
    Tags: string[] | null;
    Dependencies: string[] | null;
    ReplacePaths: string[] | null;
}

export interface Draft extends DraftFields {
    // A picture chosen as the new thumbnail ('' for none) and how it looked when chosen.
    thumbnailFrom: string;
    createDescriptor: boolean;
}

export function draftFromSaved(f: SavedFields): Draft {
    return {
        name: f.Name ?? '',
        version: f.Version ?? '',
        supportedVersion: f.SupportedVersion ?? '',
        tags: [...(f.Tags ?? [])],
        dependencies: [...(f.Dependencies ?? [])],
        replacePaths: [...(f.ReplacePaths ?? [])],
        thumbnailFrom: '',
        createDescriptor: false,
    };
}

// tidy trims each item and drops blank and repeated ones, as the backend does before saving.
export function tidyList(items: string[]): string[] {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const raw of items) {
        const item = raw.trim();
        if (item && !seen.has(item)) {
            seen.add(item);
            out.push(item);
        }
    }
    return out;
}

function sameList(a: string[], b: string[]): boolean {
    return a.length === b.length && a.every((x, i) => x === b[i]);
}

// isChanged says whether the draft would change anything: a field that differs once tidied, a new
// thumbnail, or a descriptor.mod to create.
export function isChanged(draft: Draft, saved: SavedFields): boolean {
    const base = draftFromSaved(saved);
    return draft.name.trim() !== base.name
        || draft.version.trim() !== base.version
        || draft.supportedVersion.trim() !== base.supportedVersion
        || !sameList(tidyList(draft.tags), tidyList(base.tags))
        || !sameList(tidyList(draft.dependencies), tidyList(base.dependencies))
        || !sameList(tidyList(draft.replacePaths), tidyList(base.replacePaths))
        || draft.thumbnailFrom !== ''
        || draft.createDescriptor;
}

// toEdit is the draft as the backend's ModEdit takes it.
export function toEdit(draft: Draft) {
    return {
        Fields: {
            Name: draft.name,
            Version: draft.version,
            SupportedVersion: draft.supportedVersion,
            Tags: tidyList(draft.tags),
            Dependencies: tidyList(draft.dependencies),
            ReplacePaths: tidyList(draft.replacePaths),
        },
        ThumbnailFrom: draft.thumbnailFrom,
        CreateDescriptor: draft.createDescriptor,
    };
}

// addItem adds a typed item to a list unless it is blank or already there. Several items can be
// pasted at once separated by commas or line breaks.
export function addItems(list: string[], typed: string): string[] {
    return tidyList([...list, ...typed.split(/[,\n]/)]);
}

// The mods a dependency name matches, by exact name (how Paradox descriptors name them).
export function unknownDependencies(deps: string[], installedNames: Set<string>): string[] {
    return tidyList(deps).filter((d) => !installedNames.has(d));
}

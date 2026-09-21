// Whether the load order on screen holds changes the saved playset does not: a mod
// added, removed or moved. That is what the Save button blinks for (see Workspace).
//
// saved is the order the playset had when it was last loaded or saved, or null while
// there is no saved playset behind the list - a new draft, an imported playset, a
// deleted one - where anything in the list is unsaved.
//
// Mods that are not installed any more (exists says so) are ignored on both sides:
// a mod that disappeared from disk changes the list without the person having
// changed anything, and saving would only drop it, so it is not an edit to blink about.
// Order matters: moving a mod is a change.
export function hasUnsavedChanges(order: string[], saved: string[] | null, exists: (id: string) => boolean): boolean {
    const now = order.filter(exists);
    if (saved === null) return now.length > 0;
    const then = saved.filter(exists);
    return now.length !== then.length || now.some((id, i) => id !== then[i]);
}

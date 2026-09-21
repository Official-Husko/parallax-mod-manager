// Where a drop lands in a list: 'before' an existing row (by id), or 'end' for after everything.
export type ReorderTarget = {kind: 'before'; id: string} | {kind: 'end'};

// reorderInsert moves ids (keeping their given relative order) to target's dropped position in
// list - and hands back the same array when that leaves the list as it was.
//
// The position is worked out against the rows that stay put, which is what keeps it right
// whether the dragged rows sat before or after the drop point. A drop "before" a row that is
// itself being dragged (a mod let go of where it already is, or one of several dragged mods)
// has no such row among the ones that stay, so it lands where that row was: after the stay-put
// rows that were above it. A row that is not in the list at all falls back to the end, never
// the top.
export function reorderInsert(list: string[], ids: string[], target: ReorderTarget): string[] {
    const moving = new Set(ids);
    const remaining = list.filter((id) => !moving.has(id));
    let insertAt = remaining.length;
    if (target.kind === 'before') {
        const at = list.indexOf(target.id);
        if (at >= 0) insertAt = list.slice(0, at).filter((id) => !moving.has(id)).length;
    }
    const result = [...remaining.slice(0, insertAt), ...ids, ...remaining.slice(insertAt)];
    const unchanged = result.length === list.length && result.every((id, i) => id === list[i]);
    return unchanged ? list : result;
}

import {useRef} from 'preact/hooks';

export interface DragMultiSelectHandlers {
    // Returns the ids this mousedown just selected (the single row, or the
    // shift-extended range) - [] for a button other than the left one,
    // which this ignores entirely. Callers that only care about the
    // selection side effect (the two Workspace lists, on their own) can
    // ignore the return value; Workspace's cross-list drag-to-move needs
    // it synchronously, before the resulting setSelectedX state update
    // has actually flushed.
    onRowMouseDown: (index: number, e: MouseEvent) => string[];
}

// Real OS-file-list-style row selection, shared by the Workspace's
// Available and Active lists so both behave identically: click a row to
// select just it, or shift+click to extend from the last-clicked row
// through this one (inclusive, by list position - not just the rows the
// cursor happened to land exactly on). Either replaces whatever was
// selected before, exactly like Explorer/Finder's own list view does -
// never toggles or accumulates on its own.
//
// A mousedown-and-drag across rows used to extend the selection the same
// way too, but that's gone now - both lists hand mousedown off to a real
// drag-to-move instead once the mouse actually moves (see
// data/listDragMove.ts), and having the two features fight over the same
// gesture read as a real bug (dragging to reposition a row was instead
// silently selecting every row the cursor crossed). Shift+click alone
// covers range selection now.
//
// preventDefault() on mousedown is what actually stops the browser's own
// native text-selection drag from kicking in instead (the "blue
// highlight" that used to appear when dragging, or shift-clicking, across
// the row labels) - .mod-row's own user-select:none in CSS is a second,
// redundant guard for the same thing, kept as a fallback in case
// something else on a row ever intercepts the mousedown before this
// handler runs.
export function useDragMultiSelect(
    ids: string[],
    onSelect: (ids: string[]) => void,
    onFocus: (id: string) => void,
): DragMultiSelectHandlers {
    // The last row a plain (non-shift) click landed on - shift+click
    // always extends from here, matching every real file list's own
    // "anchor" concept.
    const anchorRef = useRef<number | null>(null);

    function applyRange(fromIndex: number, toIndex: number): string[] {
        const [lo, hi] = fromIndex <= toIndex ? [fromIndex, toIndex] : [toIndex, fromIndex];
        const selected = ids.slice(lo, hi + 1);
        onSelect(selected);
        const focusId = ids[toIndex];
        if (focusId) onFocus(focusId);
        return selected;
    }

    function onRowMouseDown(index: number, e: MouseEvent): string[] {
        if (e.button !== 0) return []; // left button only - a right-click opens the context menu instead
        e.preventDefault();
        if (e.shiftKey && anchorRef.current !== null) {
            return applyRange(anchorRef.current, index);
        }
        anchorRef.current = index;
        return applyRange(index, index);
    }

    return {onRowMouseDown};
}

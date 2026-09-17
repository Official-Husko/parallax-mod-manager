import {useEffect, useRef, useState} from 'preact/hooks';

export type ListId = 'available' | 'active';

// Where a drag would move its dragged ids to - a real positional drop in
// either list ('before' an existing row, by real id, or 'end' to append
// after everything), the same insertion-line preview regardless of which
// list the drag came from or is headed to. Active's own load order is the
// real, persisted thing; Available's is a temporary, session-local
// arrangement the user can drag around for their own organizing,
// reconciled against whatever's actually available (see Workspace's own
// availableOrder state).
export type DropTarget =
    | { list: 'active'; kind: 'before'; id: string }
    | { list: 'active'; kind: 'end' }
    | { list: 'available'; kind: 'before'; id: string }
    | { list: 'available'; kind: 'end' };

function dropTargetsEqual(a: DropTarget | null, b: DropTarget | null): boolean {
    if (a === b) return true;
    if (!a || !b || a.list !== b.list || a.kind !== b.kind) return false;
    return a.kind === 'before' && b.kind === 'before' ? a.id === b.id : true;
}

function pointInRect(x: number, y: number, r: DOMRect): boolean {
    return x >= r.left && x <= r.right && y >= r.top && y <= r.bottom;
}

function positionalTarget(list: ListId, container: HTMLElement, y: number): DropTarget {
    for (const row of Array.from(container.children) as HTMLElement[]) {
        const id = row.dataset.modId;
        if (!id) continue;
        const r = row.getBoundingClientRect();
        if (y < r.top + r.height / 2) {
            return {list, kind: 'before', id} as DropTarget;
        }
    }
    return {list, kind: 'end'} as DropTarget;
}

// Real drag-and-drop between the Available and Active lists, and within
// each of them, matching how real mod managers (Mod Organizer 2, Vortex,
// RimSort/RimPy) handle their own active/plugin list: dragging a row from
// Available into Active activates it at the exact dropped position;
// dragging an Active row back into Available deactivates it, landing at
// the exact dropped position in Available's own temporary arrangement;
// dragging a row within either list reorders it to the dropped position
// within that same list. Every case gets the identical positional
// insertion-line preview - which list a drag started in or ends up in
// never changes how the drop itself is shown.
//
// startDrag is meant to be called with whatever ids a row's own
// useDragMultiSelect onRowMouseDown just selected (a single row, or a
// shift-extended range) - or, when the pressed row was already part of a
// bigger existing selection, that whole selection instead (dragging one
// of several selected rows drags all of them, matching real Explorer/
// Finder/MO2) - see Workspace.tsx's onAvailableRowMouseDown/
// onActiveRowMouseDown. Both container refs must be attached to each
// list's own row container, and every real row inside each one needs a
// matching data-mod-id attribute for the position lookup above to find it.
export function useListDragMove(onDrop: (ids: string[], source: ListId, target: DropTarget) => void) {
    const dragRef = useRef<{ ids: string[]; source: ListId; startX: number; startY: number; moved: boolean } | null>(null);
    const dropTargetRef = useRef<DropTarget | null>(null);
    const availableRowsRef = useRef<HTMLDivElement | null>(null);
    const activeRowsRef = useRef<HTMLDivElement | null>(null);
    // Mirror the refs above into state purely so rows/overlays can render
    // their own live feedback (dimmed source rows, an insertion line, a
    // ghost preview following the cursor) - the refs remain the source of
    // truth read by onUp below, which never re-subscribes and so can't
    // rely on a fresh state closure at drop time.
    const [draggedIds, setDraggedIds] = useState<string[] | null>(null);
    const [draggedSource, setDraggedSource] = useState<ListId | null>(null);
    const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
    const [pointer, setPointer] = useState<{ x: number; y: number } | null>(null);

    useEffect(() => {
        function computeTarget(x: number, y: number): DropTarget | null {
            const activeContainer = activeRowsRef.current;
            if (activeContainer && pointInRect(x, y, activeContainer.getBoundingClientRect())) {
                return positionalTarget('active', activeContainer, y);
            }
            const availableContainer = availableRowsRef.current;
            if (availableContainer && pointInRect(x, y, availableContainer.getBoundingClientRect())) {
                return positionalTarget('available', availableContainer, y);
            }
            return null;
        }

        function onMove(e: MouseEvent) {
            const drag = dragRef.current;
            if (!drag) return;
            if (!drag.moved) {
                // A few pixels of real movement before this counts as a
                // drag at all - a plain click (mousedown then mouseup with
                // no movement) must never flash the ghost preview or dim
                // its own row, even for an instant.
                if (Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY) < 4) return;
                drag.moved = true;
                setDraggedIds(drag.ids);
                setDraggedSource(drag.source);
            }
            setPointer({x: e.clientX, y: e.clientY});
            const target = computeTarget(e.clientX, e.clientY);
            if (!dropTargetsEqual(target, dropTargetRef.current)) {
                dropTargetRef.current = target;
                setDropTarget(target);
            }
        }

        function onUp() {
            const drag = dragRef.current;
            const target = dropTargetRef.current;
            dragRef.current = null;
            dropTargetRef.current = null;
            setDraggedIds(null);
            setDraggedSource(null);
            setDropTarget(null);
            setPointer(null);
            if (drag && drag.moved && target) {
                onDrop(drag.ids, drag.source, target);
            }
        }

        window.addEventListener('mousemove', onMove);
        window.addEventListener('mouseup', onUp);
        return () => {
            window.removeEventListener('mousemove', onMove);
            window.removeEventListener('mouseup', onUp);
        };
        // Deliberately [] - onDrop is an inline closure in Workspace.tsx
        // that gets a new identity every render, and re-subscribing these
        // window listeners on every render (just to keep a "fresh" onDrop)
        // would be wasteful for no benefit: onDrop only ever touches state
        // through a functional setState update, so calling whichever
        // version was captured at mount is always correct regardless of
        // how stale that closure reference itself is.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    function startDrag(ids: string[], source: ListId, e: MouseEvent) {
        if (ids.length === 0) return;
        dragRef.current = {ids, source, startX: e.clientX, startY: e.clientY, moved: false};
    }

    return {startDrag, availableRowsRef, activeRowsRef, draggedIds, draggedSource, dropTarget, pointer};
}

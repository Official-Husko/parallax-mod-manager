import {useCallback, useRef, useState} from 'preact/hooks';
import {useVirtualWindow} from './useVirtualWindow';

// Windowed rendering for a wrapping card grid (Browse's own .browse-grid: `grid-template-columns:
// repeat(auto-fill, minmax(minColumnWidth, 1fr))`) - useVirtualWindow's own flat, one-item-per-row
// list doesn't fit a grid where several items share a row and the column count itself depends on
// the container's real width. This composes that hook rather than duplicating its scroll-window
// math: it turns "how many items" into "how many grid rows" (by predicting the column count the
// same way CSS auto-fill resolves it - how many minColumnWidth-plus-gap slots fit the measured
// width), then delegates first/last-row bookkeeping to useVirtualWindow unchanged.
//
// Every item must render at exactly rowHeight tall (including its own share of the grid's row
// gap) for the same reason useVirtualWindow's own rowHeight must be exact - the windowing maths
// depends on it. Browse's own cards are effectively fixed height (every text row is a
// single-line ellipsis, never wrapped), which is what makes this safe to use there.
//
// containerPadding is the grid element's own horizontal padding (both sides combined) - ref
// measures the *scrolling* ancestor's width (it has to: that's the element whose scrollTop the
// vertical windowing math needs), which is wider than the grid's own content box by however much
// padding the grid itself declares, and the column-count prediction needs the narrower figure.
export function useVirtualGrid<T extends HTMLElement>(itemCount: number, minColumnWidth: number, gap: number, containerPadding: number, rowHeight: number, overscan: number) {
    const [viewWidth, setViewWidth] = useState(1000);
    const widthObserver = useRef<ResizeObserver | null>(null);

    // Mirrors repeat(auto-fill, minmax(minColumnWidth, 1fr)): how many minColumnWidth-plus-gap
    // slots fit the available width. Doesn't need to predict the stretched per-column pixel
    // width auto-fill resolves to (1fr distributes the remainder) - only the column *count*,
    // which is all grouping items into rows needs.
    const contentWidth = Math.max(0, viewWidth - containerPadding);
    const columns = Math.max(1, Math.floor((contentWidth + gap) / (minColumnWidth + gap)));
    const rowCount = Math.max(1, Math.ceil(itemCount / columns));
    const rows = useVirtualWindow<T>(rowCount, rowHeight, overscan);

    const ref = useCallback((el: T | null) => {
        rows.ref(el);
        widthObserver.current?.disconnect();
        widthObserver.current = null;
        if (!el) return;
        const measure = () => setViewWidth(el.clientWidth);
        measure();
        widthObserver.current = new ResizeObserver(measure);
        widthObserver.current.observe(el);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [rows.ref]);

    return {
        ref,
        onScroll: rows.onScroll,
        columns,
        rowCount,
        rowHeight, // echoed back so a caller's own height/padding math uses the same figure
        firstRow: rows.first,
        // Item indices, clamped to the real item count - the last grid row can be a partial one.
        first: rows.first * columns,
        last: Math.min(itemCount, rows.last * columns),
    };
}

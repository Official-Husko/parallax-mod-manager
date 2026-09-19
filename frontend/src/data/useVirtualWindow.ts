import {useCallback, useRef, useState} from 'preact/hooks';

// Windowed rendering for a long scroll list of fixed-height rows: works out
// which slice of `count` rows is actually on screen (plus `overscan` rows of
// margin either side, so a fast scroll never shows a blank gap before the
// next render lands), so a caller can put just that slice in the DOM instead
// of all of them.
//
// The Conflict Resolver needs this in two places - the contested-keys list
// (a big modlist can have well over ten thousand contested keys, most of them
// localisation) and each side of the diff (a mod file can run to thousands of
// lines). Rendering every row made every click pay for the whole list, and
// the browser had to lay out that many rows again each time anything near
// them changed.
//
// The caller owns the layout: give the scroll container `ref` and `onScroll`,
// make every row exactly `rowHeight` pixels tall, size a wrapper inside it to
// count * rowHeight, and offset the rendered slice by first * rowHeight
// (padding-top on that wrapper is the simplest way) - then render rows
// [first, last).
export function useVirtualWindow<T extends HTMLElement>(count: number, rowHeight: number, overscan: number) {
    // Held as a row index, not a pixel offset, so a scroll only re-renders
    // when it actually crosses into a different row.
    const [topRow, setTopRow] = useState(0);
    // A generous default so the very first render (before the container has
    // been measured) already fills a tall window rather than a blank one.
    const [viewHeight, setViewHeight] = useState(1000);
    const observer = useRef<ResizeObserver | null>(null);

    // A callback ref rather than a useEffect over a ref object: the scroll
    // container isn't always mounted when the component first renders (the
    // diff body only exists once there's no load error), and this measures
    // whenever it does appear.
    const ref = useCallback((el: T | null) => {
        observer.current?.disconnect();
        observer.current = null;
        if (!el) return;
        const measure = () => setViewHeight(el.clientHeight);
        measure();
        observer.current = new ResizeObserver(measure);
        observer.current.observe(el);
    }, []);

    const onScroll = useCallback((e: Event) => {
        setTopRow(Math.floor((e.currentTarget as HTMLElement).scrollTop / rowHeight));
    }, [rowHeight]);

    // Clamped to the real row count: filtering the list down while scrolled
    // far past its new end must still land on real rows, not an empty slice.
    const top = Math.min(topRow, Math.max(0, count - 1));
    const first = Math.max(0, top - overscan);
    const last = Math.min(count, top + Math.ceil(viewHeight / rowHeight) + overscan + 1);
    return {ref, onScroll, first, last};
}

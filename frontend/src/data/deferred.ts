// Runs fn once the browser has painted whatever the caller just rendered -
// a requestAnimationFrame callback fires *before* the frame's paint, so the
// setTimeout inside it is what pushes fn past that paint. Use it to put a
// "working on it..." state on screen first and only then start a stretch of
// synchronous work that would otherwise freeze the UI before that state ever
// showed. Returns a cancel function (call it from an effect's cleanup, so a
// stale run never lands after the thing it was for went away).
export function afterNextPaint(fn: () => void): () => void {
    let timer: number | undefined;
    const frame = requestAnimationFrame(() => {
        timer = window.setTimeout(fn, 0);
    });
    return () => {
        cancelAnimationFrame(frame);
        if (timer !== undefined) window.clearTimeout(timer);
    };
}

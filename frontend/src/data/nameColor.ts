// HUE_BUCKETS evenly spaced hues around the color wheel, rather than any of
// the full 360 possible hues - a naive hash mod 360 was tried first and, in
// practice, clustered several real mod names' hues within a few degrees of
// each other (all landing in the same reddish band), because a simple
// polynomial hash's output isn't uniform enough across a small real modlist.
// Bucketing guarantees every two *different* hues are at least
// 360 / HUE_BUCKETS degrees apart - two names can still land in the same
// bucket (a real hash collision, same as any hash), but they'll never be
// merely close-but-not-quite-the-same the way the raw approach produced.
const HUE_BUCKETS = 18;

// fnv1a is FNV-1a, a small non-cryptographic string hash with good bit
// dispersion - swapped in after the original simple polynomial hash
// (hash*31+c) turned out not to distribute real mod names well enough
// across HUE_BUCKETS.
function fnv1a(s: string): number {
    let h = 0x811c9dc5;
    for (let i = 0; i < s.length; i++) {
        h ^= s.charCodeAt(i);
        h = Math.imul(h, 0x01000193);
    }
    return h >>> 0;
}

// colorFromName derives a stable, deterministic color from a string - used
// to give each local mod's folder badge its own distinct color (by name),
// so a long list of otherwise-identical folder icons isn't one flat wall of
// the same gray. Same name always yields the same color, with no lookup
// table or state to keep in sync.
export function colorFromName(name: string): string {
    const hue = (fnv1a(name) % HUE_BUCKETS) * (360 / HUE_BUCKETS);
    return `hsl(${hue}, 60%, 62%)`;
}

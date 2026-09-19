export function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = n / 1024;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value.toFixed(value < 10 ? 2 : 1)} ${units[unit]}`;
}

// timeAgo renders a real Unix-seconds timestamp as a relative age string
// ("3 days ago") - shared between the mod detail panel's own "Updated"
// line (Workspace.tsx) and the conflict resolver's per-candidate file
// timestamps, so the two don't drift into two different phrasings for the
// same kind of value.
export function timeAgo(unixSeconds: number): string {
    const seconds = Math.max(0, Date.now() / 1000 - unixSeconds);
    const units: [number, string][] = [
        [31536000, 'year'], [2592000, 'month'], [86400, 'day'],
        [3600, 'hour'], [60, 'minute'],
    ];
    for (const [secs, label] of units) {
        const n = Math.floor(seconds / secs);
        if (n >= 1) return `${n} ${label}${n === 1 ? '' : 's'} ago`;
    }
    return 'just now';
}

// truncate caps text at maxLength real characters, replacing the last 3
// with "..." once it's exceeded - a hard guard for the handful of places
// a mod's real title or a Steam author's real display name gets shown at
// a fixed, prominent size (the detail panel's own title, the author
// card) with no natural width-driven CSS ellipsis to fall back on. Text
// already at or under the limit is returned unchanged.
export function truncate(text: string, maxLength: number): string {
    if (text.length <= maxLength) {
        return text;
    }
    return text.slice(0, Math.max(0, maxLength - 3)) + '...';
}

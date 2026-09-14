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

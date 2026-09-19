// A real, from-scratch line-level diff (longest common subsequence, the
// same algorithm behind `diff`/`git diff`'s line mode) between two file
// contents - the conflict resolver's diff panes tint each real line
// "removed" (only in the losing mod's file) or "added" (only in the
// winning mod's), rather than showing two independent, undifferentiated
// syntax-highlighted dumps side by side.
export interface LineDiffResult {
    // One entry per line in the left/right input: true means that line is
    // part of the two files' longest common subsequence (present, in the
    // same relative order, in both) - false means it's only in this side.
    leftMatched: boolean[];
    rightMatched: boolean[];
    added: number;
    removed: number;
    // True when the inputs were too large to diff safely (see
    // MAX_DIFF_LINES below) - leftMatched/rightMatched are then all true
    // (nothing flagged as changed) and added/removed are both 0, so a
    // caller that doesn't check this flag still degrades gracefully to
    // "no tinting" rather than a wrong count.
    skipped: boolean;
}

// The LCS table below is O(n*m) time *and* space - fine for a typical
// Paradox script/locale file (hundreds, sometimes low thousands of
// lines), but a 2500x2500 Int32Array is already 25MB, and that grows
// quadratically. Past this, skip diffing rather than risk a multi-second
// hang or an out-of-memory tab on a pathologically large file - the
// caller still shows the file's own real, syntax-highlighted content,
// just without added/removed tinting.
const MAX_DIFF_LINES = 2500;

export function diffLines(leftLines: string[], rightLines: string[]): LineDiffResult {
    const n = leftLines.length;
    const m = rightLines.length;

    if (n > MAX_DIFF_LINES || m > MAX_DIFF_LINES) {
        return {
            leftMatched: new Array(n).fill(true),
            rightMatched: new Array(m).fill(true),
            added: 0,
            removed: 0,
            skipped: true,
        };
    }

    // dp[i*(m+1)+j] = length of the LCS of leftLines[i:] and rightLines[j:].
    // Flattened into one Int32Array rather than an array of arrays - a few
    // times less memory and faster to index than nested JS arrays at this
    // size.
    const width = m + 1;
    const dp = new Int32Array((n + 1) * width);
    for (let i = n - 1; i >= 0; i--) {
        for (let j = m - 1; j >= 0; j--) {
            dp[i * width + j] = leftLines[i] === rightLines[j]
                ? dp[(i + 1) * width + (j + 1)] + 1
                : Math.max(dp[(i + 1) * width + j], dp[i * width + (j + 1)]);
        }
    }

    const leftMatched = new Array(n).fill(false);
    const rightMatched = new Array(m).fill(false);
    let i = 0;
    let j = 0;
    while (i < n && j < m) {
        if (leftLines[i] === rightLines[j]) {
            leftMatched[i] = true;
            rightMatched[j] = true;
            i++;
            j++;
        } else if (dp[(i + 1) * width + j] >= dp[i * width + (j + 1)]) {
            i++;
        } else {
            j++;
        }
    }

    const removed = leftMatched.filter((matched) => !matched).length;
    const added = rightMatched.filter((matched) => !matched).length;
    return {leftMatched, rightMatched, added, removed, skipped: false};
}

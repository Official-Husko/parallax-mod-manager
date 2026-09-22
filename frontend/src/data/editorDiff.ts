import {diffLines} from './lineDiff';

// A before/after text as the rows of a unified diff, for the Editor's "what will change" panel.
export interface DiffRow {
    kind: 'same' | 'add' | 'del' | 'gap';
    text: string;
}

export interface FileDiff {
    rows: DiffRow[];
    added: number;
    removed: number;
}

const CONTEXT = 2;

function splitLines(text: string): string[] {
    if (text === '') return [];
    const lines = text.replace(/\r\n/g, '\n').split('\n');
    // A text that ends with a line break has no last, empty line.
    if (lines[lines.length - 1] === '') lines.pop();
    return lines;
}

// diffText compares two texts line by line and keeps only the changed lines with a couple of lines
// of context around them (unchanged stretches in between become one 'gap' row).
export function diffText(before: string, after: string): FileDiff {
    const a = splitLines(before);
    const b = splitLines(after);
    const d = diffLines(a, b);

    // Walk both sides: lines only on the left were removed, lines only on the right were added, and
    // the rest pair up in order.
    const all: DiffRow[] = [];
    let i = 0;
    let j = 0;
    while (i < a.length || j < b.length) {
        if (i < a.length && !d.leftMatched[i]) all.push({kind: 'del', text: a[i++]});
        else if (j < b.length && !d.rightMatched[j]) all.push({kind: 'add', text: b[j++]});
        else {
            all.push({kind: 'same', text: b[j]});
            i++;
            j++;
        }
    }

    const keep = new Array(all.length).fill(false);
    all.forEach((row, idx) => {
        if (row.kind === 'same') return;
        for (let k = Math.max(0, idx - CONTEXT); k <= Math.min(all.length - 1, idx + CONTEXT); k++) keep[k] = true;
    });
    const rows: DiffRow[] = [];
    let gap = false;
    all.forEach((row, idx) => {
        if (keep[idx]) {
            gap = false;
            rows.push(row);
        } else if (!gap) {
            gap = true;
            rows.push({kind: 'gap', text: ''});
        }
    });
    // A gap only says something between changes; none is needed at either end when nothing follows.
    if (rows.length > 0 && rows[0].kind === 'gap') rows.shift();
    if (rows.length > 0 && rows[rows.length - 1].kind === 'gap') rows.pop();

    return {rows, added: all.filter((r) => r.kind === 'add').length, removed: all.filter((r) => r.kind === 'del').length};
}

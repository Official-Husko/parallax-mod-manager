import type {app} from '../../wailsjs/go/models';

// A comment reply's own Bold/Italic/Link toolbar needs somewhere to keep that formatting.
// Tracking a live "what's bold right now" typing mode against a plain <textarea> is fragile - a
// paste, an edit in the middle of the text, or moving the cursor would all need to be diffed
// correctly to know what just changed. Instead the toolbar inserts lightweight, familiar
// markdown-style markers (**bold**, *italic*, [label](url)) around the current selection - the
// same authoring pattern GitHub's own comment box toolbar uses - and the raw textarea stays one
// plain string the whole time. parseDraftText (below) turns that plain string into the
// structured paragraphs/runs LoversLabPostComment actually sends, right before posting - the
// backend only ever sees real runs with real Bold/Italic/LinkURL flags, never raw markup text to
// interpret itself (see internal/app/loverslab.go's commentParagraphsToHTML).

// wrapSelection inserts before/after around textarea el's current selection (or, with nothing
// selected, just inserts both markers with the cursor left between them) and returns the new
// full value - the caller still owns committing that value to its own draft state. Also moves
// the browser's own cursor to land where someone would expect it next, the same behaviour every
// markdown editor's own toolbar already has.
export function wrapSelection(el: HTMLTextAreaElement, before: string, after: string): string {
    const {selectionStart, selectionEnd, value} = el;
    const selected = value.slice(selectionStart, selectionEnd);
    const next = value.slice(0, selectionStart) + before + selected + after + value.slice(selectionEnd);
    const cursor = selectionStart + before.length + selected.length + (selected ? after.length : 0);
    requestAnimationFrame(() => {
        el.focus();
        el.setSelectionRange(cursor, cursor);
    });
    return next;
}

interface DraftRun {
    text: string;
    bold?: boolean;
    italic?: boolean;
    linkURL?: string;
}

// DRAFT_MARKUP matches, in order, **bold**, *italic* and [label](url) - bold is tried before
// italic in the same alternation so "**x**" is never misread as two adjacent, empty *italic*
// markers.
const DRAFT_MARKUP = /\*\*(.+?)\*\*|\*(.+?)\*|\[([^\]]+)]\(([^)]+)\)/g;

// parseDraftLine splits one line of raw text into runs.
function parseDraftLine(line: string): DraftRun[] {
    const runs: DraftRun[] = [];
    let last = 0;
    DRAFT_MARKUP.lastIndex = 0;
    let m: RegExpExecArray | null;
    while ((m = DRAFT_MARKUP.exec(line))) {
        if (m.index > last) runs.push({text: line.slice(last, m.index)});
        if (m[1] !== undefined) runs.push({text: m[1], bold: true});
        else if (m[2] !== undefined) runs.push({text: m[2], italic: true});
        else runs.push({text: m[3], linkURL: m[4]});
        last = DRAFT_MARKUP.lastIndex;
    }
    if (last < line.length) runs.push({text: line.slice(last)});
    return runs;
}

// parseDraftText turns a whole raw draft into the paragraphs/runs LoversLabPostComment expects -
// blank-line-separated paragraphs (the same convention the backend used to split on itself
// before this moved client-side), a single line break within a paragraph becoming its own run
// with an embedded "\n" (commentParagraphsToHTML turns that into <br> on the way out).
export function parseDraftText(raw: string): app.CommentParagraph[] {
    const paragraphs = raw.replace(/\r\n/g, '\n').split(/\n{2,}/);
    return paragraphs.map((para) => {
        const lines = para.split('\n');
        const runs: DraftRun[] = [];
        lines.forEach((line, i) => {
            if (i > 0) runs.push({text: '\n'});
            runs.push(...parseDraftLine(line));
        });
        return {Runs: runs.map((r) => ({Text: r.text, Bold: !!r.bold, Italic: !!r.italic, LinkURL: r.linkURL ?? ''}))} as app.CommentParagraph;
    });
}

// isBlankDraft is the same "nothing to post yet" check the Post button's own disabled state
// needs - the backend's own commentParagraphsToHTML is the real, authoritative empty check
// (this is only ever a UX nicety, not a correctness boundary).
export function isBlankDraft(raw: string): boolean {
    return raw.trim() === '';
}

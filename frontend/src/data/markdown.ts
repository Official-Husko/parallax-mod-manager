// A small markdown reader for the one document this app shows that is written in
// it: the licence (see views/LicenceModal.tsx). It understands what that text
// uses - headings, paragraphs, bullet and numbered lists, a horizontal rule, and
// bold, inline code and bare links inside them - and nothing else. The result is
// plain data, drawn as elements by the caller, so no text is ever treated as HTML.

export type Inline =
    | { kind: 'text'; text: string }
    | { kind: 'bold'; text: string }
    | { kind: 'code'; text: string }
    | { kind: 'link'; text: string };

export type Block =
    | { kind: 'h1' | 'h2' | 'p'; inlines: Inline[] }
    | { kind: 'ul' | 'ol'; items: Inline[][] }
    | { kind: 'hr' };

// **bold**, `code`, or a bare http(s) address - which stops before a trailing
// full stop, comma or bracket, since those belong to the sentence around it.
const INLINE = /\*\*(.+?)\*\*|`([^`]+)`|(https?:\/\/[^\s)]*[^\s).,;:])/g;

export function parseInline(source: string): Inline[] {
    const out: Inline[] = [];
    let last = 0;
    for (const m of source.matchAll(INLINE)) {
        const start = m.index ?? 0;
        if (start > last) out.push({kind: 'text', text: source.slice(last, start)});
        if (m[1] !== undefined) out.push({kind: 'bold', text: m[1]});
        else if (m[2] !== undefined) out.push({kind: 'code', text: m[2]});
        else out.push({kind: 'link', text: m[3]});
        last = start + m[0].length;
    }
    if (last < source.length) out.push({kind: 'text', text: source.slice(last)});
    return out;
}

export function parseMarkdown(text: string): Block[] {
    const blocks: Block[] = [];
    let paragraph: string[] = [];
    let list: { kind: 'ul' | 'ol'; items: Inline[][] } | null = null;

    const flushParagraph = () => {
        if (paragraph.length > 0) blocks.push({kind: 'p', inlines: parseInline(paragraph.join(' '))});
        paragraph = [];
    };
    const flushList = () => {
        if (list) blocks.push(list);
        list = null;
    };
    const flush = () => {
        flushParagraph();
        flushList();
    };

    for (const raw of text.split('\n')) {
        const line = raw.trim();
        if (line === '') {
            flush();
        } else if (line.startsWith('## ')) {
            flush();
            blocks.push({kind: 'h2', inlines: parseInline(line.slice(3))});
        } else if (line.startsWith('# ')) {
            flush();
            blocks.push({kind: 'h1', inlines: parseInline(line.slice(2))});
        } else if (/^-{3,}$/.test(line)) {
            flush();
            blocks.push({kind: 'hr'});
        } else if (line.startsWith('* ') || /^\d+\. /.test(line)) {
            const kind = line.startsWith('* ') ? 'ul' : 'ol';
            flushParagraph();
            if (list && list.kind !== kind) flushList();
            if (!list) list = {kind, items: []};
            list.items.push(parseInline(line.replace(/^(\* |\d+\. )/, '')));
        } else {
            flushList();
            paragraph.push(line);
        }
    }
    flush();
    return blocks;
}

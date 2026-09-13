// Lightweight syntax highlighting for the two real, editable text formats
// this project's mods actually contain: Clausewitz script (the classic
// "key = value" format almost every .txt/.gui/.gfx/.asset/.shader/.fxh
// file uses, per internal/script/lexer.go) and Paradox locale .yml files
// (per internal/locale/locale.go). Token rules here deliberately mirror
// those two real Go lexers/parsers - the source of truth for what's
// actually valid syntax - rather than inventing separate rules that could
// drift from what this project's own parsers accept.
//
// This is a per-line, best-effort highlighter for display only (the
// Conflict Resolver's real file content view) - it never feeds back into
// parsing or patch generation, so a token it gets wrong (an edge case
// neither real lexer needs to worry about, like a string split across a
// line join) only costs a slightly-off color, never incorrect behavior.

export type FileSyntax = 'script' | 'locale' | 'plain';

// scriptExtensions matches FileTree.tsx's own EXTENSION_VISUALS script
// coloring - every one of these is a real Clausewitz script file, even
// though only .txt is the common case. Locale files are always .yml.
const scriptExtensions = new Set(['txt', 'gui', 'gfx', 'asset', 'shader', 'fxh', 'mod']);

export function syntaxForPath(path: string): FileSyntax {
    const dot = path.lastIndexOf('.');
    if (dot < 0) return 'plain';
    const ext = path.slice(dot + 1).toLowerCase();
    if (ext === 'yml' || ext === 'yaml') return 'locale';
    if (scriptExtensions.has(ext)) return 'script';
    return 'plain';
}

export type TokenKind =
    | 'plain' | 'comment' | 'string' | 'number' | 'operator' | 'brace' | 'variable' | 'key' | 'bool' | 'ident'
    | 'locale-header' | 'locale-key' | 'locale-version' | 'locale-string';

export interface Token {
    text: string;
    kind: TokenKind;
}

// isIdentStop matches internal/script/lexer.go's own isIdentStop exactly -
// the same characters that end a bare identifier there.
function isIdentStop(c: string): boolean {
    return c === '' || /\s/.test(c) || c === '{' || c === '}' || c === '"' || c === '#'
        || c === '@' || c === '=' || c === '<' || c === '>' || c === '!';
}

function isDigit(c: string): boolean {
    return c >= '0' && c <= '9';
}

// tokenizeScript splits one line of real Clausewitz script into tokens,
// following internal/script/lexer.go's own rules: '#' starts a
// comment running to end of line; '"..."' is a string (with \" and \\
// escapes); '{'/'}' are blocks; '=','<','>','!' are operators, each
// optionally followed by '=' ('<=','>=','!='); '@name' is a variable
// reference; a signed-or-not run of digits (with at most one '.') is a
// number; anything else runs as a bare identifier up to the next stop
// character.
function tokenizeScript(line: string): Token[] {
    const tokens: Token[] = [];
    let i = 0;
    const n = line.length;

    while (i < n) {
        const c = line[i];

        if (/\s/.test(c)) {
            let j = i + 1;
            while (j < n && /\s/.test(line[j])) j++;
            tokens.push({text: line.slice(i, j), kind: 'plain'});
            i = j;
            continue;
        }

        if (c === '#') {
            tokens.push({text: line.slice(i), kind: 'comment'});
            break;
        }

        if (c === '"') {
            let j = i + 1;
            while (j < n) {
                if (line[j] === '\\' && j + 1 < n) {
                    j += 2;
                    continue;
                }
                if (line[j] === '"') {
                    j++;
                    break;
                }
                j++;
            }
            tokens.push({text: line.slice(i, j), kind: 'string'});
            i = j;
            continue;
        }

        if (c === '{' || c === '}') {
            tokens.push({text: c, kind: 'brace'});
            i++;
            continue;
        }

        if (c === '@') {
            let j = i + 1;
            while (j < n && !isIdentStop(line[j])) j++;
            tokens.push({text: line.slice(i, j), kind: 'variable'});
            i = j;
            continue;
        }

        if (c === '=') {
            // Unlike '<'/'>'/'!' below, a real '=' never combines with a
            // following '=' - internal/script/lexer.go's TokenEquals is
            // always exactly one character.
            tokens.push({text: '=', kind: 'operator'});
            i++;
            continue;
        }

        if (c === '<' || c === '>' || c === '!') {
            let j = i + 1;
            if (j < n && line[j] === '=') j++;
            tokens.push({text: line.slice(i, j), kind: 'operator'});
            i = j;
            continue;
        }

        if (isDigit(c) || (c === '-' && i + 1 < n && isDigit(line[i + 1]))) {
            let j = i + 1;
            while (j < n && isDigit(line[j])) j++;
            if (j < n && line[j] === '.' && j + 1 < n && isDigit(line[j + 1])) {
                j++;
                while (j < n && isDigit(line[j])) j++;
            }
            tokens.push({text: line.slice(i, j), kind: 'number'});
            i = j;
            continue;
        }

        let j = i + 1;
        while (j < n && !isIdentStop(line[j])) j++;
        const word = line.slice(i, j);
        tokens.push({text: word, kind: (word === 'yes' || word === 'no') ? 'bool' : 'ident'});
        i = j;
    }

    // A second pass: a bare identifier immediately followed (skipping any
    // single run of whitespace) by an operator is a key, not a value -
    // e.g. "cost" in "cost = 5" or "age" in "age > 20". Booleans keep
    // their own kind regardless of position, since "yes = trigger" reads
    // the same as any other key-shaped word if it happened to collide.
    for (let k = 0; k < tokens.length; k++) {
        if (tokens[k].kind !== 'ident') continue;
        let next = k + 1;
        if (next < tokens.length && tokens[next].kind === 'plain') next++;
        if (next < tokens.length && tokens[next].kind === 'operator') {
            tokens[k] = {...tokens[k], kind: 'key'};
        }
    }

    return tokens;
}

// localeHeaderPattern matches internal/locale/locale.go's parseHeader:
// "l_<language>:" with nothing but optional trailing whitespace after.
const localeHeaderPattern = /^(\s*)(l_[A-Za-z_]+:)(\s*)$/;

// localeEntryPattern matches internal/locale/locale.go's parseEntryLine:
// "KEY:VERSION "value"" - VERSION is optional digits (defaults to 0 when
// absent, and the real parser doesn't require a space before the version
// or before the quote either, though every real file checked this session
// always includes one). KEY is any run of non-colon, non-whitespace
// characters. A trailing "# comment" (real, common in Paradox locale
// files) is captured separately so it still gets comment coloring instead
// of being folded into the string.
const localeEntryPattern = /^(\s*)([^\s:]+):(\d*)(\s*)("(?:\\.|[^"\\])*")(.*)$/;

// tokenizeLocale splits one line of a real Paradox locale .yml file.
// Falls back to a single 'plain' token for anything that doesn't match
// one of the three real line shapes (header, comment, entry) - a
// best-effort display, never a strict validator.
function tokenizeLocale(line: string): Token[] {
    const trimmed = line.trimStart();
    if (trimmed.startsWith('#')) {
        const lead = line.length - trimmed.length;
        return lead > 0
            ? [{text: line.slice(0, lead), kind: 'plain'}, {text: trimmed, kind: 'comment'}]
            : [{text: line, kind: 'comment'}];
    }

    const header = localeHeaderPattern.exec(line);
    if (header) {
        const tokens: Token[] = [];
        if (header[1]) tokens.push({text: header[1], kind: 'plain'});
        tokens.push({text: header[2], kind: 'locale-header'});
        if (header[3]) tokens.push({text: header[3], kind: 'plain'});
        return tokens;
    }

    const entry = localeEntryPattern.exec(line);
    if (entry) {
        const [, lead, key, version, gap, value, rest] = entry;
        const tokens: Token[] = [];
        if (lead) tokens.push({text: lead, kind: 'plain'});
        tokens.push({text: key, kind: 'locale-key'});
        tokens.push({text: ':' + version, kind: 'locale-version'});
        if (gap) tokens.push({text: gap, kind: 'plain'});
        tokens.push({text: value, kind: 'locale-string'});
        if (rest) {
            const restTrimmed = rest.trimStart();
            const restLead = rest.length - restTrimmed.length;
            if (restLead) tokens.push({text: rest.slice(0, restLead), kind: 'plain'});
            if (restTrimmed.startsWith('#')) {
                tokens.push({text: restTrimmed, kind: 'comment'});
            } else if (restTrimmed) {
                tokens.push({text: restTrimmed, kind: 'plain'});
            }
        }
        return tokens;
    }

    return [{text: line, kind: 'plain'}];
}

// highlightLine tokenizes one line of real mod file content for display,
// according to syntax (see syntaxForPath). 'plain' syntax (an image/audio
// name shown incidentally, or any extension this project doesn't
// recognize as script/locale) always returns the whole line as one
// unstyled token - never guesses at a format with no real grammar here.
export function highlightLine(line: string, syntax: FileSyntax): Token[] {
    switch (syntax) {
        case 'script':
            return tokenizeScript(line);
        case 'locale':
            return tokenizeLocale(line);
        default:
            return [{text: line, kind: 'plain'}];
    }
}

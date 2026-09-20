import {useEffect, useState} from 'preact/hooks';
import {ClearLog, LogEntries, LogEvent} from '../../wailsjs/go/main/App';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import type {applog} from '../../wailsjs/go/models';

// The app's activity log, as seen from the frontend. The backend keeps the
// lines (internal/applog: a bounded in-memory ring plus a rotating file) and
// streams new ones as "log-batch" events; this file is the client half - a hook
// that follows them, a way to add a line from the UI, and the small pure
// helpers the log view draws with.

export type LogEntry = applog.Entry;
export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

// The most lines held here - matches the backend ring, so a snapshot never
// holds more than this anyway.
const MAX_ENTRIES = 2000;

const LEVEL_RANK: Record<LogLevel, number> = {debug: 0, info: 1, warn: 2, error: 3};

export function levelRank(level: string): number {
    return LEVEL_RANK[level as LogLevel] ?? LEVEL_RANK.info;
}

// Adds a line to the activity log from the UI - for things only the frontend
// knows happened (an autosort run, a prompt shown). Fire-and-forget: logging
// must never be able to break the thing being logged.
export function logEvent(level: LogLevel, component: string, message: string): void {
    try {
        LogEvent(level, component, message).catch(() => undefined);
    } catch {
        // The bridge itself is unavailable (or not up yet) - logging must never
        // be able to throw into the code that is trying to log.
    }
}

// Decides which error reports get through: the same text at most `perText`
// times, and at most `total` reports of any kind, per window. A render that
// throws on every frame, or a failing request retried in a loop, would
// otherwise bury the log (and the bridge) in identical lines.
export function createErrorThrottle(perText = 3, total = 20, windowMs = 60000): (text: string, now?: number) => boolean {
    let windowStart = 0;
    let reported = 0;
    const seen = new Map<string, number>();
    return (text, now = Date.now()) => {
        if (now - windowStart > windowMs) {
            windowStart = now;
            reported = 0;
            seen.clear();
        }
        if (reported >= total) return false;
        const count = seen.get(text) ?? 0;
        if (count >= perText) return false;
        seen.set(text, count + 1);
        reported++;
        return true;
    };
}

function describeError(reason: unknown): string {
    if (reason instanceof Error) return reason.message || reason.name;
    if (typeof reason === 'string') return reason;
    try {
        return JSON.stringify(reason) ?? String(reason);
    } catch {
        return String(reason);
    }
}

let globalErrorLoggingInstalled = false;

// Sends anything the UI throws that nothing caught - an uncaught error, an
// unhandled promise rejection - to the activity log as an error line, so a
// problem that would otherwise only ever show in a webview console nobody has
// open is there when someone looks at the log. Call once at startup.
export function installGlobalErrorLogging(): void {
    if (globalErrorLoggingInstalled) return;
    globalErrorLoggingInstalled = true;
    const allow = createErrorThrottle();
    window.addEventListener('error', (e) => {
        const where = e.filename ? ` (${e.filename.split('/').pop()}:${e.lineno})` : '';
        const text = `${e.message || 'unknown error'}${where}`;
        if (allow(text)) logEvent('error', 'UI', text);
    });
    window.addEventListener('unhandledrejection', (e) => {
        const text = `unhandled promise rejection: ${describeError(e.reason)}`;
        if (allow(text)) logEvent('error', 'UI', text);
    });
}

// Merges two sets of lines by sequence number (a snapshot and live events can
// overlap), oldest first, keeping the newest MAX_ENTRIES.
export function mergeEntries(a: LogEntry[], b: LogEntry[]): LogEntry[] {
    if (b.length === 0) return a;
    if (a.length === 0 || b[0].Seq > a[a.length - 1].Seq) {
        // The common case: everything new is newer than everything held.
        const joined = a.concat(b);
        return joined.length > MAX_ENTRIES ? joined.slice(joined.length - MAX_ENTRIES) : joined;
    }
    const bySeq = new Map<number, LogEntry>();
    for (const e of a) bySeq.set(e.Seq, e);
    for (const e of b) bySeq.set(e.Seq, e);
    const all = [...bySeq.values()].sort((x, y) => x.Seq - y.Seq);
    return all.length > MAX_ENTRIES ? all.slice(all.length - MAX_ENTRIES) : all;
}

// Follows the activity log while the calling component is mounted: subscribes
// to live batches first, then reads the backend's current lines, so nothing
// logged in between is missed (the overlap is dropped by sequence number).
export function useAppLog(): { entries: LogEntry[]; clear: () => void } {
    const [entries, setEntries] = useState<LogEntry[]>([]);

    useEffect(() => {
        let alive = true;
        const stop = EventsOn('log-batch', (batch: LogEntry[]) => {
            if (alive && batch?.length) setEntries((prev) => mergeEntries(prev, batch));
        });
        LogEntries()
            .then((snapshot) => { if (alive) setEntries((prev) => mergeEntries(prev, snapshot ?? [])); })
            .catch(() => undefined);
        return () => {
            alive = false;
            stop();
        };
    }, []);

    return {
        entries,
        clear: () => {
            setEntries([]);
            ClearLog().catch(() => undefined);
        },
    };
}

function pad(n: number): string {
    return n < 10 ? `0${n}` : String(n);
}

// "2026/07/06 00:09:37" - the same shape the log file uses, in local time.
export function formatLogTime(ms: number): string {
    const d = new Date(ms);
    return `${d.getFullYear()}/${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

// The line as plain text, exactly as the log file writes it - what "Copy"
// puts on the clipboard. Kept in step with applog.Entry.Line in Go.
export function entryText(e: LogEntry): string {
    const marker = e.Level === 'debug' ? 'DEBUG: ' : e.Level === 'warn' ? 'WARN: ' : e.Level === 'error' ? 'ERROR: ' : '';
    return `${formatLogTime(e.Time)} [${e.Component}] ${marker}${e.Message}${e.Timed ? ` (${e.DurationMs}ms)` : ''}`;
}

// How a step's duration is coloured: quick ones fade into the background, slow
// ones stand out. The names are deliberately not "fast"/"slow"-style bare words:
// Font Awesome Pro defines a global `.fast` class (its shorthand for the sharp
// thin style) that forces its icon font and a fixed width on any element that
// carries it - these become `dur-*` classes, which nothing else claims.
export type DurationTone = 'quick' | 'slow' | 'very-slow';

export function durationTone(ms: number): DurationTone {
    if (ms < 100) return 'quick';
    if (ms < 1000) return 'slow';
    return 'very-slow';
}

export interface MessagePart {
    text: string;
    kind: 'plain' | 'string' | 'number' | 'path' | 'version' | 'id';
}

// What a message is cut into, tried left to right at each position (so a
// quoted name wins over whatever is inside it). One capture group per kind:
//   1 a quoted name          'Stellaris', "tech_lasers_5", "/home/x/y.mod"
//   2 an absolute path       /home/user/.steam/steamapps/common/Stellaris
//   3 a relative path        common/technology/00_tech.txt
//   4 a version              4.4.6, v4.2.1, v4.4
//   5 a hash or long id      a1b2c3d4, 1466534100 (a Workshop id)
//   6 a bare number          3.42, 412
const TOKEN = new RegExp(
    String.raw`('[^']*'|"[^"]*")`
    + String.raw`|((?:~|\.{1,2})?(?:/[\w.@+\-]+)+/?)`
    + String.raw`|([\w.@+\-]+(?:/[\w.@+\-]+)+/?)`
    + String.raw`|(\bv?\d+(?:\.\d+){2,}\b|\bv\d+\.\d+\b)`
    + String.raw`|(\b(?=[0-9a-f]*[a-f])(?=[0-9a-f]*\d)[0-9a-f]{7,40}\b|\b\d{8,}\b)`
    + String.raw`|(\b\d+(?:\.\d+)?\b)`,
    'g',
);

// A relative "a/b" is only a path when it has more than one slash or ends in a
// file name with an extension - "and/or", "n/a" and "1/2" are just words.
function isRelativePath(text: string): boolean {
    return (text.match(/\//g)?.length ?? 0) >= 2 || /\.\w{1,8}\/?$/.test(text);
}

// A quoted name that is really a path (it may contain spaces, unlike a bare one).
function isQuotedPath(inner: string): boolean {
    return inner.includes('/') && (/^(?:~|\.{0,2})\//.test(inner) || isRelativePath(inner));
}

// Splits a message into runs to colour: quoted names, paths, versions, ids and
// bare numbers. Purely cosmetic - the text is never changed, only cut up, so
// joining the parts always gives back the original message.
export function tokenizeMessage(message: string): MessagePart[] {
    const parts: MessagePart[] = [];
    let last = 0;
    TOKEN.lastIndex = 0;
    for (let m = TOKEN.exec(message); m !== null; m = TOKEN.exec(message)) {
        let kind: MessagePart['kind'];
        if (m[1] !== undefined) kind = isQuotedPath(m[1].slice(1, -1)) ? 'path' : 'string';
        else if (m[2] !== undefined) kind = 'path';
        else if (m[3] !== undefined) kind = isRelativePath(m[3]) ? 'path' : 'plain';
        else if (m[4] !== undefined) kind = 'version';
        else if (m[5] !== undefined) kind = 'id';
        else kind = 'number';
        if (kind === 'plain') continue;
        if (m.index > last) parts.push({text: message.slice(last, m.index), kind: 'plain'});
        parts.push({text: m[0], kind});
        last = m.index + m[0].length;
    }
    if (last < message.length) parts.push({text: message.slice(last), kind: 'plain'});
    return parts;
}

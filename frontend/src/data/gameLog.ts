import {formatBytes} from './format';

// The game's own log files, as seen from the frontend. The backend
// (internal/gamelog) follows one file and streams "game-log" events; this is
// the pure half: what a line means, and how a batch of them updates what the
// viewer holds. Kept free of components so it can be checked on its own.

export type GameLogLevel = 'info' | 'warn' | 'error';

export interface GameLogLine {
    // Grows for as long as the viewer lives, so a line keeps its identity when
    // older ones are dropped off the top (it is the list key).
    id: number;
    text: string;
    level: GameLogLevel;
    // A line this app added to say something about the log, not one the game
    // wrote (some output was skipped).
    note?: boolean;
}

// One update from the followed file - gamelog.Batch on the Go side. Written out
// here because events aren't part of the generated bindings.
export interface GameLogBatch {
    Reset: boolean;
    Missing: boolean;
    Lines: string[];
    Skipped: number;
}

export interface GameLogState {
    lines: GameLogLine[];
    nextId: number;
    // The followed file doesn't exist (yet): the game creates its logs when it
    // starts.
    missing: boolean;
}

export const EMPTY_GAME_LOG: GameLogState = {lines: [], nextId: 1, missing: false};

// The most lines held. A broken mod can make the game write tens of thousands
// in a minute; the newest are the ones anyone is looking at.
export const MAX_GAME_LOG_LINES = 5000;

// The game's logs have no levels, only "[12:34:56][source.cpp:123]: message". So
// what a line is about is read from the words in it. It is a convenience for
// filtering - the line itself is always shown as the game wrote it.
const ERROR_WORDS = /\b(errors?|failed|failure|could not|couldn't|cannot|can't|exception|crash(?:ed)?|fatal|invalid|unexpected|not found|missing|unable)\b/i;
const WARN_WORDS = /\b(warn|warning|deprecated|overrid(?:e|es|ing)|ignor(?:e|ed|ing)|obsolete)\b/i;

const HEADER = /^\[(\d{1,2}:\d{2}:\d{2})\]\[([^\]]*)\]:\s?/;

export function classifyGameLogLine(text: string): GameLogLevel {
    // Only what the game said, not the "[time][source.cpp:line]" it says it from:
    // a source file called "error_handler.cpp" isn't an error.
    const message = text.replace(HEADER, '');
    if (ERROR_WORDS.test(message)) return 'error';
    if (WARN_WORDS.test(message)) return 'warn';
    return 'info';
}

export interface ParsedGameLogLine {
    time: string;
    source: string;
    message: string;
}

// Splits "[23:45:35][dlc.cpp:1840]: Could not find files" into its parts, or
// null for a line without that header (a continuation line, like the shader
// source the game sometimes prints in full).
export function parseGameLogLine(text: string): ParsedGameLogLine | null {
    const m = HEADER.exec(text);
    if (!m) return null;
    return {time: m[1], source: m[2], message: text.slice(m[0].length)};
}

// The source file without its line number: "dlc.cpp:1840" -> "dlc.cpp". What a
// line is coloured by, so lines from the same file share a colour.
export function sourceFile(source: string): string {
    const i = source.lastIndexOf(':');
    return i > 0 && /^\d+$/.test(source.slice(i + 1)) ? source.slice(0, i) : source;
}

// The state after one more batch from the backend.
export function applyGameLogBatch(state: GameLogState, batch: GameLogBatch): GameLogState {
    let id = state.nextId;
    const added: GameLogLine[] = [];
    if (batch.Skipped > 0) {
        added.push({
            id: id++,
            text: `... ${formatBytes(batch.Skipped)} skipped - the game wrote faster than it can be shown`,
            level: 'info',
            note: true,
        });
    }
    for (const text of batch.Lines ?? []) {
        added.push({id: id++, text, level: classifyGameLogLine(text)});
    }

    const base = batch.Reset ? [] : state.lines;
    let lines = base.length === 0 ? added : base.concat(added);
    if (lines.length > MAX_GAME_LOG_LINES) {
        lines = lines.slice(lines.length - MAX_GAME_LOG_LINES);
    }
    return {
        lines,
        nextId: id,
        missing: batch.Reset ? batch.Missing : false,
    };
}

export type GameLogLevelFilter = 'all' | 'warn' | 'error';

// Whether a line of this level shows under the filter: warnings include the
// errors, as they do in the app's own log.
export function passesLevelFilter(level: GameLogLevel, filter: GameLogLevelFilter): boolean {
    if (filter === 'all') return true;
    if (filter === 'warn') return level === 'warn' || level === 'error';
    return level === 'error';
}

// The log to open first: the game's error log if it has anything in it, else the
// first file that does, else just the first.
export function pickDefaultLog(files: {Name: string; Size: number}[]): string {
    return (files.find((f) => f.Size > 0) ?? files[0])?.Name ?? '';
}

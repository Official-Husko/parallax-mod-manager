import './LogView.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {colorFromName} from '../data/nameColor';
import {EmptyState} from './EmptyState';
import {Select} from './Select';
import {UploadLogButton} from './UploadLogButton';
import {
    durationTone,
    entryText,
    formatLogTime,
    levelRank,
    type LogEntry,
    tokenizeMessage,
    useAppLog,
} from '../data/appLog';
import {useVirtualWindow} from '../data/useVirtualWindow';

// Every line is exactly this tall (see .log-line), which is what lets only the
// visible ones be in the DOM (see useVirtualWindow).
const LINE_HEIGHT = 19;

type LevelFilter = 'all' | 'info' | 'warn' | 'error';

const LEVEL_FILTERS: { key: LevelFilter; label: string; min: number }[] = [
    {key: 'all', label: 'All', min: 0},
    {key: 'info', label: 'Info', min: 1},
    {key: 'warn', label: 'Warnings', min: 2},
    {key: 'error', label: 'Errors', min: 3},
];

function LogLine({entry}: { entry: LogEntry }) {
    return (
        <div className={`log-line ${entry.Level}`} title={entryText(entry)}>
            <span className="log-time">{formatLogTime(entry.Time)}</span>
            <span className="log-comp" style={{color: colorFromName(entry.Component)}}>[{entry.Component}]</span>
            {entry.Level === 'warn' && <span className="log-level">WARN</span>}
            {entry.Level === 'error' && <span className="log-level">ERROR</span>}
            {entry.Level === 'debug' && <span className="log-level">DEBUG</span>}
            <span className="log-msg">
                {tokenizeMessage(entry.Message).map((part, i) => (
                    <span key={i} className={part.kind === 'plain' ? undefined : `log-${part.kind}`}>{part.text}</span>
                ))}
            </span>
            {entry.Timed && <span className={`log-duration dur-${durationTone(entry.DurationMs)}`}>({entry.DurationMs}ms)</span>}
        </div>
    );
}

// The app's activity log: one coloured line per thing the app did, newest at the
// bottom, in the same shape as its log file - "2026/07/06 00:09:37 [Scan] ...
// (12ms)". Follows new lines while you're at the bottom, and stops following the
// moment you scroll up to read something. Anything in the app can add to it (see
// internal/applog, and logEvent for the frontend), so this is the one place to
// look when asking "what did it just do?".
export function LogView({onOpenFolder}: {
    // Present when the log has a file on disk - shows an "Open folder" button.
    onOpenFolder?: () => void;
}) {
    const {entries, clear} = useAppLog();
    const [level, setLevel] = useState<LevelFilter>('info');
    const [component, setComponent] = useState('');
    const [search, setSearch] = useState('');
    const [following, setFollowing] = useState(true);
    const [copied, setCopied] = useState(false);
    const scrollEl = useRef<HTMLDivElement | null>(null);

    const components = useMemo(() => [...new Set(entries.map((e) => e.Component))].sort(), [entries]);

    const componentOptions = useMemo(
        () => [{value: '', label: 'All components'}, ...components.map((c) => ({value: c, label: c}))],
        [components],
    );

    const visible = useMemo(() => {
        const min = LEVEL_FILTERS.find((f) => f.key === level)?.min ?? 0;
        const q = search.trim().toLowerCase();
        return entries.filter((e) => levelRank(e.Level) >= min
            && (!component || e.Component === component)
            && (!q || e.Message.toLowerCase().includes(q) || e.Component.toLowerCase().includes(q)));
    }, [entries, level, component, search]);

    const {ref: windowRef, onScroll: onWindowScroll, first, last} = useVirtualWindow<HTMLDivElement>(visible.length, LINE_HEIGHT, 20);
    const setRefs = (el: HTMLDivElement | null) => {
        scrollEl.current = el;
        windowRef(el);
    };

    // Stick to the bottom while following. Runs after the render that added the
    // lines, so the container is already tall enough to scroll there.
    useEffect(() => {
        const el = scrollEl.current;
        if (following && el) el.scrollTop = el.scrollHeight;
    }, [visible.length, following]);

    function onScroll(e: Event) {
        onWindowScroll(e);
        const el = e.currentTarget as HTMLDivElement;
        const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < LINE_HEIGHT;
        setFollowing((was) => (was === atBottom ? was : atBottom));
    }

    function copyVisible() {
        const text = visible.map(entryText).join('\n');
        navigator.clipboard?.writeText(text).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        }).catch(() => undefined);
    }

    const filtered = visible.length !== entries.length;

    return (
        <div className="log-view">
            <div className="log-toolbar">
                <span className="log-levels">
                    {LEVEL_FILTERS.map((f) => (
                        <span key={f.key} className={`log-level-btn ${f.key} ${level === f.key ? 'active' : ''}`} onClick={() => setLevel(f.key)}>
                            {f.label}
                        </span>
                    ))}
                </span>
                <Select
                    className="log-select"
                    value={component}
                    options={componentOptions}
                    onChange={setComponent}
                />
                <span className="log-search">
                    <i className="fa-solid fa-magnifying-glass"/>
                    <input
                        placeholder="Filter lines..."
                        value={search}
                        onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                    />
                </span>
                <span className="log-spacer"/>
                <span className="log-count">{filtered ? `${visible.length} of ${entries.length}` : entries.length} lines</span>
                <span
                    className={`log-btn ${following ? 'on' : ''}`}
                    title={following ? 'Following new lines - scroll up to stop' : 'Jump to the newest line and follow'}
                    onClick={() => setFollowing(true)}
                >
                    <i className="fa-solid fa-arrow-down-to-line"/> Follow
                </span>
                <span className="log-btn" title="Copy the lines shown, as text" onClick={copyVisible}>
                    <i className={copied ? 'fa-solid fa-check' : 'fa-regular fa-copy'}/> {copied ? 'Copied' : 'Copy'}
                </span>
                {onOpenFolder && (
                    <span className="log-btn" title="Open the folder holding the log file" onClick={onOpenFolder}>
                        <i className="fa-regular fa-folder-open"/> Files
                    </span>
                )}
                {/*
                    Upload / share the log - NOT BUILT YET. It is shown disabled on
                    purpose, to reserve the spot and make the intent visible. There
                    is no handler and nothing behind it; docs/log-sharing.md has the
                    full design. What it should do when it's built:

                    - Never upload on click. It opens a review dialog that shows
                      exactly what would be sent - the log lines as they are in the
                      file, with the home folder path and the user's name redacted -
                      and asks for a confirmation each time.

                    - The dialog offers an OPT-IN option, off by default: "Include
                      computer details". Its purpose is statistics and investigating
                      bugs and other problems faster, and it should show (and let the
                      user read before sending) things like:
                        * CPU (model, threads), GPU (model, driver), RAM
                        * OS (name, version, kernel) and session type (X11/Wayland)
                        * app version and commit, Go and Wails versions, webview
                        * the game and its version
                        * how many mods are installed and enabled, and how many are
                          Workshop versus local
                        * whether a patch has been generated (yes/no), which
                          generation, and whether it is out of date
                        * conflict count, cache figures (files, definitions, files
                          that failed to parse) and scan timings

                    - Never included, with or without that option: mod names, Steam
                      or account IDs, file contents, full paths, or anything typed
                      into the app.

                    To wire it up: give UploadLogButton an onUpload prop (like
                    onOpenFolder here) - the styling for both states exists.
                */}
                <UploadLogButton/>
                <span className="log-btn" title="Empty this view (the log file keeps its lines)" onClick={clear}>
                    <i className="fa-regular fa-trash-can"/> Clear
                </span>
            </div>
            <div className={`log-body ${visible.length === 0 ? 'empty' : ''}`} ref={setRefs} onScroll={onScroll}>
                {visible.length === 0 ? (
                    entries.length === 0
                        ? <EmptyState icon="fa-file-lines" title="Nothing logged yet" subtitle="Activity appears here as the app does things."/>
                        : <EmptyState icon="fa-filter-circle-xmark" title="No lines match these filters" subtitle="Try another level, component or search text."/>
                ) : (
                    <div style={{height: visible.length * LINE_HEIGHT, paddingTop: first * LINE_HEIGHT, boxSizing: 'border-box'}}>
                        {visible.slice(first, last).map((e) => <LogLine key={e.Seq} entry={e}/>)}
                    </div>
                )}
            </div>
        </div>
    );
}

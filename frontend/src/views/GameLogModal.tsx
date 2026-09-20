import '../components/LogView.css';
import './GameLogModal.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {GameLogFiles, OpenPath, StopWatchingGameLog, WatchGameLog} from '../../wailsjs/go/main/App';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import type {gamelog} from '../../wailsjs/go/models';
import {colorFromName} from '../data/nameColor';
import {formatBytes} from '../data/format';
import {
    applyGameLogBatch,
    EMPTY_GAME_LOG,
    type GameLogBatch,
    type GameLogLevelFilter,
    type GameLogLine,
    type GameLogState,
    parseGameLogLine,
    passesLevelFilter,
    pickDefaultLog,
    sourceFile,
} from '../data/gameLog';
import {useVirtualWindow} from '../data/useVirtualWindow';

// Every line is exactly this tall (the same .log-line as the app's own log),
// which is what lets only the visible ones be in the DOM.
const LINE_HEIGHT = 19;

// How often the list of log files is refreshed while the window is open: the
// game creates and rewrites them as it starts.
const LISTING_REFRESH_MS = 3000;

const LEVEL_FILTERS: { key: GameLogLevelFilter; label: string }[] = [
    {key: 'all', label: 'All'},
    {key: 'warn', label: 'Warnings'},
    {key: 'error', label: 'Errors'},
];

function sameListing(a: gamelog.Listing | null, b: gamelog.Listing): boolean {
    if (!a || a.Dir !== b.Dir || a.Files.length !== b.Files.length) return false;
    return a.Files.every((f, i) => f.Name === b.Files[i].Name && f.Size === b.Files[i].Size && f.Modified === b.Files[i].Modified);
}

function GameLogLineRow({line}: { line: GameLogLine }) {
    const parsed = line.note ? null : parseGameLogLine(line.text);
    return (
        <div className={`log-line game-log-line ${line.level} ${line.note ? 'note' : ''}`} title={line.text}>
            {parsed ? (
                <>
                    <span className="log-time">{parsed.time}</span>
                    <span className="log-comp" style={{color: colorFromName(sourceFile(parsed.source))}}>[{parsed.source}]</span>
                    {line.level === 'warn' && <span className="log-level">WARN</span>}
                    {line.level === 'error' && <span className="log-level">ERROR</span>}
                    <span className="log-msg">{parsed.message}</span>
                </>
            ) : (
                <span className="log-msg">{line.text}</span>
            )}
        </div>
    );
}

// The game's own log files, followed live: pick one (error.log first - it is where
// broken mods show up), and new lines appear as the game writes them. This is the
// game's output, not this app's - see the Settings > About page for that.
export function GameLogModal({gameId, gameName, running, onClose}: {
    gameId: string;
    gameName: string;
    // Whether the game is running right now, only to say so - the log is followed
    // either way, since the game may have been started from elsewhere.
    running: boolean;
    onClose: () => void;
}) {
    const [listing, setListing] = useState<gamelog.Listing | null>(null);
    const [file, setFile] = useState('');
    const [log, setLog] = useState<GameLogState>(EMPTY_GAME_LOG);
    const [level, setLevel] = useState<GameLogLevelFilter>('all');
    const [search, setSearch] = useState('');
    const [following, setFollowing] = useState(true);
    const [copied, setCopied] = useState(false);
    const [problem, setProblem] = useState('');
    const scrollEl = useRef<HTMLDivElement | null>(null);
    // The newest watch session seen. Events are tagged with the session that
    // produced them; ones from an older session (the file that was open before
    // this one) can still be in flight after switching, and are ignored.
    const sessionRef = useRef(0);

    // The list of log files, kept fresh.
    useEffect(() => {
        let cancelled = false;
        const load = () => GameLogFiles(gameId)
            .then((next) => {
                if (cancelled) return;
                setProblem('');
                setListing((prev) => (sameListing(prev, next) ? prev : next));
            })
            .catch((err) => { if (!cancelled) setProblem(String(err)); });
        load();
        const timer = window.setInterval(load, LISTING_REFRESH_MS);
        return () => {
            cancelled = true;
            window.clearInterval(timer);
        };
    }, [gameId]);

    // Open something once there is something to open.
    useEffect(() => {
        if (!file && listing && listing.Files.length > 0) {
            setFile(pickDefaultLog(listing.Files));
        }
    }, [listing, file]);

    // Updates from the file being followed.
    useEffect(() => {
        const unsubscribe = EventsOn('game-log', (session: number, batch: GameLogBatch) => {
            if (session < sessionRef.current) return;
            sessionRef.current = session;
            setLog((prev) => applyGameLogBatch(prev, batch));
        });
        return () => {
            unsubscribe();
            StopWatchingGameLog().catch(() => undefined);
        };
    }, []);

    // Follow whichever file is selected.
    useEffect(() => {
        if (!file) return;
        let cancelled = false;
        setLog((prev) => ({...EMPTY_GAME_LOG, nextId: prev.nextId}));
        WatchGameLog(gameId, file)
            .then((session) => {
                if (!cancelled) sessionRef.current = Math.max(sessionRef.current, session);
            })
            .catch((err) => { if (!cancelled) setProblem(String(err)); });
        return () => { cancelled = true; };
    }, [gameId, file]);

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [onClose]);

    const visible = useMemo(() => {
        const q = search.trim().toLowerCase();
        return log.lines.filter((l) => passesLevelFilter(l.level, level) && (!q || l.text.toLowerCase().includes(q)));
    }, [log.lines, level, search]);

    const {ref: windowRef, onScroll: onWindowScroll, first, last} = useVirtualWindow<HTMLDivElement>(visible.length, LINE_HEIGHT, 20);
    const setRefs = (el: HTMLDivElement | null) => {
        scrollEl.current = el;
        windowRef(el);
    };

    // Stick to the bottom while following.
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
        navigator.clipboard?.writeText(visible.filter((l) => !l.note).map((l) => l.text).join('\n')).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        }).catch(() => undefined);
    }

    const files = listing?.Files ?? [];
    const filtered = visible.length !== log.lines.length;

    let emptyText = '';
    if (visible.length === 0) {
        if (problem) emptyText = problem;
        else if (listing && files.length === 0) emptyText = `No game logs yet - ${gameName} writes them when it starts, and they appear here as it does.`;
        else if (log.missing) emptyText = `${file} doesn't exist yet - the game creates it when it starts.`;
        else if (log.lines.length > 0) emptyText = 'No lines match these filters.';
        else emptyText = file ? 'This log is empty so far.' : 'Loading...';
    }

    return (
        <div className="overlay" onClick={onClose}>
            <div className="game-log-modal" onClick={(e) => e.stopPropagation()}>
                <div className="game-log-header">
                    <span className="title">Game log</span>
                    <span className="mono meta">{gameName}</span>
                    <span className={`game-log-live ${running ? 'on' : ''}`} title={running ? 'The game is running - new lines appear as it writes them' : 'The game is not running - new lines appear if it starts'}>
                        <i className="fa-solid fa-circle"/> {running ? 'Game running' : 'Game not running'}
                    </span>
                    <div className="spacer"/>
                    <i className="fa-solid fa-xmark close-btn" title="Close (Esc)" onClick={onClose}/>
                </div>
                <div className="log-view">
                    <div className="log-toolbar">
                        <select
                            className="log-select game-log-file"
                            value={file}
                            disabled={files.length === 0}
                            onChange={(e) => { setFile((e.target as HTMLSelectElement).value); setFollowing(true); }}
                        >
                            {files.length === 0 && <option value="">No logs yet</option>}
                            {files.map((f) => <option key={f.Name} value={f.Name}>{f.Name} ({formatBytes(f.Size)})</option>)}
                        </select>
                        <span className="log-levels">
                            {LEVEL_FILTERS.map((f) => (
                                <span key={f.key} className={`log-level-btn ${level === f.key ? 'active' : ''}`} onClick={() => setLevel(f.key)}>
                                    {f.label}
                                </span>
                            ))}
                        </span>
                        <span className="log-search">
                            <i className="fa-solid fa-magnifying-glass"/>
                            <input
                                placeholder="Filter lines..."
                                value={search}
                                onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                            />
                        </span>
                        <span className="log-spacer"/>
                        <span className="log-count">{filtered ? `${visible.length} of ${log.lines.length}` : log.lines.length} lines</span>
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
                        {listing?.Dir && (
                            <span className="log-btn" title="Open the folder holding the game's log files" onClick={() => OpenPath(listing.Dir).catch(() => undefined)}>
                                <i className="fa-regular fa-folder-open"/> Files
                            </span>
                        )}
                        <span
                            className="log-btn"
                            title="Empty this view (the file keeps its lines; new ones still arrive)"
                            onClick={() => setLog((prev) => ({...prev, lines: []}))}
                        >
                            <i className="fa-regular fa-trash-can"/> Clear
                        </span>
                    </div>
                    <div className="log-body" ref={setRefs} onScroll={onScroll}>
                        {visible.length === 0 ? (
                            <div className="log-empty">{emptyText}</div>
                        ) : (
                            <div style={{height: visible.length * LINE_HEIGHT, paddingTop: first * LINE_HEIGHT, boxSizing: 'border-box'}}>
                                {visible.slice(first, last).map((l) => <GameLogLineRow key={l.id} line={l}/>)}
                            </div>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
}

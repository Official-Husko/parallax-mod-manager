import {Fragment, h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {CheckMod, OpenModFolder, RebuildBaseGameIndex} from '../../wailsjs/go/main/App';
import type {app, library, modcheck} from '../../wailsjs/go/models';
import {getSessionStats, recordCheck} from '../data/checksSessionStats';
import {formatBytes} from '../data/format';
import {notify} from '../data/notifications';

// The Checks tab: problems worth knowing about before publishing or sharing a mod - files and
// script keys it overwrites in the base game, syntax errors in its own script files, a
// descriptor that won't parse right, a dependency that can't be found. See internal/modcheck.
// Checking a mod's files (and, the first time for a game, the whole base game) is real work, so
// it runs on request rather than automatically every time this tab opens.
//
// FINDINGS supports grouping by Category/File/Severity, a Grouped/List toggle, a text filter, and
// Errors/Warnings toggle-chips. CHECKS' own four category rows are two lines each (a bold
// category name over a lighter status detail), a colored bar carrying that category's fixed
// identity color, and a separate severity glyph ("!"/"x", never the category's own color) -
// distinct signals kept apart on purpose, not merged into one. They link into FINDINGS via a
// "Show" jump. RESULT CACHE surfaces real numbers (this mod's own last run, a running session
// total, the base game's cached index) rather than being purely a findings viewer.

type CategoryKey = 'base_game' | 'syntax' | 'descriptor' | 'dependency';
type Status = 'idle' | 'skipped' | 'warn' | 'good';
type GroupBy = 'category' | 'file' | 'severity';
type ViewMode = 'grouped' | 'list';

const CATEGORIES: { key: CategoryKey; label: string; icon: string; color: string; detail: string; text: (status: Status, findings: modcheck.Finding[]) => string }[] = [
    {
        key: 'base_game',
        label: 'Base game conflicts',
        icon: 'fa-chess-board',
        // Blue - the category's own fixed identity color (shown on CHECKS' left bar and
        // FINDINGS' group dot regardless of status), distinct from the amber/red severity
        // glyph next to it. Not var(--amber): that's a status color, this is an identity one.
        color: 'var(--blue)',
        detail: 'Files and script keys this mod overwrites that belong to the game itself, not another mod - the base-game half of what the Conflict Resolver finds between mods.',
        text: (status, findings) => {
            if (status === 'idle') return 'Not checked yet';
            if (status === 'skipped') return "Couldn't check - the game wasn't found";
            const n = findings.length;
            if (status === 'warn') return `Overwrites ${n} file${n === 1 ? '' : 's'} from the game itself`;
            return 'No conflicts with the base game';
        },
    },
    {
        key: 'syntax',
        label: 'Syntax',
        color: 'var(--red)',
        icon: 'fa-code',
        detail: "This mod's own Clausewitz script files that fail to parse: which file, which line, and what looked wrong.",
        text: (status, findings) => {
            if (status === 'idle') return 'Not checked yet';
            if (status === 'warn') {
                const n = findings.length;
                const fileCount = new Set(findings.map((f) => f.File)).size;
                return `${n} parse error${n === 1 ? '' : 's'} in ${fileCount} file${fileCount === 1 ? '' : 's'}`;
            }
            return 'No syntax errors';
        },
    },
    {
        key: 'descriptor',
        label: 'Descriptor',
        icon: 'fa-file-circle-exclamation',
        color: 'var(--amber)',
        detail: 'Things wrong with the descriptor itself: no supported_version, a version that is not shaped like the game expects, a picture= that points at a file that does not exist.',
        text: (status, findings) => {
            if (status === 'idle') return 'Not checked yet';
            const n = findings.length;
            if (status === 'warn') return `${n} problem${n === 1 ? '' : 's'} in descriptor.mod`;
            return 'Descriptor looks fine';
        },
    },
    {
        key: 'dependency',
        label: 'Dependencies',
        color: 'var(--flag-dependency)',
        icon: 'fa-link-slash',
        detail: "A dependency this mod declares that matches no mod you have installed - the same matching the Editor's own dependency field already flags as you type.",
        text: (status, findings) => {
            if (status === 'idle') return 'Not checked yet';
            const n = findings.length;
            if (status === 'warn') return `${n} declared dependenc${n === 1 ? 'y' : 'ies'} not installed`;
            return 'Every dependency is installed';
        },
    },
];

// checkGlyph is CHECKS' own per-row status glyph: a plain character, not an icon font, colored
// by severity rather than category - green check once clean, dim dash before checked (or when
// base_game's own game wasn't found), otherwise "!" in amber except syntax, which - being a
// parse failure, not a maybe - gets the same red EditorEdit's own Problems list uses for a hard
// error. This is deliberately a different signal than the row's own colored bar (checkColor
// below), which stays that category's fixed identity color no matter its status.
function checkGlyph(category: { key: CategoryKey }, status: Status): {glyph: string; color: string} {
    if (status === 'good') return {glyph: '✓', color: 'var(--green)'};
    if (status === 'idle' || status === 'skipped') return {glyph: '-', color: 'var(--text-dim)'};
    return category.key === 'syntax' ? {glyph: '✕', color: 'var(--red)'} : {glyph: '!', color: 'var(--amber)'};
}

const SEVERITY_LABEL: Record<string, string> = {error: 'Error', warn: 'Warning', info: 'Info'};
const SEVERITY_GROUPS: {key: string; label: string; color: string}[] = [
    {key: 'error', label: 'Errors', color: 'var(--red)'},
    {key: 'warn', label: 'Warnings', color: 'var(--amber)'},
    {key: 'info', label: 'Info', color: 'var(--blue)'},
];

function categoryOf(key: string) {
    return CATEGORIES.find((c) => c.key === key);
}

interface FindingGroup {
    key: string;
    label: string;
    color: string;
    findings: modcheck.Finding[];
}

// A flattened, keyboard-navigable view of whatever FINDINGS is currently showing - group
// headers, shown findings, and "show more" rows, in the same order they render in. Arrow keys
// move a focus cursor over this list (see the findings-list's own onKeyDown), independent of
// which view/grouping produced it.
type NavItem =
    | {kind: 'group'; group: FindingGroup}
    | {kind: 'finding'; finding: modcheck.Finding}
    | {kind: 'more'; group: FindingGroup; more: number};

function buildGroups(findings: modcheck.Finding[], group: GroupBy): FindingGroup[] {
    if (group === 'category') {
        return CATEGORIES
            .map((c) => ({key: c.key, label: c.label, color: c.color, findings: findings.filter((f) => f.Category === c.key)}))
            .filter((g) => g.findings.length > 0);
    }
    if (group === 'severity') {
        return SEVERITY_GROUPS
            .map((s) => ({key: s.key, label: s.label, color: s.color, findings: findings.filter((f) => f.Severity === s.key)}))
            .filter((g) => g.findings.length > 0);
    }
    const byFile = new Map<string, modcheck.Finding[]>();
    for (const f of findings) {
        const key = f.File || '(not tied to one file)';
        const list = byFile.get(key);
        if (list) list.push(f); else byFile.set(key, [f]);
    }
    return Array.from(byFile.entries())
        .sort((a, b) => a[0].localeCompare(b[0]))
        .map(([key, list]) => ({key, label: key, color: 'var(--text-dim)', findings: list}));
}

const GROUP_SHOW_CAP = 6;

export function EditorChecks({gameId, gameName, gameVersion, mod, installedNames, initialResult, onResult}: {
    gameId: string;
    // The game's own display name and installed version - "Stellaris" and "4.0.21", say - for
    // RUN's own sentence and RESULT CACHE's base-game-index stat, both of which name the exact
    // game/version being compared against rather than speaking generically.
    gameName: string;
    gameVersion: string;
    mod: library.ModSummary;
    // Every other installed mod's name, for the dependency check - the same list the Edit tab
    // already gets from the Editor's own mod list.
    installedNames: string[];
    // A cached result from earlier this session, if this mod has one - see Editor.tsx's own
    // checkResults map. Editor.tsx also gives this component a key={mod.ID}, so a mod switch
    // remounts it fresh rather than needing an effect here to reset stale local state; the
    // result itself survives that remount only because it is re-supplied right back through
    // this same prop.
    initialResult: app.CheckResult | null;
    // Reports every new result (or its clearing) so Editor.tsx can cache it per mod - called
    // right alongside setResult below, never on its own.
    onResult: (result: app.CheckResult | null) => void;
}) {
    const [result, setResultState] = useState<app.CheckResult | null>(initialResult);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [rebuilding, setRebuilding] = useState(false);

    const [group, setGroup] = useState<GroupBy>('category');
    const [view, setView] = useState<ViewMode>('grouped');
    const [filterText, setFilterText] = useState('');
    const [severityFilter, setSeverityFilter] = useState<Set<string>>(new Set());
    const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

    function setResult(r: app.CheckResult | null) {
        setResultState(r);
        onResult(r);
    }

    function run() {
        setBusy(true);
        setError('');
        CheckMod(gameId, mod.ID, installedNames)
            .then((r) => {
                setResult(r);
                recordCheck(mod.ID, r.ResultBytes, r.DurationMS, r.IndexBuilt);
            })
            .catch((err) => setError(String(err)))
            .finally(() => setBusy(false));
    }

    function clearResults() {
        setResult(null);
        setCollapsed(new Set());
    }

    async function rebuildBaseIndex() {
        setRebuilding(true);
        try {
            await RebuildBaseGameIndex(gameId);
            notify('success', "The base game's cached index will be rebuilt on the next check.");
        } catch (err) {
            notify('error', String(err));
        } finally {
            setRebuilding(false);
        }
    }

    const allFindings = result?.Findings ?? [];
    const findingsFor = (key: CategoryKey) => allFindings.filter((f) => f.Category === key);
    const countFor = (key: CategoryKey) => findingsFor(key).length;
    const errorCount = allFindings.filter((f) => f.Severity === 'error').length;
    const warnCount = allFindings.filter((f) => f.Severity === 'warn').length;

    function statusOf(key: CategoryKey): Status {
        if (!result) return 'idle';
        if (key === 'base_game' && !result.BaseGameChecked) return 'skipped';
        return countFor(key) > 0 ? 'warn' : 'good';
    }

    // Deferred to a timeout so it runs after the group/collapsed state above has actually
    // re-rendered - flatNavRef (kept fresh every render, just below) is what lets this see
    // that new render's own flatNav rather than the one from this call's own stale closure.
    function showCategory(key: CategoryKey) {
        setGroup('category');
        setCollapsed((prev) => { const next = new Set(prev); next.delete(key); return next; });
        window.setTimeout(() => {
            const idx = flatNavRef.current.findIndex((item) => item.kind === 'group' && item.group.key === key);
            if (idx >= 0) setFocusedNav(idx);
        }, 0);
    }

    const filtered = useMemo(() => {
        const q = filterText.trim().toLowerCase();
        return allFindings.filter((f) => {
            if (severityFilter.size > 0 && !severityFilter.has(f.Severity)) return false;
            if (q && !f.Message.toLowerCase().includes(q) && !f.File.toLowerCase().includes(q)) return false;
            return true;
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [allFindings, filterText, severityFilter]);

    const groups = useMemo(() => buildGroups(filtered, group), [filtered, group]);

    function toggleSeverity(sev: string) {
        setSeverityFilter((prev) => {
            const next = new Set(prev);
            if (next.has(sev)) next.delete(sev); else next.add(sev);
            return next;
        });
    }

    function toggleGroupCollapsed(key: string) {
        setCollapsed((prev) => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key); else next.add(key);
            return next;
        });
    }

    const flatNav = useMemo<NavItem[]>(() => {
        if (view === 'list') return filtered.map((f) => ({kind: 'finding', finding: f}));
        const items: NavItem[] = [];
        for (const g of groups) {
            items.push({kind: 'group', group: g});
            if (!collapsed.has(g.key)) {
                const shown = g.findings.slice(0, GROUP_SHOW_CAP);
                for (const f of shown) items.push({kind: 'finding', finding: f});
                const more = g.findings.length - shown.length;
                if (more > 0) items.push({kind: 'more', group: g, more});
            }
        }
        return items;
    }, [view, filtered, groups, collapsed]);
    const flatNavRef = useRef<NavItem[]>([]);
    flatNavRef.current = flatNav;

    const [focusedNav, setFocusedNav] = useState(-1);
    const navRefs = useRef<Map<number, HTMLDivElement>>(new Map());

    useEffect(() => {
        if (focusedNav < 0) return;
        if (focusedNav >= flatNav.length) { setFocusedNav(flatNav.length - 1); return; }
        navRefs.current.get(focusedNav)?.scrollIntoView({block: 'nearest'});
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [focusedNav, flatNav.length]);

    // Keyboard navigation over the findings list: up/down moves a focus cursor across group
    // headers, findings and "show more" rows alike; right expands a collapsed group under the
    // cursor; Enter opens a focused finding's mod folder, expands a focused group, or reveals a
    // focused "show more" row's rest - one flat model behind all three, rather than separate
    // per-row-kind handlers.
    function handleNavKeyDown(e: KeyboardEvent) {
        if (flatNav.length === 0) return;
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            setFocusedNav((i) => Math.min(flatNav.length - 1, i + 1));
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            setFocusedNav((i) => (i < 0 ? flatNav.length - 1 : Math.max(0, i - 1)));
        } else if (e.key === 'ArrowRight' || e.key === 'Enter') {
            const item = focusedNav >= 0 ? flatNav[focusedNav] : undefined;
            if (!item) return;
            e.preventDefault();
            if (item.kind === 'group') {
                if (e.key === 'Enter' || collapsed.has(item.group.key)) toggleGroupCollapsed(item.group.key);
            } else if (item.kind === 'more' && e.key === 'Enter') {
                setCollapsed((prev) => { const next = new Set(prev); next.delete(item.group.key); return next; });
            } else if (item.kind === 'finding' && e.key === 'Enter' && item.finding.File) {
                OpenModFolder(gameId, mod.ID).catch(() => undefined);
            }
        }
    }

    const session = getSessionStats();
    const totalIndexBytes = (result?.BaseGameIndexBytes ?? 0) + (result?.ResultBytes ?? 0);
    const indexShare = totalIndexBytes > 0 ? ((result?.BaseGameIndexBytes ?? 0) / totalIndexBytes) * 100 : 0;

    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-card">
                    <div className="editor-card-title">RUN</div>
                    <p className="editor-muted">
                        Reads this mod's files and compares them with {gameVersion ? `${gameName} ${gameVersion}` : 'the installed game'}.
                        {' '}The base game is indexed once, the first time you run checks for it.
                    </p>
                    <div className="editor-actions">
                        <button type="button" className="btn-primary" disabled={busy} onClick={run}>
                            {busy ? 'Checking...' : result ? 'Run checks again' : 'Check now'}
                        </button>
                        {result && result.RanAt > 0 && (
                            <span className="editor-hint mono">
                                last run {new Date(result.RanAt * 1000).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})}
                                {' '}&middot; {(result.DurationMS / 1000).toFixed(1)} s
                            </span>
                        )}
                    </div>
                    {error && (
                        <div className="editor-alert bad">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{error}</div></div>
                        </div>
                    )}
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">
                        CHECKS<span className="editor-card-count mono">{CATEGORIES.length}</span>
                    </div>
                    <div className="check-list">
                        {CATEGORIES.map((c) => {
                            const status = statusOf(c.key);
                            const g = checkGlyph(c, status);
                            return (
                                <div key={c.key} className="check-row" title={c.detail}>
                                    <span className="check-row-bar" style={{background: c.color}}/>
                                    <span className="check-row-glyph" style={{color: g.color}}>{g.glyph}</span>
                                    <div className="check-row-body">
                                        <div className="check-row-name">{c.label}</div>
                                        <div className="check-row-detail">{c.text(status, findingsFor(c.key))}</div>
                                    </div>
                                    {status === 'warn' && <span className="check-row-show" onClick={() => showCategory(c.key)}>Show</span>}
                                </div>
                            );
                        })}
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">RESULT CACHE</div>
                    <div className="result-cache-grid">
                        <div className="result-cache-stat">
                            <span className="result-cache-stat-label">This mod</span>
                            <span className="result-cache-stat-value">{result ? formatBytes(result.ResultBytes) : '-'}</span>
                            <span className="result-cache-stat-sub">
                                {result ? `${result.Findings.length} finding${result.Findings.length === 1 ? '' : 's'} · ${result.FilesRead} file${result.FilesRead === 1 ? '' : 's'} read` : 'Not checked yet'}
                            </span>
                        </div>
                        <div className="result-cache-stat">
                            <span className="result-cache-stat-label">This session</span>
                            <span className="result-cache-stat-value">{formatBytes(session.totalBytes)}</span>
                            <span className="result-cache-stat-sub">{session.modsChecked} mod{session.modsChecked === 1 ? '' : 's'} checked</span>
                        </div>
                        <div className="result-cache-stat">
                            <span className="result-cache-stat-label">Base game index</span>
                            <span className="result-cache-stat-value">{result && result.BaseGameIndexBytes > 0 ? formatBytes(result.BaseGameIndexBytes) : '-'}</span>
                            <span className="result-cache-stat-sub">
                                {result && result.BaseGameIndexFiles > 0
                                    ? `${result.BaseGameIndexFiles.toLocaleString()} files${gameVersion ? ` · ${gameName} ${gameVersion}` : ''}`
                                    : result?.BaseGameChecked === false ? "Game wasn't found" : 'Not indexed yet'}
                            </span>
                        </div>
                        <div className="result-cache-stat">
                            <span className="result-cache-stat-label">Last run</span>
                            <span className="result-cache-stat-value">{result && result.RanAt > 0 ? `${(result.DurationMS / 1000).toFixed(1)} s` : '-'}</span>
                            <span className="result-cache-stat-sub">
                                {session.firstIndexDurationMs !== null && session.firstIndexDurationMs !== result?.DurationMS
                                    ? `first run with indexing ${(session.firstIndexDurationMs / 1000).toFixed(1)} s`
                                    : ' '}
                            </span>
                        </div>
                    </div>
                    {totalIndexBytes > 0 && (
                        <Fragment>
                            <div className="result-cache-bar">
                                <div style={{width: `${indexShare}%`, background: 'var(--blue)'}}/>
                                <div style={{width: `${100 - indexShare}%`, background: 'var(--rust)'}}/>
                            </div>
                            <div className="result-cache-legend">
                                <span className="result-cache-legend-dot"><span style={{background: 'var(--blue)'}}/> Base game index</span>
                                <span className="result-cache-legend-dot"><span style={{background: 'var(--rust)'}}/> Mod results</span>
                                <span className="editor-card-title-spacer"/>
                                <span className="mono">{formatBytes(totalIndexBytes)} total</span>
                            </div>
                        </Fragment>
                    )}
                    <p className="editor-muted">
                        Mod results clear when any mod changes on disk. The base game index is kept on disk and
                        rebuilt when the game updates.
                    </p>
                    <div className="editor-actions">
                        <button type="button" className="btn-ghost" disabled={!result} onClick={clearResults}>Clear results</button>
                        <button type="button" className="btn-ghost" disabled={rebuilding} onClick={rebuildBaseIndex}>
                            {rebuilding ? 'Rebuilding...' : 'Rebuild base index'}
                        </button>
                    </div>
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title-row">
                        <div className="editor-card-title">
                            FINDINGS{allFindings.length > 0 && <span className="editor-card-count mono">{allFindings.length}</span>}
                        </div>
                        <span className="editor-card-title-spacer"/>
                        {allFindings.length > 0 && (
                            <Fragment>
                                <span className="editor-card-title-action">Group</span>
                                <div className="findings-toolbar-group">
                                    {(['category', 'file', 'severity'] as GroupBy[]).map((g) => (
                                        <span key={g} className={group === g ? 'active' : ''} onClick={() => setGroup(g)}>
                                            {g === 'category' ? 'Category' : g === 'file' ? 'File' : 'Severity'}
                                        </span>
                                    ))}
                                </div>
                                <div className="findings-toolbar-group">
                                    <span className={view === 'grouped' ? 'active' : ''} onClick={() => setView('grouped')}>&#9638; Grouped</span>
                                    <span className={view === 'list' ? 'active' : ''} onClick={() => setView('list')}>&#9776; List</span>
                                </div>
                            </Fragment>
                        )}
                    </div>

                    {!result && !busy && <p className="editor-muted">Nothing has been checked yet. Click Check now.</p>}
                    {busy && <p className="editor-muted">Checking...</p>}
                    {result && allFindings.length === 0 && <p className="editor-muted">No problems found.</p>}

                    {allFindings.length > 0 && (
                        <Fragment>
                            <div className="findings-filter-row">
                                <div className="findings-filter-box">
                                    <i className="fa-solid fa-magnifying-glass"/>
                                    <input placeholder="Filter by message or path..." value={filterText} onInput={(e) => setFilterText((e.target as HTMLInputElement).value)}/>
                                </div>
                                {errorCount > 0 && (
                                    <span
                                        className={`chip ${severityFilter.has('error') ? 'chip-active' : ''}`}
                                        style={severityFilter.has('error') ? {cursor: 'pointer'} : {cursor: 'pointer', color: 'var(--red)', borderColor: 'var(--red)', background: 'rgba(212, 87, 78, .08)'}}
                                        onClick={() => toggleSeverity('error')}
                                    >
                                        &#10007; Errors {errorCount}
                                    </span>
                                )}
                                {warnCount > 0 && (
                                    <span
                                        className={`chip ${severityFilter.has('warn') ? 'chip-active' : ''}`}
                                        style={severityFilter.has('warn') ? {cursor: 'pointer'} : {cursor: 'pointer', color: 'var(--amber)', borderColor: 'var(--amber)', background: 'rgba(224, 163, 64, .08)'}}
                                        onClick={() => toggleSeverity('warn')}
                                    >
                                        ! Warnings {warnCount}
                                    </span>
                                )}
                                {view === 'grouped' && (
                                    <span className="findings-collapse-all" onClick={() => setCollapsed(new Set(groups.map((g) => g.key)))}>Collapse all</span>
                                )}
                            </div>

                            <div className="findings-list" tabIndex={0} onKeyDown={handleNavKeyDown}>
                                {flatNav.map((item, i) => {
                                    const focused = i === focusedNav;
                                    const setRef = (el: HTMLDivElement | null) => {
                                        if (el) navRefs.current.set(i, el); else navRefs.current.delete(i);
                                    };
                                    if (item.kind === 'group') {
                                        const isCollapsed = collapsed.has(item.group.key);
                                        return (
                                            <div
                                                key={`g-${item.group.key}`}
                                                className={`findings-group-head${focused ? ' focused' : ''}`}
                                                ref={setRef}
                                                onClick={() => toggleGroupCollapsed(item.group.key)}
                                            >
                                                <span className="chevron">{isCollapsed ? '▸' : '▾'}</span>
                                                <span className="findings-group-dot" style={{background: item.group.color}}/>
                                                <span className="findings-group-name">{item.group.label}</span>
                                                <span className="findings-group-count">{item.group.findings.length}</span>
                                            </div>
                                        );
                                    }
                                    if (item.kind === 'more') {
                                        return (
                                            <div
                                                key={`m-${item.group.key}`}
                                                ref={setRef}
                                                className={`findings-group-more${focused ? ' focused' : ''}`}
                                                onClick={() => setCollapsed((prev) => { const next = new Set(prev); next.delete(item.group.key); return next; })}
                                            >
                                                Show {item.more} more in {item.group.label}
                                            </div>
                                        );
                                    }
                                    return <FindingRow key={`f-${i}`} f={item.finding} gameId={gameId} modId={mod.ID} focused={focused} innerRef={setRef}/>;
                                })}
                            </div>

                            <div className="findings-footer">
                                <span>{filtered.length} finding{filtered.length === 1 ? '' : 's'}{view === 'grouped' ? ` · ${groups.length} group${groups.length === 1 ? '' : 's'}` : ''}</span>
                                <span className="editor-card-title-spacer"/>
                                <span>&uarr;&darr; move &middot; &rarr; expand &middot; Enter open</span>
                            </div>
                        </Fragment>
                    )}
                </div>
            </div>
        </div>
    );
}

// SEVERITY_GLYPH is the same plain-character idiom checkGlyph uses above for CHECKS' own rows -
// a severity signal, not a category one, kept separate from the row's border-left (the
// category's fixed identity color, from categoryOf below).
const SEVERITY_GLYPH: Record<string, {glyph: string; color: string}> = {
    error: {glyph: '✕', color: 'var(--red)'},
    warn: {glyph: '!', color: 'var(--amber)'},
    info: {glyph: 'i', color: 'var(--blue)'},
};

function FindingRow({f, gameId, modId, focused, innerRef}: {
    f: modcheck.Finding;
    gameId: string;
    modId: string;
    focused?: boolean;
    innerRef?: (el: HTMLDivElement | null) => void;
}) {
    const cat = categoryOf(f.Category);
    const color = cat?.color ?? 'var(--text-dim)';
    const sev = SEVERITY_GLYPH[f.Severity] ?? SEVERITY_GLYPH.info;
    return (
        <div className={`editor-finding-row${focused ? ' focused' : ''}`} style={{borderLeftColor: color}} ref={innerRef}>
            <span title={SEVERITY_LABEL[f.Severity] ?? 'Info'} className="editor-finding-glyph" style={{color: sev.color}}>{sev.glyph}</span>
            <div className="editor-finding-body">
                <span className="editor-finding-message">{f.Message}</span>
                <span className="mono editor-finding-file">{f.File}{f.Line ? `:${f.Line}` : ''}</span>
            </div>
            {f.File && (
                <span
                    className="editor-finding-open"
                    title="Open this mod's folder"
                    onClick={() => OpenModFolder(gameId, modId).catch(() => undefined)}
                >
                    Open &#8599;
                </span>
            )}
        </div>
    );
}

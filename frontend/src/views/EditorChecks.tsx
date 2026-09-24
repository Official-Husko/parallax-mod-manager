import {Fragment, h} from 'preact';
import {useMemo, useRef, useState} from 'preact/hooks';
import {CheckMod, RebuildBaseGameIndex} from '../../wailsjs/go/main/App';
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
// Errors/Warnings toggle-chips - CHECKS' own four category rows keep the older plain-list idiom
// (one line per category, a colored left bar for its identity) but now link into FINDINGS via a
// "Show" jump. RESULT CACHE surfaces real numbers (this mod's own last run, a running session
// total, the base game's cached index) rather than being purely a findings viewer.

type CategoryKey = 'base_game' | 'syntax' | 'descriptor' | 'dependency';
type Status = 'idle' | 'skipped' | 'warn' | 'good';
type GroupBy = 'category' | 'file' | 'severity';
type ViewMode = 'grouped' | 'list';

const CATEGORIES: { key: CategoryKey; label: string; icon: string; color: string; detail: string; text: (status: Status, n: number) => string }[] = [
    {
        key: 'base_game',
        label: 'Base game conflicts',
        icon: 'fa-chess-board',
        color: 'var(--amber)',
        detail: 'Files and script keys this mod overwrites that belong to the game itself, not another mod - the base-game half of what the Conflict Resolver finds between mods.',
        text: (status, n) => {
            if (status === 'idle') return 'Conflicts with the base game - not checked yet';
            if (status === 'skipped') return "Conflicts with the base game - couldn't check, the game wasn't found";
            if (status === 'warn') return `Overwrites ${n} thing${n === 1 ? '' : 's'} from the base game`;
            return 'No conflicts with the base game';
        },
    },
    {
        key: 'syntax',
        label: 'Syntax',
        // A syntax problem is a parse failure, not a maybe - it gets the same red EditorEdit's
        // own Problems list uses for a hard error, not the amber the other categories share.
        color: 'var(--red)',
        icon: 'fa-code',
        detail: "This mod's own Clausewitz script files that fail to parse: which file, which line, and what looked wrong.",
        text: (status, n) => {
            if (status === 'idle') return 'Syntax errors - not checked yet';
            if (status === 'warn') return `${n} syntax problem${n === 1 ? '' : 's'} found`;
            return 'No syntax errors';
        },
    },
    {
        key: 'descriptor',
        label: 'Descriptor',
        icon: 'fa-file-circle-exclamation',
        color: 'var(--blue)',
        detail: 'Things wrong with the descriptor itself: no supported_version, a version that is not shaped like the game expects, a picture= that points at a file that does not exist.',
        text: (status, n) => {
            if (status === 'idle') return 'Descriptor problems - not checked yet';
            if (status === 'warn') return `${n} descriptor problem${n === 1 ? '' : 's'} found`;
            return 'Descriptor looks fine';
        },
    },
    {
        key: 'dependency',
        label: 'Dependencies',
        // Same purple Workspace's own FLAGS column uses for a dependency issue there
        // (--flag-dependency) - one color for "a dependency problem" everywhere in the app.
        color: 'var(--flag-dependency)',
        icon: 'fa-link-slash',
        detail: "A dependency this mod declares that matches no mod you have installed - the same matching the Editor's own dependency field already flags as you type.",
        text: (status, n) => {
            if (status === 'idle') return 'Dependencies - not checked yet';
            if (status === 'warn') return `${n} dependency${n === 1 ? '' : ' dependencies'} not found`;
            return 'Every dependency is installed';
        },
    },
];

const SEVERITY_LABEL: Record<string, string> = {error: 'Error', warn: 'Warning', info: 'Info'};
const SEVERITY_GROUPS: {key: string; label: string; color: string}[] = [
    {key: 'error', label: 'Errors', color: 'var(--red)'},
    {key: 'warn', label: 'Warnings', color: 'var(--amber)'},
    {key: 'info', label: 'Info', color: 'var(--blue)'},
];

// checkColor is CHECKS' own row color: green once a category comes back clean, dim before it's
// checked (or, for base_game, when the game couldn't be found), otherwise that category's own
// color (CATEGORIES[].color) - never a generic "problem" amber for every category alike.
function checkColor(category: { color: string }, status: Status): string {
    if (status === 'good') return 'var(--green)';
    if (status === 'idle' || status === 'skipped') return 'var(--text-dim)';
    return category.color;
}

function categoryOf(key: string) {
    return CATEGORIES.find((c) => c.key === key);
}

interface FindingGroup {
    key: string;
    label: string;
    color: string;
    findings: modcheck.Finding[];
}

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

export function EditorChecks({gameId, mod, installedNames, initialResult, onResult}: {
    gameId: string;
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
    const groupRefs = useRef<Map<string, HTMLDivElement>>(new Map());

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
                recordCheck(mod.ID, r.ResultBytes);
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
    const countFor = (key: CategoryKey) => allFindings.filter((f) => f.Category === key).length;
    const errorCount = allFindings.filter((f) => f.Severity === 'error').length;
    const warnCount = allFindings.filter((f) => f.Severity === 'warn').length;

    function statusOf(key: CategoryKey): Status {
        if (!result) return 'idle';
        if (key === 'base_game' && !result.BaseGameChecked) return 'skipped';
        return countFor(key) > 0 ? 'warn' : 'good';
    }

    function showCategory(key: CategoryKey) {
        setGroup('category');
        setCollapsed((prev) => { const next = new Set(prev); next.delete(key); return next; });
        window.setTimeout(() => groupRefs.current.get(key)?.scrollIntoView({block: 'nearest'}), 0);
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

    const session = getSessionStats();
    const totalIndexBytes = (result?.BaseGameIndexBytes ?? 0) + (result?.ResultBytes ?? 0);
    const indexShare = totalIndexBytes > 0 ? ((result?.BaseGameIndexBytes ?? 0) / totalIndexBytes) * 100 : 0;

    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-card">
                    <div className="editor-card-title">RUN</div>
                    <p className="editor-muted">
                        Reads this mod's files and compares them with the installed game. The base game is indexed
                        once, the first time you run checks for it.
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
                            <i className="fa-solid fa-circle-xmark editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{error}</div></div>
                        </div>
                    )}
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">
                        CHECKS{allFindings.length > 0 && <span className="editor-card-count mono"> &middot; {CATEGORIES.filter((c) => countFor(c.key) > 0).length}</span>}
                    </div>
                    <div className="check-list">
                        {CATEGORIES.map((c) => {
                            const status = statusOf(c.key);
                            return (
                                <div key={c.key} className="check-row" title={c.detail} style={{borderLeftColor: checkColor(c, status)}}>
                                    <i className={`fa-duotone ${c.icon}`} style={{color: checkColor(c, status)}}/>
                                    <span className="check-row-text">{c.text(status, countFor(c.key))}</span>
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
                                {result && result.BaseGameIndexFiles > 0 ? `${result.BaseGameIndexFiles.toLocaleString()} files` : result?.BaseGameChecked === false ? "Game wasn't found" : 'Not indexed yet'}
                            </span>
                        </div>
                        <div className="result-cache-stat">
                            <span className="result-cache-stat-label">Last run</span>
                            <span className="result-cache-stat-value">{result && result.RanAt > 0 ? `${(result.DurationMS / 1000).toFixed(1)} s` : '-'}</span>
                            <span className="result-cache-stat-sub">&nbsp;</span>
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
                            FINDINGS{allFindings.length > 0 && <span className="editor-card-count mono"> &middot; {allFindings.length}</span>}
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
                                    <span className={view === 'grouped' ? 'active' : ''} onClick={() => setView('grouped')}><i className="fa-solid fa-table-cells-large"/> Grouped</span>
                                    <span className={view === 'list' ? 'active' : ''} onClick={() => setView('list')}><i className="fa-solid fa-bars"/> List</span>
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
                                    <span className={`chip ${severityFilter.has('error') ? 'chip-active' : ''}`} style={{cursor: 'pointer', color: severityFilter.has('error') ? undefined : 'var(--red)', borderColor: 'var(--red)'}} onClick={() => toggleSeverity('error')}>
                                        <i className="fa-solid fa-circle-xmark"/> Errors {errorCount}
                                    </span>
                                )}
                                {warnCount > 0 && (
                                    <span className={`chip ${severityFilter.has('warn') ? 'chip-active' : ''}`} style={{cursor: 'pointer', color: severityFilter.has('warn') ? undefined : 'var(--amber)', borderColor: 'var(--amber)'}} onClick={() => toggleSeverity('warn')}>
                                        <i className="fa-solid fa-triangle-exclamation"/> Warnings {warnCount}
                                    </span>
                                )}
                                {view === 'grouped' && (
                                    <span className="findings-collapse-all" onClick={() => setCollapsed(new Set(groups.map((g) => g.key)))}>Collapse all</span>
                                )}
                            </div>

                            <div className="findings-list">
                                {view === 'list' && filtered.map((f, i) => <FindingRow key={i} f={f}/>)}
                                {view === 'grouped' && groups.map((g) => {
                                    const isCollapsed = collapsed.has(g.key);
                                    const shown = isCollapsed ? [] : g.findings.slice(0, GROUP_SHOW_CAP);
                                    const more = isCollapsed ? 0 : g.findings.length - shown.length;
                                    return (
                                        <Fragment key={g.key}>
                                            <div
                                                className="findings-group-head"
                                                ref={(el) => { if (el) groupRefs.current.set(g.key, el); }}
                                                onClick={() => toggleGroupCollapsed(g.key)}
                                            >
                                                <i className={`fa-solid fa-chevron-${isCollapsed ? 'right' : 'down'} chevron`}/>
                                                <span className="findings-group-dot" style={{background: g.color}}/>
                                                <span className="findings-group-name">{g.label}</span>
                                                <span className="findings-group-count">{g.findings.length}</span>
                                            </div>
                                            {shown.map((f, i) => <FindingRow key={i} f={f}/>)}
                                            {more > 0 && (
                                                <div className="findings-group-more" onClick={() => setCollapsed((prev) => { const next = new Set(prev); next.delete(g.key); return next; })}>
                                                    Show {more} more in {g.label}
                                                </div>
                                            )}
                                        </Fragment>
                                    );
                                })}
                            </div>

                            <div className="findings-footer">
                                <span>{filtered.length} finding{filtered.length === 1 ? '' : 's'}{view === 'grouped' ? ` · ${groups.length} group${groups.length === 1 ? '' : 's'}` : ''}</span>
                            </div>
                        </Fragment>
                    )}
                </div>
            </div>
        </div>
    );
}

function FindingRow({f}: {f: modcheck.Finding}) {
    const cat = categoryOf(f.Category);
    const color = cat?.color ?? 'var(--text-dim)';
    const icon = cat?.icon ?? 'fa-circle-info';
    return (
        <div className="editor-finding-row" style={{borderLeftColor: color}}>
            <i title={SEVERITY_LABEL[f.Severity] ?? 'Info'} className={`fa-duotone ${icon}`} style={{color}}/>
            <div className="editor-finding-body">
                <span className="mono editor-finding-file">{f.File}{f.Line ? `:${f.Line}` : ''}</span>
                <span className="editor-finding-message">{f.Message}</span>
            </div>
        </div>
    );
}

import './ConflictResolver.css';
import {h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {
    ClearPatchOverrides,
    GeneratePatch,
    ReadModFile,
    ResolvedConflicts,
    SetConflictResolved,
    SetPatchOverride,
} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {highlightLine, syntaxForPath, type FileSyntax} from '../data/highlight';
import {diffLines} from '../data/lineDiff';
import {timeAgo} from '../data/format';
import {EmptyState} from '../components/EmptyState';
import {MarqueeText} from '../components/MarqueeText';

// maxMatrixMods caps how many mods the overlap matrix renders - a real
// modlist can have 50+ mods touching at least one contested key, and an
// NxN grid that size is both slow to render and hard for a person to
// actually scan. The mods left out are the ones involved in the fewest
// conflicts, so the ones most worth looking at stay visible.
const maxMatrixMods = 30;

function conflictKey(c: library.ConflictSummary): string {
    return `${c.Type}:${c.ID}`;
}

// A one-line, honest framing of why a contender card wins or loses, from
// nothing but its real position among conflict.Candidates (ascending load-
// order position, per ConflictSummary's own doc comment) - never a claim
// about *why* the automatic rule picked this winner (LIOS vs. FIOS isn't
// exposed to the frontend at all), just what's directly observable: is
// this the first/last candidate, and is it earlier or later than whoever
// currently wins.
function framingFor(idx: number, winnerIdx: number, candidateCount: number, overridden: boolean): string {
    if (idx === winnerIdx) {
        if (overridden) return 'Wins - manually forced';
        if (candidateCount <= 1) return 'Only candidate for this key';
        if (idx === candidateCount - 1) return 'Wins - last in load order';
        if (idx === 0) return 'Wins - first in load order';
        return 'Wins - automatic load-order winner';
    }
    return idx < winnerIdx ? 'Loses - earlier in load order' : 'Loses - later in load order';
}

export function ConflictResolver({gameId, conflicts, order, onClose, onPatchGenerated, onOverrideChanged}: {
    gameId: string;
    conflicts: library.ConflictSummary[];
    order: string[];
    onClose: () => void;
    onPatchGenerated: (modId: string) => void;
    onOverrideChanged: () => void;
}) {
    const [mode, setMode] = useState<'list' | 'matrix'>('list');
    const [search, setSearch] = useState('');
    const [selectedKey, setSelectedKey] = useState('');
    const [patching, setPatching] = useState(false);
    // kind drives both the banner's color (success/error/info - see
    // .patch-banner's own CSS) and, while patching is true, doubles as
    // the message shown on the full-window progress overlay below.
    const [patchMessage, setPatchMessage] = useState<{ kind: 'success' | 'error' | 'info'; text: string } | null>(null);
    const [resolvingAll, setResolvingAll] = useState(false);
    // Which contested keys (see conflictKey) this user has manually
    // marked reviewed for this game - see internal/resolvedconflicts.
    // Purely a personal bookkeeping flag: it never changes who actually
    // wins a key, just how the keys list below colors it. Fetched once
    // per game, then kept in sync locally by setConflictResolved so
    // marking/unmarking one doesn't need a full refetch.
    const [resolvedKeys, setResolvedKeys] = useState<Set<string>>(new Set());

    useEffect(() => {
        ResolvedConflicts(gameId).then((keys) => setResolvedKeys(new Set(keys))).catch(() => setResolvedKeys(new Set()));
    }, [gameId]);

    async function setConflictResolved(c: library.ConflictSummary, resolved: boolean) {
        const key = conflictKey(c);
        setResolvedKeys((prev) => {
            const next = new Set(prev);
            if (resolved) next.add(key); else next.delete(key);
            return next;
        });
        try {
            await SetConflictResolved(gameId, c.Type, c.ID, resolved);
        } catch (err) {
            // Revert the optimistic update - the save didn't actually
            // stick, so the UI shouldn't claim it did.
            setResolvedKeys((prev) => {
                const next = new Set(prev);
                if (resolved) next.delete(key); else next.add(key);
                return next;
            });
            setPatchMessage({kind: 'error', text: `Failed to update: ${String(err)}`});
        }
    }

    const filtered = conflicts.filter((c) => {
        const q = search.trim().toLowerCase();
        if (!q) return true;
        return c.ID.toLowerCase().includes(q) || c.Type.toLowerCase().includes(q)
            || c.Candidates.some((cand) => cand.ModName.toLowerCase().includes(q));
    });
    const selected = conflicts.find((c) => conflictKey(c) === selectedKey) ?? filtered[0] ?? null;
    const manualCount = conflicts.filter((c) => c.Overridden).length;

    async function handleAutoResolveAll() {
        setResolvingAll(true);
        try {
            await ClearPatchOverrides(gameId);
            onOverrideChanged();
        } catch (err) {
            setPatchMessage({kind: 'error', text: `Failed to auto-resolve: ${String(err)}`});
        } finally {
            setResolvingAll(false);
        }
    }

    async function handleGeneratePatch() {
        setPatching(true);
        setPatchMessage(null);
        try {
            const result = await GeneratePatch(gameId, order);
            if (result.Written) {
                let msg = `Generated a patch for ${result.PatchedKeys} conflict${result.PatchedKeys === 1 ? '' : 's'}.`;
                if (result.SkippedKeys > 0) {
                    msg += ` ${result.SkippedKeys} skipped - localization patching isn't built yet.`;
                    setPatchMessage({kind: 'info', text: msg});
                } else {
                    setPatchMessage({kind: 'success', text: msg});
                }
                onPatchGenerated(result.ModID);
            } else if (result.SkippedKeys > 0) {
                setPatchMessage({kind: 'info', text: `No conflicts could be patched - all ${result.SkippedKeys} were localization, which isn't supported yet.`});
            } else {
                setPatchMessage({kind: 'info', text: 'No conflicts needed patching.'});
            }
        } catch (err) {
            setPatchMessage({kind: 'error', text: `Failed to generate patch: ${String(err)}`});
        } finally {
            setPatching(false);
        }
    }

    return (
        <div className="overlay" onClick={onClose}>
            <div className="resolver" onClick={(e) => e.stopPropagation()}>
                <div className="resolver-header">
                    <span className="title">Conflicts</span>
                    <span className={`badge ${conflicts.length > 0 ? 'hard' : 'clean'}`}>
                        {conflicts.length === 0 && <i className="fa-solid fa-circle-check"/>}
                        {conflicts.length} contested {conflicts.length === 1 ? 'key' : 'keys'}
                    </span>
                    {manualCount > 0 && (
                        <span className="badge soft">{manualCount} manual override{manualCount === 1 ? '' : 's'}</span>
                    )}
                    <div className="spacer"/>
                    <span className="mode-toggle">
                        <span className={mode === 'list' ? 'active' : ''} onClick={() => setMode('list')}>List</span>
                        <span className={mode === 'matrix' ? 'active' : ''} onClick={() => setMode('matrix')}>Matrix</span>
                    </span>
                    {manualCount > 0 && (
                        <span className={`btn-ghost ${resolvingAll ? 'inert' : ''}`} onClick={resolvingAll ? undefined : handleAutoResolveAll}>
                            {resolvingAll ? 'Resolving...' : 'Auto-resolve all'}
                        </span>
                    )}
                    {conflicts.length > 0 && (
                        <span className={`btn-primary ${patching ? 'inert' : ''}`} onClick={patching ? undefined : handleGeneratePatch}>
                            {patching ? 'Generating...' : 'Generate patch'}
                        </span>
                    )}
                    <i className="fa-solid fa-xmark close-btn" onClick={onClose}/>
                </div>

                {patchMessage && (
                    <div className={`patch-banner ${patchMessage.kind}`}>
                        <span>{patchMessage.text}</span>
                        <i className="fa-solid fa-xmark" onClick={() => setPatchMessage(null)}/>
                    </div>
                )}

                {conflicts.length === 0 && (
                    <div className="resolver-body">
                        <EmptyState
                            icon="fa-circle-check"
                            title="No conflicts"
                            subtitle="No genuine conflicts detected in the current load order."
                        />
                    </div>
                )}
                {conflicts.length > 0 && mode === 'list' && (
                    <ListView
                        gameId={gameId}
                        conflicts={filtered}
                        search={search}
                        onSearch={setSearch}
                        selected={selected}
                        onSelect={(c) => setSelectedKey(conflictKey(c))}
                        onOverrideChanged={onOverrideChanged}
                        resolvedKeys={resolvedKeys}
                        onSetResolved={setConflictResolved}
                    />
                )}
                {conflicts.length > 0 && mode === 'matrix' && <MatrixView conflicts={conflicts}/>}

                {patching && (
                    <div className="resolver-progress-overlay">
                        <i className="fa-solid fa-spinner fa-spin"/>
                        <div className="resolver-progress-title">Generating patch...</div>
                        <div className="resolver-progress-subtitle">
                            Resolving every contested key against the current load order and writing the
                            winning content into a patch mod. Sit tight - this window is locked until it's
                            done.
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}

function ListView({gameId, conflicts, search, onSearch, selected, onSelect, onOverrideChanged, resolvedKeys, onSetResolved}: {
    gameId: string;
    conflicts: library.ConflictSummary[];
    search: string;
    onSearch: (s: string) => void;
    selected: library.ConflictSummary | null;
    onSelect: (c: library.ConflictSummary) => void;
    onOverrideChanged: () => void;
    resolvedKeys: Set<string>;
    onSetResolved: (c: library.ConflictSummary, resolved: boolean) => void;
}) {
    return (
        <div className="resolver-body">
            <div className="files-col">
                <div className="col-header">CONTESTED KEYS · {conflicts.length}</div>
                <div className="search-box" style={{margin: '8px 10px', width: 'auto'}}>
                    <i className="fa-solid fa-magnifying-glass"/>
                    <input
                        placeholder="Search keys or mods..."
                        value={search}
                        onInput={(e) => onSearch((e.target as HTMLInputElement).value)}
                    />
                </div>
                <div className="files-list">
                    {conflicts.map((c) => {
                        const isSelected = selected !== null && conflictKey(selected) === conflictKey(c);
                        const isResolved = resolvedKeys.has(conflictKey(c));
                        return (
                            <div
                                key={conflictKey(c)}
                                className="file-item"
                                style={{
                                    background: isSelected ? '#1b232e' : 'transparent',
                                    borderLeftColor: isResolved ? 'var(--green)' : 'var(--red)',
                                    cursor: 'pointer',
                                }}
                                onClick={() => onSelect(c)}
                            >
                                <div className="mono path">{c.Type}</div>
                                <div className="note">
                                    {c.ID} · {c.Candidates.length} mods{c.Overridden ? ' · manual' : ''}{isResolved ? ' · done' : ''}
                                </div>
                            </div>
                        );
                    })}
                    {conflicts.length === 0 && <p className="detail-empty" style={{padding: 14}}>No matches.</p>}
                </div>
            </div>

            {selected && (
                <ContendersAndContent
                    key={conflictKey(selected)}
                    gameId={gameId}
                    conflict={selected}
                    onOverrideChanged={onOverrideChanged}
                    resolved={resolvedKeys.has(conflictKey(selected))}
                    onSetResolved={(resolved) => onSetResolved(selected, resolved)}
                />
            )}
        </div>
    );
}

function ContendersAndContent({gameId, conflict, onOverrideChanged, resolved, onSetResolved}: {
    gameId: string;
    conflict: library.ConflictSummary;
    onOverrideChanged: () => void;
    resolved: boolean;
    onSetResolved: (resolved: boolean) => void;
}) {
    const [leftContent, setLeftContent] = useState<string | null>(null);
    const [rightContent, setRightContent] = useState<string | null>(null);
    const [leftModified, setLeftModified] = useState<number | null>(null);
    const [rightModified, setRightModified] = useState<number | null>(null);
    const [error, setError] = useState('');
    const [overrideBusy, setOverrideBusy] = useState(false);
    const [overrideError, setOverrideError] = useState('');

    async function chooseWinner(modId: string) {
        setOverrideBusy(true);
        setOverrideError('');
        try {
            await SetPatchOverride(gameId, conflict.Type, conflict.ID, modId);
            onOverrideChanged();
        } catch (err) {
            setOverrideError(String(err));
        } finally {
            setOverrideBusy(false);
        }
    }

    const winnerIdx = conflict.Candidates.findIndex((c) => c.ModID === conflict.Winner);
    // Compare the winner against whichever candidate sits right before it
    // in load order - the one it's actually overriding - rather than an
    // arbitrary pair, when there are more than two candidates.
    const loserIdx = winnerIdx > 0 ? winnerIdx - 1 : (conflict.Candidates.length > 1 ? 1 : -1);
    const winner = winnerIdx >= 0 ? conflict.Candidates[winnerIdx] : null;
    const loser = loserIdx >= 0 ? conflict.Candidates[loserIdx] : null;

    useEffect(() => {
        let cancelled = false;
        setLeftContent(null);
        setRightContent(null);
        setLeftModified(null);
        setRightModified(null);
        setError('');
        if (loser) {
            ReadModFile(gameId, loser.ModID, loser.FilePath)
                .then((f) => { if (!cancelled) { setLeftContent(f.Content); setLeftModified(f.ModifiedAt); } })
                .catch((err) => { if (!cancelled) setError(String(err)); });
        }
        if (winner) {
            ReadModFile(gameId, winner.ModID, winner.FilePath)
                .then((f) => { if (!cancelled) { setRightContent(f.Content); setRightModified(f.ModifiedAt); } })
                .catch((err) => { if (!cancelled) setError(String(err)); });
        }
        return () => { cancelled = true; };
    }, [gameId, winner?.ModID, winner?.FilePath, loser?.ModID, loser?.FilePath]);

    // Real line-level diffing (see data/lineDiff.ts) once both sides have
    // actually loaded - an empty file is zero lines, not one ('' split on
    // '\n' would otherwise report a single blank line, misclassifying it
    // against the other side).
    const diff = useMemo(() => {
        if (leftContent == null || rightContent == null) return null;
        const leftLines = leftContent === '' ? [] : leftContent.split('\n');
        const rightLines = rightContent === '' ? [] : rightContent.split('\n');
        return diffLines(leftLines, rightLines);
    }, [leftContent, rightContent]);

    return (
        <>
            <div className="contenders-col">
                <div className="col-header">CONTENDERS · LOAD ORDER</div>
                <div className="contenders-list">
                    {conflict.Candidates.map((c, i) => {
                        const wins = c.ModID === conflict.Winner;
                        const isLoser = loser?.ModID === c.ModID;
                        const isWinnerCard = winner?.ModID === c.ModID;
                        const loadedContent = isLoser ? leftContent : isWinnerCard ? rightContent : null;
                        const loadedModified = isLoser ? leftModified : isWinnerCard ? rightModified : null;
                        const metaText = loadedContent != null && loadedModified != null
                            ? (() => {
                                const lines = loadedContent === '' ? 0 : loadedContent.split('\n').length;
                                return `${lines} line${lines === 1 ? '' : 's'} · modified ${timeAgo(loadedModified)}`;
                            })()
                            : c.FilePath;
                        return (
                            <div
                                key={c.ModID}
                                className={`contender-card ${wins ? 'wins' : ''} ${overrideBusy ? 'inert' : ''}`}
                                title={wins ? 'Currently wins this conflict' : 'Click to make this mod win this conflict'}
                                onClick={() => !overrideBusy && !wins && chooseWinner(c.ModID)}
                            >
                                <div className="contender-head">
                                    <span className="mono pos">{i + 1}</span>
                                    <span className="name"><MarqueeText text={c.ModName}/></span>
                                    {wins && <span className="wins-badge">{conflict.Overridden ? 'WINS · MANUAL' : 'WINS'}</span>}
                                </div>
                                <div className="mono meta" title={metaText}>{metaText}</div>
                                <div className={`note ${wins ? 'wins-note' : ''}`}>
                                    {framingFor(i, winnerIdx, conflict.Candidates.length, conflict.Overridden)}
                                </div>
                            </div>
                        );
                    })}
                </div>

                <div className="resolution-card">
                    <div className="resolution-title">Resolution</div>
                    <span
                        className={`resolution-option ${overrideBusy ? 'inert' : ''}`}
                        onClick={() => !overrideBusy && conflict.Overridden && chooseWinner('')}
                    >
                        <span className={!conflict.Overridden ? 'radio-on' : 'radio-off'}>{!conflict.Overridden ? '●' : '○'}</span> Keep load-order winner
                    </span>
                    {conflict.Candidates.map((c) => {
                        const forced = conflict.Overridden && conflict.Winner === c.ModID;
                        return (
                            <span
                                key={c.ModID}
                                className={`resolution-option ${overrideBusy ? 'inert' : ''}`}
                                onClick={() => !overrideBusy && !forced && chooseWinner(c.ModID)}
                            >
                                <span className={forced ? 'radio-on' : 'radio-off'}>{forced ? '●' : '○'}</span>
                                <MarqueeText text={`Force ${c.ModName}`} className="resolution-option-label"/>
                            </span>
                        );
                    })}
                    <span className="resolution-option-disabled" title="Not built yet">
                        <span className="radio-off">○</span> Generate merge patch
                    </span>
                    <span className="resolution-option-disabled" title="Not built yet">
                        <span className="radio-off">○</span> Exclude file from both
                    </span>
                </div>
                {overrideError && <p className="status-page error" style={{padding: '0 13px 10px'}}>{overrideError}</p>}
            </div>

            <div className="diff-col">
                <div className="diff-toolbar">
                    <span className="mono">{winner?.FilePath ?? conflict.ID}</span>
                    <div className="spacer"/>
                    <span className="sort-label">Side by side</span>
                </div>
                {error && <p className="status-page error" style={{padding: 12}}>{error}</p>}
                {!error && (loser || winner) && (
                    <div className="diff-body mono">
                        <div className="diff-pane">
                            {loser
                                ? (
                                    <ContentPane
                                        label={`${loser.ModName} (loses)`}
                                        content={leftContent}
                                        syntax={syntaxForPath(loser.FilePath)}
                                        lineStates={diff?.leftMatched}
                                        changedClassName="removed"
                                    />
                                )
                                : <ContentPane label="Only one mod touches this key" content="" syntax="plain"/>}
                        </div>
                        <div className="diff-pane">
                            {winner && (
                                <ContentPane
                                    label={`${winner.ModName} (wins)`}
                                    content={rightContent}
                                    syntax={syntaxForPath(winner.FilePath)}
                                    lineStates={diff?.rightMatched}
                                    changedClassName="added"
                                />
                            )}
                        </div>
                    </div>
                )}
                <div className="diff-footer">
                    {diff && !diff.skipped && (
                        <span className="mono diff-stat">
                            <span className="diff-stat-added">+{diff.added}</span>{' '}
                            <span className="diff-stat-removed">&minus;{diff.removed}</span>
                        </span>
                    )}
                    {(!diff || diff.skipped) && (
                        <span className="mono">
                            {!loser
                                ? "Only one mod touches this key - there's nothing to diff against."
                                : diff?.skipped
                                    ? "This file is too large to diff line-by-line - showing its real, syntax-highlighted content as-is."
                                    : 'Comparing...'}
                        </span>
                    )}
                    <span
                        className={`resolved-toggle ${resolved ? 'active' : ''}`}
                        title={resolved ? 'Mark this key unresolved again' : "Mark this key as reviewed - doesn't change who wins it"}
                        onClick={() => onSetResolved(!resolved)}
                    >
                        <i className={`fa-solid ${resolved ? 'fa-circle-check' : 'fa-circle'}`}/>
                        {resolved ? 'Marked done' : 'Mark done'}
                    </span>
                </div>
            </div>
        </>
    );
}

function ContentPane({label, content, syntax, lineStates, changedClassName}: {
    label: string;
    content: string | null;
    syntax: FileSyntax;
    // Per real line in `content`: true if that line is also present (in
    // the same relative order) in the other side's file, false if it's
    // only here - see data/lineDiff.ts. Omitted (undefined) means "don't
    // tint anything," either because there's nothing to diff against yet
    // (still loading, or only one mod touches this key) or because the
    // diff was skipped for being too large - either way this pane still
    // shows its own real, syntax-highlighted content, just without the
    // added/removed tint.
    lineStates?: boolean[];
    // Applied to a line whose lineStates entry is false - 'removed' for
    // the losing side, 'added' for the winning side.
    changedClassName?: 'removed' | 'added';
}) {
    return (
        <>
            <div className="file-item" style={{borderLeft: 'none', padding: '5px 11px'}}>
                <span className="note">{label}</span>
            </div>
            {content === null && (
                <div className="diff-loading">
                    <i className="fa-solid fa-spinner fa-spin"/>
                    <span>Loading file...</span>
                </div>
            )}
            {content === '' && (
                <div className="diff-line"><span className="ln"/><span/></div>
            )}
            {content != null && content !== '' && content.split('\n').map((line, i) => {
                const changed = lineStates?.[i] === false;
                return (
                    <div key={i} className={`diff-line ${changed ? changedClassName : ''}`}>
                        <span className="ln">{i + 1}</span>
                        <span className="line-content">
                            {highlightLine(line, syntax).map((tok, j) => (
                                <span key={j} className={`tok-${tok.kind}`}>{tok.text}</span>
                            ))}
                        </span>
                    </div>
                );
            })}
        </>
    );
}

function MatrixView({conflicts}: { conflicts: library.ConflictSummary[] }) {
    const {mods, counts, truncated} = useMemo(() => {
        const names = new Map<string, string>();
        const totals = new Map<string, number>();
        const pairCounts = new Map<string, number>();

        for (const c of conflicts) {
            for (const cand of c.Candidates) {
                names.set(cand.ModID, cand.ModName);
            }
            for (let i = 0; i < c.Candidates.length; i++) {
                const a = c.Candidates[i].ModID;
                totals.set(a, (totals.get(a) ?? 0) + 1);
                for (let j = 0; j < c.Candidates.length; j++) {
                    if (i === j) continue;
                    const b = c.Candidates[j].ModID;
                    const key = `${a}|${b}`;
                    pairCounts.set(key, (pairCounts.get(key) ?? 0) + 1);
                }
            }
        }

        const ranked = [...names.keys()].sort((a, b) => (totals.get(b) ?? 0) - (totals.get(a) ?? 0));
        const shown = ranked.slice(0, maxMatrixMods);
        const mods = shown.map((id) => ({modId: id, modName: names.get(id) ?? id}));
        return {mods, counts: pairCounts, truncated: ranked.length > shown.length};
    }, [conflicts]);

    function colorFor(n: number): { bg: string; fg: string } {
        if (n === 0) return {bg: '#1a212b', fg: '#3c4858'};
        if (n < 20) return {bg: '#4a3a23', fg: '#e0c090'};
        if (n < 100) return {bg: '#8a5f2a', fg: '#fff'};
        return {bg: '#d4574e', fg: '#fff'};
    }

    let maxPair: { a: string; b: string; n: number } | null = null;
    for (const m of mods) {
        for (const other of mods) {
            if (m.modId === other.modId) continue;
            const n = counts.get(`${m.modId}|${other.modId}`) ?? 0;
            if (!maxPair || n > maxPair.n) {
                maxPair = {a: m.modId, b: other.modId, n};
            }
        }
    }
    const nameOf = (id: string) => mods.find((m) => m.modId === id)?.modName ?? id;

    return (
        <div className="matrix-view">
            <div className="matrix-intro">
                <div className="title">Overlap matrix</div>
                <div className="subtitle">
                    Row vs. column = how many contested keys both mods compete for. Cell darkness = shared count.
                    {truncated && ` Showing the ${maxMatrixMods} mods with the most conflicts.`}
                </div>
            </div>
            <div className="matrix-grid-wrap" style={{overflowX: 'auto'}}>
                <div className="matrix-labels">
                    {mods.map((m) => <div key={m.modId} className="matrix-label">{m.modName}</div>)}
                </div>
                <div className="matrix-cells">
                    <div className="matrix-shorts">
                        {mods.map((m) => <div key={m.modId} className="matrix-short mono">{m.modName}</div>)}
                    </div>
                    {mods.map((row) => (
                        <div key={row.modId} className="matrix-row">
                            {mods.map((col) => {
                                const n = row.modId === col.modId ? 0 : (counts.get(`${row.modId}|${col.modId}`) ?? 0);
                                const {bg, fg} = colorFor(n);
                                return (
                                    <div key={col.modId} className="matrix-cell mono" style={{background: bg, color: fg}}>
                                        {n || ''}
                                    </div>
                                );
                            })}
                        </div>
                    ))}
                </div>
                <div className="matrix-scale">
                    <div className="sidebar-label">SCALE</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#1a212b'}}/>none</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#4a3a23'}}/>1-19</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#8a5f2a'}}/>20-99</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#d4574e'}}/>100+</div>
                </div>
            </div>
            {maxPair && maxPair.n > 0 && (
                <div className="matrix-highlight">
                    <div className="matrix-highlight-title">Biggest overlap</div>
                    <div className="matrix-highlight-body">
                        {nameOf(maxPair.a)} and {nameOf(maxPair.b)} both compete for {maxPair.n} of the same contested keys.
                    </div>
                </div>
            )}
        </div>
    );
}

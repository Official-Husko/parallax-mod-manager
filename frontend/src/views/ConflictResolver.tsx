import './ConflictResolver.css';
import {h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {GeneratePatch, ReadModFile, SetPatchOverride} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {highlightLine, syntaxForPath, type FileSyntax} from '../data/highlight';
import {EmptyState} from '../components/EmptyState';

// maxMatrixMods caps how many mods the overlap matrix renders - a real
// modlist can have 50+ mods touching at least one contested key, and an
// NxN grid that size is both slow to render and hard for a person to
// actually scan. The mods left out are the ones involved in the fewest
// conflicts, so the ones most worth looking at stay visible.
const maxMatrixMods = 30;

function conflictKey(c: library.ConflictSummary): string {
    return `${c.Type}:${c.ID}`;
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
    const [patchMessage, setPatchMessage] = useState('');

    const filtered = conflicts.filter((c) => {
        const q = search.trim().toLowerCase();
        if (!q) return true;
        return c.ID.toLowerCase().includes(q) || c.Type.toLowerCase().includes(q)
            || c.Candidates.some((cand) => cand.ModName.toLowerCase().includes(q));
    });
    const selected = conflicts.find((c) => conflictKey(c) === selectedKey) ?? filtered[0] ?? null;

    async function handleGeneratePatch() {
        setPatching(true);
        setPatchMessage('');
        try {
            const result = await GeneratePatch(gameId, order);
            if (result.Written) {
                let msg = `Generated a patch for ${result.PatchedKeys} conflict${result.PatchedKeys === 1 ? '' : 's'}.`;
                if (result.SkippedKeys > 0) {
                    msg += ` ${result.SkippedKeys} skipped - localization patching isn't built yet.`;
                }
                setPatchMessage(msg);
                onPatchGenerated(result.ModID);
            } else if (result.SkippedKeys > 0) {
                setPatchMessage(`No conflicts could be patched - all ${result.SkippedKeys} were localization, which isn't supported yet.`);
            } else {
                setPatchMessage('No conflicts needed patching.');
            }
        } catch (err) {
            setPatchMessage(`Failed to generate patch: ${String(err)}`);
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
                    <div className="spacer"/>
                    <span className="mode-toggle">
                        <span className={mode === 'list' ? 'active' : ''} onClick={() => setMode('list')}>List</span>
                        <span className={mode === 'matrix' ? 'active' : ''} onClick={() => setMode('matrix')}>Matrix</span>
                    </span>
                    {conflicts.length > 0 && (
                        <span className={`btn-primary ${patching ? 'inert' : ''}`} onClick={patching ? undefined : handleGeneratePatch}>
                            {patching ? 'Generating...' : 'Generate patch'}
                        </span>
                    )}
                    <i className="fa-solid fa-xmark close-btn" onClick={onClose}/>
                </div>

                {patchMessage && (
                    <div className="patch-banner">
                        <span>{patchMessage}</span>
                        <i className="fa-solid fa-xmark" onClick={() => setPatchMessage('')}/>
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
                    />
                )}
                {conflicts.length > 0 && mode === 'matrix' && <MatrixView conflicts={conflicts}/>}
            </div>
        </div>
    );
}

function ListView({gameId, conflicts, search, onSearch, selected, onSelect, onOverrideChanged}: {
    gameId: string;
    conflicts: library.ConflictSummary[];
    search: string;
    onSearch: (s: string) => void;
    selected: library.ConflictSummary | null;
    onSelect: (c: library.ConflictSummary) => void;
    onOverrideChanged: () => void;
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
                        return (
                            <div
                                key={conflictKey(c)}
                                className="file-item"
                                style={{
                                    background: isSelected ? '#1b232e' : 'transparent',
                                    borderLeftColor: 'var(--red)',
                                    cursor: 'pointer',
                                }}
                                onClick={() => onSelect(c)}
                            >
                                <div className="mono path">{c.Type}</div>
                                <div className="note">{c.ID} · {c.Candidates.length} mods{c.Overridden ? ' · manual' : ''}</div>
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
                />
            )}
        </div>
    );
}

function ContendersAndContent({gameId, conflict, onOverrideChanged}: {
    gameId: string;
    conflict: library.ConflictSummary;
    onOverrideChanged: () => void;
}) {
    const [leftContent, setLeftContent] = useState<string | null>(null);
    const [rightContent, setRightContent] = useState<string | null>(null);
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
        setError('');
        if (loser) {
            ReadModFile(gameId, loser.ModID, loser.FilePath)
                .then((c) => { if (!cancelled) setLeftContent(c); })
                .catch((err) => { if (!cancelled) setError(String(err)); });
        }
        if (winner) {
            ReadModFile(gameId, winner.ModID, winner.FilePath)
                .then((c) => { if (!cancelled) setRightContent(c); })
                .catch((err) => { if (!cancelled) setError(String(err)); });
        }
        return () => { cancelled = true; };
    }, [gameId, winner?.ModID, winner?.FilePath, loser?.ModID, loser?.FilePath]);

    return (
        <>
            <div className="contenders-col">
                <div className="col-header">
                    CONTENDERS · LOAD ORDER
                    {conflict.Overridden && (
                        <span className={`reset-override-link ${overrideBusy ? 'inert' : ''}`} onClick={() => !overrideBusy && chooseWinner('')}>
                            Reset to automatic
                        </span>
                    )}
                </div>
                <div className="contenders-list">
                    {conflict.Candidates.map((c, i) => {
                        const wins = c.ModID === conflict.Winner;
                        return (
                            <div key={c.ModID} className={`contender-card ${wins ? 'wins' : ''}`}>
                                <div className="contender-head">
                                    <i
                                        className={`fa-solid ${wins ? 'fa-circle-dot radio-on' : 'fa-circle radio-off'} winner-radio ${overrideBusy ? 'inert' : ''}`}
                                        title={wins ? 'Currently wins this conflict' : 'Make this mod win this conflict'}
                                        onClick={() => !overrideBusy && !wins && chooseWinner(c.ModID)}
                                    />
                                    <span className="mono pos">{i + 1}</span>
                                    <span className="name">{c.ModName}</span>
                                    {wins && <span className="wins-badge">{conflict.Overridden ? 'WINS · MANUAL' : 'WINS'}</span>}
                                </div>
                                <div className="mono meta">{c.FilePath}</div>
                            </div>
                        );
                    })}
                </div>
                {overrideError && <p className="status-page error" style={{padding: '0 13px 10px'}}>{overrideError}</p>}
            </div>

            <div className="diff-col">
                <div className="diff-toolbar">
                    <span className="mono">{winner?.FilePath ?? conflict.ID}</span>
                    <div className="spacer"/>
                    <span className="sort-label">Real file content, not diffed</span>
                </div>
                {error && <p className="status-page error" style={{padding: 12}}>{error}</p>}
                {!error && (loser || winner) && (
                    <div className="diff-body mono">
                        <div className="diff-pane">
                            {loser
                                ? <ContentPane label={`${loser.ModName} (loses)`} content={leftContent} syntax={syntaxForPath(loser.FilePath)}/>
                                : <ContentPane label="Only one mod touches this key" content="" syntax="plain"/>}
                        </div>
                        <div className="diff-pane">
                            {winner && <ContentPane label={`${winner.ModName} (wins)`} content={rightContent} syntax={syntaxForPath(winner.FilePath)}/>}
                        </div>
                    </div>
                )}
                <div className="diff-footer">
                    <span className="mono">Line-level diff highlighting isn't built yet - this shows each file's real, syntax-highlighted content as-is.</span>
                </div>
            </div>
        </>
    );
}

function ContentPane({label, content, syntax}: { label: string; content: string | null; syntax: FileSyntax }) {
    return (
        <>
            <div className="file-item" style={{borderLeft: 'none', padding: '5px 11px'}}>
                <span className="note">{label}</span>
            </div>
            {content === null && (
                <div className="diff-line"><span className="ln"/><span>Loading...</span></div>
            )}
            {content === '' && (
                <div className="diff-line"><span className="ln"/><span/></div>
            )}
            {content != null && content.split('\n').map((line, i) => (
                <div key={i} className="diff-line">
                    <span className="ln">{i + 1}</span>
                    <span>
                        {highlightLine(line, syntax).map((tok, j) => (
                            <span key={j} className={`tok-${tok.kind}`}>{tok.text}</span>
                        ))}
                    </span>
                </div>
            ))}
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

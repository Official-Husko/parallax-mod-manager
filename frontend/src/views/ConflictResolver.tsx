import './ConflictResolver.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
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
import {diffLines, type LineDiffResult} from '../data/lineDiff';
import {afterNextPaint} from '../data/deferred';
import {describePatchStatus, patchNeedsAttention} from '../data/patchStatus';
import {useVirtualWindow} from '../data/useVirtualWindow';
import {timeAgo} from '../data/format';
import {notify} from '../data/notifications';
import {EmptyState} from '../components/EmptyState';
import {MarqueeText} from '../components/MarqueeText';

// maxMatrixMods caps how many mods the overlap matrix renders - a real
// modlist can have 50+ mods touching at least one contested key, and an
// NxN grid that size is both slow to render and hard for a person to
// actually scan. The mods left out are the ones involved in the fewest
// conflicts, so the ones most worth looking at stay visible.
const maxMatrixMods = 30;

// Both long scrolling areas here - the contested-keys list and each side of
// the diff - only put the rows on screen in the DOM (see useVirtualWindow),
// which needs every row to be exactly this many pixels tall. The matching CSS
// (.conflict-row, .diff-line) pins the same numbers.
const conflictRowHeight = 50;
const diffLineHeight = 19;

// Runs a task behind the resolver's "Applying... please wait" overlay - see
// ConflictResolver's runApply.
type ApplyRunner = (what: string, task: () => Promise<void>) => Promise<void>;

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

export function ConflictResolver({gameId, conflicts, patch, order, onClose, onPatchGenerated, onOverrideChanged}: {
    gameId: string;
    conflicts: library.ConflictSummary[];
    // How the generated patch mod stands against these conflicts (which keys
    // it no longer matches, and why) - see library.PatchSummary. Undefined
    // until a full scan has produced one.
    patch?: library.PatchSummary;
    order: string[];
    onClose: () => void;
    onPatchGenerated: (modId: string) => void;
    onOverrideChanged: () => void | Promise<void>;
}) {
    const [mode, setMode] = useState<'list' | 'matrix'>('list');
    const [search, setSearch] = useState('');
    const [selectedKey, setSelectedKey] = useState('');
    const [patching, setPatching] = useState(false);
    // What the full-window "Applying... please wait" overlay says is being
    // done, or null when nothing is. Applying a winner (or resetting every
    // manual pick) ends in a full rescan of the game, which takes a moment on
    // a big modlist - the overlay locks the window for that whole time so a
    // click can't land on a half-updated view, and so it's obvious something
    // is still happening.
    const [applying, setApplying] = useState<string | null>(null);
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
            notify('error', `Failed to update: ${String(err)}`);
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

    // Runs task behind the applying overlay (`what` is its explanation line),
    // and rethrows whatever task throws once the overlay is down so the
    // caller can show its own error.
    async function runApply(what: string, task: () => Promise<void>) {
        setApplying(what);
        try {
            await task();
        } finally {
            setApplying(null);
        }
    }

    async function handleAutoResolveAll() {
        try {
            await runApply(
                "Resetting every manual pick and re-checking all conflicts against the load order. This window is locked until it's done.",
                async () => {
                    await ClearPatchOverrides(gameId);
                    // Awaited so the overlay stays up until the rescan has
                    // actually brought back the reset winners, not just until
                    // the file write.
                    await onOverrideChanged();
                },
            );
        } catch (err) {
            notify('error', `Failed to auto-resolve: ${String(err)}`);
        }
    }

    async function handleGeneratePatch() {
        setPatching(true);
        try {
            const result = await GeneratePatch(gameId, order);
            if (result.Written) {
                let msg = `Generated a patch for ${result.PatchedKeys} conflict${result.PatchedKeys === 1 ? '' : 's'}.`;
                if (result.SkippedKeys > 0) {
                    msg += ` ${result.SkippedKeys} skipped - localization patching isn't built yet.`;
                    notify('info', msg);
                } else {
                    notify('success', msg);
                }
                onPatchGenerated(result.ModID);
            } else if (result.SkippedKeys > 0) {
                notify('info', `No conflicts could be patched - all ${result.SkippedKeys} were localization, which isn't supported yet.`);
            } else {
                notify('info', 'No conflicts needed patching.');
            }
        } catch (err) {
            notify('error', `Failed to generate patch: ${String(err)}`);
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
                    {patch && patch.Changed > 0 && (
                        <span className="badge review" title="Keys whose mods changed since the patch was generated - shown with an orange line">
                            {patch.Changed} to review
                        </span>
                    )}
                    <div className="spacer"/>
                    <span className="mode-toggle">
                        <span className={mode === 'list' ? 'active' : ''} onClick={() => setMode('list')}>List</span>
                        <span className={mode === 'matrix' ? 'active' : ''} onClick={() => setMode('matrix')}>Matrix</span>
                    </span>
                    {manualCount > 0 && (
                        <span className="btn-ghost" onClick={handleAutoResolveAll}>Auto-resolve all</span>
                    )}
                    {conflicts.length > 0 && (
                        <span className={`btn-primary ${patching ? 'inert' : ''}`} onClick={patching ? undefined : handleGeneratePatch}>
                            {patching ? 'Generating...' : patch?.Exists ? 'Regenerate patch' : 'Generate patch'}
                        </span>
                    )}
                    <i className="fa-solid fa-xmark close-btn" onClick={onClose}/>
                </div>

                {patchNeedsAttention(patch) && (
                    <div className="patch-banner stale">
                        <i className="fa-solid fa-circle-exclamation"/>
                        <span>
                            {describePatchStatus(patch)}{' '}
                            {patch.Changed > 0
                                ? 'Review the orange keys, then regenerate it.'
                                : 'Regenerate it to bring it up to date.'}
                        </span>
                        <div className={`btn-primary ${patching ? 'inert' : ''}`} onClick={patching ? undefined : handleGeneratePatch}>
                            Regenerate patch
                        </div>
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
                        runApply={runApply}
                        resolvedKeys={resolvedKeys}
                        onSetResolved={setConflictResolved}
                    />
                )}
                {conflicts.length > 0 && mode === 'matrix' && <MatrixView conflicts={conflicts}/>}

                {(patching || applying !== null) && (
                    <div className="resolver-progress-overlay">
                        <i className="fa-solid fa-spinner fa-spin"/>
                        <div className="resolver-progress-title">{patching ? 'Generating patch...' : 'Applying... please wait'}</div>
                        <div className="resolver-progress-subtitle">
                            {patching
                                ? "Resolving every contested key against the current load order and writing the winning content into a patch mod. Sit tight - this window is locked until it's done."
                                : applying}
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}

// One row of the contested-keys list. Exactly conflictRowHeight tall (see
// .conflict-row), which is what lets ListView render only the rows on screen.
function ConflictRow({conflict, selected, resolved, onSelect}: {
    conflict: library.ConflictSummary;
    selected: boolean;
    resolved: boolean;
    onSelect: (c: library.ConflictSummary) => void;
}) {
    // The generated patch no longer matches what its mods say for this key -
    // orange takes priority over both "done" (green) and unresolved (red),
    // since a review the user finished before the change no longer holds.
    const patchOutdated = conflict.PatchState === 'changed';
    return (
        <div
            className="file-item conflict-row"
            style={{
                background: selected ? '#1b232e' : 'transparent',
                borderLeftColor: patchOutdated ? 'var(--amber)' : resolved ? 'var(--green)' : 'var(--red)',
                cursor: 'pointer',
            }}
            title={patchOutdated ? conflict.PatchNote : undefined}
            onClick={() => onSelect(conflict)}
        >
            <div className="mono path" title={conflict.Type}>{conflict.Type}</div>
            <div className="note" title={conflict.ID}>
                {conflict.ID} · {conflict.Candidates.length} mods{conflict.Overridden ? ' · manual' : ''}{resolved ? ' · done' : ''}
                {patchOutdated ? ' · patch outdated' : conflict.PatchState === 'patched' ? ' · patched' : ''}
            </div>
        </div>
    );
}

function ListView({gameId, conflicts, search, onSearch, selected, onSelect, onOverrideChanged, runApply, resolvedKeys, onSetResolved}: {
    gameId: string;
    conflicts: library.ConflictSummary[];
    search: string;
    onSearch: (s: string) => void;
    selected: library.ConflictSummary | null;
    onSelect: (c: library.ConflictSummary) => void;
    onOverrideChanged: () => void | Promise<void>;
    runApply: ApplyRunner;
    resolvedKeys: Set<string>;
    onSetResolved: (c: library.ConflictSummary, resolved: boolean) => void;
}) {
    const selectedKey = selected ? conflictKey(selected) : '';
    const {ref: listRef, onScroll: onListScroll, first, last} = useVirtualWindow<HTMLDivElement>(conflicts.length, conflictRowHeight, 8);
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
                <div className="files-list" ref={listRef} onScroll={onListScroll}>
                    <div style={{height: conflicts.length * conflictRowHeight, paddingTop: first * conflictRowHeight, boxSizing: 'border-box'}}>
                        {conflicts.slice(first, last).map((c) => {
                            const key = conflictKey(c);
                            return (
                                <ConflictRow
                                    key={key}
                                    conflict={c}
                                    selected={selectedKey === key}
                                    resolved={resolvedKeys.has(key)}
                                    onSelect={onSelect}
                                />
                            );
                        })}
                    </div>
                    {conflicts.length === 0 && <p className="detail-empty" style={{padding: 14}}>No matches.</p>}
                </div>
            </div>

            {selected && (
                <ContendersAndContent
                    key={conflictKey(selected)}
                    gameId={gameId}
                    conflict={selected}
                    onOverrideChanged={onOverrideChanged}
                    runApply={runApply}
                    resolved={resolvedKeys.has(conflictKey(selected))}
                    onSetResolved={(resolved) => onSetResolved(selected, resolved)}
                />
            )}
        </div>
    );
}

function ContendersAndContent({gameId, conflict, onOverrideChanged, runApply, resolved, onSetResolved}: {
    gameId: string;
    conflict: library.ConflictSummary;
    onOverrideChanged: () => void | Promise<void>;
    runApply: ApplyRunner;
    resolved: boolean;
    onSetResolved: (resolved: boolean) => void;
}) {
    const [leftContent, setLeftContent] = useState<string | null>(null);
    const [rightContent, setRightContent] = useState<string | null>(null);
    const [leftModified, setLeftModified] = useState<number | null>(null);
    const [rightModified, setRightModified] = useState<number | null>(null);
    const [error, setError] = useState('');
    // What's actually saved for this key right now: a mod ID if the user has
    // manually forced that mod to win, '' if the load order decides.
    const applied = conflict.Overridden ? conflict.Winner : '';
    // The resolution the user has picked but not applied yet - clicking a
    // contender or a radio only *stages* it here (and previews it in the
    // diff), it never changes anything on disk. Only the Apply button does
    // (see applyChoice). '' means "keep the load-order winner".
    const [choice, setChoice] = useState(applied);
    // Follow what's really saved whenever that changes underneath us (an
    // apply landing, or "Auto-resolve all" clearing every override).
    useEffect(() => setChoice(applied), [applied]);
    const dirty = choice !== applied;
    // The line diff between the two sides, computed once both have loaded -
    // see the effect below for why it's state rather than a useMemo.
    const [diff, setDiff] = useState<LineDiffResult | null>(null);

    // A single, shared horizontal scroll position for *both* diff panes -
    // real side-by-side comparison means never letting one pane's own
    // long line push the other one out of view, so each pane keeps its
    // own fixed half of the width (see .diff-pane's CSS) and this instead
    // shifts both panes' real content left together by the same amount,
    // each clamped to its *own* real overflow (leftMaxScroll/
    // rightMaxScroll) rather than applied raw - once the shorter side's
    // real content is fully scrolled into view it holds there instead of
    // continuing into blank space, while the longer side keeps scrolling;
    // scrolling back the other way, both move together again as soon as
    // scrollX drops back under the shorter side's own max. The shared
    // scrollbar's own range (see .diff-hscroll) still goes all the way to
    // the *wider* side's real need, exactly like one shared scrollbar
    // under a real side-by-side code/diff viewer.
    const [scrollX, setScrollX] = useState(0);
    const [leftMaxScroll, setLeftMaxScroll] = useState(0);
    const [rightMaxScroll, setRightMaxScroll] = useState(0);
    const maxScroll = Math.max(leftMaxScroll, rightMaxScroll);
    const leftPaneRef = useRef<HTMLDivElement>(null);
    const leftInnerRef = useRef<HTMLDivElement>(null);
    const rightInnerRef = useRef<HTMLDivElement>(null);
    const hscrollRef = useRef<HTMLDivElement>(null);

    // Clicking the mod that already wins (and isn't manually forced) just
    // goes back to viewing it against what it overwrites - there's nothing
    // to stage.
    function stageChoice(modId: string) {
        setChoice(modId === conflict.Winner ? applied : modId);
    }

    async function applyChoice() {
        try {
            await runApply(
                "Saving your choice and re-checking every conflict against the load order. This window is locked until it's done.",
                async () => {
                    await SetPatchOverride(gameId, conflict.Type, conflict.ID, choice);
                    // Awaited: the refresh behind this is a full rescan of the
                    // game (seconds on a big modlist), and the overlay has to
                    // stay up until the new winner has actually come back.
                    await onOverrideChanged();
                },
            );
        } catch (err) {
            notify('error', `Couldn't apply your choice: ${String(err)}`);
        }
    }

    // What the right-hand pane shows is the contender the user is looking at
    // (the staged pick, or otherwise the current winner), and the left-hand
    // pane shows what it would replace or, if it already wins, what it
    // overwrites:
    //  - viewing a mod that isn't the current winner: left = the current
    //    winner, whose version this file would replace if applied;
    //  - viewing the current winner: left = the candidate right before it in
    //    load order (the one it's actually overriding), or the second one
    //    when it's first - rather than an arbitrary pair.
    const winnerIdx = conflict.Candidates.findIndex((c) => c.ModID === conflict.Winner);
    const viewId = choice !== '' ? choice : conflict.Winner;
    const viewIdx = conflict.Candidates.findIndex((c) => c.ModID === viewId);
    const viewingWinner = viewId === conflict.Winner;
    const againstIdx = !viewingWinner
        ? winnerIdx
        : (viewIdx > 0 ? viewIdx - 1 : (conflict.Candidates.length > 1 ? 1 : -1));
    const viewed = viewIdx >= 0 ? conflict.Candidates[viewIdx] : null;
    const against = againstIdx >= 0 ? conflict.Candidates[againstIdx] : null;

    useEffect(() => {
        let cancelled = false;
        setLeftContent(null);
        setRightContent(null);
        setLeftModified(null);
        setRightModified(null);
        setError('');
        setScrollX(0);
        if (hscrollRef.current) hscrollRef.current.scrollLeft = 0;
        if (against) {
            ReadModFile(gameId, against.ModID, against.FilePath)
                .then((f) => { if (!cancelled) { setLeftContent(f.Content); setLeftModified(f.ModifiedAt); } })
                .catch((err) => { if (!cancelled) setError(String(err)); });
        }
        if (viewed) {
            ReadModFile(gameId, viewed.ModID, viewed.FilePath)
                .then((f) => { if (!cancelled) { setRightContent(f.Content); setRightModified(f.ModifiedAt); } })
                .catch((err) => { if (!cancelled) setError(String(err)); });
        }
        return () => { cancelled = true; };
    }, [gameId, viewed?.ModID, viewed?.FilePath, against?.ModID, against?.FilePath]);

    // An empty file is zero lines, not one ('' split on '\n' would otherwise
    // report a single blank line, misclassifying it against the other side).
    // Split once per content, here, since the diff, both panes, and the
    // contender cards' line counts all need the same array.
    const leftLines = useMemo(() => splitLines(leftContent), [leftContent]);
    const rightLines = useMemo(() => splitLines(rightContent), [rightContent]);

    // Real line-level diffing (see data/lineDiff.ts) once both sides have
    // actually loaded. Deferred until after the browser has painted the
    // "Comparing..." state (see afterNextPaint) rather than computed inline
    // during render - the LCS table is O(n*m), so on a big pair of files the
    // synchronous version froze the window before that state could show.
    useEffect(() => {
        setDiff(null);
        if (!against || !viewed || leftLines === null || rightLines === null) return;
        return afterNextPaint(() => setDiff(diffLines(leftLines, rightLines)));
    }, [leftLines, rightLines]);

    // What each pane shows instead of its file while it isn't ready to: one
    // short line saying what's actually going on. Undefined means "show the
    // real content".
    const diffPending = against !== null && viewed !== null && diff === null;
    function pendingTextFor(lines: string[] | null): string | undefined {
        if (lines === null) return 'Reading the file from disk...';
        if (diffPending) return 'Comparing the two files...';
        return undefined;
    }
    // A side with no candidate at all (the one-mod-touches-this-key case)
    // has nothing to load, so it's never pending.
    const leftPending = against ? pendingTextFor(leftLines) : undefined;
    const rightPending = viewed ? pendingTextFor(rightLines) : undefined;
    const panesReady = !error && leftPending === undefined && rightPending === undefined;

    // Only the lines on screen are in the DOM (see useVirtualWindow), for the
    // same reason as the contested-keys list: a file can run to thousands of
    // lines, and building every line's syntax-highlighted DOM froze the whole
    // window - including the click on the next contested key - for as long as
    // that took. Both panes share one window so their rows stay level.
    const totalLines = Math.max(leftLines?.length ?? 0, rightLines?.length ?? 0);
    const {ref: diffBodyRef, onScroll: onDiffScroll, first: firstLine, last: lastLine} =
        useVirtualWindow<HTMLDivElement>(totalLines, diffLineHeight, 20);
    // How wide each side's longest line is, in monospace columns - each pane
    // reserves that much width up front (see ContentPane), so the shared
    // horizontal scrollbar's range doesn't change as different lines scroll in
    // and out of the rendered window.
    const leftCols = useMemo(() => maxColumns(leftLines), [leftLines]);
    const rightCols = useMemo(() => maxColumns(rightLines), [rightLines]);

    // Measure how far each pane can scroll horizontally. Re-run once the
    // panes actually exist (panesReady) - until then their inner elements
    // aren't in the DOM for the observer to attach to.
    useEffect(() => {
        function recompute() {
            const paneWidth = leftPaneRef.current?.clientWidth ?? 0;
            const leftWidth = leftInnerRef.current?.scrollWidth ?? 0;
            const rightWidth = rightInnerRef.current?.scrollWidth ?? 0;
            const leftMax = Math.max(0, leftWidth - paneWidth);
            const rightMax = Math.max(0, rightWidth - paneWidth);
            setLeftMaxScroll(leftMax);
            setRightMaxScroll(rightMax);
            setScrollX((x) => Math.min(x, Math.max(leftMax, rightMax)));
        }
        recompute();
        const observer = new ResizeObserver(recompute);
        if (leftPaneRef.current) observer.observe(leftPaneRef.current);
        if (leftInnerRef.current) observer.observe(leftInnerRef.current);
        if (rightInnerRef.current) observer.observe(rightInnerRef.current);
        return () => observer.disconnect();
    }, [leftContent, rightContent, panesReady]);

    return (
        <>
            <div className="contenders-col">
                <div className="col-header">CONTENDERS · LOAD ORDER</div>
                {conflict.PatchState === 'changed' && (
                    <div className="patch-stale-note">
                        <i className="fa-solid fa-circle-exclamation"/>
                        <div>
                            <b>The patch is out of date for this key.</b> {conflict.PatchNote} Check the files
                            below, then regenerate the patch to bring it up to date.
                        </div>
                    </div>
                )}
                {conflict.PatchState === 'new' && (
                    <div className="patch-stale-note muted">
                        <i className="fa-solid fa-circle-info"/>
                        <div>{conflict.PatchNote} Regenerate the patch to include it.</div>
                    </div>
                )}
                <div className="contenders-list">
                    {conflict.Candidates.map((c, i) => {
                        const wins = c.ModID === conflict.Winner;
                        const viewing = c.ModID === viewId;
                        const staged = dirty && choice === c.ModID;
                        const isLeft = against?.ModID === c.ModID;
                        const isRight = viewed?.ModID === c.ModID;
                        const loadedLines = isLeft ? leftLines : isRight ? rightLines : null;
                        const loadedModified = isLeft ? leftModified : isRight ? rightModified : null;
                        const metaText = loadedLines !== null && loadedModified !== null
                            ? `${loadedLines.length} line${loadedLines.length === 1 ? '' : 's'} · modified ${timeAgo(loadedModified)}`
                            : c.FilePath;
                        return (
                            <div
                                key={c.ModID}
                                className={`contender-card ${wins ? 'wins' : ''} ${viewing ? 'viewing' : ''}`}
                                title={wins
                                    ? "Currently wins this conflict - click to compare it with what it overwrites"
                                    : "Click to compare this file with the current winner's (nothing changes until you apply)"}
                                onClick={() => stageChoice(c.ModID)}
                            >
                                <div className="contender-head">
                                    <span className="mono pos">{i + 1}</span>
                                    <span className="name"><MarqueeText text={c.ModName}/></span>
                                    {wins && <span className="wins-badge">{conflict.Overridden ? 'WINS · MANUAL' : 'WINS'}</span>}
                                    {staged && <span className="selected-badge">SELECTED</span>}
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
                    <span className="resolution-option" onClick={() => setChoice('')}>
                        <span className={choice === '' ? 'radio-on' : 'radio-off'}>{choice === '' ? '●' : '○'}</span> Keep load-order winner
                    </span>
                    {conflict.Candidates.map((c) => {
                        const picked = choice === c.ModID;
                        return (
                            <span
                                key={c.ModID}
                                className="resolution-option"
                                onClick={() => setChoice(c.ModID)}
                            >
                                <span className={picked ? 'radio-on' : 'radio-off'}>{picked ? '●' : '○'}</span>
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
                    <div className={`resolution-hint ${dirty ? 'pending' : ''}`}>
                        {dirty
                            ? (choice === ''
                                ? "Your manual pick will be removed and the load order will decide the winner again. Nothing is saved until you apply."
                                : choice === conflict.Winner
                                    ? `${viewed?.ModName ?? 'This mod'} already wins - this locks it in as the winner even if the load order changes later. Nothing is saved until you apply.`
                                    : `${viewed?.ModName ?? 'This mod'} will win this key instead of ${against?.ModName ?? 'the current winner'}, so its file replaces that one. The comparison on the right shows exactly what changes. Nothing is saved until you apply.`)
                            : conflict.Candidates.length > 1
                                ? "Select a mod to compare its file with the current winner's. Nothing changes until you apply a choice."
                                : ''}
                    </div>
                    {dirty && (
                        <div className="resolution-actions">
                            <div className="btn-primary" onClick={applyChoice}>Apply</div>
                            <div className="btn-ghost" onClick={() => setChoice(applied)}>Cancel</div>
                        </div>
                    )}
                </div>
            </div>

            <div className="diff-col">
                <div className="diff-toolbar">
                    <span className="mono">{viewed?.FilePath ?? conflict.ID}</span>
                    <div className="spacer"/>
                    <span className="sort-label">Side by side</span>
                </div>
                {error && <p className="status-page error" style={{padding: 12}}>{error}</p>}
                {!error && (against || viewed) && (
                    <div className="diff-body mono" ref={diffBodyRef} onScroll={onDiffScroll}>
                        <div className={`diff-pane ${leftPending !== undefined ? 'loading' : ''}`} ref={leftPaneRef}>
                            {against
                                ? (
                                    <ContentPane
                                        label={`${against.ModName} (${viewingWinner ? 'loses' : 'current winner'})`}
                                        lines={leftLines}
                                        pendingText={leftPending}
                                        syntax={syntaxForPath(against.FilePath)}
                                        lineStates={diff?.leftMatched}
                                        changedClassName="removed"
                                        firstLine={firstLine}
                                        lastLine={lastLine}
                                        maxCols={leftCols}
                                        scrollX={Math.min(scrollX, leftMaxScroll)}
                                        innerRef={leftInnerRef}
                                    />
                                )
                                : <ContentPane label="Only one mod touches this key" lines={noLines} syntax="plain" firstLine={0} lastLine={1} maxCols={0}/>}
                        </div>
                        <div className={`diff-pane ${rightPending !== undefined ? 'loading' : ''}`}>
                            {viewed && (
                                <ContentPane
                                    label={`${viewed.ModName} (${viewingWinner ? 'wins' : 'selected'})`}
                                    lines={rightLines}
                                    pendingText={rightPending}
                                    syntax={syntaxForPath(viewed.FilePath)}
                                    lineStates={diff?.rightMatched}
                                    changedClassName="added"
                                    firstLine={firstLine}
                                    lastLine={lastLine}
                                    maxCols={rightCols}
                                    scrollX={Math.min(scrollX, rightMaxScroll)}
                                    innerRef={rightInnerRef}
                                />
                            )}
                        </div>
                    </div>
                )}
                {maxScroll > 0 && (
                    <div
                        className="diff-hscroll"
                        ref={hscrollRef}
                        onScroll={(e) => setScrollX((e.target as HTMLDivElement).scrollLeft)}
                    >
                        <div className="diff-hscroll-spacer" style={{width: `calc(100% + ${maxScroll}px)`}}/>
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
                            {!against
                                ? "Only one mod touches this key - there's nothing to diff against."
                                : diff?.skipped
                                    ? "This file is too large to diff line-by-line - showing its real, syntax-highlighted content as-is."
                                    : error ? "Couldn't load these files."
                                    : leftLines === null || rightLines === null ? 'Reading files...' : 'Comparing...'}
                        </span>
                    )}
                    <span
                        className={`resolved-toggle ${resolved ? 'active' : ''}`}
                        title={resolved ? 'Mark this key unresolved again' : "Mark this key as reviewed - doesn't change who wins it"}
                        onClick={() => onSetResolved(!resolved)}
                    >
                        <i className="fa-solid fa-check"/>
                        {resolved ? 'Marked done' : 'Mark done'}
                    </span>
                </div>
            </div>
        </>
    );
}

// One shared empty array, so the single-candidate fallback pane gets the same
// reference on every render (a fresh [] each time would look like new content).
const noLines: string[] = [];

// A file's real lines - an empty file is zero lines, not one, and null means
// it hasn't loaded yet.
function splitLines(content: string | null): string[] | null {
    if (content === null) return null;
    return content === '' ? noLines : content.split('\n');
}

// How wide a file's longest line is, in monospace columns. A tab counts as
// the distance to the next 8-column stop, the way white-space: pre renders it
// (Clausewitz files are usually tab-indented, so counting a tab as one column
// would badly under-reserve). Approximate for wide glyphs, which is fine -
// this only sizes the scroll range.
function maxColumns(lines: string[] | null): number {
    let max = 0;
    if (lines === null) return max;
    for (const line of lines) {
        let col = 0;
        for (let i = 0; i < line.length; i++) {
            col = line.charCodeAt(i) === 9 ? col + 8 - (col % 8) : col + 1;
        }
        if (col > max) max = col;
    }
    return max;
}

function ContentPane({label, lines, pendingText, syntax, lineStates, changedClassName, firstLine, lastLine, maxCols, scrollX, innerRef}: {
    label: string;
    // The file's real lines, or null while it hasn't loaded.
    lines: string[] | null;
    // Set while this pane can't show its file yet - what's actually going on,
    // in a few words ("Reading the file from disk...", "Comparing the two
    // files...", "Applying your choice...") - and swaps the file for a
    // spinner and that text. Unset means "show `lines`". A pane whose file
    // is still on its way (lines === null) shows the spinner regardless.
    pendingText?: string;
    syntax: FileSyntax;
    // Per real line in `lines`: true if that line is also present (in the
    // same relative order) in the other side's file, false if it's only here
    // - see data/lineDiff.ts. Omitted (undefined) means "don't tint
    // anything," either because there's nothing to diff against (only one
    // mod touches this key) or because the diff was skipped for being too
    // large - either way this pane still shows its own real, syntax-
    // highlighted content, just without the added/removed tint.
    lineStates?: boolean[];
    // Applied to a line whose lineStates entry is false - 'removed' for
    // the losing side, 'added' for the winning side.
    changedClassName?: 'removed' | 'added';
    // The slice of `lines` [firstLine, lastLine) that's actually on screen
    // and so actually rendered - everything else is just reserved height (see
    // useVirtualWindow).
    firstLine: number;
    lastLine: number;
    // Width of this file's longest line in monospace columns (see
    // maxColumns), reserved up front for the shared horizontal scrollbar.
    maxCols: number;
    // The one shared horizontal scroll position both panes apply to their
    // own real code (see ContendersAndContent's own comment on scrollX) -
    // 0 when omitted, for the single-candidate fallback pane that has no
    // scrollable content in the first place.
    scrollX?: number;
    // Measures this pane's own real, unscrolled content width, so
    // ContendersAndContent can work out how far there is to scroll -
    // omitted for that same fallback pane.
    innerRef?: { current: HTMLDivElement | null };
}) {
    const loading = lines === null || pendingText !== undefined;
    return (
        <>
            <div className="file-item" style={{borderLeft: 'none', padding: '5px 11px'}}>
                <span className="note">{label}</span>
            </div>
            {loading && (
                <div className="diff-loading">
                    <i className="fa-solid fa-spinner fa-spin"/>
                    <span className="diff-loading-title">Loading file...</span>
                    {pendingText && <span className="diff-loading-info">{pendingText}</span>}
                </div>
            )}
            {!loading && lines !== null && lines.length === 0 && (
                <div className="diff-line"><span className="ln"/><span/></div>
            )}
            {!loading && lines !== null && lines.length > 0 && (
                <div
                    className="diff-pane-inner"
                    ref={innerRef}
                    style={{
                        height: `${lines.length * diffLineHeight}px`,
                        paddingTop: `${firstLine * diffLineHeight}px`,
                        transform: `translateX(-${scrollX ?? 0}px)`,
                        '--diff-cols': maxCols,
                    } as h.JSX.CSSProperties}
                >
                    {lines.slice(firstLine, lastLine).map((line, k) => {
                        const i = firstLine + k;
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
                </div>
            )}
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

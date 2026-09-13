import './Workspace.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {
    GetPreferences,
    LaunchGame,
    ListModFiles,
    ListPlaysets,
    LoadPlayset,
    ModThumbnail,
    OpenModFolder,
    SavePlayset,
    ScanGame,
    WatchMods,
} from '../../wailsjs/go/main/App';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import type {library, playset, preferences} from '../../wailsjs/go/models';
import {autosort} from '../data/autosort';
import {domains, preflight} from '../data/mockData';
import {SourceBadge} from '../components/SourceBadge';
import {ConflictResolver} from './ConflictResolver';
import {PlaysetsModal} from './PlaysetsModal';
import {PreflightModal} from './PreflightModal';

type Status =
    | { kind: 'idle' }
    | { kind: 'busy'; message: string }
    | { kind: 'error'; message: string };

type DetailTab = 'overview' | 'files' | 'conflicts' | 'changes';

export function Workspace({games, selectedGame, onPlaysetNameChange, onOpenUpdates}: {
    games: library.GameInfo[];
    selectedGame: string;
    onPlaysetNameChange: (name: string) => void;
    onOpenUpdates: () => void;
}) {
    const [summary, setSummary] = useState<library.Summary | null>(null);
    const [order, setOrder] = useState<string[]>([]);
    const [selectedId, setSelectedId] = useState('');
    const [selectedAvailable, setSelectedAvailable] = useState<Set<string>>(new Set());
    const [detailTab, setDetailTab] = useState<DetailTab>('overview');
    const [playsetName, setPlaysetNameState] = useState('');
    const [playsetList, setPlaysetList] = useState<string[]>([]);
    const [showPlaysets, setShowPlaysets] = useState(false);
    const [showPreflight, setShowPreflight] = useState(false);
    const [showConflictResolver, setShowConflictResolver] = useState(false);
    const [search, setSearch] = useState('');
    const [status, setStatus] = useState<Status>({kind: 'idle'});
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    // Guards the 'scan-quick' listener below so it only ever applies to the
    // fresh load it belongs to - a watcher-triggered background refresh
    // (refreshMods(true)) runs the exact same backend scan and fires the
    // same event, but must never have its quick preview reset the load
    // order/selection the way a fresh load's first paint is supposed to.
    const expectingFreshQuickRef = useRef(false);
    const quickAppliedRef = useRef(false);

    function setPlaysetName(name: string) {
        setPlaysetNameState(name);
        onPlaysetNameChange(name);
    }

    // refreshMods re-fetches the mod summary for the current game.
    // preserveSelection=false is a real game switch (a full reset: clear
    // the playset name and Available selection, rebuild the load order
    // from scratch - normally already done early by the 'scan-quick'
    // preview below, this is just the fallback if that never arrives).
    // preserveSelection=true is a background refresh triggered by the
    // mod-folder watcher, or the final result landing after an already-
    // applied quick preview: either way, only prune IDs that no longer
    // exist from the current load order and Available selection, leaving
    // everything else exactly as the user left it.
    async function refreshMods(preserveSelection: boolean) {
        if (!preserveSelection) {
            setPlaysetName('');
            setSelectedAvailable(new Set());
            setStatus({kind: 'busy', message: 'Scanning...'});
            expectingFreshQuickRef.current = true;
            quickAppliedRef.current = false;
        }
        try {
            const result = await ScanGame(selectedGame, '');
            expectingFreshQuickRef.current = false;
            setSummary(result);
            if (preserveSelection || quickAppliedRef.current) {
                const freshIds = new Set(result.Mods.map((m) => m.ID));
                setOrder((prev) => prev.filter((id) => freshIds.has(id)));
                setSelectedAvailable((prev) => new Set([...prev].filter((id) => freshIds.has(id))));
            } else {
                setOrder(result.Mods.filter((m) => m.Enabled).map((m) => m.ID));
            }
            if (!preserveSelection) {
                setStatus({kind: 'idle'});
            }
        } catch (err) {
            expectingFreshQuickRef.current = false;
            if (!preserveSelection) {
                setStatus({kind: 'error', message: String(err)});
            }
        }
    }

    useEffect(() => {
        if (!selectedGame) {
            return;
        }
        WatchMods(selectedGame).catch(() => undefined);
        refreshMods(false);
        ListPlaysets(selectedGame)
            .then(setPlaysetList)
            .catch((err) => setStatus({kind: 'error', message: String(err)}));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    useEffect(() => {
        if (!selectedGame) {
            return;
        }
        const unsubscribe = EventsOn('mods-changed', (gameId: string) => {
            if (gameId === selectedGame) {
                refreshMods(true);
            }
        });
        return () => unsubscribe();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    // A scan's mod list (names/versions/sources) is known well before
    // conflict detection's slower per-mod parsing finishes - this preview
    // arrives first so the list appears immediately instead of staying
    // hidden behind a blocking "Scanning..." state. Conflicts populate
    // once the ScanGame call above actually resolves.
    useEffect(() => {
        if (!selectedGame) {
            return;
        }
        const unsubscribe = EventsOn('scan-quick', (gameId: string, quick: library.Summary) => {
            if (gameId !== selectedGame || !expectingFreshQuickRef.current) {
                return;
            }
            setSummary(quick);
            setOrder(quick.Mods.filter((m) => m.Enabled).map((m) => m.ID));
            setStatus({kind: 'busy', message: 'Resolving conflicts...'});
            quickAppliedRef.current = true;
        });
        return () => unsubscribe();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    const allMods = summary?.Mods ?? [];
    const modsById = useMemo(() => {
        const m = new Map<string, library.ModSummary>();
        for (const mod of allMods) {
            m.set(mod.ID, mod);
        }
        return m;
    }, [allMods]);

    const conflictedIds = useMemo(() => {
        const s = new Set<string>();
        for (const c of summary?.Conflicts ?? []) {
            for (const cand of c.Candidates) {
                s.add(cand.ModID);
            }
        }
        return s;
    }, [summary]);

    const orderSet = useMemo(() => new Set(order), [order]);
    const available = allMods.filter((m) => !orderSet.has(m.ID) && matchesSearch(m, search));
    const active = order.map((id) => modsById.get(id)).filter((m): m is library.ModSummary => !!m);
    const selectedMod = selectedId ? modsById.get(selectedId) ?? null : null;

    const workshopCount = allMods.filter((m) => m.Source === 'workshop').length;
    const localCount = allMods.filter((m) => m.Source !== 'workshop').length;

    function toggleAvailable(id: string) {
        const next = new Set(selectedAvailable);
        if (next.has(id)) next.delete(id); else next.add(id);
        setSelectedAvailable(next);
    }

    function addSelectedToOrder() {
        if (selectedAvailable.size === 0) return;
        setOrder([...order, ...allMods.filter((m) => selectedAvailable.has(m.ID)).map((m) => m.ID)]);
        setSelectedAvailable(new Set());
    }

    function removeFromOrder(id: string) {
        setOrder(order.filter((x) => x !== id));
    }

    function moveInOrder(id: string, delta: number) {
        const i = order.indexOf(id);
        const j = i + delta;
        if (i < 0 || j < 0 || j >= order.length) return;
        const next = order.slice();
        [next[i], next[j]] = [next[j], next[i]];
        setOrder(next);
    }

    function handleAutosort() {
        if (!prefs || order.length === 0) return;
        const result = autosort(order, modsById, {
            dependencies: prefs.autosortDependencies,
            fixesLast: prefs.autosortFixesLast,
        });
        setOrder(result.order);
        if (result.cycleMods.length > 0) {
            const names = result.cycleMods.map((id) => modsById.get(id)?.Name ?? id).join(', ');
            setStatus({kind: 'error', message: `Autosort: circular dependency involving ${names} - these couldn't be fully ordered.`});
        }
    }

    async function refreshAfterSave(name: string) {
        const names = await ListPlaysets(selectedGame);
        setPlaysetList(names);
        const result = await ScanGame(selectedGame, name);
        setSummary(result);
    }

    async function handleSave() {
        if (!playsetName.trim()) return;
        setStatus({kind: 'busy', message: 'Saving...'});
        try {
            const p = {name: playsetName.trim(), gameKey: selectedGame, modIds: order, disabledDlc: []} as playset.Playset;
            await SavePlayset(p);
            await refreshAfterSave(p.name);
            setStatus({kind: 'idle'});
        } catch (err) {
            setStatus({kind: 'error', message: String(err)});
        }
    }

    async function handleLoadPlayset(name: string) {
        setStatus({kind: 'busy', message: 'Loading playset...'});
        try {
            const p = await LoadPlayset(selectedGame, name);
            setOrder(p.modIds ?? []);
            setPlaysetName(p.name);
            const result = await ScanGame(selectedGame, p.name);
            setSummary(result);
            setShowPlaysets(false);
            setStatus({kind: 'idle'});
        } catch (err) {
            setStatus({kind: 'error', message: String(err)});
        }
    }

    async function handleLaunchAnyway() {
        if (!playsetName.trim()) return;
        setShowPreflight(false);
        setStatus({kind: 'busy', message: 'Launching...'});
        try {
            const p = {name: playsetName.trim(), gameKey: selectedGame, modIds: order, disabledDlc: []} as playset.Playset;
            await SavePlayset(p);
            await LaunchGame(selectedGame, p.name);
            setStatus({kind: 'idle'});
        } catch (err) {
            setStatus({kind: 'error', message: String(err)});
        }
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;

    return (
        <div className="workspace">
            {status.kind === 'busy' && <p className="workspace-status">{status.message}</p>}
            {status.kind === 'error' && <p className="workspace-status error">{status.message}</p>}

            {summary && (
                <div className="workspace-body">
                    <DetailPanel
                        mod={selectedMod}
                        tab={detailTab}
                        onTab={setDetailTab}
                        onOpenResolver={() => setShowConflictResolver(true)}
                        gameId={selectedGame}
                        allMods={allMods}
                        conflicts={summary?.Conflicts ?? []}
                        onError={(message) => setStatus({kind: 'error', message})}
                    />

                    <div className="list-pane">
                        <div className="list-pane-header">
                            <div className="list-pane-title-row">
                                <span className="title">AVAILABLE</span>
                                <span className="count mono">{available.length}</span>
                                <div className="spacer"/>
                                <span className="sort-label">Sort: Name <i className="fa-solid fa-chevron-down"/></span>
                            </div>
                            <div className="search-box">
                                <i className="fa-solid fa-magnifying-glass"/>
                                <input
                                    placeholder={`Search ${available.length} mods...`}
                                    value={search}
                                    onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                                />
                                <span className="kbd">⌘K</span>
                            </div>
                            <div className="filter-chips">
                                <span className="chip chip-active">Workshop {workshopCount}</span>
                                <span className="chip">Local {localCount}</span>
                                <span className="chip chip-inert">Outdated</span>
                                <span className="chip chip-inert">Never used</span>
                                <span className="chip chip-inert">+ Filter</span>
                            </div>
                        </div>
                        <div className="list-rows">
                            {available.map((m) => (
                                <div key={m.ID} className={`mod-row ${m.ID === selectedId ? 'selected' : ''}`} onClick={() => setSelectedId(m.ID)}>
                                    <input
                                        type="checkbox"
                                        checked={selectedAvailable.has(m.ID)}
                                        onClick={(e) => e.stopPropagation()}
                                        onChange={() => toggleAvailable(m.ID)}
                                    />
                                    <SourceBadge source={m.Source} name={m.Name}/>
                                    <span className="name">{m.Name}</span>
                                    <span className="ver mono">{m.Version || '-'}</span>
                                    <span className="author mono">-</span>
                                </div>
                            ))}
                        </div>
                        <div className="list-pane-footer">
                            <span className="mono">{selectedAvailable.size} selected</span>
                            <span className="link-btn" onClick={addSelectedToOrder}>Add to load order <i className="fa-solid fa-arrow-right"/></span>
                        </div>
                    </div>

                    <div className="list-pane">
                        <div className="list-pane-header">
                            <div className="list-pane-title-row">
                                <span className="title">ACTIVE LOAD ORDER</span>
                                <span className="count mono">{active.length}</span>
                                <div className="spacer"/>
                                <span className="sort-label">Bottom wins <i className="fa-solid fa-chevron-down"/></span>
                            </div>
                            {conflictedIds.size > 0 && (
                                <div className="conflict-banner">
                                    <span className="dot"/>
                                    <span className="text">{conflictedIds.size} mods in hard conflicts</span>
                                    <span className="link-btn amber" onClick={() => setShowConflictResolver(true)}>Resolve</span>
                                </div>
                            )}
                            <div className="active-actions">
                                <span className="btn-amber" onClick={handleAutosort}><i className="fa-solid fa-arrow-down-arrow-up"/> Autosort</span>
                                <span className="btn-ghost">Validate</span>
                                <span className="btn-ghost" onClick={() => setOrder([])}>Clear</span>
                            </div>
                        </div>
                        <div className="domain-header">
                            <span className="label">OVERLAP BY DOMAIN <i className="fa-solid fa-arrow-right"/></span>
                            <span className="domain-cols">
                                {domains.map((d) => <span key={d}>{d}</span>)}
                            </span>
                        </div>
                        <div className="list-rows">
                            {active.map((m, i) => {
                                const conflicted = conflictedIds.has(m.ID);
                                return (
                                    <div
                                        key={m.ID}
                                        className={`mod-row active-row ${m.ID === selectedId ? 'selected' : ''} ${conflicted ? 'has-conflict' : ''}`}
                                        onClick={() => setSelectedId(m.ID)}
                                    >
                                        <span className="position mono">{i + 1}</span>
                                        <i className="fa-solid fa-grip-vertical drag-handle"/>
                                        <span className="name">{m.Name}</span>
                                        <span className="domain-segments">
                                            {domains.map((d) => <span key={d} className="segment clean"/>)}
                                        </span>
                                        <span className={`flag mono ${conflicted ? 'conflict' : ''}`}>{conflicted ? 'CONF' : ''}</span>
                                        <span className="row-actions">
                                            <i className="fa-solid fa-chevron-up" onClick={(e) => { e.stopPropagation(); moveInOrder(m.ID, -1); }}/>
                                            <i className="fa-solid fa-chevron-down" onClick={(e) => { e.stopPropagation(); moveInOrder(m.ID, 1); }}/>
                                            <i className="fa-solid fa-xmark" onClick={(e) => { e.stopPropagation(); removeFromOrder(m.ID); }}/>
                                        </span>
                                    </div>
                                );
                            })}
                        </div>
                        <div className="active-legend">
                            <div className="legend-keys">
                                <span><span className="swatch overwritten"/>overwritten</span>
                                <span><span className="swatch partial"/>partial</span>
                                <span><span className="swatch clean"/>clean</span>
                            </div>
                            <div className="legend-caption mono">C common · E events · G gfx · I interface · L localisation · M map</div>
                        </div>
                    </div>

                    <aside className="actions-rail">
                        <div className="rail-section">
                            <div className="rail-label">PLAYSET</div>
                            <div className="playset-card">
                                <input
                                    className="playset-name-input"
                                    placeholder="Playset name"
                                    value={playsetName}
                                    onInput={(e) => setPlaysetName((e.target as HTMLInputElement).value)}
                                />
                                <div className="mono meta">{active.length} mods</div>
                                <div className="playset-card-actions">
                                    <span className="btn-ghost" onClick={handleSave}>Save</span>
                                    <span className="btn-ghost" onClick={() => setShowPlaysets(true)}>Switch</span>
                                    <span className="btn-ghost inert">Share <i className="fa-solid fa-arrow-up-right-from-square"/></span>
                                </div>
                            </div>
                        </div>

                        <div className="rail-section">
                            <div className="rail-label">PRE-FLIGHT</div>
                            <div className="preflight-mini">
                                {preflight.map((p) => (
                                    <div key={p.title} className="preflight-mini-row">
                                        <i className={`fa-solid ${p.icon}`} style={{color: p.c}}/>
                                        <span>{p.title}</span>
                                    </div>
                                ))}
                            </div>
                        </div>

                        <div className="rail-section">
                            <div className="rail-label">UPDATES</div>
                            <div className="updates-card">
                                <span>9 available</span>
                                <span className="link-btn amber" onClick={onOpenUpdates}>Review</span>
                            </div>
                        </div>

                        <div className="rail-spacer"/>

                        <div className="play-block">
                            <button
                                className="play-button"
                                disabled={!playsetName.trim()}
                                onClick={() => setShowPreflight(true)}
                            >
                                <div className="play-title">PLAY {gameName.toUpperCase()}</div>
                                <div className="play-subtitle mono">{active.length} mods · launch via Steam</div>
                            </button>
                            <div className="play-secondary">
                                <span className="btn-ghost inert">Vanilla</span>
                                <span className="btn-ghost inert">Export log</span>
                            </div>
                        </div>
                    </aside>
                </div>
            )}

            {showPlaysets && (
                <PlaysetsModal
                    gameName={gameName}
                    names={playsetList}
                    onActivate={handleLoadPlayset}
                    onNew={() => { setOrder([]); setPlaysetName(''); setShowPlaysets(false); }}
                    onClose={() => setShowPlaysets(false)}
                />
            )}

            {showPreflight && (
                <PreflightModal
                    gameName={gameName}
                    modCount={active.length}
                    onFixConflicts={() => { setShowPreflight(false); setShowConflictResolver(true); }}
                    onLaunchAnyway={handleLaunchAnyway}
                    onClose={() => setShowPreflight(false)}
                />
            )}

            {showConflictResolver && (
                <ConflictResolver
                    gameId={selectedGame}
                    conflicts={summary?.Conflicts ?? []}
                    order={order}
                    onClose={() => setShowConflictResolver(false)}
                    onPatchGenerated={(modId) => {
                        setOrder((prev) => (prev.includes(modId) ? prev : [...prev, modId]));
                        refreshMods(true);
                    }}
                />
            )}
        </div>
    );
}

function matchesSearch(m: library.ModSummary, search: string): boolean {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    return m.Name.toLowerCase().includes(q) || m.ID.toLowerCase().includes(q);
}

function DetailPanel({mod, tab, onTab, onOpenResolver, gameId, allMods, conflicts, onError}: {
    mod: library.ModSummary | null;
    tab: DetailTab;
    onTab: (t: DetailTab) => void;
    onOpenResolver: () => void;
    gameId: string;
    allMods: library.ModSummary[];
    conflicts: library.ConflictSummary[];
    onError: (message: string) => void;
}) {
    const [files, setFiles] = useState<library.ModFiles | null>(null);
    const [filesError, setFilesError] = useState('');
    // null = still loading (or nothing selected); '' = loaded, confirmed no
    // thumbnail; anything else = a real data: URI. Kept distinct from ''
    // so the box can show a loading placeholder instead of momentarily
    // flashing "NO THUMBNAIL" every time the selected mod changes.
    const [thumbnail, setThumbnail] = useState<string | null>(null);

    useEffect(() => {
        setFiles(null);
        setFilesError('');
        if (!mod) {
            return;
        }
        let cancelled = false;
        ListModFiles(gameId, mod.ID)
            .then((f) => { if (!cancelled) setFiles(f); })
            .catch((err) => { if (!cancelled) setFilesError(String(err)); });
        return () => { cancelled = true; };
    }, [gameId, mod?.ID]);

    useEffect(() => {
        setThumbnail(null);
        if (!mod) {
            return;
        }
        let cancelled = false;
        ModThumbnail(gameId, mod.ID)
            .then((src) => { if (!cancelled) setThumbnail(src); })
            .catch(() => { if (!cancelled) setThumbnail(''); });
        return () => { cancelled = true; };
    }, [gameId, mod?.ID]);

    const filesLoading = !!mod && !files && !filesError;

    function openFolder() {
        if (mod) {
            OpenModFolder(gameId, mod.ID).catch((err) => onError(String(err)));
        }
    }

    const myConflicts = mod ? conflicts.filter((c) => c.Candidates.some((cand) => cand.ModID === mod.ID)) : [];

    return (
        <div className="detail-panel">
            <div className="thumbnail">
                {mod && thumbnail === null
                    ? <div className="skeleton thumbnail-skeleton"/>
                    : thumbnail
                        ? <>
                            <div className="thumbnail-backdrop" style={{backgroundImage: `url(${thumbnail})`}}/>
                            <img className="thumbnail-fg" src={thumbnail} alt={mod ? `${mod.Name} thumbnail` : ''}/>
                        </>
                        : <span className="mono">{mod ? 'NO THUMBNAIL' : 'MOD THUMBNAIL'}</span>}
            </div>
            {!mod && <p className="detail-empty">Select a mod to see its details.</p>}
            {mod && (
                <>
                    <div className="detail-header">
                        <div className="detail-badges">
                            <SourceBadge source={mod.Source} name={mod.Name}/>
                            <span className="mono id">{mod.Source === 'workshop' && mod.RemoteFileID ? mod.RemoteFileID : mod.ID}</span>
                        </div>
                        <div className="detail-name">{mod.Name}</div>
                        <div className="detail-sub">
                            {filesLoading
                                ? <span className="skeleton skeleton-text" style={{width: '90px'}}/>
                                : files?.LastModified ? `Updated ${timeAgo(files.LastModified)}` : ''}
                        </div>
                    </div>
                    <div className="detail-tabs">
                        {(['overview', 'files', 'conflicts', 'changes'] as DetailTab[]).map((t) => (
                            <span key={t} className={`detail-tab ${tab === t ? 'active' : ''}`} onClick={() => onTab(t)}>
                                {t[0].toUpperCase() + t.slice(1)}
                            </span>
                        ))}
                    </div>
                    <div className="detail-content">
                        {tab === 'overview' && (
                            <OverviewTab mod={mod} files={files} filesLoading={filesLoading} allMods={allMods} conflicts={myConflicts} onOpenFolder={openFolder}/>
                        )}
                        {tab === 'files' && (
                            <div className="file-tree">
                                {filesError && <p className="status-page error">{filesError}</p>}
                                {!filesError && !files && (
                                    <>
                                        {[80, 60, 70, 50, 65].map((w, i) => (
                                            <div key={i} className="file-row">
                                                <span className="skeleton" style={{width: '10px', height: '10px'}}/>
                                                <span className="skeleton skeleton-text" style={{width: `${w}px`}}/>
                                            </div>
                                        ))}
                                    </>
                                )}
                                {!filesError && files && files.Entries.length === 0 && (
                                    <p className="detail-empty">This mod's content folder is empty.</p>
                                )}
                                {files?.Entries.map((f) => (
                                    <div key={f.RelPath} className="file-row" style={{paddingLeft: `${(f.RelPath.split('/').length - 1) * 14 + 12}px`}}>
                                        <i className={`fa-solid ${f.IsDir ? 'fa-folder' : 'fa-file'}`}/>
                                        <span className="mono name">{f.RelPath.split('/').pop()}</span>
                                        {!f.IsDir && <span className="mono badge">{formatBytes(f.Size)}</span>}
                                    </div>
                                ))}
                                {files?.Truncated && (
                                    <p className="detail-sub" style={{padding: '8px 12px'}}>
                                        Showing the first {files.Entries.length.toLocaleString()} entries - this mod has more.
                                    </p>
                                )}
                            </div>
                        )}
                        {tab === 'conflicts' && (
                            <div className="conflicts-tab">
                                <div className="conflict-stats">
                                    <div>
                                        <div className="stat-num" style={{color: myConflicts.length ? '#d4574e' : '#5fae7e'}}>{myConflicts.length}</div>
                                        <div className="stat-label">contested {myConflicts.length === 1 ? 'key' : 'keys'}</div>
                                    </div>
                                </div>
                                {myConflicts.length === 0 && (
                                    <p className="detail-empty">No genuine conflicts detected for this mod.</p>
                                )}
                                {myConflicts.length > 0 && (
                                    <div className="section">
                                        <div className="section-label">CONFLICTS WITH</div>
                                        {myConflicts.map((c) => (
                                            <div key={c.Type + c.ID} className="loses-row">
                                                <div className="loses-head">
                                                    <span className="mono">{c.Type}: {c.ID}</span>
                                                    {c.Winner === mod.ID
                                                        ? <span className="wins-badge">WINS</span>
                                                        : <span className="mono note">loses</span>}
                                                </div>
                                                <div className="section-body">
                                                    vs. {c.Candidates.filter((cand) => cand.ModID !== mod.ID).map((cand) => cand.ModName).join(', ')}
                                                </div>
                                            </div>
                                        ))}
                                    </div>
                                )}
                                <span className="resolver-btn" onClick={onOpenResolver}>Open in resolver</span>
                            </div>
                        )}
                        {tab === 'changes' && (
                            <div className="changes-tab">
                                <p className="detail-empty">
                                    Update history isn't available - Parallax Mod Manager doesn't fetch anything from
                                    Steam Workshop, so this mod's own change log isn't something it can show.
                                </p>
                            </div>
                        )}
                    </div>
                </>
            )}
        </div>
    );
}

function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = n / 1024;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value.toFixed(value < 10 ? 2 : 1)} ${units[unit]}`;
}

function timeAgo(unixSeconds: number): string {
    const seconds = Math.max(0, Date.now() / 1000 - unixSeconds);
    const units: [number, string][] = [
        [31536000, 'year'], [2592000, 'month'], [86400, 'day'],
        [3600, 'hour'], [60, 'minute'],
    ];
    for (const [secs, label] of units) {
        const n = Math.floor(seconds / secs);
        if (n >= 1) return `${n} ${label}${n === 1 ? '' : 's'} ago`;
    }
    return 'just now';
}

function OverviewTab({mod, files, filesLoading, allMods, conflicts, onOpenFolder}: {
    mod: library.ModSummary;
    files: library.ModFiles | null;
    filesLoading: boolean;
    allMods: library.ModSummary[];
    conflicts: library.ConflictSummary[];
    onOpenFolder: () => void;
}) {
    const knownNames = useMemo(() => new Set(allMods.map((m) => m.Name)), [allMods]);
    const workshopUrl = mod.Source === 'workshop' && mod.RemoteFileID
        ? `https://steamcommunity.com/sharedfiles/filedetails/?id=${mod.RemoteFileID}`
        : '';

    return (
        <>
            <div className="overview-grid">
                <span className="label">Version</span><span className="value mono">{mod.Version || '-'}</span>
                <span className="label">Supports</span><span className="value mono ok">{mod.SupportedVersion || '-'}</span>
                <span className="label">Size</span>
                <span className="value mono">
                    {files
                        ? `${formatBytes(files.TotalSize)} · ${files.Entries.length.toLocaleString()}${files.Truncated ? '+' : ''} files`
                        : filesLoading
                            ? <span className="skeleton skeleton-text" style={{width: '110px'}}/>
                            : '-'}
                </span>
                <span className="label">Tags</span><span className="value">{mod.Tags.length ? mod.Tags.join(', ') : '-'}</span>
            </div>
            <div className="section">
                <div className="section-label">DESCRIPTION</div>
                <div className="section-body">{mod.ShortDescription || 'No description provided.'}</div>
            </div>
            <div className="section">
                <div className="section-label">REQUIRES</div>
                {mod.Dependencies.length === 0 && <div className="section-body">This mod declares no dependencies.</div>}
                {mod.Dependencies.map((name) => {
                    const found = knownNames.has(name);
                    return (
                        <div key={name} className="requires-row">
                            <span style={{color: found ? '#5fae7e' : '#e0a340'}}>●</span>{name}
                            <span className="mono note" style={{color: found ? '#5fae7e' : '#e0a340'}}>
                                {found ? 'found' : 'not found'}
                            </span>
                        </div>
                    );
                })}
            </div>
            {conflicts.length > 0 && (
                <div className="section">
                    <div className="section-label">CONFLICTS · {conflicts.length} {conflicts.length === 1 ? 'KEY' : 'KEYS'}</div>
                    <div className="section-body">See the Conflicts tab for details.</div>
                </div>
            )}
            <div className="detail-footer-actions">
                <span className="btn-ghost" onClick={onOpenFolder}>Open folder</span>
                {workshopUrl
                    ? <span className="btn-ghost" onClick={() => BrowserOpenURL(workshopUrl)}>Workshop page</span>
                    : <span className="btn-ghost inert">Workshop page</span>}
            </div>
        </>
    );
}


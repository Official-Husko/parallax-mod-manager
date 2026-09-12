import './Workspace.css';
import {h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {
    LaunchGame,
    ListPlaysets,
    LoadPlayset,
    SavePlayset,
    ScanGame,
    WatchMods,
} from '../../wailsjs/go/main/App';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import type {library, playset} from '../../wailsjs/go/models';
import {
    byDomain,
    changelog,
    domains,
    fileTree,
    losesTo,
    preflight,
    workspaceStaticDetail,
} from '../data/mockData';
import {PlaysetsModal} from './PlaysetsModal';
import {PreflightModal} from './PreflightModal';

type Status =
    | { kind: 'idle' }
    | { kind: 'busy'; message: string }
    | { kind: 'error'; message: string };

type DetailTab = 'overview' | 'files' | 'conflicts' | 'changes';

export function Workspace({games, selectedGame, onPlaysetNameChange, onOpenConflictResolver, onOpenUpdates}: {
    games: library.GameInfo[];
    selectedGame: string;
    onPlaysetNameChange: (name: string) => void;
    onOpenConflictResolver: () => void;
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
    const [search, setSearch] = useState('');
    const [status, setStatus] = useState<Status>({kind: 'idle'});

    function setPlaysetName(name: string) {
        setPlaysetNameState(name);
        onPlaysetNameChange(name);
    }

    // refreshMods re-fetches the mod summary for the current game.
    // preserveSelection=false is a real game switch (today's full reset:
    // clear the playset name and Available selection, rebuild the load
    // order from scratch). preserveSelection=true is a background refresh
    // triggered by the mod-folder watcher: it only prunes IDs that no
    // longer exist from the current load order and Available selection,
    // leaving everything else exactly as the user left it.
    async function refreshMods(preserveSelection: boolean) {
        if (!preserveSelection) {
            setPlaysetName('');
            setSelectedAvailable(new Set());
            setStatus({kind: 'busy', message: 'Scanning...'});
        }
        try {
            const result = await ScanGame(selectedGame, '');
            setSummary(result);
            if (preserveSelection) {
                const freshIds = new Set(result.Mods.map((m) => m.ID));
                setOrder((prev) => prev.filter((id) => freshIds.has(id)));
                setSelectedAvailable((prev) => new Set([...prev].filter((id) => freshIds.has(id))));
            } else {
                setOrder(result.Mods.filter((m) => m.Enabled).map((m) => m.ID));
                setStatus({kind: 'idle'});
            }
        } catch (err) {
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
            for (const id of c.Candidates) {
                s.add(id);
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
                    <DetailPanel mod={selectedMod} tab={detailTab} onTab={setDetailTab} onOpenResolver={onOpenConflictResolver}/>

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
                                <div key={m.ID} className="mod-row" onClick={() => setSelectedId(m.ID)}>
                                    <input
                                        type="checkbox"
                                        checked={selectedAvailable.has(m.ID)}
                                        onClick={(e) => e.stopPropagation()}
                                        onChange={() => toggleAvailable(m.ID)}
                                    />
                                    <span className={`src-badge ${sourceBadgeClass(m.Source)}`}>{sourceBadgeLabel(m.Source)}</span>
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
                                    <span className="link-btn amber" onClick={onOpenConflictResolver}>Resolve</span>
                                </div>
                            )}
                            <div className="active-actions">
                                <span className="btn-amber"><i className="fa-solid fa-arrow-down-arrow-up"/> Autosort</span>
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
                    onFixConflicts={() => { setShowPreflight(false); onOpenConflictResolver(); }}
                    onLaunchAnyway={handleLaunchAnyway}
                    onClose={() => setShowPreflight(false)}
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

function DetailPanel({mod, tab, onTab, onOpenResolver}: {
    mod: library.ModSummary | null;
    tab: DetailTab;
    onTab: (t: DetailTab) => void;
    onOpenResolver: () => void;
}) {
    return (
        <div className="detail-panel">
            <div className="thumbnail">
                <span className="mono">{workspaceStaticDetail.thumbnailLabel}</span>
            </div>
            {!mod && <p className="detail-empty">Select a mod to see its details.</p>}
            {mod && (
                <>
                    <div className="detail-header">
                        <div className="detail-badges">
                            <span className={`src-badge ${sourceBadgeClass(mod.Source)}`}>{sourceBadgeLabel(mod.Source)}</span>
                            <span className="mono id">{mod.ID}</span>
                        </div>
                        <div className="detail-name">{mod.Name}</div>
                        <div className="detail-sub">{workspaceStaticDetail.updatedLine}</div>
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
                            <OverviewTab mod={mod}/>
                        )}
                        {tab === 'files' && (
                            <div className="file-tree">
                                {fileTree.map((f, ix) => (
                                    <div key={ix} className="file-row" style={{paddingLeft: `${f.pad}px`}}>
                                        {f.icon && <i className={`fa-solid ${f.icon}`}/>}
                                        <span className="mono name" style={{color: f.c}}>{f.name}</span>
                                        <span className="mono badge" style={{color: f.badgeC}}>{f.badge}</span>
                                    </div>
                                ))}
                            </div>
                        )}
                        {tab === 'conflicts' && (
                            <div className="conflicts-tab">
                                <div className="conflict-stats">
                                    <div><div className="stat-num" style={{color: '#d4574e'}}>481</div><div className="stat-label">files contested</div></div>
                                    <div><div className="stat-num" style={{color: '#e0a340'}}>3</div><div className="stat-label">mods involved</div></div>
                                    <div><div className="stat-num" style={{color: '#5fae7e'}}>1.2%</div><div className="stat-label">of this mod</div></div>
                                </div>
                                <div className="section">
                                    <div className="section-label">LOSES TO</div>
                                    {losesTo.map((l) => (
                                        <div key={l.name} className="loses-row">
                                            <div className="loses-head"><span>{l.name}</span><span className="mono">{l.n}</span></div>
                                            <div className="loses-bar"><div style={{width: l.w, background: l.c}}/></div>
                                        </div>
                                    ))}
                                </div>
                                <div className="section">
                                    <div className="section-label">BY DOMAIN</div>
                                    {byDomain.map((d) => (
                                        <div key={d.path} className="domain-row mono">
                                            <span>{d.path}</span><span style={{color: d.c}}>{d.n}</span>
                                        </div>
                                    ))}
                                </div>
                                <span className="resolver-btn" onClick={onOpenResolver}>Open in resolver</span>
                            </div>
                        )}
                        {tab === 'changes' && (
                            <div className="changes-tab">
                                {changelog.map((c) => (
                                    <div key={c.ver} className="change-entry" style={{borderColor: c.edge}}>
                                        <div className="change-head">
                                            <span className="mono ver">{c.ver}</span>
                                            <span className="mono date">{c.date}</span>
                                            {c.tag && <span className="mono tag" style={{color: c.tagC}}>{c.tag}</span>}
                                        </div>
                                        <div className="change-body">{c.body}</div>
                                        <div className="change-files mono">{c.files}</div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </>
            )}
        </div>
    );
}

function OverviewTab({mod}: { mod: library.ModSummary }) {
    return (
        <>
            <div className="overview-grid">
                <span className="label">Version</span><span className="value mono">{mod.Version || '-'}</span>
                <span className="label">Supports</span><span className="value mono ok">{workspaceStaticDetail.supports}</span>
                <span className="label">Size</span><span className="value mono">{workspaceStaticDetail.size}</span>
                <span className="label">Category</span><span className="value">{workspaceStaticDetail.category}</span>
            </div>
            <div className="section">
                <div className="section-label">DESCRIPTION</div>
                <div className="section-body">{workspaceStaticDetail.description}</div>
            </div>
            <div className="section">
                <div className="section-label">REQUIRES</div>
                {workspaceStaticDetail.requires.map((r) => (
                    <div key={r.name} className="requires-row">
                        <span style={{color: r.c}}>●</span>{r.name}
                        <span className="mono note" style={{color: r.c}}>{r.note}</span>
                    </div>
                ))}
            </div>
            <div className="section">
                <div className="section-label">OVERWRITES · {workspaceStaticDetail.overwrites.length} MODS</div>
                {workspaceStaticDetail.overwrites.map((o) => (
                    <div key={o.name} className="overwrite-row">
                        <div className="overwrite-bar"><div style={{width: `${o.pct}%`, background: o.c}}/></div>
                        <span className="mono">{o.name}</span>
                    </div>
                ))}
            </div>
            <div className="detail-footer-actions">
                <span className="btn-ghost inert">Open folder</span>
                <span className="btn-ghost inert">Workshop page</span>
            </div>
        </>
    );
}

function sourceBadgeClass(source: string): string {
    switch (source) {
        case 'workshop':
            return 'badge-workshop';
        case 'paradox-launcher':
            return 'badge-paradox-launcher';
        default:
            return 'badge-local';
    }
}

function sourceBadgeLabel(source: string): string {
    switch (source) {
        case 'workshop':
            return 'W';
        case 'paradox-launcher':
            return 'P';
        default:
            return 'L';
    }
}

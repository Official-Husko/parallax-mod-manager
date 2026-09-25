import './Library.css';
import {Fragment, h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {
    DeleteCollection,
    DetectGames,
    ListCollections,
    ListPlaysets,
    LoadPlayset,
    ModSizes,
    PinnedMods,
    SaveCollection,
    SavePlayset,
    ScanGame,
    SetModPinned,
} from '../../wailsjs/go/main/App';
import type {collection, library, playset} from '../../wailsjs/go/models';
import {Checkbox} from '../components/Checkbox';
import {GameLogo} from '../components/GameLogo';
import {SourceBadge} from '../components/SourceBadge';
import {openContextMenu} from '../data/contextMenu';
import {notify} from '../data/notifications';

type LoadState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

type Row = {
    modId: string;
    name: string;
    version: string;
    source: string;
    gameId: string;
    gameName: string;
};

// What the main table is currently showing - "all games" (the default),
// one specific game (the sidebar's own GAMES section), or one collection
// (the sidebar's own COLLECTIONS section, real data now - see
// ListCollections). Mutually exclusive with each other, matching the
// mockup's own sidebar layout of two alternate top-level sections.
type LibraryFilter =
    | { kind: 'all' }
    | { kind: 'game'; gameId: string }
    | { kind: 'collection'; name: string };

function rowKey(gameId: string, modId: string): string {
    return `${gameId}:${modId}`;
}

export function Library() {
    const [state, setState] = useState<LoadState>({kind: 'loading'});
    const [rows, setRows] = useState<Row[]>([]);
    const [sizes, setSizes] = useState<Record<string, number>>({});
    // Per-game pinned mod IDs - see internal/modpins. Pinning is per game (a bare modId alone
    // isn't unique across games either), so this is keyed the same way sizes could have been but
    // isn't: by game ID, holding that game's own set.
    const [pinned, setPinned] = useState<Record<string, Set<string>>>({});
    const [filter, setFilter] = useState<LibraryFilter>({kind: 'all'});
    const [search, setSearch] = useState('');
    const [collections, setCollections] = useState<collection.Collection[]>([]);
    // Keyed by rowKey(gameId, modId) - a bare modId alone isn't unique
    // across games (see internal/mod.Mod.ID's own doc comment), and this
    // table spans every managed game at once.
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [creatingCollection, setCreatingCollection] = useState(false);
    const [newCollectionName, setNewCollectionName] = useState('');
    const [showAddToPlayset, setShowAddToPlayset] = useState(false);
    const [showMoveToCollection, setShowMoveToCollection] = useState(false);
    const [playsetNamesForGame, setPlaysetNamesForGame] = useState<string[]>([]);

    useEffect(() => {
        let cancelled = false;
        DetectGames()
            .then((games) => {
                if (cancelled) return;
                setState({kind: 'ready', games});
                // Each game's mod list (fast - just descriptor reads) and its
                // real on-disk sizes (a separate, independent call) are
                // fetched per game rather than awaited all at once, so the
                // table starts filling in as soon as the first game
                // responds instead of waiting on the slowest one.
                for (const g of games) {
                    ScanGame(g.ID, '')
                        .then((summary) => {
                            if (cancelled) return;
                            const gameRows = summary.Mods.map((m): Row => ({
                                modId: m.ID, name: m.Name, version: m.Version, source: m.Source,
                                gameId: g.ID, gameName: g.DisplayName,
                            }));
                            setRows((prev) => [...prev.filter((r) => r.gameId !== g.ID), ...gameRows]);
                        })
                        .catch(() => undefined);
                    ModSizes(g.ID)
                        .then((s) => { if (!cancelled) setSizes((prev) => ({...prev, ...s})); })
                        .catch(() => undefined);
                    PinnedMods(g.ID)
                        .then((ids) => { if (!cancelled) setPinned((prev) => ({...prev, [g.ID]: new Set(ids)})); })
                        .catch(() => undefined);
                }
            })
            .catch((err) => { if (!cancelled) setState({kind: 'error', message: String(err)}); });
        return () => { cancelled = true; };
    }, []);

    function refreshCollections() {
        ListCollections().then(setCollections).catch(() => undefined);
    }
    useEffect(refreshCollections, []);

    const games = state.kind === 'ready' ? state.games : [];
    const totalMods = games.reduce((sum, g) => sum + g.ModCount, 0);

    const activeCollection = filter.kind === 'collection'
        ? collections.find((c) => c.name === filter.name)
        : undefined;
    const activeCollectionKeys = useMemo(
        () => activeCollection ? new Set(activeCollection.mods.map((m) => rowKey(m.gameId, m.modId))) : null,
        [activeCollection],
    );

    function isPinned(r: Row): boolean {
        return pinned[r.gameId]?.has(r.modId) ?? false;
    }

    function togglePin(r: Row) {
        const next = !isPinned(r);
        setPinned((prev) => {
            const copy = new Set(prev[r.gameId] ?? []);
            if (next) copy.add(r.modId); else copy.delete(r.modId);
            return {...prev, [r.gameId]: copy};
        });
        SetModPinned(r.gameId, r.modId, next).catch((err) => notify('error', String(err)));
    }

    const visibleRows = rows
        .filter((r) => {
            if (filter.kind === 'game') return r.gameId === filter.gameId;
            if (filter.kind === 'collection') return activeCollectionKeys?.has(rowKey(r.gameId, r.modId)) ?? false;
            return true;
        })
        .filter((r) => !search.trim() || r.name.toLowerCase().includes(search.toLowerCase()))
        .sort((a, b) => Number(isPinned(b)) - Number(isPinned(a)));

    function toggleRow(key: string) {
        setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key); else next.add(key);
            return next;
        });
    }

    const visibleKeys = visibleRows.map((r) => rowKey(r.gameId, r.modId));
    const allVisibleSelected = visibleKeys.length > 0 && visibleKeys.every((k) => selected.has(k));

    function toggleAllVisible() {
        setSelected((prev) => {
            if (allVisibleSelected) {
                const next = new Set(prev);
                for (const k of visibleKeys) next.delete(k);
                return next;
            }
            return new Set([...prev, ...visibleKeys]);
        });
    }

    const selectedRows = rows.filter((r) => selected.has(rowKey(r.gameId, r.modId)));
    const selectedGameIds = new Set(selectedRows.map((r) => r.gameId));

    function openAddToPlayset() {
        if (selectedRows.length === 0) return;
        if (selectedGameIds.size !== 1) {
            notify('error', 'Add to playset only works for mods from a single game at a time - the selection spans more than one.');
            return;
        }
        const gameId = [...selectedGameIds][0];
        ListPlaysets(gameId).then(setPlaysetNamesForGame).catch(() => setPlaysetNamesForGame([]));
        setShowAddToPlayset(true);
    }

    async function handleAddToPlayset(name: string, isNew: boolean) {
        const gameId = [...selectedGameIds][0];
        const ids = selectedRows.map((r) => r.modId);
        try {
            if (isNew) {
                await SavePlayset({name, gameKey: gameId, modIds: ids, disabledDlc: [], lockedModIds: []} as playset.Playset);
            } else {
                const existing = await LoadPlayset(gameId, name);
                const merged = [...existing.modIds, ...ids.filter((id) => !existing.modIds.includes(id))];
                await SavePlayset({...existing, modIds: merged} as playset.Playset);
            }
            notify('success', `Added ${ids.length} mod${ids.length === 1 ? '' : 's'} to "${name}".`);
            setSelected(new Set());
        } catch (err) {
            notify('error', `Couldn't add to playset: ${String(err)}`);
        }
    }

    async function handleMoveToCollection(name: string, isNew: boolean) {
        const refs = selectedRows.map((r) => ({gameId: r.gameId, modId: r.modId}));
        try {
            const target = isNew ? {name, mods: [] as collection.ModRef[]} : (collections.find((c) => c.name === name) ?? {name, mods: []});
            const existingKeys = new Set(target.mods.map((m) => rowKey(m.gameId, m.modId)));
            const mergedMods = [...target.mods, ...refs.filter((r) => !existingKeys.has(rowKey(r.gameId, r.modId)))];
            await SaveCollection({name: target.name, mods: mergedMods} as collection.Collection);

            // A real move, not just an add, when the mods actually came
            // from a currently-viewed source collection different from
            // the target - drop them from that source too.
            if (filter.kind === 'collection' && filter.name !== name && activeCollection) {
                const remaining = activeCollection.mods.filter(
                    (m) => !refs.some((r) => r.gameId === m.gameId && r.modId === m.modId),
                );
                await SaveCollection({name: activeCollection.name, mods: remaining} as collection.Collection);
            }
            refreshCollections();
            notify('success', `Added ${refs.length} mod${refs.length === 1 ? '' : 's'} to "${name}".`);
            setSelected(new Set());
        } catch (err) {
            notify('error', `Couldn't update collection: ${String(err)}`);
        }
    }

    async function createCollection(name: string) {
        const trimmed = name.trim();
        if (!trimmed) return;
        if (collections.some((c) => c.name === trimmed)) {
            notify('error', `A collection named "${trimmed}" already exists.`);
            return;
        }
        try {
            await SaveCollection({name: trimmed, mods: [] as collection.ModRef[]} as collection.Collection);
            refreshCollections();
            setFilter({kind: 'collection', name: trimmed});
        } catch (err) {
            notify('error', `Couldn't create collection: ${String(err)}`);
        } finally {
            setCreatingCollection(false);
            setNewCollectionName('');
        }
    }

    async function deleteActiveCollection() {
        if (filter.kind !== 'collection') return;
        const name = filter.name;
        try {
            await DeleteCollection(name);
            setFilter({kind: 'all'});
            refreshCollections();
            notify('info', `Deleted collection "${name}".`);
        } catch (err) {
            notify('error', `Couldn't delete collection: ${String(err)}`);
        }
    }

    return (
        <div className="library">
            <div className="library-sidebar">
                <div className="sidebar-label">GAMES</div>
                {state.kind === 'loading' && <p className="status-page">Checking games...</p>}
                {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
                {state.kind === 'ready' && (
                    <>
                        <div
                            className="sidebar-game"
                            style={{background: filter.kind === 'all' ? 'var(--bg-highlight)' : 'transparent', cursor: 'pointer'}}
                            onClick={() => setFilter({kind: 'all'})}
                        >
                            <span className="swatch all-games fallback"><i className="fa-solid fa-layer-group"/></span>
                            <span className="name" style={{color: filter.kind === 'all' ? 'var(--text-bright)' : 'var(--text-mid)'}}>All games</span>
                            <span className="mono n">{totalMods}</span>
                        </div>
                        {games.map((g) => (
                            <div
                                key={g.ID}
                                className="sidebar-game"
                                style={{background: filter.kind === 'game' && filter.gameId === g.ID ? 'var(--bg-highlight)' : 'transparent', cursor: 'pointer'}}
                                onClick={() => setFilter({kind: 'game', gameId: g.ID})}
                            >
                                <GameLogo gameId={g.ID} className="swatch"/>
                                <span className="name" style={{color: filter.kind === 'game' && filter.gameId === g.ID ? 'var(--text-bright)' : 'var(--text-mid)'}}>{g.DisplayName}</span>
                                <span className="mono n">{g.ModCount}</span>
                            </div>
                        ))}
                    </>
                )}
                <div className="sidebar-label" style={{marginTop: 18}}>COLLECTIONS</div>
                {collections.map((c) => (
                    <div
                        key={c.name}
                        className="sidebar-collection"
                        style={{
                            background: filter.kind === 'collection' && filter.name === c.name ? 'var(--bg-highlight)' : 'transparent',
                            color: filter.kind === 'collection' && filter.name === c.name ? 'var(--text-bright)' : 'var(--text-muted)',
                            cursor: 'pointer',
                        }}
                        onClick={() => setFilter({kind: 'collection', name: c.name})}
                    >
                        <span>{c.name}</span><span className="mono n">{c.mods.length}</span>
                    </div>
                ))}
                {!creatingCollection && (
                    <div className="sidebar-collection new" onClick={() => setCreatingCollection(true)}>
                        <span><i className="fa-solid fa-plus"/> New collection</span>
                    </div>
                )}
                {creatingCollection && (
                    <div className="sidebar-collection-create">
                        <input
                            autoFocus
                            placeholder="Collection name..."
                            value={newCollectionName}
                            onInput={(e) => setNewCollectionName((e.target as HTMLInputElement).value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') createCollection(newCollectionName);
                                if (e.key === 'Escape') { setCreatingCollection(false); setNewCollectionName(''); }
                            }}
                            onBlur={() => { if (!newCollectionName.trim()) setCreatingCollection(false); }}
                        />
                    </div>
                )}
                <div className="sidebar-footer">
                    <div className="mono line">{totalMods} mods across {games.length} game{games.length === 1 ? '' : 's'}</div>
                    <div className="find-unused">Find unused</div>
                </div>
            </div>
            <div className="library-main">
                <div className="library-toolbar">
                    <div className="search-box">
                        <i className="fa-solid fa-magnifying-glass"/>
                        <input
                            placeholder="Search by name..."
                            value={search}
                            onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                        />
                        <span className="kbd">⌘K</span>
                    </div>
                    {filter.kind === 'collection' && (
                        <span className="library-collection-badge">
                            <i className="fa-solid fa-layer-group"/> {filter.name}
                            <i className="fa-solid fa-trash-can" onClick={deleteActiveCollection} title="Delete this collection"/>
                        </span>
                    )}
                    <div className="spacer"/>
                    <span className="sort-label">Table <i className="fa-solid fa-chevron-down"/></span>
                </div>
                <div className="library-columns mono">
                    <span className="col-check">
                        <Checkbox checked={allVisibleSelected} onChange={toggleAllVisible} disabled={visibleRows.length === 0}/>
                    </span>
                    <span className="col-src">SRC</span>
                    <span className="col-name">NAME</span>
                    <span className="col-game">GAME</span>
                    <span className="col-ver">VERSION</span>
                    <span className="col-size">SIZE</span>
                    <span className="col-played">LAST PLAYED</span>
                    <span className="col-state">STATE</span>
                </div>
                <div className="library-rows">
                    {visibleRows.map((r) => {
                        const key = rowKey(r.gameId, r.modId);
                        return (
                            <div
                                key={key}
                                className="library-row"
                                onContextMenu={(e) => openContextMenu(e, [
                                    {label: isPinned(r) ? 'Unpin' : 'Pin', onClick: () => togglePin(r)},
                                ])}
                            >
                                <span className="col-check">
                                    <Checkbox checked={selected.has(key)} onChange={() => toggleRow(key)}/>
                                </span>
                                <span className="col-src">
                                    <SourceBadge source={r.source} name={r.name}/>
                                </span>
                                <span className="col-name name">
                                    {r.name}
                                    {isPinned(r) && <i className="fa-solid fa-thumbtack library-pin" title="Pinned"/>}
                                </span>
                                <span className="col-game">
                                    <span className="game-name">{r.gameName}</span>
                                </span>
                                <span className="col-ver mono">{r.version || '-'}</span>
                                <span className="col-size mono">{r.modId in sizes ? formatBytes(sizes[r.modId]) : '-'}</span>
                                <span className="col-played">-</span>
                                <span className="col-state mono">-</span>
                            </div>
                        );
                    })}
                    {state.kind === 'ready' && visibleRows.length === 0 && (
                        <p className="status-page">No mods found.</p>
                    )}
                </div>
                <div className="library-footer">
                    <span className="mono">{visibleRows.length} shown{selected.size > 0 ? ` · ${selected.size} selected` : ''}</span>
                    <div className="library-action-wrap">
                        <span
                            className={`link-btn ${selected.size === 0 ? 'inert' : ''}`}
                            style={{color: 'var(--text-muted)'}}
                            onClick={selected.size > 0 ? openAddToPlayset : undefined}
                        >
                            Add to playset
                        </span>
                        {showAddToPlayset && (
                            <ActionPicker
                                label="ADD TO PLAYSET"
                                existingNames={playsetNamesForGame}
                                onPick={(name) => { handleAddToPlayset(name, false); setShowAddToPlayset(false); }}
                                onCreate={(name) => { handleAddToPlayset(name, true); setShowAddToPlayset(false); }}
                                onClose={() => setShowAddToPlayset(false)}
                            />
                        )}
                    </div>
                    <div className="library-action-wrap">
                        <span
                            className={`link-btn ${selected.size === 0 ? 'inert' : ''}`}
                            style={{color: 'var(--text-muted)'}}
                            onClick={selected.size > 0 ? () => setShowMoveToCollection(true) : undefined}
                        >
                            Move to collection
                        </span>
                        {showMoveToCollection && (
                            <ActionPicker
                                label="MOVE TO COLLECTION"
                                existingNames={collections.map((c) => c.name)}
                                onPick={(name) => { handleMoveToCollection(name, false); setShowMoveToCollection(false); }}
                                onCreate={(name) => { handleMoveToCollection(name, true); setShowMoveToCollection(false); }}
                                onClose={() => setShowMoveToCollection(false)}
                            />
                        )}
                    </div>
                    <span className="link-btn inert" style={{color: 'var(--red)'}}>Uninstall</span>
                </div>
            </div>
        </div>
    );
}

// ActionPicker is a small click-away dropdown shared by the "Add to
// playset"/"Move to collection" bulk actions - pick an existing target by
// name, or switch to a one-field inline form to create a new one. Kept
// local (not a shared component) since nothing else in the app needs this
// exact "existing list, or a name field" shape yet.
function ActionPicker({label, existingNames, onPick, onCreate, onClose}: {
    label: string;
    existingNames: string[];
    onPick: (name: string) => void;
    onCreate: (name: string) => void;
    onClose: () => void;
}) {
    const [creating, setCreating] = useState(false);
    const [newName, setNewName] = useState('');
    const ref = useRef<HTMLDivElement>(null);

    useEffect(() => {
        function onClickAway(e: MouseEvent) {
            if (ref.current && !ref.current.contains(e.target as Node)) onClose();
        }
        document.addEventListener('mousedown', onClickAway);
        return () => document.removeEventListener('mousedown', onClickAway);
    }, [onClose]);

    return (
        <div className="library-action-picker" ref={ref}>
            <div className="library-action-picker-label mono">{label}</div>
            {!creating && (
                <>
                    {existingNames.length === 0 && <div className="library-action-picker-empty">None yet</div>}
                    {existingNames.map((name) => (
                        <div key={name} className="library-action-picker-item" onClick={() => onPick(name)}>{name}</div>
                    ))}
                    <div className="library-action-picker-item new" onClick={() => setCreating(true)}>
                        <i className="fa-solid fa-plus"/> New...
                    </div>
                </>
            )}
            {creating && (
                <div className="library-action-picker-create">
                    <input
                        autoFocus
                        placeholder="Name..."
                        value={newName}
                        onInput={(e) => setNewName((e.target as HTMLInputElement).value)}
                        onKeyDown={(e) => { if (e.key === 'Enter' && newName.trim()) onCreate(newName.trim()); }}
                    />
                    <span className="btn-ghost" onClick={() => { if (newName.trim()) onCreate(newName.trim()); }}>Create</span>
                </div>
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

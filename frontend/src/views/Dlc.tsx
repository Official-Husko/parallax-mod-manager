import './Dlc.css';
import {h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {DLCStoreData, ListDLC, ListPlaysets, LoadPlayset, SavePlayset} from '../../wailsjs/go/main/App';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import type {dlc, dlcstore, library, playset} from '../../wailsjs/go/models';
import {Toggle} from '../components/Toggle';
import {formatBytes} from '../data/format';
import {type ContextMenuItem, openContextMenu} from '../data/contextMenu';

type DLCState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; entries: dlc.Entry[] };

type Status =
    | { kind: 'idle' }
    | { kind: 'busy'; message: string }
    | { kind: 'error'; message: string }
    | { kind: 'success'; message: string };

// formatCategory turns a real Steam-style category slug ("species_pack")
// into a readable label ("Species Pack") - the raw value stays untouched
// on the wire, this is presentation only.
function formatCategory(cat: string): string {
    if (!cat) return 'Uncategorized';
    return cat.split('_').map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
}

// extractYear pulls the year out of Steam's own already-localized
// release_date string (e.g. "Feb 22, 2018") - robust regardless of locale
// format, since only the 4-digit year is ever used.
function extractYear(dateStr: string): string {
    const m = dateStr.match(/\d{4}/);
    return m ? m[0] : '-';
}

// formatRelease shows "Coming soon" for real unreleased DLC (Steam's own
// ComingSoon flag - reliable, unlike trying to detect that same text
// inside ReleaseDate) rather than extractYear's own '-' fallback, which
// would otherwise be all a genuinely upcoming DLC ever showed here.
function formatRelease(sd: dlcstore.StoreData | undefined): string {
    if (!sd) return '-';
    if (sd.ComingSoon) return 'Coming soon';
    return sd.ReleaseDate ? extractYear(sd.ReleaseDate) : '-';
}

// Dlc lets a user disable specific installed DLC for one saved playset -
// disabled_dlcs is real, already-launched functionality (see
// internal/launch's writeClassicState), the only missing piece was
// discovering what DLC actually exists (see internal/dlc) and a place to
// toggle it. Edits one saved playset at a time, picked here - a
// deliberately separate, simpler flow from Workspace's own live,
// not-yet-saved editing session.
//
// The type filter and per-pack size/release year are all real: Category
// and SizeBytes come straight from internal/dlc's own local discovery
// (a folder walk and the .dlc file's own field), and release year comes
// from internal/dlcstore's Steam Store cache. A real Steam ownership check
// needs an authenticated call this project doesn't make, so instead of
// guessing at "owned", every DLC in the base game's own official Steam
// catalog that isn't installed here (library.MergeDLCCatalog) still
// shows up - real name and header image, honestly labeled NOT INSTALLED,
// never toggleable since there's no local folder to actually enable. This
// project also never shows price, "required by mods", or a multiplayer
// checksum - none of that is something it can honestly compute (Paradox
// mods don't declare DLC dependencies anywhere this project can read).
export function Dlc({games, selectedGame}: {
    games: library.GameInfo[];
    selectedGame: string;
}) {
    const [dlcState, setDlcState] = useState<DLCState>({kind: 'loading'});
    const [playsetNames, setPlaysetNames] = useState<string[]>([]);
    const [selectedPlayset, setSelectedPlayset] = useState('');
    const [modIds, setModIds] = useState<string[]>([]);
    const [disabled, setDisabled] = useState<Set<string>>(new Set());
    const [status, setStatus] = useState<Status>({kind: 'idle'});
    const [storeData, setStoreData] = useState<Map<string, dlcstore.StoreData>>(new Map());
    const [selectedDLCId, setSelectedDLCId] = useState('');
    const [selectedType, setSelectedType] = useState('');
    const [search, setSearch] = useState('');

    useEffect(() => {
        if (!selectedGame) return;
        setDlcState({kind: 'loading'});
        ListDLC(selectedGame)
            .then((entries) => setDlcState({kind: 'ready', entries}))
            .catch((err) => setDlcState({kind: 'error', message: String(err)}));
        ListPlaysets(selectedGame)
            .then((names) => {
                setPlaysetNames(names);
                setSelectedPlayset((prev) => (names.includes(prev) ? prev : names[0] ?? ''));
            })
            .catch(() => undefined);
    }, [selectedGame]);

    useEffect(() => {
        if (!selectedGame) {
            setStoreData(new Map());
            return;
        }
        function fetchStoreData() {
            DLCStoreData(selectedGame)
                .then((list) => setStoreData(new Map(list.map((d) => [d.SteamAppID, d]))))
                .catch(() => undefined);
        }
        fetchStoreData();
        const unsubscribe = EventsOn('dlc-store-refreshed', (gameId: string) => {
            if (gameId === selectedGame) fetchStoreData();
        });
        return () => unsubscribe();
    }, [selectedGame]);

    useEffect(() => {
        if (!selectedGame || !selectedPlayset) {
            setModIds([]);
            setDisabled(new Set());
            return;
        }
        let cancelled = false;
        LoadPlayset(selectedGame, selectedPlayset)
            .then((p) => {
                if (cancelled) return;
                setModIds(p.modIds ?? []);
                setDisabled(new Set(p.disabledDlc ?? []));
                setStatus({kind: 'idle'});
            })
            .catch((err) => { if (!cancelled) setStatus({kind: 'error', message: String(err)}); });
        return () => { cancelled = true; };
    }, [selectedGame, selectedPlayset]);

    function toggle(id: string) {
        setDisabled((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id); else next.add(id);
            return next;
        });
    }

    // Real context menu for one DLC row - the enable/disable toggle (only
    // for something actually installed, matching the row's own Toggle)
    // plus a real link to its Steam Store page when a Steam app id is
    // known (true for both installed DLC and a catalog-only entry, so
    // this works even for something with no local folder to open at
    // all).
    function dlcContextMenuItems(d: dlc.Entry): ContextMenuItem[] {
        const items: ContextMenuItem[] = [];
        if (d.Installed) {
            const isDisabled = disabled.has(d.ID);
            items.push({label: isDisabled ? 'Enable' : 'Disable', onClick: () => toggle(d.ID)});
        }
        if (d.SteamID) {
            items.push({label: 'Open Steam Store page', onClick: () => BrowserOpenURL(`https://store.steampowered.com/app/${d.SteamID}`)});
        }
        return items;
    }

    // Drops an id from the disabled set outright - used for a DLC this
    // playset disables that no longer exists locally (see orphanedIds
    // below). There's no toggle to offer for something that can't be
    // found, only a way to clear the stale reference.
    function forget(id: string) {
        setDisabled((prev) => {
            const next = new Set(prev);
            next.delete(id);
            return next;
        });
    }

    async function handleSave() {
        if (!selectedGame || !selectedPlayset) return;
        setStatus({kind: 'busy', message: 'Saving...'});
        try {
            const p = {
                name: selectedPlayset,
                gameKey: selectedGame,
                modIds,
                disabledDlc: [...disabled],
            } as playset.Playset;
            await SavePlayset(p);
            setStatus({kind: 'success', message: `Saved ${selectedPlayset}.`});
        } catch (err) {
            setStatus({kind: 'error', message: String(err)});
        }
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;
    const entries = dlcState.kind === 'ready' ? dlcState.entries : [];
    // A catalog-only entry (Installed: false) has no local folder, so no
    // ID - use its SteamID as identity instead. At least one of the two
    // is always real for anything worth showing.
    const rowKey = (e: dlc.Entry) => e.ID || e.SteamID;

    const installedEntries = useMemo(() => entries.filter((e) => e.Installed), [entries]);
    const catalogOnlyEntries = useMemo(() => entries.filter((e) => !e.Installed), [entries]);
    const enabledCount = installedEntries.filter((e) => !disabled.has(e.ID)).length;

    // Category only really describes installed content (a catalog-only
    // entry's Steam Store data has no equivalent field) - the type filter
    // is built from installed DLC only; catalog-only entries always show
    // under "All packs" instead, via filteredEntries below.
    const typeCounts = useMemo(() => {
        const counts = new Map<string, number>();
        for (const e of installedEntries) {
            const key = e.Category || '';
            counts.set(key, (counts.get(key) ?? 0) + 1);
        }
        return [...counts.entries()].sort((a, b) => b[1] - a[1]);
    }, [installedEntries]);

    const filteredEntries = useMemo(() => {
        const q = search.trim().toLowerCase();
        const matchesSearch = (e: dlc.Entry) => !q || e.Name.toLowerCase().includes(q);
        const installed = installedEntries.filter((e) => (selectedType === '' || (e.Category || '') === selectedType) && matchesSearch(e));
        const catalogOnly = selectedType === '' ? catalogOnlyEntries.filter(matchesSearch) : [];
        return [...installed, ...catalogOnly];
    }, [installedEntries, catalogOnlyEntries, selectedType, search]);

    const selectedEntry = entries.find((e) => rowKey(e) === selectedDLCId);
    const selectedStoreData = selectedEntry?.SteamID ? storeData.get(selectedEntry.SteamID) : undefined;

    // This playset disables an id that isn't among the currently
    // discovered DLC - moved, removed, or (this project can't tell which)
    // never really installed for this Steam account to begin with. Shown
    // rather than silently dropped: saving still preserves it either way,
    // so it should be visible and removable, not invisible.
    const orphanedIds = useMemo(
        () => [...disabled].filter((id) => !entries.some((e) => e.ID === id)),
        [disabled, entries],
    );

    return (
        <div className="dlc">
            <div className="dlc-detail">
                {!selectedEntry && (
                    <p className="detail-empty">Select a pack from the list to see its details here.</p>
                )}
                {selectedEntry && (
                    <>
                        <div className="thumbnail dlc-detail-image">
                            {selectedStoreData?.HeaderImage
                                ? <>
                                    <div className="thumbnail-backdrop" style={{backgroundImage: `url(${selectedStoreData.HeaderImage})`}}/>
                                    <img className="thumbnail-fg" src={selectedStoreData.HeaderImage} alt=""/>
                                </>
                                : <span className="mono">NO PREVIEW</span>}
                        </div>
                        <div className="dlc-detail-name">{selectedEntry.Name}</div>
                        <div className="dlc-detail-meta mono">
                            {selectedEntry.Installed ? formatCategory(selectedEntry.Category) : 'Not installed'}
                            {selectedStoreData && ` · ${formatRelease(selectedStoreData)}`}
                            {selectedEntry.SizeBytes > 0 && ` · ${formatBytes(selectedEntry.SizeBytes)}`}
                        </div>
                        {selectedStoreData?.ShortDescription ? (
                            <div className="dlc-detail-desc">{selectedStoreData.ShortDescription}</div>
                        ) : (
                            <p className="detail-empty">
                                Steam Store details for this DLC haven't been fetched yet - they're refreshed at
                                most once a day in the background.
                            </p>
                        )}
                    </>
                )}
            </div>

            <div className="dlc-main">
                {!selectedGame && <p className="detail-empty">Select a game to see its DLC.</p>}
                {selectedGame && dlcState.kind === 'loading' && <p className="detail-empty">Checking installed DLC...</p>}
                {selectedGame && dlcState.kind === 'error' && (
                    <div className="content-missing">
                        <i className="fa-solid fa-circle-exclamation"/>
                        <p>{dlcState.message}</p>
                    </div>
                )}
                {selectedGame && dlcState.kind === 'ready' && entries.length === 0 && (
                    <p className="detail-empty">No DLC detected for {gameName} - either none is installed, or the game itself isn't detected.</p>
                )}
                {selectedGame && dlcState.kind === 'ready' && entries.length > 0 && (
                    <>
                        <div className="dlc-toolbar">
                            <div className="search-box">
                                <i className="fa-solid fa-magnifying-glass"/>
                                <input
                                    placeholder={`Search ${entries.length} packs...`}
                                    value={search}
                                    onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                                />
                            </div>
                            <div className="spacer"/>
                            {status.kind === 'error' && <span className="mono" style={{color: 'var(--red)'}}>{status.message}</span>}
                            {status.kind === 'success' && <span className="mono" style={{color: 'var(--green)'}}>{status.message}</span>}
                            <span className="btn-primary" onClick={handleSave}>
                                {status.kind === 'busy' ? 'Saving...' : `Save to ${selectedPlayset || '...'}`}
                            </span>
                        </div>
                        <div className="dlc-columns mono">
                            <span className="col-on">ON</span>
                            <span className="col-name">PACK</span>
                            <span className="col-type">TYPE</span>
                            <span className="col-year">RELEASED</span>
                            <span className="col-size">SIZE</span>
                        </div>
                        <div className="dlc-rows">
                            {filteredEntries.map((d) => {
                                const key = rowKey(d);
                                const sd = d.SteamID ? storeData.get(d.SteamID) : undefined;
                                return (
                                    <div
                                        key={key}
                                        className={`dlc-row ${key === selectedDLCId ? 'active' : ''} ${!d.Installed ? 'not-installed' : ''}`}
                                        onClick={() => setSelectedDLCId(key)}
                                        onContextMenu={(e) => {
                                            e.preventDefault();
                                            setSelectedDLCId(key);
                                            const items = dlcContextMenuItems(d);
                                            if (items.length > 0) openContextMenu(e, items);
                                        }}
                                    >
                                        <span className="col-on" onClick={(e) => e.stopPropagation()}>
                                            {d.Installed && <Toggle on={!disabled.has(d.ID)} onClick={() => toggle(d.ID)}/>}
                                        </span>
                                        <span className="col-name">{d.Name}</span>
                                        <span className="col-type">
                                            {d.Installed ? formatCategory(d.Category) : <span className="dlc-badge muted">NOT INSTALLED</span>}
                                        </span>
                                        <span className="col-year mono">{formatRelease(sd)}</span>
                                        <span className="col-size mono">{d.SizeBytes > 0 ? formatBytes(d.SizeBytes) : '-'}</span>
                                    </div>
                                );
                            })}
                            {orphanedIds.map((id) => (
                                <div
                                    key={id}
                                    className="dlc-row orphaned"
                                    onContextMenu={(e) => openContextMenu(e, [{label: 'Remove this stale reference', onClick: () => forget(id), danger: true}])}
                                >
                                    <span className="col-on"/>
                                    <span className="col-name mono">{id}</span>
                                    <span className="col-type"><span className="dlc-badge">NOT FOUND</span></span>
                                    <span className="col-year"/>
                                    <span className="col-size"/>
                                    <i className="fa-solid fa-xmark dlc-forget" onClick={() => forget(id)} title="Remove this stale reference"/>
                                </div>
                            ))}
                        </div>
                        <div className="dlc-footer">
                            <span className="mono">{enabledCount} of {installedEntries.length} installed DLC enabled</span>
                            {orphanedIds.length > 0 && (
                                <span className="mono" style={{color: 'var(--amber)'}}>
                                    {orphanedIds.length} disabled DLC not found on this install
                                </span>
                            )}
                        </div>
                    </>
                )}
            </div>

            <div className="dlc-sidebar">
                <div className="sidebar-label">TYPE</div>
                <div className={`dlc-type-row ${selectedType === '' ? 'active' : ''}`} onClick={() => setSelectedType('')}>
                    <span>All packs</span><span className="mono n">{entries.length}</span>
                </div>
                {typeCounts.map(([cat, n]) => (
                    <div key={cat} className={`dlc-type-row ${selectedType === cat ? 'active' : ''}`} onClick={() => setSelectedType(cat)}>
                        <span>{formatCategory(cat)}</span><span className="mono n">{n}</span>
                    </div>
                ))}
                <div className="sidebar-label" style={{marginTop: 18}}>PLAYSET</div>
                {playsetNames.length === 0 && (
                    <p className="dlc-empty">No saved playsets for {gameName} yet - save one from the Workspace first.</p>
                )}
                {playsetNames.map((name) => (
                    <div
                        key={name}
                        className={`dlc-playset-row ${name === selectedPlayset ? 'active' : ''}`}
                        onClick={() => setSelectedPlayset(name)}
                    >
                        {name}
                    </div>
                ))}
                <div className="sidebar-footer">
                    <div className="mono line">DLC state is stored per playset, alongside the load order.</div>
                </div>
            </div>
        </div>
    );
}

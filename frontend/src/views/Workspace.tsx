import './Workspace.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {
    AuthorProfiles,
    GetPreferences,
    LaunchGame,
    ListModFiles,
    ListPlaysets,
    LoadPlayset,
    ModChangelog,
    ModThumbnail,
    OpenModFolder,
    SavePlayset,
    ScanGame,
    WatchMods,
    WorkshopDetails,
} from '../../wailsjs/go/main/App';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import type {library, playset, preferences, steamapi} from '../../wailsjs/go/models';
import {autosort} from '../data/autosort';
import {dismiss, notify, updateNotification} from '../data/notifications';
import {domains, preflight} from '../data/mockData';
import {formatBytes} from '../data/format';
import {SourceBadge} from '../components/SourceBadge';
import {FileTree} from '../components/FileTree';
import {ConflictResolver} from './ConflictResolver';
import {PlaysetsModal} from './PlaysetsModal';
import {PreflightModal} from './PreflightModal';
import {PurgeEmptyModal} from './PurgeEmptyModal';

type Status =
    | { kind: 'idle' }
    | { kind: 'busy'; message: string }
    | { kind: 'error'; message: string }
    | { kind: 'success'; message: string };

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
    // The active playset's DLC toggles - preserved across save/launch so
    // Workspace never silently wipes what the DLC screen set. Empty for a
    // brand-new, unsaved playset; loaded from the real file otherwise.
    const [disabledDlc, setDisabledDlc] = useState<string[]>([]);
    const [showPlaysets, setShowPlaysets] = useState(false);
    const [showPreflight, setShowPreflight] = useState(false);
    const [showConflictResolver, setShowConflictResolver] = useState(false);
    const [showPurgeModal, setShowPurgeModal] = useState(false);
    const [search, setSearch] = useState('');
    const [activeSearch, setActiveSearch] = useState('');
    const [status, setStatus] = useState<Status>({kind: 'idle'});
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    // Guards the 'scan-quick' listener below so it only ever applies to the
    // fresh load it belongs to - a watcher-triggered background refresh
    // (refreshMods(true)) runs the exact same backend scan and fires the
    // same event, but must never have its quick preview reset the load
    // order/selection the way a fresh load's first paint is supposed to.
    const expectingFreshQuickRef = useRef(false);
    const quickAppliedRef = useRef(false);
    // Mirrors playsetName for refreshMods' background-refresh path below,
    // which is also called from the 'mods-changed' watcher subscription -
    // a useEffect closure that only re-subscribes on [selectedGame], so a
    // playsetName read there directly would see whatever it was when that
    // effect last ran, not necessarily the currently loaded playset.
    const playsetNameRef = useRef('');

    function setPlaysetName(name: string) {
        setPlaysetNameState(name);
        playsetNameRef.current = name;
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
            setDisabledDlc([]);
            setSelectedAvailable(new Set());
            setStatus({kind: 'busy', message: 'Scanning...'});
            expectingFreshQuickRef.current = true;
            quickAppliedRef.current = false;
        }
        try {
            // A background refresh (preserveSelection=true) must re-scan
            // against whichever playset is actually currently loaded, not
            // an empty selection - otherwise it would recompute Conflicts
            // from a real conflict list back down to none (the backend
            // treats an empty playset name as "nothing enabled yet"),
            // silently emptying the Conflict Resolver every time a
            // background refresh fires (the mod-folder watcher, a purge,
            // a patch generation, or a manual conflict override) even
            // though nothing about the user's actual selection changed.
            const result = await ScanGame(selectedGame, preserveSelection ? playsetNameRef.current : '');
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

    // Real Steam Workshop metadata (title stats, description, author id)
    // for every currently scanned Workshop mod - fetched once per game, in
    // one batched request, as soon as a scan actually produces a mod list,
    // not lazily per-mod-selection - the Available list's author column
    // and every mod's detail header need this before any particular mod
    // is even selected. Lives here (not in DetailPanel) so the Available/
    // Active lists can show real author names too, not just the detail
    // panel.
    const [workshopDetails, setWorkshopDetails] = useState<Map<string, steamapi.PublishedFileDetails>>(new Map());
    const [workshopDetailsState, setWorkshopDetailsState] = useState<'idle' | 'loading' | 'loaded' | 'error'>('idle');
    const [workshopDetailsError, setWorkshopDetailsError] = useState('');
    // Guards re-entry into the fetch effect below via a ref, not state -
    // a real bug found and fixed here: the effect used to gate on
    // workshopDetailsState itself (idle/loading/loaded/error) while also
    // listing that same state in its own dependency array. Calling
    // setWorkshopDetailsState('loading') right after starting the fetch
    // changed that dependency, which made Preact run this effect's own
    // cleanup (setting cancelled = true) as part of the very next render
    // - typically within milliseconds, always well before a real network
    // request could possibly resolve. So by the time WorkshopDetails(...)
    // actually came back, its .then/.catch always saw cancelled === true
    // and silently bailed out without ever calling setWorkshopDetails or
    // setWorkshopDetailsState('loaded') - meaning this fetch could never
    // succeed, not occasionally but every single time, regardless of
    // network speed or tab-switching. Confirmed with a real, isolated
    // Preact+hooks reproduction before and after this fix. A ref sits
    // outside the render/effect dependency system entirely, so setting
    // it doesn't trigger Preact to re-evaluate this effect at all.
    const workshopDetailsStartedRef = useRef(false);
    // A dedicated retry trigger, separate from workshopDetailsState -
    // changing it is the only thing besides selectedGame that's allowed
    // to make the fetch effect below run again, and unlike
    // workshopDetailsState, the effect itself never touches this, so
    // including it as a dependency can't cause the same self-cancelling
    // bug the fix above is about.
    const [workshopDetailsRetryTick, setWorkshopDetailsRetryTick] = useState(0);

    useEffect(() => {
        // A previous game's fetched data must never be shown against this
        // game's mods.
        setWorkshopDetails(new Map());
        setWorkshopDetailsState('idle');
        workshopDetailsStartedRef.current = false;
    }, [selectedGame]);

    useEffect(() => {
        if (!summary || workshopDetailsStartedRef.current) {
            return;
        }
        workshopDetailsStartedRef.current = true;
        setWorkshopDetailsState('loading');
        const notifId = notify('progress', 'Fetching Steam Workshop mod details...');
        let cancelled = false;
        let settled = false;
        WorkshopDetails(selectedGame)
            .then((list) => {
                settled = true;
                if (cancelled) { dismiss(notifId); return; }
                setWorkshopDetails(new Map(list.map((d) => [d.ID, d])));
                setWorkshopDetailsState('loaded');
                updateNotification(notifId, {
                    kind: 'success',
                    message: `Fetched Steam Workshop details for ${list.length} mod${list.length === 1 ? '' : 's'}.`,
                    action: undefined,
                });
            })
            .catch((err) => {
                settled = true;
                if (cancelled) { dismiss(notifId); return; }
                setWorkshopDetailsError(String(err));
                setWorkshopDetailsState('error');
                updateNotification(notifId, {
                    kind: 'error',
                    message: `Couldn't load Steam Workshop details: ${String(err)}`,
                    action: {
                        label: 'Retry',
                        onClick: () => {
                            workshopDetailsStartedRef.current = false;
                            setWorkshopDetailsRetryTick((t) => t + 1);
                            dismiss(notifId);
                        },
                    },
                });
            });
        return () => {
            cancelled = true;
            // Only a fetch that's still genuinely in flight when this
            // effect tears down should have its "fetching..." toast
            // pulled - once settled, the success/error message it became
            // is real information the user hasn't necessarily seen yet
            // and should live out its own normal lifecycle (auto-dismiss
            // for success, stay until dismissed for error), not vanish
            // just because selectedGame changed or the effect re-ran.
            if (!settled) dismiss(notifId);
        };
        // Deliberately !!summary, not summary itself - summary gets a new
        // object reference on every rescan (the quick preview, then the
        // full result, then any later background refresh), and none of
        // those later reference changes should tear down and restart an
        // already-in-flight or already-finished fetch; only "did a
        // summary become available at all yet" (or a real user Retry)
        // should.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [!!summary, selectedGame, workshopDetailsRetryTick]);

    // Real Steam Community profiles for Workshop mod authors - fetched
    // once the Workshop details themselves have loaded (AuthorProfiles
    // reuses that same in-memory data on the backend, so this never
    // triggers a second Workshop metadata fetch), keyed by creator
    // SteamID64.
    const [authorProfiles, setAuthorProfiles] = useState<Map<string, steamapi.Profile>>(new Map());
    const [authorProfilesState, setAuthorProfilesState] = useState<'idle' | 'loading' | 'loaded' | 'error'>('idle');
    // Same self-cancelling-effect bug as workshopDetailsStartedRef above,
    // and the same fix - a ref-based re-entry guard instead of gating on
    // (and listing as a dependency of) the very state this effect itself
    // sets.
    const authorProfilesStartedRef = useRef(false);

    useEffect(() => {
        setAuthorProfiles(new Map());
        setAuthorProfilesState('idle');
        authorProfilesStartedRef.current = false;
    }, [selectedGame]);

    const [authorProfilesRetryTick, setAuthorProfilesRetryTick] = useState(0);

    useEffect(() => {
        if (workshopDetailsState !== 'loaded' || authorProfilesStartedRef.current) {
            return;
        }
        authorProfilesStartedRef.current = true;
        setAuthorProfilesState('loading');
        // How many distinct real authors this game's Workshop mods
        // actually have - known once workshopDetails itself is loaded,
        // used below to tell "no authors to look up" from "look-up came
        // back suspiciously empty" (see the doc comment this replaced).
        const expectedCount = new Set(
            [...workshopDetails.values()].map((d) => d.Creator).filter((id) => !!id),
        ).size;
        const notifId = notify('progress', 'Fetching Steam author profiles...');
        let cancelled = false;
        let settled = false;
        AuthorProfiles(selectedGame)
            .then((list) => {
                settled = true;
                if (cancelled) { dismiss(notifId); return; }
                setAuthorProfiles(new Map(list.map((p) => [p.SteamID, p.Profile])));
                setAuthorProfilesState('loaded');
                const retryAction = {
                    label: 'Retry',
                    onClick: () => {
                        authorProfilesStartedRef.current = false;
                        setAuthorProfilesRetryTick((t) => t + 1);
                        dismiss(notifId);
                    },
                };
                if (list.length === 0 && expectedCount > 0) {
                    // AuthorProfileCache.Get swallows every individual
                    // profile-fetch failure (matching this project's own
                    // non-fatal-per-item philosophy elsewhere - one bad id
                    // must not fail the whole call), so a genuine failure
                    // here (Steam Community unreachable, rate-limited)
                    // resolves as a real, error-free empty result, not a
                    // rejected promise - correct behavior, but otherwise
                    // silently indistinguishable from "there was nothing to
                    // look up at all". This is specifically the suspicious
                    // case: authors were expected, none came back.
                    updateNotification(notifId, {
                        kind: 'error',
                        message: `Steam author profiles came back empty for ${expectedCount} author${expectedCount === 1 ? '' : 's'} - Steam Community may be temporarily unreachable.`,
                        action: retryAction,
                    });
                } else {
                    updateNotification(notifId, {
                        kind: 'success',
                        message: `Fetched ${list.length} author profile${list.length === 1 ? '' : 's'}.`,
                        action: undefined,
                    });
                }
            })
            .catch((err) => {
                settled = true;
                if (cancelled) { dismiss(notifId); return; }
                setAuthorProfilesState('error');
                updateNotification(notifId, {
                    kind: 'error',
                    message: `Couldn't load Steam author profiles: ${String(err)}`,
                    action: {
                        label: 'Retry',
                        onClick: () => {
                            authorProfilesStartedRef.current = false;
                            setAuthorProfilesRetryTick((t) => t + 1);
                            dismiss(notifId);
                        },
                    },
                });
            });
        return () => {
            cancelled = true;
            if (!settled) dismiss(notifId);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [workshopDetailsState, selectedGame, authorProfilesRetryTick]);

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
    // Active's own row list, search-filtered for display only - Autosort,
    // Save, and the legend/footer all still operate on the full, real
    // active list above, matching Available's own filtered-list-is-
    // display-only precedent (reordering/removal act on a mod id
    // directly, never a filtered index, so this never risks moving or
    // dropping the wrong mod).
    const visibleActive = active.filter((m) => matchesSearch(m, activeSearch));
    // Real load-order position (1-based), independent of activeSearch
    // filtering - a filtered row must still show where it actually sits
    // in the real load order, not its index within the filtered results.
    const positionById = useMemo(() => new Map(active.map((m, i) => [m.ID, i + 1])), [active]);
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

    function handlePurged(result: library.PurgeResult) {
        const deleted = result.Deleted ?? [];
        const errors = result.Errors ?? [];
        if (deleted.length > 0) {
            setOrder((prev) => prev.filter((id) => !deleted.includes(id)));
        }
        if (errors.length > 0) {
            setStatus({kind: 'error', message: `Deleted ${deleted.length} mod${deleted.length === 1 ? '' : 's'}, but: ${errors.join('; ')}`});
        } else if (deleted.length > 0) {
            setStatus({kind: 'success', message: `Deleted ${deleted.length} empty mod${deleted.length === 1 ? '' : 's'}.`});
        }
        if (deleted.length > 0) {
            refreshMods(true);
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
            // disabledDlc is preserved as loaded (see handleLoadPlayset),
            // not reset - Workspace edits the load order, not DLC toggles
            // (that's the DLC screen's job), so a save here must never
            // silently wipe whatever was really set there.
            const p = {name: playsetName.trim(), gameKey: selectedGame, modIds: order, disabledDlc: disabledDlc} as playset.Playset;
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
            setDisabledDlc(p.disabledDlc ?? []);
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
            const p = {name: playsetName.trim(), gameKey: selectedGame, modIds: order, disabledDlc: disabledDlc} as playset.Playset;
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
            {status.kind === 'success' && <p className="workspace-status success">{status.message}</p>}

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
                        workshopDetails={workshopDetails}
                        workshopDetailsState={workshopDetailsState}
                        workshopDetailsError={workshopDetailsError}
                        authorProfiles={authorProfiles}
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
                                <div className="spacer"/>
                                <span className="chip chip-danger" onClick={() => setShowPurgeModal(true)}>Purge empty</span>
                            </div>
                        </div>
                        <div className="column-header">
                            <span className="column-header-spacer"/>
                            <span className="column-header-name">NAME</span>
                            <span className="column-header-ver">VERSION</span>
                            <span className="column-header-author">AUTHOR</span>
                        </div>
                        <div className="list-rows">
                            {available.map((m) => {
                                const author = authorNameFor(m, workshopDetails, authorProfiles);
                                const authorLoading = m.Source === 'workshop'
                                    && (workshopDetailsState === 'loading' || (workshopDetailsState === 'loaded' && authorProfilesState === 'loading'));
                                return (
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
                                    <span className="author mono" title={author}>
                                        {authorLoading
                                            ? <span className="skeleton skeleton-text" style={{width: '50px', marginLeft: 'auto'}}/>
                                            : (author || '-')}
                                    </span>
                                </div>
                                );
                            })}
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
                                <span className="count mono">{visibleActive.length}</span>
                                <div className="spacer"/>
                                <span className="sort-label">Bottom wins <i className="fa-solid fa-chevron-down"/></span>
                            </div>
                            <div className="search-box">
                                <i className="fa-solid fa-magnifying-glass"/>
                                <input
                                    placeholder={`Search ${active.length} mods...`}
                                    value={activeSearch}
                                    onInput={(e) => setActiveSearch((e.target as HTMLInputElement).value)}
                                />
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
                            {visibleActive.length === 0 && active.length > 0 && (
                                <p className="detail-empty" style={{padding: 14}}>No matches.</p>
                            )}
                            {visibleActive.map((m) => {
                                const conflicted = conflictedIds.has(m.ID);
                                return (
                                    <div
                                        key={m.ID}
                                        className={`mod-row active-row ${m.ID === selectedId ? 'selected' : ''} ${conflicted ? 'has-conflict' : ''}`}
                                        onClick={() => setSelectedId(m.ID)}
                                    >
                                        <span className="position mono">{positionById.get(m.ID)}</span>
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
                    onNew={() => { setOrder([]); setPlaysetName(''); setDisabledDlc([]); setShowPlaysets(false); }}
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
                    onOverrideChanged={() => refreshMods(true)}
                />
            )}

            {showPurgeModal && (
                <PurgeEmptyModal
                    gameId={selectedGame}
                    onClose={() => setShowPurgeModal(false)}
                    onPurged={handlePurged}
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

// authorNameFor resolves a mod's real Steam Workshop author name, if it's
// a Workshop mod and both its own Workshop metadata and that creator's
// profile have been fetched - "" otherwise (a local mod, or data not
// loaded yet), for callers to fall back to their own placeholder.
function authorNameFor(
    m: library.ModSummary | null,
    workshopDetails: Map<string, steamapi.PublishedFileDetails>,
    authorProfiles: Map<string, steamapi.Profile>,
): string {
    if (!m || m.Source !== 'workshop' || !m.RemoteFileID) return '';
    const d = workshopDetails.get(m.RemoteFileID);
    if (!d || d.Result !== 1 || !d.Creator) return '';
    return authorProfiles.get(d.Creator)?.Name ?? '';
}

function DetailPanel({mod, tab, onTab, onOpenResolver, gameId, allMods, conflicts, onError, workshopDetails, workshopDetailsState, workshopDetailsError, authorProfiles}: {
    mod: library.ModSummary | null;
    tab: DetailTab;
    onTab: (t: DetailTab) => void;
    onOpenResolver: () => void;
    gameId: string;
    allMods: library.ModSummary[];
    conflicts: library.ConflictSummary[];
    onError: (message: string) => void;
    // Real Steam Workshop metadata/author profiles - fetched once per game
    // at the Workspace level (not lazily per mod-selection here), since
    // the Available/Active lists need the same data too. See Workspace's
    // own state of the same name for how/when this populates.
    workshopDetails: Map<string, steamapi.PublishedFileDetails>;
    workshopDetailsState: 'idle' | 'loading' | 'loaded' | 'error';
    workshopDetailsError: string;
    authorProfiles: Map<string, steamapi.Profile>;
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

    // steamDetails is this mod's own real Workshop metadata, if it's a
    // Workshop mod and it's been fetched yet (see Workspace's
    // workshopDetails state) - Result !== 1 means Steam itself reports
    // the item as gone/banned/private, treated the same as "not found"
    // everywhere this is used (the Changes tab, the detail header's
    // author/updated line, Overview's description fallback).
    const steamDetails = mod?.RemoteFileID ? workshopDetails.get(mod.RemoteFileID) : undefined;
    const validSteamDetails = steamDetails && steamDetails.Result === 1 ? steamDetails : undefined;
    const authorName = authorNameFor(mod, workshopDetails, authorProfiles);

    // Real Steam Workshop update notes for the Changes tab - fetched per
    // mod, on demand, the first time that mod's own Changes tab is
    // opened (there's no batching endpoint for this, unlike
    // WorkshopDetails - see docs/steam-web-api.md), then kept in memory
    // keyed by published file id.
    const [changelogs, setChangelogs] = useState<Map<string, steamapi.ChangelogEntry[]>>(new Map());
    const [changelogState, setChangelogState] = useState<'idle' | 'loading' | 'loaded' | 'error'>('idle');
    const [changelogError, setChangelogError] = useState('');

    useEffect(() => {
        setChangelogs(new Map());
        setChangelogState('idle');
    }, [gameId]);

    useEffect(() => {
        const remoteFileID = mod?.RemoteFileID;
        if (tab !== 'changes' || !mod || mod.Source !== 'workshop' || !remoteFileID || changelogs.has(remoteFileID)) {
            return;
        }
        setChangelogState('loading');
        let cancelled = false;
        ModChangelog(remoteFileID)
            .then((entries) => {
                if (cancelled) return;
                setChangelogs((prev) => new Map(prev).set(remoteFileID, entries));
                setChangelogState('loaded');
            })
            .catch((err) => {
                if (cancelled) return;
                setChangelogError(String(err));
                setChangelogState('error');
            });
        return () => { cancelled = true; };
    }, [tab, mod, changelogs]);

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
                            {(() => {
                                // A workshop mod's real Steam data (once fetched) gives a
                                // more meaningful "updated" than local file mtime - the
                                // Workshop item's own last-updated time, plus the real
                                // author name when it's resolved, matching the mockup's
                                // own "Author Name · updated 3 days ago" convention.
                                if (validSteamDetails) {
                                    return authorName
                                        ? `${authorName} · updated ${timeAgo(validSteamDetails.TimeUpdated)}`
                                        : `Updated ${timeAgo(validSteamDetails.TimeUpdated)}`;
                                }
                                if (filesLoading || (mod.Source === 'workshop' && workshopDetailsState === 'loading')) {
                                    return <span className="skeleton skeleton-text" style={{width: '90px'}}/>;
                                }
                                return files?.LastModified ? `Updated ${timeAgo(files.LastModified)}` : '';
                            })()}
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
                            <OverviewTab
                                mod={mod}
                                files={files}
                                filesLoading={filesLoading}
                                allMods={allMods}
                                conflicts={myConflicts}
                                onOpenFolder={openFolder}
                                steamDescription={validSteamDetails?.Description ? stripBBCode(validSteamDetails.Description) : ''}
                            />
                        )}
                        {tab === 'files' && (
                            <div className="file-tree">
                                {filesError && (
                                    <div className="content-missing">
                                        <i className="fa-solid fa-folder-xmark"/>
                                        <p>{filesError}</p>
                                    </div>
                                )}
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
                                {files && files.Entries.length > 0 && <FileTree entries={files.Entries}/>}
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
                                {mod.Source !== 'workshop' && (
                                    <p className="detail-empty">
                                        Update history isn't available - only Steam Workshop mods have a real page to
                                        fetch it from.
                                    </p>
                                )}
                                {mod.Source === 'workshop' && workshopDetailsState === 'loading' && (
                                    <p className="detail-empty">Checking Steam Workshop...</p>
                                )}
                                {mod.Source === 'workshop' && workshopDetailsState === 'error' && (
                                    <div className="content-missing">
                                        <i className="fa-solid fa-circle-exclamation"/>
                                        <p>{workshopDetailsError}</p>
                                    </div>
                                )}
                                {mod.Source === 'workshop' && workshopDetailsState === 'loaded' && (() => {
                                    const d = validSteamDetails;
                                    if (!d) {
                                        return <p className="detail-empty">No Steam Workshop data found for this mod.</p>;
                                    }
                                    const author = d.Creator ? authorProfiles.get(d.Creator) : undefined;
                                    return (
                                        <>
                                            {author && (
                                                <div className="author-row" onClick={() => BrowserOpenURL(author.ProfileURL)}>
                                                    {author.AvatarURL
                                                        ? <img className="author-avatar" src={author.AvatarURL} alt={author.Name}/>
                                                        : <span className="author-avatar author-avatar-fallback"/>}
                                                    <div className="author-info">
                                                        <span className="author-name">{author.Name}</span>
                                                        {author.MemberSince && (
                                                            <span className="author-sub">Member since {author.MemberSince}</span>
                                                        )}
                                                    </div>
                                                    <i className="fa-solid fa-arrow-up-right-from-square author-link-icon"/>
                                                </div>
                                            )}
                                            <div className="overview-grid">
                                                <span className="label">Subscribers</span><span className="value mono">{d.Subscriptions.toLocaleString()}</span>
                                                <span className="label">Favorited</span><span className="value mono">{d.Favorited.toLocaleString()}</span>
                                                <span className="label">Views</span><span className="value mono">{d.Views.toLocaleString()}</span>
                                                <span className="label">Last updated</span><span className="value mono">{timeAgo(d.TimeUpdated)}</span>
                                            </div>
                                            {d.Description && (
                                                <div className="section">
                                                    <div className="section-label">STEAM DESCRIPTION</div>
                                                    <div className="section-body">{stripBBCode(d.Description)}</div>
                                                </div>
                                            )}
                                            <div className="section">
                                                <div className="section-label">RECENT UPDATES</div>
                                                {changelogState === 'loading' && (
                                                    <p className="detail-empty">Checking Steam Workshop...</p>
                                                )}
                                                {changelogState === 'error' && (
                                                    <div className="content-missing">
                                                        <i className="fa-solid fa-circle-exclamation"/>
                                                        <p>{changelogError}</p>
                                                    </div>
                                                )}
                                                {mod.RemoteFileID && changelogs.has(mod.RemoteFileID) && (() => {
                                                    const entries = changelogs.get(mod.RemoteFileID)!;
                                                    if (entries.length === 0) {
                                                        return <p className="detail-empty">No update notes found for this mod yet.</p>;
                                                    }
                                                    return (
                                                        <div className="changelog-list">
                                                            {entries.map((entry, i) => (
                                                                <div key={i} className="changelog-entry">
                                                                    <div className="changelog-entry-head">
                                                                        <span className="mono">{entry.Headline}</span>
                                                                        {entry.Author && <span className="changelog-entry-author">by {entry.Author}</span>}
                                                                    </div>
                                                                    {entry.Body && <div className="changelog-entry-body">{entry.Body}</div>}
                                                                </div>
                                                            ))}
                                                        </div>
                                                    );
                                                })()}
                                            </div>
                                        </>
                                    );
                                })()}
                            </div>
                        )}
                    </div>
                </>
            )}
        </div>
    );
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

// stripBBCode gives a readable plain-text preview of a real Steam Workshop
// description - those are BBCode ([b], [url=...], [img]...), not something
// worth building a real renderer for here. Not a full parser, just enough
// to keep raw markup out of view.
function stripBBCode(s: string): string {
    return s.replace(/\[[^\]]*\]/g, '').trim();
}

function OverviewTab({mod, files, filesLoading, allMods, conflicts, onOpenFolder, steamDescription}: {
    mod: library.ModSummary;
    files: library.ModFiles | null;
    filesLoading: boolean;
    allMods: library.ModSummary[];
    conflicts: library.ConflictSummary[];
    onOpenFolder: () => void;
    // The mod's real Steam Workshop description (BBCode already stripped),
    // when it's a Workshop mod and that's been fetched - classic
    // descriptors rarely set their own short_description field, so this is
    // the real content behind "DESCRIPTION" for most Workshop mods.
    steamDescription: string;
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
                <div className="section-body">{mod.ShortDescription || steamDescription || 'No description provided.'}</div>
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


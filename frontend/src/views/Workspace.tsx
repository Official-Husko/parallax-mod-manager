import './Workspace.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {
    AuthorProfiles,
    GetPreferences,
    IgnoredIncompatibleMods,
    ImportLauncherPlaysets,
    LaunchGame,
    ListModFiles,
    DeletePlayset,
    ListPlaysets,
    LoadPlayset,
    ModChangelog,
    ModThumbnail,
    OpenModFolder,
    RenamePlayset,
    SavePlayset,
    ScanGame,
    SetModIncompatibilityIgnored,
    StopGame,
    WatchMods,
    BackupMod,
    WorkshopAvailability,
    WorkshopDetails,
} from '../../wailsjs/go/main/App';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import type {launcherdb, library, playset, preferences, steamapi} from '../../wailsjs/go/models';
import {autosort, findMissingActiveDependencies, type MissingActiveDependencies} from '../data/autosort';
import {dismiss, notify, trackTask, updateNotification} from '../data/notifications';
import {backupNote, type WorkshopFlag, workshopFlags, workshopFlagStyle, workshopFlagText} from '../data/workshopAvailability';
import {describePatchStatus, patchNeedsAttention} from '../data/patchStatus';
import {logEvent} from '../data/appLog';
import {useGameRunning} from '../data/gameStatus';
import {type ContextMenuItem, openContextMenu} from '../data/contextMenu';
import {useDragMultiSelect} from '../data/dragMultiSelect';
import {useListDragMove} from '../data/listDragMove';
import {reorderInsert} from '../data/listReorder';
import {noteMatches, useModNotes} from '../data/modNotes';
import {ModNote, noteTip} from '../components/ModNote';
import {domains} from '../data/mockData';
import {playsetAutoloadTarget} from '../data/playsetAutoload';
import {computeDomainOverlap} from '../data/domainOverlap';
import {buildPreflightItems, checksumPreflightItem, findDependencyIssues} from '../data/preflight';
import {checkVersionCompatibility, displayVersion} from '../data/versionCompat';
import {formatBytes, timeAgo, truncate} from '../data/format';
import {SourceBadge} from '../components/SourceBadge';
import {FLAG, conflictsByMod} from '../data/flags';
import {tip} from '../data/tooltip';
import {colorFromName} from '../data/nameColor';
import {listEditedSinceScan, liveConflicts} from '../data/liveConflicts';
import {hasUnsavedChanges} from '../data/playsetDirty';
import {checksumBadge, usePlaysetChecksum} from '../data/checksum';
import {patchPreferences} from '../data/preferencesPatch';
import {domainLegendTip, domainTip, flagLegendTip, modFlagsTip} from '../components/FlagTips';
import {TipItem} from '../components/Tooltip';
import {UpdatesCard} from '../components/UpdatesCard';
import {checkModUpdates} from '../data/modUpdates';
import {EmptyState} from '../components/EmptyState';
import {FileTree} from '../components/FileTree';
import {AutosortMissingDepsModal} from './AutosortMissingDepsModal';
import {AutosortUnresolvedDepsModal} from './AutosortUnresolvedDepsModal';
import {ConflictResolver} from './ConflictResolver';
import {GameLogModal} from './GameLogModal';
import {PlaysetsModal} from './PlaysetsModal';
import {PreflightModal} from './PreflightModal';
import {PurgeEmptyModal} from './PurgeEmptyModal';

type DetailTab = 'overview' | 'files' | 'conflicts' | 'changelog';

// Hard caps for the couple of places a mod's real title or a Steam
// author's real display name gets shown at a fixed, prominent size with
// no natural width-driven CSS ellipsis to lean on (the detail panel's own
// big title, the author card) - see truncate() in data/format.ts. List
// rows elsewhere already handle overflow via CSS (a real, bounded column
// width), so they don't need this.
const MAX_DETAIL_NAME_LENGTH = 70;
const MAX_AUTHOR_NAME_LENGTH = 40;

export function Workspace({games, selectedGame, gameVersion, onPlaysetNameChange, onOpenUpdates, showPlaysets, setShowPlaysets}: {
    games: library.GameInfo[];
    selectedGame: string;
    // The real, currently-installed game version (e.g. "v4.4.6"), fetched
    // once by app.tsx - "" when it couldn't be determined. Used to flag a
    // mod whose own declared supported_version is confirmed incompatible
    // (see data/versionCompat.ts) - never to block or hide anything, just
    // to color/mark it.
    gameVersion: string;
    onPlaysetNameChange: (name: string) => void;
    onOpenUpdates: () => void;
    // Controlled from app.tsx, not local state - the TopBar's own
    // playset pill (a sibling of this component, not a parent/child)
    // needs to open this same real switcher, not a second one of its
    // own.
    showPlaysets: boolean;
    setShowPlaysets: (show: boolean) => void;
}) {
    const [summary, setSummary] = useState<library.Summary | null>(null);
    const [order, setOrderState] = useState<string[]>([]);
    // Every deliberate change to the load order (a playset loaded, a mod added,
    // dragged, removed, the list cleared...) goes through setOrder, which counts
    // it. A scan that finishes later must not put its own idea of the order over
    // one of these - see the scan-result refs below. What a scan itself derives
    // (its enabled mods, pruning what no longer exists) uses setOrderState and is
    // not counted.
    const orderEditsRef = useRef(0);
    const setOrder: typeof setOrderState = (value) => {
        orderEditsRef.current++;
        setOrderState(value);
    };
    const [selectedId, setSelectedId] = useState('');
    const [selectedAvailable, setSelectedAvailable] = useState<Set<string>>(new Set());
    // Active gets the exact same real multi-select gesture as Available -
    // see useDragMultiSelect below - so a row's own highlight can reflect a
    // real multi-row selection there too, not just the single mod shown
    // in the detail panel.
    const [selectedActive, setSelectedActive] = useState<Set<string>>(new Set());
    // The Available list's own temporary, session-local drag arrangement -
    // unlike Active's `order`, this is never saved anywhere; it exists
    // purely so dragging a row within Available (to organize while working,
    // not to activate it) has real positions to reorder, mirroring Active's
    // own drag-reorder rather than doing nothing. Kept reconciled against
    // the real Available set by the effect below, not derived fresh every
    // render, so a drop's own reorder logic (see dragMove's onDrop) can
    // safely update it with a functional setState the same way Active's
    // real order already does.
    const [availableOrder, setAvailableOrder] = useState<string[]>([]);
    const [detailTab, setDetailTab] = useState<DetailTab>('overview');
    const [playsetName, setPlaysetNameState] = useState('');
    // The load order the playset had when it was last loaded or saved - what the list is
    // compared with to tell whether there is anything to save (the Save button blinks
    // while there is). null while no saved playset is behind the list: a new draft, an
    // imported playset, a deleted one. See data/playsetDirty.ts.
    const [savedOrder, setSavedOrder] = useState<string[] | null>(null);
    const playsetNameInputRef = useRef<HTMLInputElement>(null);
    const [playsetList, setPlaysetList] = useState<string[]>([]);
    // Real playsets found in the Paradox Launcher's own database
    // (launcher-v2.sqlite), read-only - see docs/launcher-database.md. A
    // fetch failure here is non-fatal and left silent (an empty list) -
    // this is a bonus convenience on top of this project's own real
    // playset storage above, not something worth interrupting the rest of
    // Workspace with an error banner over.
    const [launcherPlaysets, setLauncherPlaysets] = useState<launcherdb.Playset[]>([]);
    // The active playset's DLC toggles - preserved across save/launch so
    // Workspace never silently wipes what the DLC screen set. Empty for a
    // brand-new, unsaved playset; loaded from the real file otherwise.
    const [disabledDlc, setDisabledDlc] = useState<string[]>([]);
    // Mods whose position in order is locked - can't be dragged, moved up/down, or touched by
    // Autosort (see data/autosort.ts). Position-only: turning a locked mod off still works
    // normally, and the effect below drops its lock the moment that happens, so a lock on a mod
    // no longer in order never lingers into a save.
    const [locked, setLocked] = useState<Set<string>>(new Set());
    useEffect(() => {
        setLocked((prev) => {
            const next = new Set([...prev].filter((id) => order.includes(id)));
            return next.size === prev.size ? prev : next;
        });
    }, [order]);
    // dragMove's onDrop below is captured once at mount (see data/listDragMove.ts's own
    // comment) and can only read state fresh through a functional setState update or a ref -
    // never a closed-over variable - so it reads this ref rather than `locked` directly.
    const lockedRef = useRef(locked);
    useEffect(() => { lockedRef.current = locked; }, [locked]);
    const [showPreflight, setShowPreflight] = useState(false);
    const [showGameLog, setShowGameLog] = useState(false);
    // The multiplayer checksum of the saved playset: worked out whenever one is saved or
    // loaded (see handleSave and handleLoadPlayset), and again when mods change on disk.
    const checksum = usePlaysetChecksum(selectedGame);
    // Whether the game is running - whoever started it - so Play can become Stop.
    const game = useGameRunning(selectedGame);
    // Stopping the game loses whatever it hadn't saved, so the first press only
    // asks; a second, within a few seconds, does it.
    const [stopStep, setStopStep] = useState<'idle' | 'confirm' | 'stopping'>('idle');
    const stopConfirmTimer = useRef<number | undefined>(undefined);
    const [showConflictResolver, setShowConflictResolver] = useState(false);
    // The notification currently asking the user to review a stale generated
    // patch (see the effect below), and the situation it was raised for - so
    // a rescan that finds the very same staleness doesn't raise a second one,
    // but a different situation (or a fixed one) replaces or clears it.
    const patchAlertRef = useRef<{ id: string; signature: string } | null>(null);
    const [showPurgeModal, setShowPurgeModal] = useState(false);
    // Set by handleAutosort when it finds a currently-active mod's own
    // declared dependency isn't itself active yet - null means no pending
    // question. See AutosortMissingDepsModal. Only holds the resolvable
    // half (see MissingActiveDependencies) - the unresolved half is shown
    // afterward instead, via unresolvedDeps below, since there's nothing
    // to ask about those (see finishAutosort).
    const [missingDeps, setMissingDeps] = useState<MissingActiveDependencies | null>(null);
    // Set once Autosort has actually run, if it found any dependency that
    // doesn't match anything scanned at all - see AutosortUnresolvedDepsModal.
    const [unresolvedDeps, setUnresolvedDeps] = useState<string[] | null>(null);
    const [search, setSearch] = useState('');
    const [activeSearch, setActiveSearch] = useState('');
    // The 'Scanning...' / 'Resolving conflicts...' progress notification of the
    // scan in flight, if any - see refreshMods and the scan-quick listener.
    const scanNoticeRef = useRef<string | null>(null);
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    // Mod IDs whose version-incompatibility warning the user has
    // explicitly chosen to suppress for this game (see the mod context
    // menu's own "Ignore incompatibility" - data/versionCompat.ts still
    // computes the real compatibility; this only controls whether a
    // confirmed mismatch is shown as a live warning or a quiet ignored one.
    const [ignoredIncompatible, setIgnoredIncompatible] = useState<Set<string>>(new Set());
    // Guards the 'scan-quick' listener below so it only ever applies to the
    // fresh load it belongs to - a watcher-triggered background refresh
    // (refreshMods(true)) runs the exact same backend scan and fires the
    // same event, but must never have its quick preview reset the load
    // order/selection the way a fresh load's first paint is supposed to.
    const expectingFreshQuickRef = useRef(false);
    const quickAppliedRef = useRef(false);
    // Scans run concurrently and finish in any order: the full scan a game switch
    // starts (no playset yet), the scan that loading the remembered playset starts
    // right after it, background refreshes. Applying each result the moment it
    // lands let a late one from an older scan overwrite a newer one - most visibly
    // the playset's name showing with none of its mods, and conflicts from the
    // wrong scan. So: every scan takes a number (latestScanRef), only the newest
    // is applied (applyScan), and a scan's own order only lands if the order was
    // not edited since its fresh load began (editsAtFreshScanRef).
    const latestScanRef = useRef(0);
    const freshScanRef = useRef(0);
    const editsAtFreshScanRef = useRef(0);
    const summaryRef = useRef<library.Summary | null>(null);
    summaryRef.current = summary;
    const selectedGameRef = useRef(selectedGame);
    selectedGameRef.current = selectedGame;

    // applyScan shows a scan's result unless it is stale: for a game that is no
    // longer selected, or older than a scan started since - unless nothing is held
    // for this game yet, when something beats a blank list (the newer scan
    // replaces it when it lands). Says whether it was applied.
    function applyScan(seq: number, result: library.Summary): boolean {
        if (result.Game.ID !== selectedGameRef.current) return false;
        const holdsThisGame = summaryRef.current?.Game.ID === result.Game.ID;
        if (seq !== latestScanRef.current && holdsThisGame) return false;
        summaryRef.current = result;
        setSummary(result);
        return true;
    }
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

    // Renames a saved playset. The backend also repoints the settings that
    // remember a playset by name, so this refetches them - otherwise this
    // component's own copy, still holding the old name, would write it straight
    // back the next time it saves a setting. If it's the playset currently
    // loaded, the loaded name follows. Resolves to null, or a message for the
    // playset's row.
    async function renamePlayset(oldName: string, newName: string): Promise<string | null> {
        try {
            await RenamePlayset(selectedGame, oldName, newName);
        } catch (err) {
            return String(err);
        }
        ListPlaysets(selectedGame).then(setPlaysetList).catch(() => undefined);
        GetPreferences().then(setPrefs).catch(() => undefined);
        if (playsetNameRef.current === oldName) {
            setPlaysetName(newName.trim());
            checksum.calculate(newName);
        }
        return null;
    }

    // Deletes a saved playset. If it's the one currently loaded, the load order
    // on screen stays as it is but becomes an unsaved draft again (no name) -
    // deleting a playset never throws away what you're looking at.
    async function deletePlayset(name: string): Promise<string | null> {
        try {
            await DeletePlayset(selectedGame, name);
        } catch (err) {
            return String(err);
        }
        ListPlaysets(selectedGame).then(setPlaysetList).catch(() => undefined);
        GetPreferences().then(setPrefs).catch(() => undefined);
        if (playsetNameRef.current === name) {
            setPlaysetName('');
            setSavedOrder(null);
        }
        return null;
    }

    // Persists name as selectedGame's "last active playset" (see
    // preferences.Preferences.LastActivePlaysets) so the mount effect
    // below can reload it automatically next time this game is opened,
    // instead of always starting blank with Play disabled. Called after an
    // explicit save or switch, never for an unsaved draft (importing a
    // Paradox Launcher playset, or "New") - those aren't in ListPlaysets
    // yet, so pointing this at one of them would just fail to auto-load
    // next time anyway. Best-effort and silently skipped if prefs hasn't
    // loaded yet - worse case is simply not auto-loading next time, not a
    // lost setting, so this never blocks the save/load it's attached to.
    function rememberActivePlayset(name: string) {
        // Built on the current settings, not this view's copy: that was read when the view
        // mounted and would put back whatever Settings changed since (see preferencesPatch).
        const game = selectedGame;
        patchPreferences((current) => ({lastActivePlaysets: {...current.lastActivePlaysets, [game]: name}}))
            .then(setPrefs)
            .catch(() => undefined);
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
        // Held locally, not read back from the ref: a second full scan (the
        // game was switched mid-scan) replaces the ref, and this one must
        // still settle its own notification, not the newer scan's.
        let scanId = '';
        let freshToken = 0;
        if (!preserveSelection) {
            if (scanNoticeRef.current) dismiss(scanNoticeRef.current);
            setPlaysetName('');
            setSavedOrder(null);
            setDisabledDlc([]);
            setSelectedAvailable(new Set());
            scanNoticeRef.current = scanId = notify('progress', 'Scanning mods...');
            expectingFreshQuickRef.current = true;
            quickAppliedRef.current = false;
            freshToken = ++freshScanRef.current;
            editsAtFreshScanRef.current = orderEditsRef.current;
        }
        const seq = ++latestScanRef.current;
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
            // Only the fresh scan that armed it may disarm the early preview.
            if (freshToken !== 0 && freshToken === freshScanRef.current) expectingFreshQuickRef.current = false;
            if (applyScan(seq, result)) {
                const untouched = orderEditsRef.current === editsAtFreshScanRef.current;
                if (preserveSelection || quickAppliedRef.current || !untouched) {
                    const freshIds = new Set(result.Mods.map((m) => m.ID));
                    setOrderState((prev) => prev.filter((id) => freshIds.has(id)));
                    setSelectedAvailable((prev) => new Set([...prev].filter((id) => freshIds.has(id))));
                } else {
                    setOrderState(result.Mods.filter((m) => m.Enabled).map((m) => m.ID));
                }
            }
            if (!preserveSelection) {
                dismiss(scanId);
            }
        } catch (err) {
            if (freshToken !== 0 && freshToken === freshScanRef.current) expectingFreshQuickRef.current = false;
            if (!preserveSelection) {
                updateNotification(scanId, {kind: 'error', message: `Scan failed: ${String(err)}`});
            }
        } finally {
            if (scanNoticeRef.current === scanId) scanNoticeRef.current = null;
        }
    }

    useEffect(() => {
        if (!selectedGame) {
            return;
        }
        WatchMods(selectedGame).catch(() => undefined);
        refreshMods(false);
        ListPlaysets(selectedGame)
            .then(async (names) => {
                setPlaysetList(names);
                // Auto-loads per Settings' Playsets panel - "last" (the
                // default) reloads LastActivePlaysets, "custom" always
                // reloads whichever playset is pinned there, "off" leaves
                // this game blank until the user explicitly picks one
                // (Play still works either way - see LaunchGame). Fetched
                // fresh here rather than read from this component's own
                // `prefs` state, which only loads once on mount (a
                // separate effect below) and could still be null by the
                // time this resolves. Re-verified against the names list
                // that just came back, in case the target playset was
                // since renamed or deleted.
                const savedPrefs = await GetPreferences().catch(() => null);
                const wanted = playsetAutoloadTarget(savedPrefs, selectedGame);
                if (wanted && names.includes(wanted)) {
                    handleLoadPlayset(wanted);
                }
            })
            .catch((err) => notify('error', `Couldn't load playsets: ${String(err)}`));
        ImportLauncherPlaysets(selectedGame)
            .then(setLauncherPlaysets)
            .catch(() => setLauncherPlaysets([]));
        IgnoredIncompatibleMods(selectedGame)
            .then((ids) => setIgnoredIncompatible(new Set(ids)))
            .catch(() => setIgnoredIncompatible(new Set()));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    // Toggles modId's membership in the current game's ignored-
    // incompatibility set, optimistically - reverted if the backend save
    // actually fails, same pattern as Settings.tsx's own togglePref.
    async function setIncompatibilityIgnored(modId: string, ignored: boolean) {
        setIgnoredIncompatible((prev) => {
            const next = new Set(prev);
            if (ignored) next.add(modId); else next.delete(modId);
            return next;
        });
        try {
            await SetModIncompatibilityIgnored(selectedGame, modId, ignored);
        } catch (err) {
            setIgnoredIncompatible((prev) => {
                const next = new Set(prev);
                if (ignored) next.delete(modId); else next.add(modId);
                return next;
            });
            notify('error', `Couldn't save that setting: ${String(err)}`);
        }
    }

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
                // Mods were added, removed or rewritten on disk: what changed since
                // the last startup may have changed with them - and so may the checksum.
                checkModUpdates(gameId);
                checksum.refresh();
            }
        });
        return () => unsubscribe();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    // Tell the user when the generated patch has gone stale - a mod it was
    // built from updated, a new conflict appeared, or the chosen winners no
    // longer match - instead of leaving them to notice in the resolver. It's a
    // persistent notification (they can act on it later), raised once per
    // distinct situation and taken down as soon as the patch is current again.
    useEffect(() => {
        const patch = summary?.Patch;
        if (!patch) {
            // The quick scan preview has no patch information yet - leave
            // whatever is showing alone until the full result arrives.
            return;
        }
        const current = patchAlertRef.current;
        if (!patchNeedsAttention(patch)) {
            if (current) {
                dismiss(current.id);
                patchAlertRef.current = null;
            }
            return;
        }
        const signature = `${selectedGame}:${patch.Generation}:${patch.Changed}:${patch.New}:${patch.Obsolete}:${patch.GameChanged}:${(patch.ChangedMods ?? []).join('|')}`;
        if (current?.signature === signature) {
            return;
        }
        if (current) {
            dismiss(current.id);
        }
        const id = notify('warning', describePatchStatus(patch), {
            action: {label: 'Review', onClick: () => setShowConflictResolver(true)},
        });
        patchAlertRef.current = {id, signature};
    }, [summary, selectedGame]);

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
            // Only the first paint for a game: once anything is held for it, a later
            // scan's early preview (no conflicts yet) is never newer than that.
            if (summaryRef.current?.Game.ID !== gameId) {
                summaryRef.current = quick;
                setSummary(quick);
            }
            // And only over an order nobody has set since this fresh load began - a
            // playset loaded in the meantime is not this preview's to replace.
            if (orderEditsRef.current === editsAtFreshScanRef.current) {
                setOrderState(quick.Mods.filter((m) => m.Enabled).map((m) => m.ID));
            }
            if (scanNoticeRef.current) updateNotification(scanNoticeRef.current, {message: 'Resolving conflicts...'});
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
    // Which Workshop mods are unlisted, private or deleted (by Workshop item id) -
    // worked out by the backend once the details above are in. Empty until then, and
    // for a game whose mods are all ordinary.
    const [workshopFlagsById, setWorkshopFlagsById] = useState<Map<string, WorkshopFlag>>(new Map());
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
        setWorkshopFlagsById(new Map());
        setWorkshopDetailsState('idle');
        workshopDetailsStartedRef.current = false;
    }, [selectedGame]);

    // A copy finishing, or the background check finding that some mod's Workshop status
    // changed, updates the flags: asking again is cheap (the details and the item pages
    // are remembered) and never starts a copy that is already made.
    useEffect(() => {
        const refetch = (gameId?: string) => {
            if (gameId && gameId !== selectedGame) return;
            WorkshopAvailability(selectedGame).then((flags) => setWorkshopFlagsById(workshopFlags(flags))).catch(() => undefined);
        };
        const offChanged = EventsOn('backups-changed', () => refetch());
        const offAvail = EventsOn('workshop-availability-changed', (gameId: string) => refetch(gameId));
        return () => { offChanged(); offAvail(); };
    }, [selectedGame]);

    // Saving, removing or switching the Steam API key (Settings > Steam API) changes what
    // Steam can return - an unlisted mod is "not found" to the free API and complete with a
    // key - so the details are fetched again the new way.
    useEffect(() => {
        const off = EventsOn('steam-api-changed', () => {
            workshopDetailsStartedRef.current = false;
            setWorkshopDetailsRetryTick((t) => t + 1);
        });
        return () => { off(); };
    }, []);

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
                // The flags come after the details (they are worked out from them) and never
                // fail the fetch: without them the lists just show no Workshop flags.
                WorkshopAvailability(selectedGame)
                    .then((flags) => { if (!cancelled) setWorkshopFlagsById(workshopFlags(flags)); })
                    .catch(() => undefined);
                updateNotification(notifId, {
                    kind: 'success',
                    message: `Fetched Steam Workshop details for ${list.length} mod${list.length === 1 ? '' : 's'}.`,
                    action: undefined,
                });
            })
            .catch((err) => {
                settled = true;
                if (cancelled) { dismiss(notifId); return; }
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

    const orderSet = useMemo(() => new Set(order), [order]);
    // The scan's conflicts, narrowed to the mods in the load order right now -
    // see data/liveConflicts.ts. Everything conflict-related below reads these,
    // not summary.Conflicts, so clearing or editing the list is reflected at once.
    const conflicts = useMemo(() => liveConflicts(summary?.Conflicts ?? [], orderSet), [summary, orderSet]);
    const listEdited = useMemo(() => listEditedSinceScan(order, summary?.Mods ?? []), [order, summary]);
    // Something in the load order is not saved yet: the Save button blinks.
    const unsaved = useMemo(() => hasUnsavedChanges(order, savedOrder, (id) => modsById.has(id)), [order, savedOrder, modsById]);
    const conflictedIds = useMemo(() => {
        const s = new Set<string>();
        for (const c of conflicts) {
            for (const cand of c.Candidates) {
                s.add(cand.ModID);
            }
        }
        return s;
    }, [conflicts]);
    // Per mod: how many keys it contests and with whom - the conflict flag's tooltip.
    const conflictInfo = useMemo(() => conflictsByMod(conflicts), [conflicts]);
    // The Active list's own per-row domain segments - see data/domainOverlap.ts.
    const domainOverlap = useMemo(() => computeDomainOverlap(conflicts), [conflicts]);

    // Personal notes about mods, by mod ID (Notes box on the Overview tab, note icon on rows).
    const modNotes = useModNotes(selectedGame);
    // Asked for by right-click > Edit note: which mod's Notes box should take focus, and a
    // number that changes per request so asking again works.
    const [noteFocus, setNoteFocus] = useState<{id: string; n: number} | null>(null);
    const notes = modNotes.notes;

    const available = useMemo(
        () => allMods.filter((m) => !orderSet.has(m.ID) && matchesSearch(m, search, notes)),
        [allMods, orderSet, search, notes],
    );
    // Reconciles availableOrder against the real Available set whenever it
    // changes: ids that left (activated, purged) are dropped, newly
    // available ids are appended in their natural (name-sorted) order, and
    // everything else keeps exactly the position the user last dragged it
    // to. Runs as an effect (not a plain derived value) specifically so
    // dragMove's own reorder-within-Available onDrop can update this state
    // the same safe way Active's real `order` already does - a functional
    // setAvailableOrder call, never reading a possibly-stale `available`
    // from inside that closure.
    useEffect(() => {
        const availIds = new Set(available.map((m) => m.ID));
        setAvailableOrder((prev) => {
            const kept = prev.filter((id) => availIds.has(id));
            const keptSet = new Set(kept);
            const appended = available.filter((m) => !keptSet.has(m.ID)).map((m) => m.ID);
            const next = kept.length === prev.length && appended.length === 0 ? prev : [...kept, ...appended];
            return next;
        });
    }, [available]);
    const availableOrderedMods = useMemo(
        () => availableOrder.map((id) => modsById.get(id)).filter((m): m is library.ModSummary => !!m),
        [availableOrder, modsById],
    );
    const active = order.map((id) => modsById.get(id)).filter((m): m is library.ModSummary => !!m);
    // Active's own row list, search-filtered for display only - Autosort,
    // Save, and the legend/footer all still operate on the full, real
    // active list above, matching Available's own filtered-list-is-
    // display-only precedent (reordering/removal act on a mod id
    // directly, never a filtered index, so this never risks moving or
    // dropping the wrong mod).
    const visibleActive = active.filter((m) => matchesSearch(m, activeSearch, notes));
    // Real load-order position (1-based), independent of activeSearch
    // filtering - a filtered row must still show where it actually sits
    // in the real load order, not its index within the filtered results.
    const positionById = useMemo(() => new Map(active.map((m, i) => [m.ID, i + 1])), [active]);
    const selectedMod = selectedId ? modsById.get(selectedId) ?? null : null;
    const dependencyIssues = useMemo(() => findDependencyIssues(active), [active]);
    const preflightItems = useMemo(
        () => buildPreflightItems(active, conflicts, summary?.Errors ?? [], dependencyIssues, {state: checksum.state, playsetName, unsaved}),
        [active, summary, conflicts, dependencyIssues, checksum.state, playsetName, unsaved],
    );

    const checksumShownBadge = checksumBadge(checksum.state, playsetName, unsaved);
    const workshopCount = allMods.filter((m) => m.Source === 'workshop').length;
    const localCount = allMods.filter((m) => m.Source !== 'workshop').length;

    // Real OS-file-list-style multi-select (click/shift+click) for both
    // the Available and Active lists - see data/dragMultiSelect.ts for the
    // shared mechanics both of these instances drive identically.
    const availableDrag = useDragMultiSelect(
        availableOrder,
        (ids) => setSelectedAvailable(new Set(ids)),
        setSelectedId,
    );
    const activeIds = useMemo(() => visibleActive.map((m) => m.ID), [visibleActive]);
    const activeDrag = useDragMultiSelect(
        activeIds,
        (ids) => setSelectedActive(new Set(ids)),
        setSelectedId,
    );

    // Real drag-and-drop between Available and Active, and within each of
    // them, matching Mod Organizer 2/Vortex/RimSort's own plugin-list
    // behavior - see data/listDragMove.ts. onDrop only ever touches state
    // through a functional setState update (never reads `order`/
    // `availableOrder` from this closure directly), since the hook's own
    // mouseup listener is registered once and would otherwise see
    // whichever value was current the first time a drag started.
    const dragMove = useListDragMove((ids, source, target) => {
        if (source === 'available' && target.list === 'active') {
            // Activate: insert at the dropped position in the load order.
            setOrder((prev) => {
                const toInsert = ids.filter((id) => !prev.includes(id));
                if (toInsert.length === 0) return prev;
                return reorderInsert(prev, toInsert, target);
            });
            setSelectedAvailable(new Set());
        } else if (source === 'active' && target.list === 'available') {
            // Deactivate: drop out of the load order, landing at the exact
            // dropped position in Available's own temporary arrangement -
            // the reconcile effect above then just confirms it's still
            // there (it is, it was just dropped there) rather than
            // re-appending it somewhere else.
            setOrder((prev) => prev.filter((id) => !ids.includes(id)));
            setAvailableOrder((prev) => reorderInsert(prev, ids, target));
            setSelectedActive(new Set());
        } else if (source === 'active' && target.list === 'active') {
            // Reorder within Active - a locked mod among the dragged ids just doesn't move,
            // even when it was dragged as part of a bigger selection; the rest still does.
            setOrder((prev) => {
                const moving = ids.filter((id) => !lockedRef.current.has(id));
                return moving.length === 0 ? prev : reorderInsert(prev, moving, target);
            });
        } else if (source === 'available' && target.list === 'available') {
            // Reorder within Available: same idea, against the user's own
            // temporary arrangement rather than a real persisted order.
            setAvailableOrder((prev) => reorderInsert(prev, ids, target));
        }
    });

    // Wraps availableDrag/activeDrag's own onRowMouseDown so the exact ids
    // a mousedown just selected (a single row, or a shift-extended range)
    // also arm a potential drag-move - it only actually becomes one if the
    // mouse moves a few real pixels before release (see useListDragMove);
    // a plain click never does. Mousedown on a row that's already part of
    // a bigger existing selection preserves that whole selection instead
    // of collapsing it to just this row first, so dragging one of several
    // selected rows drags all of them - matching real Explorer/Finder/MO2
    // (though unlike those, a plain, driftless click here still leaves the
    // bigger selection in place rather than collapsing it on release; a
    // deliberately simpler rule than reproducing that exact nuance).
    function onAvailableRowMouseDown(index: number, e: MouseEvent) {
        const id = availableOrder[index];
        if (e.button === 0 && !e.shiftKey && selectedAvailable.has(id) && selectedAvailable.size > 1) {
            e.preventDefault();
            dragMove.startDrag(availableOrder.filter((x) => selectedAvailable.has(x)), 'available', e);
            return;
        }
        const ids = availableDrag.onRowMouseDown(index, e);
        dragMove.startDrag(ids, 'available', e);
    }

    function onActiveRowMouseDown(index: number, e: MouseEvent) {
        const id = activeIds[index];
        // A locked row never visually lifts as if it were being dragged - dropping it
        // wouldn't move it anyway (see the dragMove callback above).
        if (e.button === 0 && !e.shiftKey && selectedActive.has(id) && selectedActive.size > 1) {
            e.preventDefault();
            dragMove.startDrag(activeIds.filter((x) => selectedActive.has(x) && !locked.has(x)), 'active', e);
            return;
        }
        const ids = activeDrag.onRowMouseDown(index, e);
        dragMove.startDrag(ids.filter((x) => !locked.has(x)), 'active', e);
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
        // Moving either position's own mod is a change to a locked mod's position - the swap
        // below touches both, so either one being locked refuses the whole move.
        if (locked.has(order[i]) || locked.has(order[j])) return;
        const next = order.slice();
        [next[i], next[j]] = [next[j], next[i]];
        setOrder(next);
    }

    function toggleLocked(id: string) {
        setLocked((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id); else next.add(id);
            return next;
        });
    }

    // addToOrder adds one specific mod directly - distinct from
    // addSelectedToOrder above, which acts on the Available list's own
    // current row selection; this is for the context menu's "Add to load
    // order" on a row that may not be selected at all.
    function addToOrder(id: string) {
        if (order.includes(id)) return;
        setOrder([...order, id]);
    }

    function openModFolder(id: string) {
        OpenModFolder(selectedGame, id).catch((err) => notify('error', `Couldn't open the mod folder: ${String(err)}`));
    }

    function copyModId(id: string) {
        navigator.clipboard.writeText(id).catch(() => undefined);
    }

    // Real context menu items for one mod row - shared between the
    // Available and Active lists, since most actions apply to both; the
    // load-order-specific ones (reorder/remove vs. add) are the only real
    // difference, controlled by whether the mod is already in order.
    // Copies a Workshop mod into the backup folder now (Settings > Backup). A copy that
    // is made announces itself; one that was already up to date is said here.
    function backUpNow(m: library.ModSummary) {
        BackupMod(selectedGame, m.ID)
            .then((result) => {
                if (result === 'current') notify('info', `'${m.Name}' is already backed up and has not changed.`);
            })
            .catch((err) => notify('error', `Couldn't back up '${m.Name}': ${String(err).replace(/^Error:\s*/, '')}`));
    }

    // Shows a mod's Notes box and puts the cursor in it.
    function editNote(id: string) {
        setSelectedId(id);
        setDetailTab('overview');
        setNoteFocus((prev) => ({id, n: (prev?.n ?? 0) + 1}));
    }

    function modContextMenuItems(m: library.ModSummary): ContextMenuItem[] {
        const inOrder = order.includes(m.ID);
        const i = order.indexOf(m.ID);
        const items: ContextMenuItem[] = inOrder
            ? [
                {label: 'Move up', onClick: () => moveInOrder(m.ID, -1), disabled: i <= 0 || locked.has(order[i]) || locked.has(order[i - 1])},
                {label: 'Move down', onClick: () => moveInOrder(m.ID, 1), disabled: i < 0 || i >= order.length - 1 || locked.has(order[i]) || locked.has(order[i + 1])},
                {label: locked.has(m.ID) ? 'Unlock position' : 'Lock position', onClick: () => toggleLocked(m.ID)},
                {label: 'Remove from load order', onClick: () => removeFromOrder(m.ID), danger: true},
            ]
            : [
                {label: 'Add to load order', onClick: () => addToOrder(m.ID)},
            ];
        items.push({label: 'Open folder', onClick: () => openModFolder(m.ID), separatorBefore: true});
        if (m.Source === 'workshop' && m.RemoteFileID) {
            items.push({label: 'Open Workshop page', onClick: () => BrowserOpenURL(`https://steamcommunity.com/sharedfiles/filedetails/?id=${m.RemoteFileID}`)});
        }
        if (m.Source === 'workshop' && m.RemoteFileID) {
            items.push({label: 'Back up now', onClick: () => backUpNow(m)});
        }
        items.push({label: 'Copy mod ID', onClick: () => copyModId(m.ID)});
        items.push({label: notes.has(m.ID) ? 'Edit note' : 'Add note', onClick: () => editNote(m.ID), separatorBefore: true});
        if (notes.has(m.ID)) {
            items.push({label: 'Delete note', onClick: () => { void modNotes.save(m.ID, m.Name, ''); }, danger: true});
        }
        const isIgnored = ignoredIncompatible.has(m.ID);
        const compat = checkVersionCompatibility(m.SupportedVersion, gameVersion);
        const isIncompatible = compat.known && !compat.compatible;
        // A previously-ignored mod always offers a way back, even if it
        // happens to no longer be incompatible right now (the game got
        // updated, say) - otherwise a stale ignore could never be cleared
        // except by editing the settings file directly.
        if (isIgnored) {
            items.push({label: 'Stop ignoring incompatibility', onClick: () => setIncompatibilityIgnored(m.ID, false), separatorBefore: true});
        } else if (isIncompatible) {
            items.push({label: 'Ignore incompatibility', onClick: () => setIncompatibilityIgnored(m.ID, true), separatorBefore: true});
        }
        return items;
    }

    function runAutosort(withOrder: string[]) {
        if (!prefs) return;
        const result = autosort(withOrder, modsById, {
            dependencies: prefs.autosortDependencies,
            fixesLast: prefs.autosortFixesLast,
            patchLast: prefs.autosortPatchLast,
        }, locked);
        setOrder(result.order);
        logEvent('info', 'Autosort', `sorted ${withOrder.length} mods: ${result.moves.length} moved (dependencies ${prefs.autosortDependencies ? 'on' : 'off'}, fixes last ${prefs.autosortFixesLast ? 'on' : 'off'}, patch last ${prefs.autosortPatchLast ? 'on' : 'off'})`);
        if (result.cycleMods.length > 0) {
            const names = result.cycleMods.map((id) => modsById.get(id)?.Name ?? id).join(', ');
            logEvent('warn', 'Autosort', `circular dependency involving ${names} - these couldn't be fully ordered`);
            notify('warning', `Autosort: circular dependency involving ${names} - these couldn't be fully ordered.`);
        }
    }

    // Autosort's own dependency rule can only reorder mods already active,
    // never add to it - so a mod that declares a dependency which isn't
    // active at all would just silently sort around the gap. Ask first,
    // rather than doing that quietly: load the resolvable ones and sort,
    // or sort without them - see AutosortMissingDepsModal. A dependency
    // that isn't even in this project's scanned mods at all can't be
    // offered to load, so it's reported afterward instead, once Autosort
    // has actually run (see finishAutosort) - see
    // AutosortUnresolvedDepsModal. Only asked when the dependency rule is
    // actually on (see prefs.autosortDependencies) - nothing to warn
    // about otherwise, since the rule wouldn't run at all.
    function handleAutosort() {
        if (!prefs || order.length === 0) return;
        if (prefs.autosortDependencies) {
            const missing = findMissingActiveDependencies(order, modsById, allMods);
            if (missing.resolvable.length > 0) {
                setMissingDeps(missing);
                return;
            }
            if (missing.unresolved.length > 0) {
                runAutosort(order);
                setUnresolvedDeps(missing.unresolved);
                return;
            }
        }
        runAutosort(order);
    }

    // Runs after the user answers AutosortMissingDepsModal's own question
    // (loadResolvable: whether to add its resolvable mods before
    // sorting), then surfaces AutosortUnresolvedDepsModal if handleAutosort
    // found any dependency that couldn't be resolved at all.
    function finishAutosort(loadResolvable: boolean) {
        if (!missingDeps) return;
        const {resolvable, unresolved} = missingDeps;
        setMissingDeps(null);
        runAutosort(loadResolvable ? [...order, ...resolvable.map((d) => d.id)] : order);
        if (unresolved.length > 0) {
            setUnresolvedDeps(unresolved);
        }
    }

    function handlePurged(result: library.PurgeResult) {
        const deleted = result.Deleted ?? [];
        const errors = result.Errors ?? [];
        if (deleted.length > 0) {
            setOrder((prev) => prev.filter((id) => !deleted.includes(id)));
        }
        if (errors.length > 0) {
            notify('error', `Deleted ${deleted.length} mod${deleted.length === 1 ? '' : 's'}, but: ${errors.join('; ')}`);
        } else if (deleted.length > 0) {
            notify('success', `Deleted ${deleted.length} empty mod${deleted.length === 1 ? '' : 's'}.`);
        }
        if (deleted.length > 0) {
            refreshMods(true);
        }
    }

    async function refreshAfterSave(name: string) {
        const names = await ListPlaysets(selectedGame);
        setPlaysetList(names);
        const seq = ++latestScanRef.current;
        const result = await ScanGame(selectedGame, name);
        applyScan(seq, result);
    }

    async function handleSave() {
        const name = playsetName.trim();
        if (!name) {
            // The Save button blinks for an unnamed draft too, so say what it needs.
            notify('info', 'Type a name for this playset first, then press Save.');
            playsetNameInputRef.current?.focus();
            return;
        }
        await trackTask(`Saving "${name}"...`, async () => {
            // disabledDlc is preserved as loaded (see handleLoadPlayset),
            // not reset - Workspace edits the load order, not DLC toggles
            // (that's the DLC screen's job), so a save here must never
            // silently wipe whatever was really set there.
            const p = {name, gameKey: selectedGame, modIds: order, disabledDlc: disabledDlc, lockedModIds: [...locked]} as playset.Playset;
            await SavePlayset(p);
            setSavedOrder(order);
            checksum.calculate(p.name);
            await refreshAfterSave(p.name);
            rememberActivePlayset(p.name);
        }, {success: `Saved "${name}".`, failure: `Couldn't save "${name}"`});
    }

    async function handleLoadPlayset(name: string) {
        await trackTask(`Loading "${name}"...`, async () => {
            const p = await LoadPlayset(selectedGame, name);
            setOrder(p.modIds ?? []);
            setSavedOrder(p.modIds ?? []);
            setPlaysetName(p.name);
            setDisabledDlc(p.disabledDlc ?? []);
            setLocked(new Set(p.lockedModIds ?? []));
            rememberActivePlayset(p.name);
            checksum.calculate(p.name);
            const seq = ++latestScanRef.current;
            const result = await ScanGame(selectedGame, p.name);
            applyScan(seq, result);
            setShowPlaysets(false);
        }, {failure: `Couldn't load "${name}"`});
    }

    // Imports one of the Paradox Launcher's own real playsets (see
    // ImportLauncherPlaysets above) as the current, unsaved draft - the
    // same "build it, then explicitly Save" flow "New" already uses in
    // PlaysetsModal, not an immediate write to this project's own playset
    // storage. GameRegistryID ("mod/<id>.mod") is converted back to this
    // project's own bare mod id the same way internal/launch's
    // classicDescriptorPath builds it going the other direction; only
    // enabled entries that are actually part of the currently scanned mod
    // list end up in the imported order - anything else (installed via
    // the Launcher but not found by this project's own scan, or simply no
    // longer enabled in the source playset) is skipped and reported.
    function handleImportLauncherPlayset(p: launcherdb.Playset) {
        const enabledMods = p.Mods.filter((m) => m.Enabled);
        const resolved = enabledMods
            .map((m) => m.GameRegistryID.replace(/^mod\//, '').replace(/\.mod$/, ''))
            .filter((id) => modsById.has(id));
        setOrder(resolved);
        setSavedOrder(null);
        setPlaysetName(p.Name);
        setSelectedAvailable(new Set());
        setShowPlaysets(false);
        const skipped = enabledMods.length - resolved.length;
        if (skipped > 0) {
            const subject = skipped === 1 ? '1 mod' : `${skipped} mods`;
            notify('info', `Imported "${p.Name}" from the Paradox Launcher - ${subject} not currently found here ${skipped === 1 ? 'was' : 'were'} skipped. Save to keep this playset.`);
        } else {
            notify('success', `Imported "${p.Name}" from the Paradox Launcher (${resolved.length} mods). Save to keep it.`);
        }
    }

    // No playset name is a real, supported case here, not a blocked one -
    // see LaunchGame's own doc comment on the Go side. Skips SavePlayset
    // entirely (there's nothing to name) and passes "" straight through,
    // which tells LaunchGame to skip writing any state and launch the
    // game against whatever's already on disk untouched - either this
    // app's own last real playset write, or the game's own state from
    // before this app ever touched it.
    async function handleLaunchAnyway() {
        setShowPreflight(false);
        const launched = await trackTask('Launching...', async () => {
            if (playsetName.trim()) {
                const p = {name: playsetName.trim(), gameKey: selectedGame, modIds: order, disabledDlc: disabledDlc, lockedModIds: [...locked]} as playset.Playset;
                await SavePlayset(p);
                setSavedOrder(order);
                checksum.calculate(p.name);
                await LaunchGame(selectedGame, p.name);
            } else {
                await LaunchGame(selectedGame, '');
            }
        }, {failure: "Couldn't launch"});
        // The game appears a moment after this returns: look often for a while.
        if (launched) game.checkSoon();
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;

    // A closed game (or a different one selected) leaves nothing to confirm.
    useEffect(() => {
        if (!game.running) {
            window.clearTimeout(stopConfirmTimer.current);
            setStopStep('idle');
        }
    }, [game.running, selectedGame]);
    useEffect(() => () => window.clearTimeout(stopConfirmTimer.current), []);

    async function handleStopGame() {
        if (stopStep === 'stopping') {
            return;
        }
        if (stopStep === 'idle') {
            setStopStep('confirm');
            window.clearTimeout(stopConfirmTimer.current);
            stopConfirmTimer.current = window.setTimeout(() => setStopStep('idle'), 4000);
            return;
        }
        window.clearTimeout(stopConfirmTimer.current);
        setStopStep('stopping');
        logEvent('info', 'Launch', `stop requested for '${gameName}'`);
        try {
            await StopGame(selectedGame);
        } catch (err) {
            notify('error', String(err));
        } finally {
            setStopStep('idle');
            game.checkSoon();
        }
    }

    return (
        <div className="workspace">
            {summary && (
                <div className="workspace-body">
                    <DetailPanel
                        mod={selectedMod}
                        tab={detailTab}
                        onTab={setDetailTab}
                        onOpenResolver={() => setShowConflictResolver(true)}
                        gameId={selectedGame}
                        gameVersion={gameVersion}
                        allMods={allMods}
                        conflicts={conflicts}
                        onError={(message) => notify('error', message)}
                        onSelectMod={setSelectedId}
                        workshopDetails={workshopDetails}
                        workshopDetailsState={workshopDetailsState}
                        workshopFlagsById={workshopFlagsById}
                        authorProfiles={authorProfiles}
                        ignoredIncompatible={ignoredIncompatible}
                        note={selectedMod ? notes.get(selectedMod.ID) ?? '' : ''}
                        noteError={modNotes.error}
                        noteFocusRequest={selectedMod && noteFocus?.id === selectedMod.ID ? noteFocus.n : 0}
                        onNoteFocused={() => setNoteFocus(null)}
                        onSaveNote={(text) => selectedMod ? modNotes.save(selectedMod.ID, selectedMod.Name, text) : Promise.resolve(false)}
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
                            <span className="column-header-ver">SUPPORTS</span>
                            <span className="column-header-author">AUTHOR</span>
                        </div>
                        <div
                            className={`list-rows ${dragMove.dropTarget?.list === 'available' && dragMove.dropTarget.kind === 'end' ? 'drop-at-end' : ''}`}
                            ref={dragMove.availableRowsRef}
                        >
                            {available.length === 0 && allMods.length === 0 && (
                                <EmptyState
                                    icon="fa-box-open"
                                    title="No mods found"
                                    subtitle="Install some mods for this game, or add an extra folder to search under Settings → Paths & folders."
                                />
                            )}
                            {available.length === 0 && allMods.length > 0 && search.trim() !== '' && (
                                <EmptyState icon="fa-magnifying-glass" title="No matches" subtitle={`Nothing found for "${search}".`}/>
                            )}
                            {available.length === 0 && allMods.length > 0 && search.trim() === '' && (
                                <EmptyState icon="fa-circle-check" title="Everything's active" subtitle="Every scanned mod is already in your load order."/>
                            )}
                            {availableOrderedMods.map((m, index) => {
                                const author = authorNameFor(m, workshopDetails, authorProfiles);
                                const authorLoading = m.Source === 'workshop'
                                    && (workshopDetailsState === 'loading' || (workshopDetailsState === 'loaded' && authorProfilesState === 'loading'));
                                const isDropBefore = dragMove.dropTarget?.list === 'available' && dragMove.dropTarget.kind === 'before' && dragMove.dropTarget.id === m.ID;
                                const compat = checkVersionCompatibility(m.SupportedVersion, gameVersion);
                                const incompatible = compat.known && !compat.compatible;
                                const compatible = compat.known && compat.compatible;
                                const ignored = ignoredIncompatible.has(m.ID);
                                const workshopFlag = m.RemoteFileID ? workshopFlagsById.get(m.RemoteFileID) : undefined;
                                return (
                                <div
                                    key={m.ID}
                                    data-mod-id={m.ID}
                                    className={`mod-row ${selectedAvailable.has(m.ID) ? 'selected' : ''} ${dragMove.draggedIds?.includes(m.ID) ? 'dragging' : ''} ${isDropBefore ? 'drop-before' : ''}`}
                                    onContextMenu={(e) => { setSelectedId(m.ID); openContextMenu(e, modContextMenuItems(m)); }}
                                    onMouseDown={(e) => onAvailableRowMouseDown(index, e)}
                                >
                                    <SourceBadge source={m.Source} name={m.Name}/>
                                    <span className="name">{m.Name}</span>
                                    {notes.has(m.ID) && (
                                        <i className="fa-solid fa-note-sticky row-note" {...tip(() => noteTip(notes.get(m.ID) ?? ''))}/>
                                    )}
                                    {workshopFlag && (
                                        <i
                                            className={`fa-solid ${FLAG[workshopFlag.state].icon} row-workshop workshop-${workshopFlag.state}`}
                                            {...tip(() => modFlagsTip({workshop: workshopFlag}))}
                                        />
                                    )}
                                    <span
                                        className={`ver mono ${compatible ? 'compatible' : incompatible && !ignored ? 'incompatible' : ''}`}
                                        {...tip(() => incompatible ? modFlagsTip({version: {supported: m.SupportedVersion, game: gameVersion, ignored}}) : null)}
                                    >
                                        <span className="ver-text">{m.SupportedVersion ? displayVersion(m.SupportedVersion) : '-'}</span>
                                        {incompatible && ignored && <i className={`fa-solid ${FLAG.version.icon} ver-ignored-icon`}/>}
                                    </span>
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
                            <div className="active-actions">
                                <span className="btn-amber" onClick={handleAutosort}><i className="fa-solid fa-arrow-down-arrow-up"/> Autosort</span>
                                <span className="btn-ghost">Validate</span>
                                <span className="btn-ghost" onClick={() => setOrder([])}>Clear</span>
                            </div>
                        </div>
                        <div className="column-header">
                            <span className="column-header-position">NR</span>
                            <span className="column-header-spacer"/>
                            <span className="column-header-name">NAME</span>
                            <span className="column-header-domains" {...tip(domainLegendTip)}>DOMAINS</span>
                            <span className="column-header-warnings" {...tip(flagLegendTip)}>FLAGS</span>
                        </div>
                        <div className={`list-rows ${dragMove.dropTarget?.list === 'active' && dragMove.dropTarget.kind === 'end' ? 'drop-at-end' : ''}`} ref={dragMove.activeRowsRef}>
                            {active.length === 0 && (
                                <EmptyState
                                    icon="fa-layer-group"
                                    title="Load order is empty"
                                    subtitle="Drag mods here from Available, or select some and use Add to load order."
                                />
                            )}
                            {visibleActive.length === 0 && active.length > 0 && (
                                <p className="detail-empty" style={{padding: 14}}>No matches.</p>
                            )}
                            {visibleActive.map((m, index) => {
                                const conflicted = conflictedIds.has(m.ID);
                                const isDropBefore = dragMove.dropTarget?.list === 'active' && dragMove.dropTarget.kind === 'before' && dragMove.dropTarget.id === m.ID;
                                const compat = checkVersionCompatibility(m.SupportedVersion, gameVersion);
                                const incompatible = compat.known && !compat.compatible;
                                const ignored = ignoredIncompatible.has(m.ID);
                                const hasDependencyIssue = dependencyIssues.affectedIds.has(m.ID);
                                const workshopFlag = m.RemoteFileID ? workshopFlagsById.get(m.RemoteFileID) : undefined;
                                return (
                                    <div
                                        key={m.ID}
                                        data-mod-id={m.ID}
                                        className={`mod-row active-row ${selectedActive.has(m.ID) ? 'selected' : ''} ${conflicted ? 'has-conflict' : ''} ${isDropBefore ? 'drop-before' : ''} ${dragMove.draggedIds?.includes(m.ID) ? 'dragging' : ''}`}
                                        onContextMenu={(e) => { setSelectedId(m.ID); openContextMenu(e, modContextMenuItems(m)); }}
                                        onMouseDown={(e) => onActiveRowMouseDown(index, e)}
                                    >
                                        <span className="position mono">{positionById.get(m.ID)}</span>
                                        <SourceBadge source={m.Source} name={m.Name}/>
                                        <span className="name">{m.Name}</span>
                                        {locked.has(m.ID) && (
                                            <i className="fa-solid fa-lock row-lock" title="Locked - won't be moved by dragging, Move up/down or Autosort"/>
                                        )}
                                        {notes.has(m.ID) && (
                                            <i className="fa-solid fa-note-sticky row-note" {...tip(() => noteTip(notes.get(m.ID) ?? ''))}/>
                                        )}
                                        <span className="domain-segments">
                                            {domains.map((d) => {
                                                const state = domainOverlap.get(m.ID)?.[d] ?? 'clean';
                                                return <span key={d} className={`segment l-${d} ${state}`} {...tip(() => domainTip(d, state))}/>;
                                            })}
                                        </span>
                                        <span
                                            className="row-warnings"
                                            {...tip(() => modFlagsTip({
                                                version: incompatible ? {supported: m.SupportedVersion, game: gameVersion, ignored} : undefined,
                                                conflict: conflicted ? conflictInfo.get(m.ID) : undefined,
                                                dependency: hasDependencyIssue ? dependencyIssues.byMod.get(m.ID) : undefined,
                                                workshop: workshopFlag,
                                            }))}
                                        >
                                            {incompatible && (
                                                <i className={`fa-solid ${FLAG.version.icon} warning-icon ${ignored ? 'ignored' : 'version'}`}/>
                                            )}
                                            {conflicted && (
                                                <i className={`fa-solid ${FLAG.conflict.icon} warning-icon conflict`}/>
                                            )}
                                            {hasDependencyIssue && (
                                                <i className={`fa-solid ${FLAG.dependency.icon} warning-icon dependency`}/>
                                            )}
                                            {workshopFlag && (
                                                <i className={`fa-solid ${FLAG[workshopFlag.state].icon} warning-icon workshop-${workshopFlag.state}`}/>
                                            )}
                                        </span>
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
                                <span><span className="swatch won"/>winning</span>
                                <span><span className="swatch clean"/>clean</span>
                            </div>
                        </div>
                    </div>

                    <aside className="actions-rail">
                        <div className="rail-section">
                            <div className="rail-label">PLAYSET</div>
                            <div className="playset-card">
                                <input
                                    ref={playsetNameInputRef}
                                    className="playset-name-input"
                                    placeholder="Playset name"
                                    {...tip(() => (
                                        <TipItem icon="fa-pen" color="var(--text-muted)" title={playsetName.trim() ? 'Rename this playset' : 'Name this playset'}>
                                            Type here, then press Save to keep it.
                                        </TipItem>
                                    ))}
                                    // The same color the Playsets window gives this playset's row (see
                                    // PlaysetsModal), so the name is recognisable in both places.
                                    style={playsetName.trim() ? `--playset-color: ${colorFromName(playsetName.trim())}` : undefined}
                                    value={playsetName}
                                    onInput={(e) => setPlaysetName((e.target as HTMLInputElement).value)}
                                />
                                <div className="mono meta">{active.length} mods</div>
                                <div className="playset-card-actions">
                                    <span
                                        className={`btn-ghost ${unsaved ? 'unsaved' : ''}`}
                                        onClick={handleSave}
                                        {...tip(() => unsaved ? (
                                            <TipItem icon="fa-floppy-disk" color="var(--rust)" title="Unsaved changes">
                                                {playsetName.trim()
                                                    ? 'The load order is different from the saved playset. Press Save to keep it.'
                                                    : 'This load order is not saved. Name the playset, then press Save.'}
                                            </TipItem>
                                        ) : null)}
                                    >Save</span>
                                    <span className="btn-ghost" onClick={() => setShowPlaysets(true)}>Switch</span>
                                    <span className="btn-ghost inert">Share <i className="fa-solid fa-arrow-up-right-from-square"/></span>
                                </div>
                            </div>
                        </div>

                        {conflictedIds.size > 0 && (
                            <div className="rail-message conflict">
                                <div className="rail-message-head">
                                    <i className={`fa-solid ${FLAG.conflict.icon}`}/>
                                    <span className="rail-message-title">
                                        {conflictedIds.size} {conflictedIds.size === 1 ? 'mod' : 'mods'} in hard conflicts
                                    </span>
                                </div>
                                <div className="rail-message-detail">
                                    {listEdited
                                        ? 'The list changed since the last check - save it to re-check.'
                                        : 'Pick which mod wins each key, or generate a patch.'}
                                </div>
                                <span className="btn-ghost" onClick={() => setShowConflictResolver(true)}>Resolve</span>
                            </div>
                        )}

                        <div className="rail-section">
                            <div className="rail-label">PRE-FLIGHT</div>
                            <div className="preflight-mini">
                                {preflightItems.map((p) => (
                                    <div key={p.title} className="preflight-mini-row" {...tip(() => <TipItem icon={p.icon} color={p.color} title={p.title}>{p.detail}</TipItem>)}>
                                        <i className={`fa-solid ${p.icon}`} style={{color: p.color}}/>
                                        <span>{p.title}</span>
                                    </div>
                                ))}
                            </div>
                        </div>

                        <div className="rail-section">
                            <div className="rail-label">UPDATES</div>
                            <UpdatesCard gameId={selectedGame} onReview={onOpenUpdates}/>
                        </div>

                        <div className="rail-spacer"/>

                        <div className="play-block">
                            {checksumShownBadge && (
                                <div
                                    className={`play-checksum mono ${checksumShownBadge.tone}`}
                                    onClick={() => checksum.refresh()}
                                    {...tip(() => {
                                        const p = checksumPreflightItem(checksum.state, unsaved);
                                        return <TipItem icon={p.icon} color={p.color} title={p.title}>{p.detail} Click to work it out again.</TipItem>;
                                    })}
                                >
                                    {checksumShownBadge.tone === 'busy' && <i className="fa-solid fa-spinner fa-spin"/>}
                                    checksum <span className="play-checksum-value">{checksumShownBadge.text}</span>
                                </div>
                            )}
                            {game.running ? (
                                <button
                                    className={`play-button stop ${stopStep}`}
                                    disabled={stopStep === 'stopping'}
                                    title="Ends the running game. Anything it hasn't saved is lost."
                                    onClick={handleStopGame}
                                >
                                    <div className="play-title">
                                        {stopStep === 'confirm' ? 'CLICK AGAIN TO STOP'
                                            : stopStep === 'stopping' ? 'STOPPING...'
                                                : `STOP PLAYING ${gameName.toUpperCase()}`}
                                    </div>
                                    <div className="play-subtitle mono">
                                        {stopStep === 'confirm'
                                            ? 'unsaved progress is lost'
                                            : game.pids.length === 1 ? `running · pid ${game.pids[0]}` : `running · ${game.pids.length} processes`}
                                    </div>
                                </button>
                            ) : (
                                <button
                                    className="play-button"
                                    onClick={() => setShowPreflight(true)}
                                >
                                    <div className="play-title">PLAY {gameName.toUpperCase()}</div>
                                    <div className="play-subtitle mono">
                                        {playsetName.trim()
                                            ? `${active.length} mods · launch via Steam`
                                            : 'No playset selected · launches as-is'}
                                    </div>
                                </button>
                            )}
                            <div className="play-secondary">
                                <span className="btn-ghost inert">Vanilla</span>
                                <span
                                    className="btn-ghost"
                                    title={`Watch ${gameName}'s own log files as it writes them`}
                                    onClick={() => setShowGameLog(true)}
                                >
                                    {game.running && <i className="fa-solid fa-circle live-dot"/>}View log
                                </span>
                            </div>
                        </div>
                    </aside>
                </div>
            )}

            {dragMove.pointer && dragMove.draggedIds && (() => {
                // The icon reflects where this would actually land right
                // now, not just where it started from - moving into the
                // other list points toward it (right for Available ->
                // Active, left for the reverse); reordering in place
                // (source and target are the same list) gets its own icon
                // instead of implying movement toward either side.
                const icon = dragMove.dropTarget?.list === dragMove.draggedSource
                    ? 'fa-arrows-up-down'
                    : dragMove.draggedSource === 'available'
                        ? 'fa-arrow-right'
                        : 'fa-arrow-left';
                const style = {left: `${dragMove.pointer.x + 14}px`, top: `${dragMove.pointer.y + 10}px`};
                const names = dragMove.draggedIds.map((id) => modsById.get(id)?.Name ?? id);
                if (names.length === 1) {
                    return (
                        <div className="drag-ghost" style={style}>
                            <i className={`fa-solid ${icon}`}/>
                            <span>{truncate(names[0], 40)}</span>
                        </div>
                    );
                }
                // The exact mods being dragged, not just a count - capped
                // so a huge selection doesn't turn the preview into its
                // own scrollable list.
                const shown = names.slice(0, 5);
                const extra = names.length - shown.length;
                return (
                    <div className="drag-ghost drag-ghost-multi" style={style}>
                        <div className="drag-ghost-head">
                            <i className={`fa-solid ${icon}`}/>
                            <span className="mono">{names.length} mods</span>
                        </div>
                        {shown.map((name, i) => <div key={i} className="drag-ghost-name">{truncate(name, 40)}</div>)}
                        {extra > 0 && <div className="drag-ghost-more mono">+{extra} more</div>}
                    </div>
                );
            })()}

            {showPlaysets && (
                <PlaysetsModal
                    gameName={gameName}
                    names={playsetList}
                    launcherPlaysets={launcherPlaysets}
                    onActivate={handleLoadPlayset}
                    onNew={() => { setOrder([]); setSavedOrder(null); setPlaysetName(''); setDisabledDlc([]); setShowPlaysets(false); }}
                    onImport={handleImportLauncherPlayset}
                    onRename={renamePlayset}
                    onDelete={deletePlayset}
                    onClose={() => setShowPlaysets(false)}
                />
            )}

            {showGameLog && (
                <GameLogModal
                    gameId={selectedGame}
                    gameName={gameName}
                    running={game.running}
                    onClose={() => setShowGameLog(false)}
                />
            )}

            {showPreflight && (
                <PreflightModal
                    gameName={gameName}
                    modCount={active.length}
                    items={preflightItems}
                    checksum={checksumShownBadge}
                    onFixConflicts={() => { setShowPreflight(false); setShowConflictResolver(true); }}
                    onLaunchAnyway={handleLaunchAnyway}
                    onClose={() => setShowPreflight(false)}
                />
            )}

            {showConflictResolver && (
                <ConflictResolver
                    gameId={selectedGame}
                    conflicts={conflicts}
                    patch={summary?.Patch}
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
            {missingDeps && (
                <AutosortMissingDepsModal
                    missing={missingDeps.resolvable}
                    onClose={() => setMissingDeps(null)}
                    onLoadAndSort={() => finishAutosort(true)}
                    onSortAnyway={() => finishAutosort(false)}
                />
            )}
            {unresolvedDeps && (
                <AutosortUnresolvedDepsModal names={unresolvedDeps} onClose={() => setUnresolvedDeps(null)}/>
            )}
        </div>
    );
}

function matchesSearch(m: library.ModSummary, search: string, notes: Map<string, string>): boolean {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    return m.Name.toLowerCase().includes(q) || m.ID.toLowerCase().includes(q) || noteMatches(notes, m.ID, q);
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

function DetailPanel({mod, tab, onTab, onOpenResolver, gameId, gameVersion, allMods, conflicts, onError, onSelectMod, workshopDetails, workshopDetailsState, workshopFlagsById, authorProfiles, ignoredIncompatible, note, noteError, noteFocusRequest, onNoteFocused, onSaveNote}: {
    mod: library.ModSummary | null;
    tab: DetailTab;
    onTab: (t: DetailTab) => void;
    onOpenResolver: () => void;
    gameId: string;
    gameVersion: string;
    allMods: library.ModSummary[];
    conflicts: library.ConflictSummary[];
    onError: (message: string) => void;
    onSelectMod: (id: string) => void;
    // Real Steam Workshop metadata/author profiles - fetched once per game
    // at the Workspace level (not lazily per mod-selection here), since
    // the Available/Active lists need the same data too. See Workspace's
    // own state of the same name for how/when this populates.
    workshopDetails: Map<string, steamapi.PublishedFileDetails>;
    workshopDetailsState: 'idle' | 'loading' | 'loaded' | 'error';
    workshopFlagsById: Map<string, WorkshopFlag>;
    authorProfiles: Map<string, steamapi.Profile>;
    // Same set as the Available/Active lists use - so the "Supports" row
    // below shows the same acknowledged-not-live warning treatment as the
    // row this mod was selected from, instead of contradicting it.
    ignoredIncompatible: Set<string>;
    // The person's note about this mod ('' for none), whether notes are switched off (an
    // unreadable file), a request to focus the box, and saving - see components/ModNote.tsx.
    note: string;
    noteError: string;
    noteFocusRequest: number;
    onNoteFocused: () => void;
    onSaveNote: (text: string) => Promise<boolean>;
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
    // updated line, Overview's author card/stats/description).
    const steamDetails = mod?.RemoteFileID ? workshopDetails.get(mod.RemoteFileID) : undefined;
    const validSteamDetails = steamDetails && steamDetails.Result === 1 ? steamDetails : undefined;
    const author = validSteamDetails?.Creator ? authorProfiles.get(validSteamDetails.Creator) : undefined;

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
        if (tab !== 'changelog' || !mod || mod.Source !== 'workshop' || !remoteFileID || changelogs.has(remoteFileID)) {
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
            {!mod && (
                <EmptyState
                    icon="fa-cube"
                    title="No mod selected"
                    subtitle="Select a mod from Available or your load order to see its details."
                />
            )}
            {mod && (
                <>
                    <div className="detail-header">
                        <div className="detail-badges">
                            <SourceBadge source={mod.Source} name={mod.Name}/>
                            <span className="mono id">{mod.Source === 'workshop' && mod.RemoteFileID ? mod.RemoteFileID : mod.ID}</span>
                        </div>
                        <div className="detail-name" title={mod.Name}>{truncate(mod.Name, MAX_DETAIL_NAME_LENGTH)}</div>
                        <div className="detail-sub">
                            {(() => {
                                // A workshop mod's real Steam data (once fetched) gives a
                                // more meaningful "updated" than local file mtime - the
                                // Workshop item's own real last-updated time. The author
                                // name used to sit here too, but now has its own real card
                                // in the Overview tab, right below the details grid - no
                                // need to duplicate it in this compact header line as well.
                                if (validSteamDetails) {
                                    return `Updated ${timeAgo(validSteamDetails.TimeUpdated)}`;
                                }
                                if (filesLoading || (mod.Source === 'workshop' && workshopDetailsState === 'loading')) {
                                    return <span className="skeleton skeleton-text" style={{width: '90px'}}/>;
                                }
                                return files?.LastModified ? `Updated ${timeAgo(files.LastModified)}` : '';
                            })()}
                        </div>
                    </div>
                    <div className="detail-tabs">
                        {(['overview', 'files', 'conflicts', 'changelog'] as DetailTab[]).map((t) => (
                            <span key={t} className={`detail-tab ${tab === t ? 'active' : ''}`} onClick={() => onTab(t)}>
                                {t[0].toUpperCase() + t.slice(1)}
                                {t === 'conflicts' && (
                                    myConflicts.length > 0
                                        ? <i className={`fa-solid ${FLAG.conflict.icon} detail-tab-icon warn`} {...tip(() => modFlagsTip({conflict: conflictsByMod(myConflicts).get(mod.ID)}))}/>
                                        : <i className="fa-solid fa-circle-check detail-tab-icon ok" title="No genuine conflicts"/>
                                )}
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
                                onSelectMod={onSelectMod}
                                steamDetails={validSteamDetails}
                                workshopFlag={mod.RemoteFileID ? workshopFlagsById.get(mod.RemoteFileID) : undefined}
                                author={author}
                                gameVersion={gameVersion}
                                ignoredIncompatible={ignoredIncompatible}
                                note={note}
                                noteError={noteError}
                                noteFocusRequest={noteFocusRequest}
                                onNoteFocused={onNoteFocused}
                                onSaveNote={onSaveNote}
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
                        {tab === 'conflicts' && (() => {
                            // Every other mod this one shares at least one
                            // contested key with, regardless of who wins it -
                            // "N mods involved" per mockup/Mod Manager.dc.html's
                            // own Conflicts tab.
                            const otherModIDs = new Set(
                                myConflicts.flatMap((c) => c.Candidates.filter((cand) => cand.ModID !== mod.ID).map((cand) => cand.ModID)),
                            );
                            const wins = myConflicts.filter((c) => c.Winner === mod.ID).length;

                            // LOSES TO: every key this mod doesn't win,
                            // attributed to whichever mod actually does (not
                            // just "any other candidate") and tallied per
                            // winner - the mockup's own aggregate bar chart,
                            // not a flat per-key list (that detail already
                            // lives in the full resolver - see "Open in
                            // resolver" below).
                            const losesToByWinner = new Map<string, { name: string; count: number }>();
                            for (const c of myConflicts) {
                                if (c.Winner === mod.ID) continue;
                                const winner = c.Candidates.find((cand) => cand.ModID === c.Winner);
                                if (!winner) continue;
                                const entry = losesToByWinner.get(winner.ModID) ?? {name: winner.ModName, count: 0};
                                entry.count += 1;
                                losesToByWinner.set(winner.ModID, entry);
                            }
                            const losesTo = [...losesToByWinner.values()].sort((a, b) => b.count - a.count);
                            const totalLost = losesTo.reduce((sum, l) => sum + l.count, 0);

                            // BY TYPE: every contested key this mod is a
                            // candidate for (win or lose), grouped by its own
                            // Type - already the definition's real containing
                            // folder path (e.g. "common/buildings"), not a
                            // fabricated category - see conflict.Key's own
                            // comment in internal/conflict.
                            const byType = new Map<string, number>();
                            for (const c of myConflicts) {
                                byType.set(c.Type, (byType.get(c.Type) ?? 0) + 1);
                            }
                            const byTypeSorted = [...byType.entries()].sort((a, b) => b[1] - a[1]);

                            return (
                                <div className="conflicts-tab">
                                    <div className="conflict-stats">
                                        <div>
                                            <div className="stat-num" style={{color: myConflicts.length ? 'var(--red)' : 'var(--green)'}}>{myConflicts.length}</div>
                                            <div className="stat-label">contested {myConflicts.length === 1 ? 'key' : 'keys'}</div>
                                        </div>
                                        {myConflicts.length > 0 && (
                                            <>
                                                <div>
                                                    <div className="stat-num" style={{color: 'var(--amber)'}}>{otherModIDs.size}</div>
                                                    <div className="stat-label">{otherModIDs.size === 1 ? 'mod' : 'mods'} involved</div>
                                                </div>
                                                <div>
                                                    <div className="stat-num" style={{color: 'var(--green)'}}>{wins}</div>
                                                    <div className="stat-label">{wins === 1 ? 'key' : 'keys'} won</div>
                                                </div>
                                            </>
                                        )}
                                    </div>

                                    {myConflicts.length === 0 && (
                                        <EmptyState icon="fa-circle-check" title="No conflicts" subtitle="No genuine conflicts detected for this mod."/>
                                    )}

                                    {losesTo.length > 0 && (
                                        <div className="section">
                                            <div className="section-label">LOSES TO</div>
                                            {losesTo.map((l) => (
                                                <div key={l.name} className="loses-row">
                                                    <div className="loses-head">
                                                        <span>{l.name}</span>
                                                        <span className="mono">{l.count} {l.count === 1 ? 'key' : 'keys'}</span>
                                                    </div>
                                                    <div className="loses-bar">
                                                        <div style={{width: `${totalLost ? Math.round((l.count / totalLost) * 100) : 0}%`, background: 'var(--red)'}}/>
                                                    </div>
                                                </div>
                                            ))}
                                        </div>
                                    )}

                                    {byTypeSorted.length > 0 && (
                                        <div className="section">
                                            <div className="section-label">BY TYPE</div>
                                            {byTypeSorted.map(([type, count]) => (
                                                <div key={type} className="domain-row">
                                                    <span>{type}</span>
                                                    <span className="mono" style={{color: 'var(--amber)'}}>{count}</span>
                                                </div>
                                            ))}
                                        </div>
                                    )}

                                    <span className="resolver-btn" onClick={onOpenResolver}>Open in resolver</span>
                                </div>
                            );
                        })()}
                        {tab === 'changelog' && (
                            <div className="changelog-tab">
                                {mod.Source !== 'workshop' && (
                                    <EmptyState
                                        icon="fa-clock-rotate-left"
                                        title="No update history"
                                        subtitle="Only Steam Workshop mods have a real page to fetch update history from."
                                    />
                                )}
                                {mod.Source === 'workshop' && !mod.RemoteFileID && (
                                    <EmptyState
                                        icon="fa-clock-rotate-left"
                                        title="No Workshop data found"
                                        subtitle="This mod couldn't be matched to a Steam Workshop item."
                                    />
                                )}
                                {mod.Source === 'workshop' && mod.RemoteFileID && (
                                    <>
                                        {changelogState === 'loading' && (
                                            <p className="detail-empty">Checking Steam Workshop...</p>
                                        )}
                                        {changelogState === 'error' && (
                                            <div className="content-missing">
                                                <i className="fa-solid fa-circle-exclamation"/>
                                                <p>{changelogError}</p>
                                            </div>
                                        )}
                                        {changelogs.has(mod.RemoteFileID) && (() => {
                                            const entries = changelogs.get(mod.RemoteFileID!)!;
                                            if (entries.length === 0) {
                                                return (
                                                    <EmptyState
                                                        icon="fa-clock-rotate-left"
                                                        title="No update notes"
                                                        subtitle="No update notes found for this mod yet."
                                                    />
                                                );
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
                                    </>
                                )}
                            </div>
                        )}
                    </div>
                </>
            )}
        </div>
    );
}


// stripBBCode gives a readable plain-text preview of a real Steam Workshop
// description - those are BBCode ([b], [url=...], [img]...), not something
// worth building a real renderer for here. Not a full parser, just enough
// to keep raw markup out of view.
function stripBBCode(s: string): string {
    return s.replace(/\[[^\]]*\]/g, '').trim();
}

function OverviewTab({mod, files, filesLoading, allMods, conflicts, onOpenFolder, onSelectMod, steamDetails, workshopFlag, author, gameVersion, ignoredIncompatible, note, noteError, noteFocusRequest, onNoteFocused, onSaveNote}: {
    mod: library.ModSummary;
    files: library.ModFiles | null;
    filesLoading: boolean;
    allMods: library.ModSummary[];
    conflicts: library.ConflictSummary[];
    onOpenFolder: () => void;
    // Selects a dependency's own real mod in the Available/Active list,
    // the same way clicking its row there would - see the REQUIRES
    // section below.
    onSelectMod: (id: string) => void;
    // This mod's real Steam Workshop metadata (subscriber/view counts,
    // description) and its real uploader's Steam Community profile, when
    // it's a Workshop mod and both have been fetched - undefined
    // otherwise (a local mod, or not loaded/found). Both used to live in
    // the Changes tab; that tab is for real update history now, so the
    // author card and item stats moved here, alongside the description
    // they came with.
    steamDetails: steamapi.PublishedFileDetails | undefined;
    // Unlisted, private or deleted on the Steam Workshop, when it is one of those.
    workshopFlag: WorkshopFlag | undefined;
    author: steamapi.Profile | undefined;
    // The real, currently-installed game version - drives the "Supports"
    // row's own color below (see data/versionCompat.ts), instead of the
    // flat hardcoded "ok" green it used to always show regardless of
    // whether that was actually true.
    gameVersion: string;
    ignoredIncompatible: Set<string>;
    // The person's note about this mod ('' for none), whether notes are switched off (an
    // unreadable file), a request to focus the box, and saving - see components/ModNote.tsx.
    note: string;
    noteError: string;
    noteFocusRequest: number;
    onNoteFocused: () => void;
    onSaveNote: (text: string) => Promise<boolean>;
}) {
    // Maps a dependency's declared name to the real mod ID it resolves
    // to, when one of the currently-scanned mods actually has that name -
    // lets a REQUIRES row both show "installed" and jump straight to
    // that mod, from the one real match this project can find (Paradox
    // mods only ever declare a dependency by name, never a stable ID).
    const idByName = useMemo(() => {
        const m = new Map<string, string>();
        for (const other of allMods) m.set(other.Name, other.ID);
        return m;
    }, [allMods]);
    const workshopUrl = mod.Source === 'workshop' && mod.RemoteFileID
        ? `https://steamcommunity.com/sharedfiles/filedetails/?id=${mod.RemoteFileID}`
        : '';
    const steamDescription = steamDetails?.Description ? stripBBCode(steamDetails.Description) : '';
    const supportsCompat = checkVersionCompatibility(mod.SupportedVersion, gameVersion);
    const supportsIncompatible = supportsCompat.known && !supportsCompat.compatible;
    const supportsIgnored = ignoredIncompatible.has(mod.ID);

    return (
        <>
            {/* The author card's own spot: a mod flagged unlisted, private, deleted or banned
                on the Workshop is more important to see here than who made it (and Steam
                rarely answers a usable author for one of those anyway) - showing the flag
                notice where the author card would sit, instead of below the whole overview
                grid with that spot simply left blank, is what actually explains what's going
                on with this mod before anything else in the panel. */}
            {workshopFlag ? (() => {
                const style = workshopFlagStyle(workshopFlag);
                const text = workshopFlagText(workshopFlag);
                return (
                    <div className="workshop-notice" style={{'--notice-color': style.color} as h.JSX.CSSProperties}>
                        <i className={`fa-solid ${style.icon}`}/>
                        <div>
                            <div className="workshop-notice-title">{text.title}</div>
                            <div className="workshop-notice-text">{text.detail}</div>
                            {backupNote(workshopFlag) && <div className="workshop-notice-backup"><i className="fa-solid fa-box-archive"/> {backupNote(workshopFlag)}</div>}
                        </div>
                    </div>
                );
            })() : author && (
                <div className="author-row" onClick={() => BrowserOpenURL(author.ProfileURL)}>
                    {author.AvatarURL
                        ? <img className="author-avatar" src={author.AvatarURL} alt={author.Name}/>
                        : <span className="author-avatar author-avatar-fallback"/>}
                    <div className="author-info">
                        <span className="author-name" title={author.Name}>{truncate(author.Name, MAX_AUTHOR_NAME_LENGTH)}</span>
                        {author.MemberSince && (
                            <span className="author-sub">Member since {author.MemberSince}</span>
                        )}
                    </div>
                    <i className="fa-solid fa-arrow-up-right-from-square author-link-icon"/>
                </div>
            )}
            <div className="overview-grid">
                <span className="label">Version</span><span className="value mono">{mod.Version || '-'}</span>
                <span className="label">Supports</span>
                <span className="supports-cell">
                    <span className={`value mono ${supportsCompat.known ? (supportsCompat.compatible ? 'ok' : 'warn') : ''}`}>
                        {mod.SupportedVersion ? displayVersion(mod.SupportedVersion) : '-'}
                        {supportsIncompatible && supportsIgnored && <i className={`fa-solid ${FLAG.version.icon} ver-ignored-icon`}/>}
                    </span>
                    {supportsIncompatible && (
                        <div className={`supports-notice ${supportsIgnored ? 'ignored' : ''}`}>
                            <i className={`fa-solid ${FLAG.version.icon}`}/>
                            <div>
                                <div>Built for {displayVersion(mod.SupportedVersion)} - you have {displayVersion(gameVersion)}</div>
                                {supportsIgnored && <div className="supports-notice-subnote">Incompatibility warning ignored</div>}
                            </div>
                        </div>
                    )}
                </span>
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
            {steamDetails && (
                <div className="overview-grid">
                    <span className="label">Subscribers</span><span className="value mono">{steamDetails.Subscriptions.toLocaleString()}</span>
                    <span className="label">Favorited</span><span className="value mono">{steamDetails.Favorited.toLocaleString()}</span>
                    <span className="label">Views</span><span className="value mono">{steamDetails.Views.toLocaleString()}</span>
                </div>
            )}
            <ModNote
                key={mod.ID}
                note={note}
                error={noteError}
                focusRequest={noteFocusRequest}
                onFocused={onNoteFocused}
                onSave={onSaveNote}
            />
            <div className="section">
                <div className="section-label">DESCRIPTION</div>
                <div className="section-body description-scroll">{mod.ShortDescription || steamDescription || 'No description provided.'}</div>
            </div>
            <div className="section">
                <div className="section-label">REQUIRES</div>
                {mod.Dependencies.length === 0 && <div className="section-body">This mod declares no dependencies.</div>}
                {mod.Dependencies.map((name) => {
                    const id = idByName.get(name);
                    const found = id !== undefined;
                    return (
                        <div
                            key={name}
                            className={`requires-row ${found ? 'clickable' : ''}`}
                            onClick={found ? () => onSelectMod(id) : undefined}
                            title={found ? 'View this mod' : 'Not found among the currently scanned mods'}
                        >
                            <span className={`requires-dot ${found ? 'found' : 'missing'}`}>●</span>{name}
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


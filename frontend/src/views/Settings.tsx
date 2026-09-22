import './Settings.css';
import {Fragment, h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {
    BackgroundCatalog,
    BrowseForExtraModFolder,
    BrowseForGameInstall,
    ClearGamePath,
    DetectGames,
    DeveloperToolsStatus,
    GetPreferences,
    ListPlaysets,
    OpenPath,
    RemoveExtraModFolder,
    SetGameManaged,
    SetPreferences,
} from '../../wailsjs/go/main/App';
import type {library, preferences} from '../../wailsjs/go/models';
import {GameLogo} from '../components/GameLogo';
import {Toggle} from '../components/Toggle';
import {settingsNav} from '../data/mockData';
import {formatBytes} from '../data/format';
import {DEFAULT_BACKGROUND_INTERVAL_SECONDS} from '../components/AppBackground';
import {
    blurPixels,
    DEFAULT_BACKGROUND_BLUR,
    DEFAULT_BACKGROUND_DARKEN,
    previewBackgroundLook,
} from '../data/backgroundLook';
import {requestRandomBackground, useCurrentBackground} from '../data/backgroundControl';
import {type PlaysetAutoloadMode, playsetAutoloadModeFor} from '../data/playsetAutoload';
import {AboutPanel} from './About';
import {GamePickerChips, type ManageGamesState, useManagedGamePicker} from './GamePicker';
import {BackgroundDownloadModal} from './BackgroundDownloadModal';
import {SteamApiPanel} from './SteamApiPanel';
import {BackupPanel} from './BackupPanel';
import {DebugPanel} from './DebugPanel';
import {AccentSettings} from './AccentSettings';

type Section = 'manage' | 'launch' | 'playsets' | 'sort' | 'steam' | 'backup' | 'appearance' | 'advanced' | 'debug' | 'about';

export function Settings({jumpToManageGames, jumpToBackup, onGamesChanged, onPreferencesChanged}: {
    // Incremented by app.tsx (the TopBar's own "Manage games" entry) to
    // ask this view to switch to the "Manage games" panel - 0 (the
    // default, falsy) means no pending request, so this never fights the
    // section the user's own click already put them on.
    jumpToManageGames?: number;
    // The same, for the Backup panel (0 means no request).
    jumpToBackup?: number;
    // Called after a change here (toggling a game managed, or browsing to
    // a new install path) might have changed which games app.tsx should
    // show - lets the game switcher, Library, DLC, and Workspace pick up
    // the change immediately instead of only on next launch.
    onGamesChanged?: () => void;
    // Called after a preference changes here in a way app.tsx's own
    // top-level state also depends on (currently just AppearancePanel's
    // background toggles, which drive the AppBackground component
    // app.tsx renders) - each panel here otherwise keeps its own local
    // prefs copy for editing, which never reaches app.tsx's on its own.
    onPreferencesChanged?: () => void;
}) {
    const [section, setSection] = useState<Section>('sort');
    // The Debug tab exists in development builds only (wails dev, F5 in VS Code).
    const [debugAvailable, setDebugAvailable] = useState(false);
    useEffect(() => {
        DeveloperToolsStatus().then((s) => setDebugAvailable(s.Available)).catch(() => undefined);
    }, []);

    useEffect(() => {
        if (jumpToManageGames) setSection('manage');
    }, [jumpToManageGames]);

    useEffect(() => {
        if (jumpToBackup) setSection('backup');
    }, [jumpToBackup]);

    return (
        <div className="settings">
            <div className="settings-nav">
                <div className="sidebar-label">SETTINGS</div>
                {settingsNav.filter((s) => s.key !== 'debug' || debugAvailable).map((s) => {
                    const clickable = s.key === 'manage' || s.key === 'launch' || s.key === 'playsets' || s.key === 'sort' || s.key === 'steam' || s.key === 'backup' || s.key === 'appearance' || s.key === 'advanced' || s.key === 'debug' || s.key === 'about';
                    const active = clickable && s.key === section;
                    return (
                        <div
                            key={s.key}
                            className={`settings-nav-row ${clickable ? 'clickable' : 'inert'} ${s.key === 'about' ? 'pinned-bottom' : ''}`}
                            style={{background: active ? 'var(--bg-highlight)' : 'transparent', color: active ? 'var(--text-bright)' : 'var(--text-mid)'}}
                            onClick={() => clickable && setSection(s.key as Section)}
                        >
                            {s.label}
                        </div>
                    );
                })}
            </div>

            {section === 'manage' && <ManageGamesPanel onGamesChanged={onGamesChanged}/>}
            {section === 'launch' && <LaunchOptionsPanel/>}
            {section === 'playsets' && <PlaysetsSettingsPanel/>}
            {section === 'sort' && <SortRulesPanel/>}
            {section === 'steam' && <SteamApiPanel/>}
            {section === 'backup' && <BackupPanel/>}
            {section === 'appearance' && <AppearancePanel onPreferencesChanged={onPreferencesChanged}/>}
            {section === 'advanced' && <AdvancedPanel/>}
            {section === 'debug' && debugAvailable && <DebugPanel/>}
            {section === 'about' && <AboutPanel/>}
        </div>
    );
}

// ManageGamesPanel combines what used to be two separate panels ("Manage games" and "Paths &
// folders"): which games are managed, and - expand a card for it - where each is installed and
// every folder searched for its mods. They shared one DetectGames() call's worth of state and
// the same per-game card styling already (see the .paths-card comment below), so splitting them
// only meant fetching the same games twice and making the person hunt across two tabs for what
// is really one "this game, configured" concern.
function ManageGamesPanel({onGamesChanged}: { onGamesChanged?: () => void }) {
    const [state, setState] = useState<ManageGamesState>({kind: 'loading'});
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [busy, setBusy] = useState<Set<string>>(new Set());
    const [togglingManaged, setTogglingManaged] = useState<Set<string>>(new Set());
    // Every card starts collapsed (showing just its install path, mod count and the managed
    // toggle) - with all 6+ registered games always rendered here regardless of detection
    // state, showing every one's mod folder and extra-folders section by default wastes most
    // of the panel on games most people aren't even using. Expanding is per-game and explicit.
    const [expanded, setExpanded] = useState<Set<string>>(new Set());

    useEffect(() => {
        DetectGames()
            .then((games) => setState({kind: 'ready', games}))
            .catch((err) => setState({kind: 'error', message: String(err)}));
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    function toggleExpanded(gameId: string) {
        setExpanded((prev) => {
            const next = new Set(prev);
            if (next.has(gameId)) next.delete(gameId); else next.add(gameId);
            return next;
        });
    }

    function updateGame(updated: library.DetectedGame) {
        setState((prev) => prev.kind === 'ready'
            ? {kind: 'ready', games: prev.games.map((g) => (g.ID === updated.ID ? updated : g))}
            : prev);
    }

    function setBusyFlag(key: string, on: boolean) {
        setBusy((prev) => {
            const next = new Set(prev);
            if (on) next.add(key); else next.delete(key);
            return next;
        });
    }

    async function withBusy(key: string, fn: () => Promise<library.DetectedGame>) {
        setBusyFlag(key, true);
        try {
            updateGame(await fn());
            onGamesChanged?.();
        } catch {
            // A bad pick, a cancelled dialog, or a failed clear just
            // leaves the row as it was.
        } finally {
            setBusyFlag(key, false);
        }
    }

    function openFolder(path: string) {
        if (path) OpenPath(path).catch(() => undefined);
    }

    async function addExtraFolder(gameId: string) {
        const busyKey = `extra:${gameId}`;
        setBusyFlag(busyKey, true);
        try {
            const picked = await BrowseForExtraModFolder(gameId);
            if (picked) setPrefs(await GetPreferences());
        } catch {
            // A cancelled dialog just leaves the list as it was.
        } finally {
            setBusyFlag(busyKey, false);
        }
    }

    async function removeExtraFolder(gameId: string, folder: string) {
        try {
            await RemoveExtraModFolder(gameId, folder);
            setPrefs(await GetPreferences());
        } catch {
            // Leave the list as it was.
        }
    }

    function togglePref(key: 'scanForNewMods' | 'closeAfterLaunch' | 'warnOnPatchMismatch') {
        if (!prefs) return;
        const next = {...prefs, [key]: !prefs[key]};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    async function toggleManaged(gameId: string, managed: boolean) {
        if (!prefs) return;
        setTogglingManaged((prev) => new Set(prev).add(gameId));
        const existing = prefs.managedGames ?? [];
        const optimistic = {
            ...prefs,
            managedGames: managed ? Array.from(new Set([...existing, gameId])) : existing.filter((id) => id !== gameId),
        };
        setPrefs(optimistic);
        try {
            await SetGameManaged(gameId, managed);
            onGamesChanged?.();
        } catch {
            setPrefs(prefs);
        } finally {
            setTogglingManaged((prev) => {
                const next = new Set(prev);
                next.delete(gameId);
                return next;
            });
        }
    }

    return (
        <div className="settings-content wide">
            <div>
                <div className="settings-title">Manage games</div>
                <div className="settings-subtitle">
                    Only games marked managed here show up in the game switcher, Library, DLC, and
                    Workspace - each one keeps its own paths, playsets and sort rules. Expand a
                    game for its install path, mod folder and extra mod folders (searched
                    recursively alongside it).
                </div>
            </div>
            {state.kind === 'loading' && <p className="status-page">Checking installed games...</p>}
            {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
            {state.kind === 'ready' && (
                <div className="profile-list">
                    {state.games.map((g) => {
                        const managed = prefs?.managedGames?.includes(g.ID) ?? false;
                        const extra = prefs?.extraModFolders?.[g.ID] ?? [];
                        const isExpanded = expanded.has(g.ID);
                        return (
                            <div
                                key={g.ID}
                                className="paths-card"
                                style={{borderColor: g.Installed ? '#4a3826' : 'var(--border)', background: g.Installed ? '#191510' : 'var(--bg-rail)'}}
                            >
                                <div className="paths-card-main clickable" onClick={() => toggleExpanded(g.ID)}>
                                    <i className={`fa-solid fa-chevron-${isExpanded ? 'down' : 'right'} paths-card-chevron`}/>
                                    <GameLogo gameId={g.ID} className="profile-swatch"/>
                                    <div className="profile-main">
                                        <div className="profile-name">{g.DisplayName}</div>
                                        <div className="mono profile-path">
                                            {g.Installed ? g.InstallPath : 'not detected'}
                                        </div>
                                    </div>
                                    {!isExpanded && extra.length > 0 && (
                                        <span className="mono paths-card-hint">
                                            {extra.length} extra folder{extra.length === 1 ? '' : 's'}
                                        </span>
                                    )}
                                    <div className="paths-card-status" onClick={(e) => e.stopPropagation()}>
                                        {g.Installed ? (
                                            <span className="mono profile-state" style={{color: 'var(--green)'}}>
                                                {g.ModCount} mod{g.ModCount === 1 ? '' : 's'}
                                            </span>
                                        ) : (
                                            <span
                                                className="mono profile-state actionable"
                                                style={{color: 'var(--amber)'}}
                                                onClick={() => withBusy(g.ID, () => BrowseForGameInstall(g.ID))}
                                            >
                                                {busy.has(g.ID) ? 'looking...' : 'set path'}
                                            </span>
                                        )}
                                        <Toggle
                                            on={managed}
                                            onClick={g.Installed && !togglingManaged.has(g.ID) ? () => toggleManaged(g.ID, !managed) : undefined}
                                        />
                                    </div>
                                </div>

                                {isExpanded && (
                                <div className="paths-card-actions" onClick={(e) => e.stopPropagation()}>
                                    {g.Installed && (
                                        <span className="link-btn" onClick={() => openFolder(g.InstallPath)}>Open</span>
                                    )}
                                    <span className="link-btn" onClick={() => withBusy(g.ID, () => BrowseForGameInstall(g.ID))}>
                                        {busy.has(g.ID) ? 'Looking...' : g.Installed ? 'Change...' : 'Browse...'}
                                    </span>
                                    {g.PathOverridden && (
                                        <span className="link-btn" onClick={() => withBusy(g.ID, () => ClearGamePath(g.ID))}>
                                            Reset
                                        </span>
                                    )}
                                </div>
                                )}

                                {isExpanded && (
                                <div className="paths-modfolder">
                                    <span className="paths-modfolder-label">Mod folder</span>
                                    <span className="mono paths-modfolder-value" title={g.ModFolder}>{g.ModFolder}</span>
                                    <span className="link-btn" onClick={() => openFolder(g.ModFolder)}>Open</span>
                                </div>
                                )}

                                {isExpanded && (
                                <div className="paths-extra">
                                    <span className="paths-extra-label">Extra mod folders (searched recursively)</span>
                                    {extra.length === 0 && (
                                        <span className="paths-extra-empty">None - mods are only found in the folder above.</span>
                                    )}
                                    {extra.map((folder) => (
                                        <div key={folder} className="paths-extra-item">
                                            <span className="mono paths-extra-path" title={folder}>{folder}</span>
                                            <span className="link-btn" onClick={() => openFolder(folder)}>Open</span>
                                            <i
                                                className="fa-solid fa-xmark paths-extra-remove"
                                                title="Remove"
                                                onClick={() => removeExtraFolder(g.ID, folder)}
                                            />
                                        </div>
                                    ))}
                                    <span className="link-btn" onClick={() => addExtraFolder(g.ID)}>
                                        {busy.has(`extra:${g.ID}`) ? 'Looking...' : '+ Add folder'}
                                    </span>
                                </div>
                                )}
                            </div>
                        );
                    })}
                </div>
            )}
            {prefs && (
                <div className="profile-toggles">
                    <div className="profile-toggle-row">
                        <span>Scan for new mods automatically</span>
                        <Toggle on={prefs.scanForNewMods} onClick={() => togglePref('scanForNewMods')}/>
                    </div>
                    <div className="profile-toggle-row">
                        <span>Warn on patch mismatch</span>
                        <Toggle on={prefs.warnOnPatchMismatch} onClick={() => togglePref('warnOnPatchMismatch')}/>
                    </div>
                    <div className="profile-toggle-row">
                        <span>Close manager after launch</span>
                        <Toggle on={prefs.closeAfterLaunch} onClick={() => togglePref('closeAfterLaunch')}/>
                    </div>
                </div>
            )}
        </div>
    );
}

// Mirrors internal/launch.LaunchMode's two values - preferences.launchModes
// is stored as a plain map[string]string on the Go side (see that type's
// own comment for why), so there's no generated binding to import here.
type LaunchMode = 'steam' | 'direct';

function launchModeFor(prefs: preferences.Preferences | null, gameId: string): LaunchMode {
    return prefs?.launchModes?.[gameId] === 'direct' ? 'direct' : 'steam';
}

function LaunchOptionsPanel() {
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    // Asked before switching TO Steam / Paradox Launcher (never shown for switching
    // away from it, or if it's already chosen) - see pickSteam below.
    const [confirmSteam, setConfirmSteam] = useState(false);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    const {state, managedGames, selectedGame, selectedGameId, setSelectedGameId} = useManagedGamePicker(prefs);

    useEffect(() => { setConfirmSteam(false); }, [selectedGameId]);

    function setMode(mode: LaunchMode) {
        if (!prefs || !selectedGame) return;
        const next = {...prefs, launchModes: {...prefs.launchModes, [selectedGame.ID]: mode}};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    // Steam / Paradox Launcher is the one option with a real, known downside (an extra
    // launcher window, on top of Steam itself, every single time) that the other two
    // don't have - worth a second look before switching to it, not just picking it by
    // habit because it's the old default.
    function pickSteam(currentMode: LaunchMode) {
        if (currentMode === 'steam') return;
        setConfirmSteam(true);
    }

    function confirmPickSteam() {
        setConfirmSteam(false);
        setMode('steam');
    }

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Launch options</div>
                <div className="settings-subtitle">
                    How Play starts this game - configured one game at a time, since install
                    layout and launcher quirks differ per game. Pick which one to configure below.
                </div>
            </div>

            {state.kind === 'loading' && <p className="status-page">Checking installed games...</p>}
            {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
            {state.kind === 'ready' && managedGames.length === 0 && (
                <p className="status-page">No games are set up to manage yet - open Manage games to pick one.</p>
            )}

            {managedGames.length > 0 && (
                <GamePickerChips games={managedGames} selectedGameId={selectedGameId} onSelect={setSelectedGameId}/>
            )}

            {selectedGame && prefs && (() => {
                const mode = launchModeFor(prefs, selectedGame.ID);
                return (
                    <div className="mode-option-list tiled">
                        {!selectedGame.Installed && (
                            <p className="status-page">
                                {selectedGame.DisplayName} isn't installed yet - set its path under
                                Paths & folders before switching it to Parallax Direct.
                            </p>
                        )}
                        <div className="mode-option disabled">
                            <i className="fa-solid fa-circle mode-option-radio off"/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">
                                    Steam Direct
                                    <span className="mode-option-recommended">(Recommended)</span>
                                    <span className="chip">Planned</span>
                                </div>
                                <div className="mode-option-desc">
                                    Replaces the launcher's own entry point so Steam launches the game
                                    directly while keeping Steam's full process context - overlay,
                                    achievements, and DLC checks all intact, unlike Parallax Direct
                                    below. Will become the default launch mode once it's built, since
                                    it keeps the most Steam integration of any bypass here.
                                </div>
                            </div>
                        </div>
                        <div
                            className={`mode-option ${mode === 'direct' ? 'active' : ''} ${!selectedGame.Installed ? 'disabled' : ''}`}
                            onClick={() => selectedGame.Installed && setMode('direct')}
                        >
                            <i className={`fa-solid ${mode === 'direct' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'direct' ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">Parallax Direct</div>
                                <div className="mode-option-desc">
                                    Skips the Paradox Launcher entirely and starts {selectedGame.DisplayName}'s
                                    own executable straight away. Steam overlay, achievements, and DLC
                                    checks may not work on every game - switch back to Steam / Paradox
                                    Launcher if something goes missing.
                                </div>
                            </div>
                        </div>
                        <div className={`mode-option ${mode === 'steam' ? 'active' : ''}`} onClick={() => pickSteam(mode)}>
                            <i className={`fa-solid ${mode === 'steam' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'steam' ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">
                                    Steam / Paradox Launcher
                                    <span className="mode-option-not-recommended">(Not recommended)</span>
                                </div>
                                <div className="mode-option-desc">
                                    The normal path - Steam opens the Paradox Launcher, which starts
                                    the game. Keeps full Steam integration: overlay, achievements, DLC
                                    ownership checks - but an extra step and window Steam Direct (above)
                                    will skip once it's built.
                                </div>
                            </div>
                        </div>
                        {confirmSteam && (
                            <div className="steam-confirm">
                                <span>
                                    Steam / Paradox Launcher opens an extra window on top of Steam itself
                                    every time - Parallax Direct above skips it, and Steam Direct will too
                                    once it's built. Switch anyway?
                                </span>
                                <span className="steam-confirm-actions">
                                    <button className="btn-primary" onClick={confirmPickSteam}>Switch anyway</button>
                                    <button className="btn-ghost" onClick={() => setConfirmSteam(false)}>Cancel</button>
                                </span>
                            </div>
                        )}
                    </div>
                );
            })()}
        </div>
    );
}

function PlaysetsSettingsPanel() {
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [playsetNames, setPlaysetNames] = useState<string[]>([]);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    const {state, managedGames, selectedGame, selectedGameId, setSelectedGameId} = useManagedGamePicker(prefs);

    useEffect(() => {
        if (!selectedGame) {
            setPlaysetNames([]);
            return;
        }
        ListPlaysets(selectedGame.ID).then(setPlaysetNames).catch(() => setPlaysetNames([]));
    }, [selectedGame?.ID]);

    function setAutoloadMode(mode: PlaysetAutoloadMode) {
        if (!prefs || !selectedGame) return;
        const next = {...prefs, playsetAutoloadModes: {...prefs.playsetAutoloadModes, [selectedGame.ID]: mode}};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    function setAutoloadCustomTarget(name: string) {
        if (!prefs || !selectedGame) return;
        const next = {...prefs, playsetAutoloadCustom: {...prefs.playsetAutoloadCustom, [selectedGame.ID]: name}};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Playsets</div>
                <div className="settings-subtitle">
                    Which playset (if any) Workspace loads automatically when you open a game -
                    configured one game at a time. Play never depends on one being loaded, so this
                    only controls what's already selected by the time you get there, not whether
                    you can launch at all.
                </div>
            </div>

            {state.kind === 'loading' && <p className="status-page">Checking installed games...</p>}
            {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
            {state.kind === 'ready' && managedGames.length === 0 && (
                <p className="status-page">No games are set up to manage yet - open Manage games to pick one.</p>
            )}

            {managedGames.length > 0 && (
                <GamePickerChips games={managedGames} selectedGameId={selectedGameId} onSelect={setSelectedGameId}/>
            )}

            {selectedGame && prefs && (() => {
                const mode = playsetAutoloadModeFor(prefs, selectedGame.ID);
                const customTarget = prefs.playsetAutoloadCustom?.[selectedGame.ID] ?? '';
                return (
                    <div className="mode-option-list tiled">
                        <div className={`mode-option ${mode === 'off' ? 'active' : ''}`} onClick={() => setAutoloadMode('off')}>
                            <i className={`fa-solid ${mode === 'off' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'off' ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">Start blank</div>
                                <div className="mode-option-desc">
                                    No playset loads automatically - Play still works, launching
                                    whatever's already on disk untouched until you pick or type a
                                    name yourself.
                                </div>
                            </div>
                        </div>
                        <div className={`mode-option ${mode === 'last' ? 'active' : ''}`} onClick={() => setAutoloadMode('last')}>
                            <i className={`fa-solid ${mode === 'last' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'last' ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">Autoload last used</div>
                                <div className="mode-option-desc">
                                    Whichever playset you most recently saved or switched to for
                                    {' '}{selectedGame.DisplayName} loads automatically next time.
                                </div>
                            </div>
                        </div>
                        <div className={`mode-option ${mode === 'custom' ? 'active' : ''}`} onClick={() => setAutoloadMode('custom')}>
                            <i className={`fa-solid ${mode === 'custom' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'custom' ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">Always load a specific playset</div>
                                <div className="mode-option-desc">
                                    Always the one playset you pick below, no matter what you
                                    switched to most recently.
                                </div>
                                {mode === 'custom' && (
                                    playsetNames.length > 0 ? (
                                        <div className="playset-autoload-targets" onClick={(e) => e.stopPropagation()}>
                                            {playsetNames.map((name) => (
                                                <span
                                                    key={name}
                                                    className={`chip ${name === customTarget ? 'chip-active' : ''}`}
                                                    onClick={() => setAutoloadCustomTarget(name)}
                                                >
                                                    {name}
                                                </span>
                                            ))}
                                        </div>
                                    ) : (
                                        <div className="playset-autoload-empty">
                                            No playsets saved yet for {selectedGame.DisplayName} - save one in
                                            Workspace first.
                                        </div>
                                    )
                                )}
                            </div>
                        </div>
                    </div>
                );
            })()}

            <div className="playset-sharing-block">
                <div className="playset-sharing-label">PLAYSET SHARING</div>
                <div className="settings-subtitle">
                    Export a playset as a shareable code, or import one someone sent you, so a
                    group can stay on the exact same mod list without hand-copying it. Not built
                    yet - Workspace's own "Share" button next to Save/Switch is this feature's
                    future home.
                </div>
                <span className="btn-ghost inert">Share a playset <i className="fa-solid fa-arrow-up-right-from-square"/></span>
            </div>
        </div>
    );
}

function SortRulesPanel() {
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    function togglePref(key: 'autosortDependencies' | 'autosortFixesLast') {
        if (!prefs) return;
        const next = {...prefs, [key]: !prefs[key]};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Sort rules</div>
                <div className="settings-subtitle">
                    Workspace's Autosort button applies whichever of these are on, top first, only
                    when you click it - it never reorders anything on its own. Toggle a rule off to
                    leave it out.
                </div>
            </div>
            {prefs && (
                <div className="sort-rules-list">
                    <div className="sort-rule-row">
                        <span className="mono index">1</span>
                        <div className="sort-rule-main">
                            <div className="sort-rule-name">Fixes/Utilities/Patch last</div>
                            <div className="sort-rule-desc">
                                A mod tagged Fixes, Utilities, or Patch moves to the end of the load
                                order - the same convention this app's own generated patch mod follows.
                            </div>
                        </div>
                        <Toggle on={prefs.autosortFixesLast} onClick={() => togglePref('autosortFixesLast')}/>
                    </div>
                    <div className="sort-rule-row">
                        <span className="mono index">2</span>
                        <div className="sort-rule-main">
                            <div className="sort-rule-name">Declared dependencies</div>
                            <div className="sort-rule-desc">
                                A mod moves to load right after every dependency it declares in its own
                                descriptor, matched by name against your other enabled mods.
                            </div>
                        </div>
                        <Toggle on={prefs.autosortDependencies} onClick={() => togglePref('autosortDependencies')}/>
                    </div>
                </div>
            )}
            <div className="sort-rule-actions">
                <span className="btn-ghost inert">+ Custom rule</span>
                <span className="btn-ghost inert">Import community ruleset</span>
                <span className="btn-ghost inert" style={{border: 'none', background: 'none'}}>Reset to defaults</span>
            </div>
        </div>
    );
}

// Settings that change how the app treats its own generated files, rather than
// anything about how it looks or which games it manages.
function AdvancedPanel() {
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    function togglePref(key: 'autosortPatchLast') {
        if (!prefs) return;
        const next = {...prefs, [key]: !prefs[key]};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Advanced</div>
                <div className="settings-subtitle">
                    How the app handles the files it generates for you.
                </div>
            </div>
            {prefs && (
                <div className="sort-rules-list">
                    <div className="sort-rule-row">
                        <div className="sort-rule-main">
                            <div className="sort-rule-name">Keep the generated patch last</div>
                            <div className="sort-rule-desc">
                                When you Autosort, always move Parallax's generated patch mod to the very
                                end of the load order, after every other mod, so the conflict resolutions
                                it carries win. Turn this off and Autosort leaves the patch exactly where
                                you put it.
                            </div>
                        </div>
                        <Toggle on={prefs.autosortPatchLast} onClick={() => togglePref('autosortPatchLast')}/>
                    </div>
                </div>
            )}
        </div>
    );
}

const MIN_BACKGROUND_INTERVAL_SECONDS = 5;
const MAX_BACKGROUND_INTERVAL_SECONDS = 86400;
const BACKGROUND_INTERVAL_STEP = 10;

// "300" on its own doesn't tell you much at a glance - shown next to the
// stepper as the unit label instead of a static "seconds".
function formatIntervalDuration(totalSeconds: number): string {
    const seconds = Math.max(0, Math.round(totalSeconds));
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) {
        const minutes = Math.floor(seconds / 60);
        const rest = seconds % 60;
        return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`;
    }
    const hours = Math.floor(seconds / 3600);
    const restMinutes = Math.floor((seconds % 3600) / 60);
    return restMinutes === 0 ? `${hours}h` : `${hours}h ${restMinutes}m`;
}

function AppearancePanel({onPreferencesChanged}: { onPreferencesChanged?: () => void }) {
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    // Its own local, text-editable copy of backgroundIntervalSeconds - if
    // this field wrote straight to prefs (and the backend) on every
    // keystroke the way the toggles below do, typing "300" would fire
    // three separate SetPreferences calls for "3", "30", and "300" in a
    // row. Only committed (parsed, clamped, and actually saved) on blur -
    // see commitInterval.
    const [intervalInput, setIntervalInput] = useState('');
    // The download window (see BackgroundDownloadModal): opened by choosing Offline,
    // and again from "Manage downloads" once offline.
    const [downloadMode, setDownloadMode] = useState<'switch' | 'manage' | null>(null);
    // What is on this computer, for the line under the source switch.
    const [onDisk, setOnDisk] = useState<{files: number; bytes: number; repo: string} | null>(null);
    // The Blur and Darken sliders' own positions while they are being dragged: the background
    // follows at once (previewBackgroundLook) and the setting is saved when the thumb is let go,
    // not on every pixel of the drag.
    const [look, setLook] = useState({blur: DEFAULT_BACKGROUND_BLUR, darken: DEFAULT_BACKGROUND_DARKEN});
    // The picture on screen, and whether the background can pick another one.
    const {current: currentBackground, canRandom} = useCurrentBackground();

    function refreshOnDisk() {
        BackgroundCatalog(false)
            .then((c) => setOnDisk({
                files: c.Packs.reduce((n, p) => n + p.LocalFiles, 0),
                bytes: c.Packs.reduce((n, p) => n + p.LocalBytes, 0),
                repo: c.Source,
            }))
            .catch(() => undefined);
    }

    useEffect(() => {
        GetPreferences().then((p) => {
            setPrefs(p);
            setIntervalInput(String(p.backgroundIntervalSeconds || DEFAULT_BACKGROUND_INTERVAL_SECONDS));
            setLook({blur: p.backgroundBlur ?? DEFAULT_BACKGROUND_BLUR, darken: p.backgroundDarken ?? DEFAULT_BACKGROUND_DARKEN});
        }).catch(() => undefined);
        refreshOnDisk();
        // Leaving the panel ends any preview; what was let go of is saved by then.
        return () => previewBackgroundLook(null);
    }, []);

    // Follows a slider as it moves.
    function previewLook(next: {blur: number; darken: number}) {
        setLook(next);
        previewBackgroundLook(next);
    }

    // Saves the look once a slider is let go (or a key or the reset button changed it).
    function commitLook(next: {blur: number; darken: number}) {
        setLook(next);
        previewBackgroundLook(next);
        if (!prefs) return;
        if (next.blur === prefs.backgroundBlur && next.darken === prefs.backgroundDarken) return;
        const saved = {...prefs, backgroundBlur: next.blur, backgroundDarken: next.darken};
        setPrefs(saved);
        SetPreferences(saved).then(onPreferencesChanged).catch(() => setPrefs(prefs));
    }

    // Online is a plain switch. Offline first asks which games to download (the
    // window below) and only takes effect once that is done - declining it leaves
    // the source as it was, online.
    function setSource(source: 'online' | 'offline') {
        if (!prefs) return;
        const next = {...prefs, backgroundSource: source};
        setPrefs(next);
        SetPreferences(next).then(onPreferencesChanged).catch(() => setPrefs(prefs));
    }

    function closeDownloadModal(result: { useOffline: boolean }) {
        const mode = downloadMode;
        setDownloadMode(null);
        refreshOnDisk();
        if (mode === 'switch' && result.useOffline) setSource('offline');
    }

    function togglePref(key: 'backgroundDisabled' | 'backgroundRotationPaused') {
        if (!prefs) return;
        const next = {...prefs, [key]: !prefs[key]};
        setPrefs(next);
        SetPreferences(next).then(onPreferencesChanged).catch(() => setPrefs(prefs));
    }

    // Rotating or Static (backgroundRotationPaused is the stored form of "Static").
    function setRotating(rotating: boolean) {
        if (!prefs || prefs.backgroundRotationPaused === !rotating) return;
        const next = {...prefs, backgroundRotationPaused: !rotating};
        setPrefs(next);
        SetPreferences(next).then(onPreferencesChanged).catch(() => setPrefs(prefs));
    }

    function commitIntervalValue(rawValue: number) {
        if (!prefs) return;
        const clamped = Number.isFinite(rawValue)
            ? Math.min(MAX_BACKGROUND_INTERVAL_SECONDS, Math.max(MIN_BACKGROUND_INTERVAL_SECONDS, Math.round(rawValue)))
            : DEFAULT_BACKGROUND_INTERVAL_SECONDS;
        setIntervalInput(String(clamped));
        if (clamped === prefs.backgroundIntervalSeconds) return;
        const next = {...prefs, backgroundIntervalSeconds: clamped};
        setPrefs(next);
        SetPreferences(next).then(onPreferencesChanged).catch(() => setPrefs(prefs));
    }

    function commitInterval() {
        commitIntervalValue(Number(intervalInput));
    }

    // The custom up/down buttons commit immediately (like the toggles
    // above), rather than waiting for blur the way typing into the field
    // does - there's no in-progress keystroke to debounce.
    function stepInterval(delta: number) {
        const current = Number(intervalInput);
        commitIntervalValue((Number.isFinite(current) ? current : DEFAULT_BACKGROUND_INTERVAL_SECONDS) + delta);
    }

    if (!prefs) {
        return <div className="settings-content single"/>;
    }

    const backgroundOn = !prefs.backgroundDisabled;
    const rotationOn = !prefs.backgroundRotationPaused;
    const offline = prefs.backgroundSource === 'offline';

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Appearance</div>
                <div className="settings-subtitle">
                    The interface's accent colour, and a background image behind the whole app,
                    drawn from the currently selected game's own art - see Manage Games for which
                    games have any. Let it rotate, or keep one picture.
                </div>
            </div>
            <div className="settings-columns">
                <div className="settings-column">
                    <AccentSettings onPreferencesChanged={onPreferencesChanged}/>
                </div>
                <div className="settings-column">
                    <div className="appearance-group-label">BACKGROUND</div>
                    <div className="profile-toggles">
                        <div className="profile-toggle-row">
                            <span>Backgrounds</span>
                            <Toggle on={backgroundOn} onClick={() => togglePref('backgroundDisabled')}/>
                        </div>
                        <div className={`profile-toggle-row ${backgroundOn ? '' : 'disabled'}`}>
                            <span>Background source</span>
                            <span className="source-toggle">
                                <span className={offline ? '' : 'active'} onClick={backgroundOn && offline ? () => setSource('online') : undefined}>
                                    <i className="fa-solid fa-cloud"/> Online
                                </span>
                                <span className={offline ? 'active' : ''} onClick={backgroundOn && !offline ? () => setDownloadMode('switch') : undefined}>
                                    <i className="fa-solid fa-hard-drive"/> Offline
                                </span>
                            </span>
                        </div>
                        <div className={`appearance-source-note ${backgroundOn ? '' : 'disabled'}`}>
                            {offline ? (
                                <>
                                    <span>
                                        Only images downloaded to this computer are used - nothing is fetched.{' '}
                                        {onDisk && onDisk.files > 0
                                            ? `${onDisk.files} images (${formatBytes(onDisk.bytes)}) on disk.`
                                            : 'None downloaded yet, so no background will show.'}
                                    </span>
                                    <span className="link-btn amber" onClick={() => setDownloadMode('manage')}>Manage downloads</span>
                                </>
                            ) : (
                                <span>
                                    Images are streamed from {onDisk?.repo ? `GitHub (${onDisk.repo})` : 'GitHub'} as they are needed: a small
                                    listing is fetched at startup and each image loads about 30 seconds before it appears. Nothing is
                                    stored. Choose Offline to download them instead.
                                </span>
                            )}
                        </div>
                        <div className={`profile-toggle-row ${backgroundOn ? '' : 'disabled'}`}>
                            <span>Background mode</span>
                            <span className="source-toggle">
                                <span className={rotationOn ? 'active' : ''} onClick={backgroundOn && !rotationOn ? () => setRotating(true) : undefined}>
                                    <i className="fa-solid fa-arrows-rotate"/> Rotating
                                </span>
                                <span className={rotationOn ? '' : 'active'} onClick={backgroundOn && rotationOn ? () => setRotating(false) : undefined}>
                                    <i className="fa-solid fa-image"/> Static
                                </span>
                            </span>
                        </div>
                        <div className={`appearance-source-note ${backgroundOn ? '' : 'disabled'}`}>
                            {rotationOn
                                ? <>A new random image every {formatIntervalDuration(Number(intervalInput))}. The next one is loaded a little before, so the swap is instant.</>
                                : <>One picture that never changes by itself: the one showing when you chose Static, or the last one you picked with Random. It is saved, so the same one loads every time; each game keeps its own.</>}
                        </div>
                        <div className={`profile-toggle-row ${backgroundOn && rotationOn ? '' : 'disabled'}`}>
                            <span>Change every</span>
                            <span className="appearance-interval">
                                <span className={`interval-stepper ${!backgroundOn || !rotationOn ? 'disabled' : ''}`}>
                                    <input
                                        type="number"
                                        step={BACKGROUND_INTERVAL_STEP}
                                        min={MIN_BACKGROUND_INTERVAL_SECONDS}
                                        max={MAX_BACKGROUND_INTERVAL_SECONDS}
                                        value={intervalInput}
                                        disabled={!backgroundOn || !rotationOn}
                                        onInput={(e) => setIntervalInput((e.target as HTMLInputElement).value)}
                                        onBlur={commitInterval}
                                        onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
                                    />
                                    <span className="interval-stepper-buttons">
                                        <button
                                            type="button"
                                            className="interval-stepper-btn up"
                                            tabIndex={-1}
                                            disabled={!backgroundOn || !rotationOn}
                                            onClick={() => stepInterval(BACKGROUND_INTERVAL_STEP)}
                                        >
                                            <i className="fa-solid fa-chevron-up"/>
                                        </button>
                                        <button
                                            type="button"
                                            className="interval-stepper-btn down"
                                            tabIndex={-1}
                                            disabled={!backgroundOn || !rotationOn}
                                            onClick={() => stepInterval(-BACKGROUND_INTERVAL_STEP)}
                                        >
                                            <i className="fa-solid fa-chevron-down"/>
                                        </button>
                                    </span>
                                </span>
                                <span className="mono unit">{formatIntervalDuration(Number(intervalInput))}</span>
                            </span>
                        </div>
                        <div className={`profile-toggle-row ${backgroundOn ? '' : 'disabled'}`}>
                            <span>Image</span>
                            <span className="appearance-random">
                                <span className="mono image-name" title={currentBackground ? `${currentBackground.name} (${currentBackground.origin})` : ''}>
                                    {backgroundOn && currentBackground ? currentBackground.name : '-'}
                                </span>
                                <button
                                    type="button"
                                    className="btn-ghost"
                                    disabled={!backgroundOn || !canRandom}
                                    title={!backgroundOn ? '' : canRandom ? (rotationOn ? 'Show another random image now' : 'Pick another random image and keep it') : 'This game has only one image to choose from'}
                                    onClick={() => requestRandomBackground()}
                                >
                                    <i className="fa-solid fa-shuffle"/> Random
                                </button>
                            </span>
                        </div>
                        <div className={`profile-toggle-row ${backgroundOn ? '' : 'disabled'}`}>
                            <span>Blur</span>
                            <span className="look-slider">
                                <input
                                    type="range"
                                    min={0}
                                    max={100}
                                    step={1}
                                    value={look.blur}
                                    disabled={!backgroundOn}
                                    aria-label="Background blur"
                                    onInput={(e) => previewLook({...look, blur: Number((e.target as HTMLInputElement).value)})}
                                    onChange={(e) => commitLook({...look, blur: Number((e.target as HTMLInputElement).value)})}
                                />
                                <span className="mono unit">{look.blur === 0 ? 'Off' : `${blurPixels(look.blur)} px`}</span>
                                <span
                                    className={`link-btn look-reset ${look.blur === DEFAULT_BACKGROUND_BLUR || !backgroundOn ? 'hidden' : ''}`}
                                    onClick={() => commitLook({...look, blur: DEFAULT_BACKGROUND_BLUR})}
                                >Reset</span>
                            </span>
                        </div>
                        <div className={`profile-toggle-row ${backgroundOn ? '' : 'disabled'}`}>
                            <span>Darken</span>
                            <span className="look-slider">
                                <input
                                    type="range"
                                    min={0}
                                    max={100}
                                    step={1}
                                    value={look.darken}
                                    disabled={!backgroundOn}
                                    aria-label="Background darkening"
                                    onInput={(e) => previewLook({...look, darken: Number((e.target as HTMLInputElement).value)})}
                                    onChange={(e) => commitLook({...look, darken: Number((e.target as HTMLInputElement).value)})}
                                />
                                <span className="mono unit">{look.darken}%</span>
                                <span
                                    className={`link-btn look-reset ${look.darken === DEFAULT_BACKGROUND_DARKEN || !backgroundOn ? 'hidden' : ''}`}
                                    onClick={() => commitLook({...look, darken: DEFAULT_BACKGROUND_DARKEN})}
                                >Reset</span>
                            </span>
                        </div>
                        <div className={`appearance-source-note ${backgroundOn ? '' : 'disabled'}`}>
                            Blur softens the image. Darken is how dark the layer over it is; {DEFAULT_BACKGROUND_DARKEN}% is how the app
                            has always looked, and less lets more of the art show through behind the text.
                        </div>
                    </div>
                </div>
            </div>
            {downloadMode && <BackgroundDownloadModal mode={downloadMode} onClose={closeDownloadModal}/>}
        </div>
    );
}

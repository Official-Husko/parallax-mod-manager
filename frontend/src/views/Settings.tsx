import './Settings.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {
    BrowseForExtraModFolder,
    BrowseForGameInstall,
    ClearGamePath,
    DetectGames,
    GetPreferences,
    OpenPath,
    RemoveExtraModFolder,
    SetGameManaged,
    SetPreferences,
} from '../../wailsjs/go/main/App';
import type {library, preferences} from '../../wailsjs/go/models';
import {GameLogo} from '../components/GameLogo';
import {Toggle} from '../components/Toggle';
import {settingsNav} from '../data/mockData';
import {DEFAULT_BACKGROUND_INTERVAL_SECONDS} from '../components/AppBackground';

type Section = 'manage' | 'paths' | 'launch' | 'sort' | 'appearance';

export function Settings({jumpToManageGames, onGamesChanged, onPreferencesChanged}: {
    // Incremented by app.tsx (the TopBar's own "Manage games" entry) to
    // ask this view to switch to the "Manage games" panel - 0 (the
    // default, falsy) means no pending request, so this never fights the
    // section the user's own click already put them on.
    jumpToManageGames?: number;
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

    useEffect(() => {
        if (jumpToManageGames) setSection('manage');
    }, [jumpToManageGames]);

    return (
        <div className="settings">
            <div className="settings-nav">
                <div className="sidebar-label">SETTINGS</div>
                {settingsNav.map((s) => {
                    const clickable = s.key === 'manage' || s.key === 'paths' || s.key === 'launch' || s.key === 'sort' || s.key === 'appearance';
                    const active = clickable && s.key === section;
                    return (
                        <div
                            key={s.key}
                            className={`settings-nav-row ${clickable ? 'clickable' : 'inert'}`}
                            style={{background: active ? '#1e2734' : 'transparent', color: active ? 'var(--text-bright)' : 'var(--text-mid)'}}
                            onClick={() => clickable && setSection(s.key as Section)}
                        >
                            {s.label}
                        </div>
                    );
                })}
            </div>

            {section === 'manage' && <ManageGamesPanel onGamesChanged={onGamesChanged}/>}
            {section === 'paths' && <PathsPanel/>}
            {section === 'launch' && <LaunchOptionsPanel/>}
            {section === 'sort' && <SortRulesPanel/>}
            {section === 'appearance' && <AppearancePanel onPreferencesChanged={onPreferencesChanged}/>}
        </div>
    );
}

type ManageGamesState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

function ManageGamesPanel({onGamesChanged}: { onGamesChanged?: () => void }) {
    const [state, setState] = useState<ManageGamesState>({kind: 'loading'});
    const [browsing, setBrowsing] = useState<Set<string>>(new Set());
    const [togglingManaged, setTogglingManaged] = useState<Set<string>>(new Set());
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);

    useEffect(() => {
        DetectGames()
            .then((games) => setState({kind: 'ready', games}))
            .catch((err) => setState({kind: 'error', message: String(err)}));
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    function togglePref(key: 'scanForNewMods' | 'closeAfterLaunch' | 'warnOnPatchMismatch') {
        if (!prefs) return;
        const next = {...prefs, [key]: !prefs[key]};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    async function setPath(gameId: string) {
        setBrowsing((prev) => new Set(prev).add(gameId));
        try {
            const updated = await BrowseForGameInstall(gameId);
            setState((prev) => prev.kind === 'ready'
                ? {kind: 'ready', games: prev.games.map((g) => (g.ID === gameId ? updated : g))}
                : prev);
            onGamesChanged?.();
        } catch {
            // A bad pick or a cancelled dialog just leaves the row as it was.
        } finally {
            setBrowsing((prev) => {
                const next = new Set(prev);
                next.delete(gameId);
                return next;
            });
        }
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
                    Workspace - each one keeps its own paths, playsets and sort rules.
                </div>
            </div>
            {state.kind === 'loading' && <p className="status-page">Checking installed games...</p>}
            {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
            {state.kind === 'ready' && (
                <div className="profile-list">
                    {state.games.map((g) => {
                        const managed = prefs?.managedGames?.includes(g.ID) ?? false;
                        return (
                            <div
                                key={g.ID}
                                className="profile-row"
                                style={{borderColor: g.Installed ? '#4a3826' : 'var(--border)', background: g.Installed ? '#191510' : 'var(--bg-rail)'}}
                            >
                                <GameLogo gameId={g.ID} className="profile-swatch"/>
                                <div className="profile-main">
                                    <div className="profile-name">{g.DisplayName}</div>
                                    <div className="mono profile-path">{g.Installed ? g.InstallPath : 'not detected'}</div>
                                </div>
                                {g.Installed ? (
                                    <span className="mono profile-state" style={{color: 'var(--green)'}}>
                                        {g.ModCount} mod{g.ModCount === 1 ? '' : 's'}
                                    </span>
                                ) : (
                                    <span
                                        className="mono profile-state actionable"
                                        style={{color: 'var(--amber)'}}
                                        onClick={() => setPath(g.ID)}
                                    >
                                        {browsing.has(g.ID) ? 'looking...' : 'set path'}
                                    </span>
                                )}
                                <Toggle
                                    on={managed}
                                    onClick={g.Installed && !togglingManaged.has(g.ID) ? () => toggleManaged(g.ID, !managed) : undefined}
                                />
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

type PathsState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

function PathsPanel() {
    const [state, setState] = useState<PathsState>({kind: 'loading'});
    const [busy, setBusy] = useState<Set<string>>(new Set());
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    // Every card starts collapsed (showing just its install-path row) -
    // with all 6+ registered games always rendered here regardless of
    // detection state, showing every one's mod folder and extra-folders
    // section by default wastes most of the panel on games most people
    // aren't even using. Expanding is per-game and explicit.
    const [expanded, setExpanded] = useState<Set<string>>(new Set());

    function toggleExpanded(gameId: string) {
        setExpanded((prev) => {
            const next = new Set(prev);
            if (next.has(gameId)) next.delete(gameId); else next.add(gameId);
            return next;
        });
    }

    useEffect(() => {
        DetectGames()
            .then((games) => setState({kind: 'ready', games}))
            .catch((err) => setState({kind: 'error', message: String(err)}));
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

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

    return (
        <div className="settings-content wide">
            <div>
                <div className="settings-title">Paths & folders</div>
                <div className="settings-subtitle">
                    Where each game is actually installed, and every folder Parallax Mod Manager
                    searches for its mods - its own managed mod folder, plus any extra folders you
                    add below, searched recursively for more.
                </div>
            </div>
            {state.kind === 'loading' && <p className="status-page">Checking installed games...</p>}
            {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
            {state.kind === 'ready' && (
                <div className="profile-list">
                    {state.games.map((g) => {
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
                                </div>

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
    const [state, setState] = useState<ManageGamesState>({kind: 'loading'});
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [selectedGameId, setSelectedGameId] = useState('');

    useEffect(() => {
        DetectGames()
            .then((games) => setState({kind: 'ready', games}))
            .catch((err) => setState({kind: 'error', message: String(err)}));
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    const managedGames = state.kind === 'ready'
        ? (prefs?.managedGames && prefs.managedGames.length > 0
            ? state.games.filter((g) => prefs.managedGames!.includes(g.ID))
            : state.games)
        : [];

    useEffect(() => {
        if (selectedGameId || managedGames.length === 0) return;
        setSelectedGameId(prefs?.lastSelectedGame && managedGames.some((g) => g.ID === prefs.lastSelectedGame)
            ? prefs.lastSelectedGame
            : managedGames[0].ID);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [managedGames.length, prefs]);

    const selectedGame = managedGames.find((g) => g.ID === selectedGameId);

    function setMode(mode: LaunchMode) {
        if (!prefs || !selectedGame) return;
        const next = {...prefs, launchModes: {...prefs.launchModes, [selectedGame.ID]: mode}};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
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
                <div className="launch-game-picker">
                    {managedGames.map((g) => (
                        <span
                            key={g.ID}
                            className={`chip ${g.ID === selectedGameId ? 'chip-active' : ''}`}
                            onClick={() => setSelectedGameId(g.ID)}
                        >
                            {g.DisplayName}
                        </span>
                    ))}
                </div>
            )}

            {selectedGame && prefs && (() => {
                const mode = launchModeFor(prefs, selectedGame.ID);
                return (
                    <div className="launch-mode-list">
                        {!selectedGame.Installed && (
                            <p className="status-page">
                                {selectedGame.DisplayName} isn't installed yet - set its path under
                                Paths & folders before switching it to Parallax Direct.
                            </p>
                        )}
                        <div className={`launch-mode-option ${mode === 'steam' ? 'active' : ''}`} onClick={() => setMode('steam')}>
                            <i className={`fa-solid ${mode === 'steam' ? 'fa-circle-dot' : 'fa-circle'} launch-mode-radio ${mode === 'steam' ? 'on' : 'off'}`}/>
                            <div className="launch-mode-main">
                                <div className="launch-mode-name">Steam / Paradox Launcher</div>
                                <div className="launch-mode-desc">
                                    The normal path - Steam opens the Paradox Launcher, which starts
                                    the game. Keeps full Steam integration: overlay, achievements, DLC
                                    ownership checks.
                                </div>
                            </div>
                        </div>
                        <div
                            className={`launch-mode-option ${mode === 'direct' ? 'active' : ''} ${!selectedGame.Installed ? 'disabled' : ''}`}
                            onClick={() => selectedGame.Installed && setMode('direct')}
                        >
                            <i className={`fa-solid ${mode === 'direct' ? 'fa-circle-dot' : 'fa-circle'} launch-mode-radio ${mode === 'direct' ? 'on' : 'off'}`}/>
                            <div className="launch-mode-main">
                                <div className="launch-mode-name">Parallax Direct</div>
                                <div className="launch-mode-desc">
                                    Skips the Paradox Launcher entirely and starts {selectedGame.DisplayName}'s
                                    own executable straight away. Steam overlay, achievements, and DLC
                                    checks may not work on every game - switch back to Steam / Paradox
                                    Launcher if something goes missing.
                                </div>
                            </div>
                        </div>
                        <div className="launch-mode-option disabled">
                            <i className="fa-solid fa-circle launch-mode-radio off"/>
                            <div className="launch-mode-main">
                                <div className="launch-mode-name">
                                    Steam Direct
                                    <span className="chip">Planned</span>
                                </div>
                                <div className="launch-mode-desc">
                                    Replaces the launcher's own entry point so Steam launches the game
                                    directly while keeping Steam's process context - best of both,
                                    once it's built.
                                </div>
                            </div>
                        </div>
                    </div>
                );
            })()}
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

    useEffect(() => {
        GetPreferences().then((p) => {
            setPrefs(p);
            setIntervalInput(String(p.backgroundIntervalSeconds || DEFAULT_BACKGROUND_INTERVAL_SECONDS));
        }).catch(() => undefined);
    }, []);

    function togglePref(key: 'backgroundDisabled' | 'backgroundRotationPaused') {
        if (!prefs) return;
        const next = {...prefs, [key]: !prefs[key]};
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

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Appearance</div>
                <div className="settings-subtitle">
                    A rotating background image behind the whole app, drawn from the currently
                    selected game's own art - see Manage Games for which games have any.
                </div>
            </div>
            <div className="profile-toggles">
                <div className="profile-toggle-row">
                    <span>Rotating background</span>
                    <Toggle on={backgroundOn} onClick={() => togglePref('backgroundDisabled')}/>
                </div>
                <div className={`profile-toggle-row ${backgroundOn ? '' : 'disabled'}`}>
                    <span>Change automatically</span>
                    <Toggle on={rotationOn} onClick={backgroundOn ? () => togglePref('backgroundRotationPaused') : undefined}/>
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
            </div>
        </div>
    );
}

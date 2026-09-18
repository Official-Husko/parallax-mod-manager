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

type Section = 'manage' | 'paths' | 'sort';

export function Settings({jumpToManageGames, onGamesChanged}: {
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
                    const clickable = s.key === 'manage' || s.key === 'paths' || s.key === 'sort';
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
            {section === 'sort' && <SortRulesPanel/>}
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
                        return (
                            <div
                                key={g.ID}
                                className="paths-card"
                                style={{borderColor: g.Installed ? '#4a3826' : 'var(--border)', background: g.Installed ? '#191510' : 'var(--bg-rail)'}}
                            >
                                <div className="paths-card-main">
                                    <GameLogo gameId={g.ID} className="profile-swatch"/>
                                    <div className="profile-main">
                                        <div className="profile-name">{g.DisplayName}</div>
                                        <div className="mono profile-path">
                                            {g.Installed ? g.InstallPath : 'not detected'}
                                        </div>
                                    </div>
                                    <div className="paths-card-actions">
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

                                <div className="paths-modfolder">
                                    <span className="paths-modfolder-label">Mod folder</span>
                                    <span className="mono paths-modfolder-value" title={g.ModFolder}>{g.ModFolder}</span>
                                    <span className="link-btn" onClick={() => openFolder(g.ModFolder)}>Open</span>
                                </div>

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
                            </div>
                        );
                    })}
                </div>
            )}
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

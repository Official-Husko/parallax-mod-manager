import './App.css'
import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {CheckGameUpdates, GameVersion, GetPreferences, ListGames, SetPreferences, StartupNotice} from '../wailsjs/go/main/App';
import type {library, preferences} from '../wailsjs/go/models';
import {TopBar} from './components/TopBar';
import type {ViewKey} from './components/TopBar';
import {NotificationStack} from './components/NotificationStack';
import {ContextMenu} from './components/ContextMenu';
import {Tooltip} from './components/Tooltip';
import {AppBackground} from './components/AppBackground';
import {dismiss, notify} from './data/notifications';
import {displayVersion} from './data/versionCompat';
import {Workspace} from './views/Workspace';
import {Library} from './views/Library';
import {Dlc} from './views/Dlc';
import {Settings} from './views/Settings';
import {UpdatesModal} from './views/UpdatesModal';
import {FirstRunWizard} from './views/FirstRunWizard';
import {getLegacyManagedGames} from './data/managedGames';
import {useGameAccent} from './data/useGameAccent';

const ONBOARDED_KEY = 'parallax-onboarded';

function wasOnboarded(): boolean {
    try {
        return localStorage.getItem(ONBOARDED_KEY) === '1';
    } catch {
        return true;
    }
}

function markOnboarded() {
    try {
        localStorage.setItem(ONBOARDED_KEY, '1');
    } catch {
        // Private browsing / blocked storage - just skip persisting the flag.
    }
}

// How soon after one game-update check another may run - see checkForGameUpdates.
const GAME_UPDATE_CHECK_MIN_MS = 30_000;

// Every game's installed version, keyed by game ID ("" when unknown).
function fetchGameVersions(list: library.GameInfo[]): Promise<Record<string, string>> {
    return Promise.all(list.map((g) => GameVersion(g.ID).then((v) => [g.ID, v] as const).catch(() => [g.ID, ''] as const)))
        .then((pairs) => Object.fromEntries(pairs));
}

export function App() {
    const [view, setView] = useState<ViewKey>('workspace');
    const [games, setGames] = useState<library.GameInfo[]>([]);
    const [selectedGame, setSelectedGame] = useState('');
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [playsetName, setPlaysetName] = useState('');
    const [onboarded, setOnboarded] = useState(wasOnboarded());
    const [showUpdates, setShowUpdates] = useState(false);
    // Lives here, not inside Workspace, so the TopBar's own playset pill
    // (a sibling of Workspace, not a parent/child) can open the exact
    // same real switcher - Workspace still owns everything else about
    // playsets (the real saved-playset list, loading/saving one), only
    // this one boolean is controlled from outside it.
    const [showPlaysets, setShowPlaysets] = useState(false);
    // True while there are no usable games (none set up, or loading them
    // failed) - the game-dependent views stay unmounted meanwhile, and the
    // reason is reported through a notification, not on the page.
    const [gamesUnavailable, setGamesUnavailable] = useState(false);
    // The standing "no games are set up" notice, so it can be taken down again
    // once a game does get set up (see loadGames).
    const noGamesNoticeRef = useRef<string | null>(null);
    // The real, currently-installed version of every managed game (e.g.
    // "v4.4.6"), keyed by game ID, read from each one's own real Paradox
    // Launcher launcher-settings.json - see
    // internal/game.GameConfig.GameVersion. Fetched fresh whenever the
    // managed-games list itself changes (which includes the very first
    // resolve, at startup) - every game at once, not just the selected
    // one, so the TopBar's own game switcher dropdown can show each
    // game's version right there, not only the currently active one. A
    // game missing from this map (not yet resolved, or resolved to "")
    // just shows nothing - never a placeholder or an error.
    const [gameVersions, setGameVersions] = useState<Record<string, string>>({});
    const gameVersion = gameVersions[selectedGame] ?? '';
    // Incremented to ask the (already-mounted, once visited) Settings
    // view to jump to its "Manage games" panel - see the TopBar's own
    // "Manage games" entry in its game switcher dropdown. 0 (falsy) means
    // "no pending request," so Settings' own default section on first
    // mount is untouched by this.
    const [settingsManageGamesRequest, setSettingsManageGamesRequest] = useState(0);
    // Every view this session has actually navigated to at least once -
    // 'workspace' up front since it's the default. Once a view is in
    // here it's mounted for good (see the render below); this set only
    // ever grows, so a view already visited is never torn down and
    // rebuilt again just because the user looked at a different tab -
    // see the comment above the view wrappers for why that mattered.
    // Library isn't in the initial set on purpose: unlike the other
    // three, its own first mount kicks off a real scan across every
    // managed game at once (Library.tsx), not just the selected one -
    // eagerly mounting it up front would run that unasked-for work on
    // every single app launch, even for a session that never opens it.
    const [visitedViews, setVisitedViews] = useState<Set<ViewKey>>(new Set(['workspace']));

    useEffect(() => {
        setVisitedViews((prev) => (prev.has(view) ? prev : new Set(prev).add(view)));
    }, [view]);

    // Replaces the webview's own native right-click menu app-wide with
    // this app's real one (components/ContextMenu.tsx) - anywhere that
    // doesn't open a real one of its own via openContextMenu() (see
    // data/contextMenu.ts) just gets no menu at all on right-click,
    // rather than ever falling back to the browser's default (reload,
    // inspect element, and the like - not meaningful chrome for a
    // packaged desktop app). Runs unconditionally, before the onboarding
    // check below, so this holds even on the first-run wizard screen.
    useEffect(() => {
        const onContextMenu = (e: MouseEvent) => e.preventDefault();
        document.addEventListener('contextmenu', onContextMenu);
        return () => document.removeEventListener('contextmenu', onContextMenu);
    }, []);

    // Recomputes the visible/selectable game list from ListGames() plus
    // whichever games are currently marked managed in preferences - the
    // single source of truth for "which games show up in the game
    // switcher, Library, DLC, and Workspace" (see
    // preferences.Preferences.ManagedGames). Exposed to the Manage Games
    // settings panel as a callback so toggling a game there takes effect
    // immediately, without needing a restart - the earlier, localStorage-
    // only version of this state had no such hook, which was exactly why a
    // freshly-detected game could show as installed in Settings yet never
    // actually become selectable anywhere.
    function loadGames() {
        return Promise.all([ListGames(), GetPreferences().catch(() => null)])
            .then(async ([list, loadedPrefs]) => {
                let effectivePrefs = loadedPrefs;
                // One-time migration for an existing install: the managed-
                // games choice used to live only in this browser's
                // localStorage (see data/managedGames.ts). If the backend
                // preference has never been set, but a legacy selection is
                // still sitting in localStorage, fold it into preferences
                // now so it survives from here on like every other setting.
                if (effectivePrefs && (!effectivePrefs.managedGames || effectivePrefs.managedGames.length === 0)) {
                    const legacy = getLegacyManagedGames();
                    if (legacy && legacy.length > 0) {
                        const migrated = {...effectivePrefs, managedGames: legacy};
                        try {
                            await SetPreferences(migrated);
                            effectivePrefs = migrated;
                        } catch {
                            // Best effort - fall through with the
                            // un-migrated preferences below.
                        }
                    }
                }

                setPrefs(effectivePrefs);
                const managedKeys = effectivePrefs?.managedGames ?? null;
                const visible = managedKeys && managedKeys.length > 0
                    ? list.filter((g) => managedKeys.includes(g.ID))
                    : list;
                setGames(visible);
                if (visible.length === 0) {
                    setSelectedGame('');
                    setGamesUnavailable(true);
                    if (!noGamesNoticeRef.current) {
                        noGamesNoticeRef.current = notify('warning', 'No games are set up to manage yet - open Manage games to pick one.');
                    }
                    return;
                }
                setGamesUnavailable(false);
                if (noGamesNoticeRef.current) {
                    dismiss(noGamesNoticeRef.current);
                    noGamesNoticeRef.current = null;
                }
                setSelectedGame((prev) => {
                    if (visible.some((g) => g.ID === prev)) {
                        return prev;
                    }
                    const lastSelected = effectivePrefs?.lastSelectedGame ?? '';
                    return visible.find((g) => g.ID === lastSelected)?.ID ?? visible[0].ID;
                });
            })
            .catch((err) => {
                setGamesUnavailable(true);
                notify('error', `Couldn't load your games: ${String(err)}`);
            });
    }

    useEffect(() => {
        if (!onboarded) {
            return;
        }
        StartupNotice().then((text) => { if (text) notify('info', text); }).catch(() => undefined);
        loadGames();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [onboarded]);

    useEffect(() => {
        if (games.length === 0) {
            setGameVersions({});
            return;
        }
        let cancelled = false;
        fetchGameVersions(games).then((versions) => { if (!cancelled) setGameVersions(versions); });
        return () => { cancelled = true; };
    }, [games]);

    // Tell the user when a game has updated - at startup, and again whenever
    // the window regains focus, since Steam may well have updated the game while
    // this app was in the background. The backend remembers the last version it
    // saw per game, so each update is reported once; the throttle only keeps a
    // burst of focus events (alt-tabbing back and forth) from asking every time.
    const gameUpdateCheck = useRef({lastAt: 0, running: false});
    const gamesRef = useRef(games);
    gamesRef.current = games;
    async function checkForGameUpdates() {
        const state = gameUpdateCheck.current;
        if (state.running || Date.now() - state.lastAt < GAME_UPDATE_CHECK_MIN_MS) return;
        state.running = true;
        try {
            const updates = await CheckGameUpdates();
            state.lastAt = Date.now();
            for (const u of updates) {
                notify('warning', `${u.GameName} is now ${displayVersion(u.To)} (was ${displayVersion(u.From)}). Mods made for the old version may need updates.`, {
                    action: {label: 'Review mods', onClick: () => setView('workspace')},
                });
            }
            if (updates.length > 0) {
                // The top bar's version pill should show the new version now.
                fetchGameVersions(gamesRef.current).then(setGameVersions);
            }
        } catch {
            // Best effort: not being able to check must never get in the way.
        } finally {
            state.running = false;
        }
    }
    useEffect(() => {
        if (!onboarded || games.length === 0) return;
        checkForGameUpdates();
        window.addEventListener('focus', checkForGameUpdates);
        return () => window.removeEventListener('focus', checkForGameUpdates);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [onboarded, games.length > 0]);

    function selectGame(id: string) {
        setSelectedGame(id);
        if (prefs) {
            const next = {...prefs, lastSelectedGame: id};
            setPrefs(next);
            SetPreferences(next).catch(() => undefined);
        }
    }

    const accent = useGameAccent(onboarded ? selectedGame : '');

    if (!onboarded) {
        return (
            <div id="app">
                <NotificationStack/>
                <FirstRunWizard onFinish={() => { markOnboarded(); setOnboarded(true); }}/>
                <ContextMenu/>
                <Tooltip/>
            </div>
        );
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;

    function openManageGames() {
        setView('settings');
        setSettingsManageGamesRequest((n) => n + 1);
    }

    const gamePicker = view === 'workspace'
        ? {
            gameLabel: gameName,
            gameVersion,
            gameVersions,
            playsetLabel: playsetName || '(unsaved)',
            onOpenPlaysetSwitcher: () => setShowPlaysets(true),
            games: games.map((g) => ({ID: g.ID, DisplayName: g.DisplayName})),
            onSelectGame: selectGame,
            onManageGames: openManageGames,
        }
        : view === 'dlc'
            ? {
                gameLabel: gameName,
                gameVersion,
                gameVersions,
                games: games.map((g) => ({ID: g.ID, DisplayName: g.DisplayName})),
                onSelectGame: selectGame,
                onManageGames: openManageGames,
            }
            : undefined;

    const accentStyle = accent
        ? ({'--rust': accent.color, '--rust-text': accent.textColor} as unknown as h.JSX.CSSProperties)
        : undefined;

    return (
        <div id="app" style={accentStyle}>
            <AppBackground
                gameId={selectedGame}
                disabled={prefs?.backgroundDisabled ?? false}
                rotationPaused={prefs?.backgroundRotationPaused ?? false}
                intervalSeconds={prefs?.backgroundIntervalSeconds ?? 0}
            />
            <TopBar view={view} onNavigate={setView} gamePicker={gamePicker}/>
            <NotificationStack/>

            {/* Every view stays mounted once shown, switching only via
                display:contents/none rather than a real conditional
                render - a real conditional (view === 'x' && <X/>) used to
                fully unmount a view the moment the user navigated away,
                which was a real bug, not just wasted re-fetching:
                Workspace's own eager Steam Workshop fetch (author names,
                descriptions - see Workspace.tsx) never got a chance to
                finish if the user checked the DLC page and came back
                before it resolved, since the whole component - and its
                in-flight request - was torn down and restarted from zero
                every single time. The same unmount was silently dropping
                any *unsaved* live edits too (Workspace's in-progress load
                order, Dlc's own not-yet-saved toggle changes) the instant
                the user navigated elsewhere, which is a correctness
                problem on its own, independent of the fetch-timing one.
                display:contents makes the wrapper itself invisible to
                layout when active (its children become #app's own direct
                flex items, exactly as if there were no wrapper at all);
                display:none removes it and its children from layout
                entirely when inactive, without unmounting anything. */}
            {!gamesUnavailable && visitedViews.has('workspace') && (
                <div style={{display: view === 'workspace' ? 'contents' : 'none'}}>
                    <Workspace
                        games={games}
                        selectedGame={selectedGame}
                        gameVersion={gameVersion}
                        onPlaysetNameChange={setPlaysetName}
                        onOpenUpdates={() => setShowUpdates(true)}
                        showPlaysets={showPlaysets}
                        setShowPlaysets={setShowPlaysets}
                    />
                </div>
            )}
            {!gamesUnavailable && visitedViews.has('library') && (
                <div style={{display: view === 'library' ? 'contents' : 'none'}}>
                    <Library/>
                </div>
            )}
            {!gamesUnavailable && visitedViews.has('dlc') && (
                <div style={{display: view === 'dlc' ? 'contents' : 'none'}}>
                    <Dlc games={games} selectedGame={selectedGame}/>
                </div>
            )}
            {/* Settings isn't gated on gamesUnavailable like the views above - unlike
                them it doesn't depend on games/selectedGame, and it's the
                only way to recover from the "no games managed" error (e.g.
                after un-managing the last one from its own Manage Games
                panel), so it must stay reachable even while that error is
                showing. */}
            {visitedViews.has('settings') && (
                <div style={{display: view === 'settings' ? 'contents' : 'none'}}>
                    <Settings
                        jumpToManageGames={settingsManageGamesRequest}
                        onGamesChanged={loadGames}
                        // loadGames also refetches preferences (see its own
                        // comment) - the same function as onGamesChanged
                        // above, just under the name each caller actually
                        // means: AppearancePanel's own background toggles
                        // change nothing about games, but do need this
                        // component's own prefs state (the AppBackground
                        // props below) refreshed, or a change made in
                        // Settings would only take effect after a restart.
                        onPreferencesChanged={loadGames}
                    />
                </div>
            )}

            {showUpdates && <UpdatesModal onClose={() => setShowUpdates(false)}/>}
            <ContextMenu/>
            <Tooltip/>
        </div>
    );
}

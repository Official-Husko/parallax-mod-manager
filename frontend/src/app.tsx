import './App.css'
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {GameVersion, GetPreferences, ListGames, SetPreferences, StartupNotice} from '../wailsjs/go/main/App';
import type {library, preferences} from '../wailsjs/go/models';
import {TopBar} from './components/TopBar';
import type {ViewKey} from './components/TopBar';
import {NotificationStack} from './components/NotificationStack';
import {ContextMenu} from './components/ContextMenu';
import {notify} from './data/notifications';
import {Workspace} from './views/Workspace';
import {Library} from './views/Library';
import {Dlc} from './views/Dlc';
import {Settings} from './views/Settings';
import {UpdatesModal} from './views/UpdatesModal';
import {FirstRunWizard} from './views/FirstRunWizard';
import {getManagedGames} from './data/managedGames';
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
    const [error, setError] = useState('');
    // The real, currently-installed game version (e.g. "v4.4.6"), read
    // from the real Paradox Launcher's own launcher-settings.json - see
    // internal/game.GameConfig.GameVersion. Fetched fresh on every game
    // switch (which includes the very first one, at startup); "" (shown
    // as nothing at all, not an error) when it can't be determined - a
    // game that isn't installed, or whose launcher-settings.json simply
    // doesn't carry one. Lives here, not inside Workspace, so the TopBar's
    // own game pill (a sibling of Workspace) can show it too.
    const [gameVersion, setGameVersion] = useState('');
    // Incremented to ask the (already-mounted, once visited) Settings
    // view to jump to its "Game profiles" panel - see the TopBar's own
    // "+ Add game" entry in its game switcher dropdown. 0 (falsy) means
    // "no pending request," so Settings' own default section on first
    // mount is untouched by this.
    const [settingsProfilesRequest, setSettingsProfilesRequest] = useState(0);
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

    useEffect(() => {
        if (!onboarded) {
            return;
        }
        StartupNotice().then((text) => { if (text) notify('info', text); }).catch(() => undefined);
        Promise.all([ListGames(), GetPreferences().catch(() => null)])
            .then(([list, loadedPrefs]) => {
                setPrefs(loadedPrefs);
                const managedKeys = getManagedGames();
                const visible = managedKeys && managedKeys.length > 0
                    ? list.filter((g) => managedKeys.includes(g.ID))
                    : list;
                setGames(visible);
                if (visible.length > 0) {
                    const lastSelected = loadedPrefs?.lastSelectedGame ?? '';
                    const initial = visible.find((g) => g.ID === lastSelected)?.ID ?? visible[0].ID;
                    setSelectedGame(initial);
                } else {
                    setError('No games are set up to manage yet - run setup again to select one.');
                }
            })
            .catch((err) => setError(String(err)));
    }, [onboarded]);

    useEffect(() => {
        if (!selectedGame) {
            setGameVersion('');
            return;
        }
        let cancelled = false;
        GameVersion(selectedGame)
            .then((v) => { if (!cancelled) setGameVersion(v); })
            .catch(() => { if (!cancelled) setGameVersion(''); });
        return () => { cancelled = true; };
    }, [selectedGame]);

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
                <FirstRunWizard onFinish={() => { markOnboarded(); setOnboarded(true); }}/>
                <ContextMenu/>
            </div>
        );
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;

    function openGameProfiles() {
        setView('settings');
        setSettingsProfilesRequest((n) => n + 1);
    }

    const gamePicker = view === 'workspace'
        ? {
            gameLabel: gameName,
            gameVersion,
            playsetLabel: playsetName || '(unsaved)',
            onOpenPlaysetSwitcher: () => setShowPlaysets(true),
            games: games.map((g) => ({ID: g.ID, DisplayName: g.DisplayName})),
            onSelectGame: selectGame,
            onAddGame: openGameProfiles,
        }
        : view === 'dlc'
            ? {
                gameLabel: gameName,
                gameVersion,
                games: games.map((g) => ({ID: g.ID, DisplayName: g.DisplayName})),
                onSelectGame: selectGame,
                onAddGame: openGameProfiles,
            }
            : undefined;

    const accentStyle = accent
        ? ({'--rust': accent.color, '--rust-text': accent.textColor} as unknown as h.JSX.CSSProperties)
        : undefined;

    return (
        <div id="app" style={accentStyle}>
            <TopBar view={view} onNavigate={setView} gamePicker={gamePicker}/>
            <NotificationStack/>

            {error && <p className="status-page error">{error}</p>}

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
            {!error && visitedViews.has('workspace') && (
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
            {!error && visitedViews.has('library') && (
                <div style={{display: view === 'library' ? 'contents' : 'none'}}>
                    <Library/>
                </div>
            )}
            {!error && visitedViews.has('dlc') && (
                <div style={{display: view === 'dlc' ? 'contents' : 'none'}}>
                    <Dlc games={games} selectedGame={selectedGame}/>
                </div>
            )}
            {!error && visitedViews.has('settings') && (
                <div style={{display: view === 'settings' ? 'contents' : 'none'}}>
                    <Settings jumpToProfiles={settingsProfilesRequest}/>
                </div>
            )}

            {showUpdates && <UpdatesModal onClose={() => setShowUpdates(false)}/>}
            <ContextMenu/>
        </div>
    );
}

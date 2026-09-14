import './App.css'
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {GetPreferences, ListGames, SetPreferences, StartupNotice} from '../wailsjs/go/main/App';
import type {library, preferences} from '../wailsjs/go/models';
import {TopBar} from './components/TopBar';
import type {ViewKey} from './components/TopBar';
import {NotificationStack} from './components/NotificationStack';
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
    const [error, setError] = useState('');
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
            </div>
        );
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;

    const gamePicker = view === 'workspace'
        ? {
            gameLabel: gameName,
            playsetLabel: playsetName || '(unsaved)',
            games: games.map((g) => ({ID: g.ID, DisplayName: g.DisplayName})),
            onSelectGame: selectGame,
        }
        : view === 'dlc'
            ? {
                gameLabel: gameName,
                games: games.map((g) => ({ID: g.ID, DisplayName: g.DisplayName})),
                onSelectGame: selectGame,
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
                        onPlaysetNameChange={setPlaysetName}
                        onOpenUpdates={() => setShowUpdates(true)}
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
                    <Settings/>
                </div>
            )}

            {showUpdates && <UpdatesModal onClose={() => setShowUpdates(false)}/>}
        </div>
    );
}

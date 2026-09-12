import './App.css'
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {GetPreferences, ListGames, SetPreferences, StartupNotice} from '../wailsjs/go/main/App';
import type {library, preferences} from '../wailsjs/go/models';
import {TopBar} from './components/TopBar';
import type {ViewKey} from './components/TopBar';
import {Workspace} from './views/Workspace';
import {Library} from './views/Library';
import {Dlc} from './views/Dlc';
import {Settings} from './views/Settings';
import {ConflictResolver} from './views/ConflictResolver';
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
    const [showConflictResolver, setShowConflictResolver] = useState(false);
    const [showUpdates, setShowUpdates] = useState(false);
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');

    useEffect(() => {
        if (!onboarded) {
            return;
        }
        StartupNotice().then(setNotice).catch(() => undefined);
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
            profileLabel: playsetName || '(unsaved)',
            games: games.map((g) => ({ID: g.ID, DisplayName: g.DisplayName})),
            onSelectGame: selectGame,
        }
        : view === 'dlc'
            ? {gameLabel: 'Hearts of Iron IV', profileLabel: 'Kaiserreich MP'}
            : undefined;

    const accentStyle = accent
        ? ({'--rust': accent.color, '--rust-text': accent.textColor} as unknown as h.JSX.CSSProperties)
        : undefined;

    return (
        <div id="app" style={accentStyle}>
            {notice && (
                <div className="app-notice">
                    <span>{notice}</span>
                    <i className="fa-solid fa-xmark" onClick={() => setNotice('')}/>
                </div>
            )}
            <TopBar view={view} onNavigate={setView} gamePicker={gamePicker}/>

            {error && <p className="status-page error">{error}</p>}

            {!error && view === 'workspace' && (
                <Workspace
                    games={games}
                    selectedGame={selectedGame}
                    onPlaysetNameChange={setPlaysetName}
                    onOpenConflictResolver={() => setShowConflictResolver(true)}
                    onOpenUpdates={() => setShowUpdates(true)}
                />
            )}
            {!error && view === 'library' && <Library/>}
            {!error && view === 'dlc' && <Dlc/>}
            {!error && view === 'settings' && <Settings/>}

            {showConflictResolver && <ConflictResolver onClose={() => setShowConflictResolver(false)}/>}
            {showUpdates && <UpdatesModal onClose={() => setShowUpdates(false)}/>}
        </div>
    );
}

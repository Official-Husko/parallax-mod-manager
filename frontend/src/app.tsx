import './App.css'
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {ListGames} from '../wailsjs/go/main/App';
import type {library} from '../wailsjs/go/models';
import {TopBar} from './components/TopBar';
import type {ViewKey} from './components/TopBar';
import {Workspace} from './views/Workspace';
import {Library} from './views/Library';
import {Dlc} from './views/Dlc';
import {Settings} from './views/Settings';
import {ConflictResolver} from './views/ConflictResolver';
import {UpdatesModal} from './views/UpdatesModal';
import {FirstRunWizard} from './views/FirstRunWizard';

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
    const [playsetName, setPlaysetName] = useState('');
    const [onboarded, setOnboarded] = useState(wasOnboarded());
    const [showConflictResolver, setShowConflictResolver] = useState(false);
    const [showUpdates, setShowUpdates] = useState(false);
    const [error, setError] = useState('');

    useEffect(() => {
        ListGames()
            .then((list) => {
                setGames(list);
                if (list.length > 0) {
                    setSelectedGame(list[0].Key);
                } else {
                    setError('No games are registered in this build.');
                }
            })
            .catch((err) => setError(String(err)));
    }, []);

    if (!onboarded) {
        return (
            <div id="app">
                <FirstRunWizard onFinish={() => { markOnboarded(); setOnboarded(true); }}/>
            </div>
        );
    }

    const gameName = games.find((g) => g.Key === selectedGame)?.DisplayName ?? selectedGame;

    const gamePicker = view === 'workspace'
        ? {gameLabel: gameName, profileLabel: playsetName || '(unsaved)'}
        : view === 'dlc'
            ? {gameLabel: 'Hearts of Iron IV', profileLabel: 'Kaiserreich MP'}
            : undefined;

    return (
        <div id="app">
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

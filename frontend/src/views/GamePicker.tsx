import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DetectGames} from '../../wailsjs/go/main/App';
import type {library, preferences} from '../../wailsjs/go/models';
import {GameLogo} from '../components/GameLogo';

export type ManageGamesState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

// Shared "pick one managed game to configure" behavior between the
// per-game Settings panels (Launch Options, Playsets, Backup) - each configures per-game settings one game
// at a time (see each panel's own subtitle), so this is the same
// selection logic, not two subtly different ones. prefs is read (for
// managedGames/lastSelectedGame) but never written here - each panel
// keeps its own prefs state for that, since each writes different fields.
export function useManagedGamePicker(prefs: preferences.Preferences | null) {
    const [state, setState] = useState<ManageGamesState>({kind: 'loading'});
    const [selectedGameId, setSelectedGameId] = useState('');

    useEffect(() => {
        DetectGames()
            .then((games) => setState({kind: 'ready', games}))
            .catch((err) => setState({kind: 'error', message: String(err)}));
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
    return {state, managedGames, selectedGame, selectedGameId, setSelectedGameId};
}

// The chip row itself, shared by the same two panels useManagedGamePicker
// is - each game's real logo (GameLogo already handles the no-art-yet
// fallback) next to its name, so picking one among several games isn't
// just reading text.
export function GamePickerChips({games, selectedGameId, onSelect}: {
    games: library.DetectedGame[];
    selectedGameId: string;
    onSelect: (gameId: string) => void;
}) {
    return (
        <div className="settings-game-picker">
            {games.map((g) => (
                <span
                    key={g.ID}
                    className={`chip ${g.ID === selectedGameId ? 'chip-active' : ''}`}
                    onClick={() => onSelect(g.ID)}
                >
                    <GameLogo gameId={g.ID} className="chip-logo"/>
                    {g.DisplayName}
                </span>
            ))}
        </div>
    );
}

// ManagedGamePicker is useManagedGamePicker's own state (loading/error/ready) plus
// GamePickerChips, combined into the one block every per-game Settings panel (Launch
// Options, Playsets, Conflict Rules, Backup) was repeating identically: the same
// "Checking installed games..."/error text while DetectGames() is in flight, then either
// emptyMessage (nothing managed yet - open Manage games first) or the chip row itself.
// Renders nothing once ready with something to pick from and no chips are wanted for it -
// this only ever returns null when emptyMessage is left unset and there is nothing managed,
// which is the one caller (BackupPanel) that already explains that state elsewhere on its
// own page.
export function ManagedGamePicker({state, managedGames, selectedGameId, onSelect, emptyMessage}: {
    state: ManageGamesState;
    managedGames: library.DetectedGame[];
    selectedGameId: string;
    onSelect: (gameId: string) => void;
    emptyMessage?: string;
}) {
    if (state.kind === 'loading') return <p className="status-page">Checking installed games...</p>;
    if (state.kind === 'error') return <p className="status-page error">{state.message}</p>;
    if (managedGames.length === 0) {
        return emptyMessage ? <p className="status-page">{emptyMessage}</p> : null;
    }
    return <GamePickerChips games={managedGames} selectedGameId={selectedGameId} onSelect={onSelect}/>;
}


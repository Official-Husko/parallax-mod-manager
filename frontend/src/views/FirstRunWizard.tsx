import './FirstRunWizard.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DetectGames, ListPlaysets} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {APP_NAME, wizardSteps} from '../data/mockData';
import {gameLogos} from '../data/gameLogos';

type LoadState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

export function FirstRunWizard({onFinish}: { onFinish: () => void }) {
    const [step, setStep] = useState(1);
    const [state, setState] = useState<LoadState>({kind: 'loading'});
    const [playsetCounts, setPlaysetCounts] = useState<Record<string, string[]>>({});

    useEffect(() => {
        DetectGames()
            .then((games) => setState({kind: 'ready', games}))
            .catch((err) => setState({kind: 'error', message: String(err)}));
    }, []);

    useEffect(() => {
        if (step !== 3 || state.kind !== 'ready') {
            return;
        }
        for (const g of state.games) {
            if (g.Key in playsetCounts) {
                continue;
            }
            ListPlaysets(g.Key)
                .then((names) => setPlaysetCounts((prev) => ({...prev, [g.Key]: names})))
                .catch(() => setPlaysetCounts((prev) => ({...prev, [g.Key]: []})));
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [step, state]);

    const games = state.kind === 'ready' ? state.games : [];

    return (
        <div className="wizard-overlay">
            <div className="wizard">
                <div className="wizard-sidebar">
                    <img className="wizard-icon" src="/favicon.png" alt=""/>
                    <div className="wizard-title">Set up {APP_NAME}</div>
                    <div className="wizard-steps">
                        {wizardSteps.map((w) => {
                            const status = w.n === step ? 'active' : w.n < step ? 'done' : 'pending';
                            return (
                                <div key={w.n} className={`wizard-step ${status}`}>
                                    <span className="wizard-step-num">
                                        {status === 'done' ? <i className="fa-solid fa-check"/> : w.n}
                                    </span>
                                    <span>{w.label}</span>
                                </div>
                            );
                        })}
                    </div>
                    <div className="wizard-footnote">
                        Nothing is uploaded. Playset codes are only generated when you press Share.
                    </div>
                </div>
                <div className="wizard-main">
                    {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
                    {state.kind === 'loading' && <p className="status-page">Looking for installed games...</p>}
                    {state.kind === 'ready' && step === 1 && <FindGamesStep games={games}/>}
                    {state.kind === 'ready' && step === 2 && <ModFoldersStep games={games}/>}
                    {state.kind === 'ready' && step === 3 && <ImportPlaysetsStep games={games} counts={playsetCounts}/>}
                    {state.kind === 'ready' && step === 4 && <PreferencesStep/>}

                    <div className="wizard-bottom">
                        {step > 1 && <span className="wizard-back" onClick={() => setStep(step - 1)}>Back</span>}
                        <div className="spacer"/>
                        {step < 4
                            ? <span className="btn-primary" onClick={() => setStep(step + 1)}>Continue</span>
                            : <span className="btn-primary" onClick={onFinish}>Finish</span>}
                    </div>
                </div>
            </div>
        </div>
    );
}

function GameSwatch({gameKey}: { gameKey: string }) {
    const logo = gameLogos[gameKey];
    if (logo) {
        return <img className="wizard-detected-swatch" src={logo} alt=""/>;
    }
    return <div className="wizard-detected-swatch fallback"/>;
}

function FindGamesStep({games}: { games: library.DetectedGame[] }) {
    const foundCount = games.filter((g) => g.Installed).length;
    return (
        <>
            <div className="wizard-heading">Games found</div>
            <div className="wizard-subheading">
                {foundCount} of {games.length} registered game{games.length === 1 ? '' : 's'} detected in a Steam
                library on this machine.
            </div>
            <div className="wizard-detected-list">
                {games.map((g) => (
                    <div key={g.Key} className={`wizard-detected-row ${g.Installed ? 'found' : ''}`}>
                        <span className={`wizard-checkbox ${g.Installed ? 'checked' : ''}`}/>
                        <GameSwatch gameKey={g.Key}/>
                        <span className="wizard-detected-name">{g.DisplayName}</span>
                        <span className="mono wizard-detected-mods">
                            {g.Installed ? `${g.ModCount} mod${g.ModCount === 1 ? '' : 's'} found` : 'not installed'}
                        </span>
                    </div>
                ))}
            </div>
        </>
    );
}

function ModFoldersStep({games}: { games: library.DetectedGame[] }) {
    return (
        <>
            <div className="wizard-heading">Mod folders</div>
            <div className="wizard-subheading">Where Parallax Mod Manager looks for each game's installed mods.</div>
            <div className="wizard-detected-list">
                {games.map((g) => (
                    <div key={g.Key} className={`wizard-detected-row ${g.Installed ? 'found' : ''}`}>
                        <GameSwatch gameKey={g.Key}/>
                        <div className="wizard-folder-main">
                            <div className="wizard-detected-name">{g.DisplayName}</div>
                            <div className="mono wizard-folder-path">{g.ModFolder}</div>
                        </div>
                    </div>
                ))}
            </div>
        </>
    );
}

function ImportPlaysetsStep({games, counts}: { games: library.DetectedGame[]; counts: Record<string, string[]> }) {
    return (
        <>
            <div className="wizard-heading">Import playsets</div>
            <div className="wizard-subheading">Playsets you've already saved locally for each game.</div>
            <div className="wizard-detected-list">
                {games.map((g) => {
                    const names = counts[g.Key];
                    return (
                        <div key={g.Key} className="wizard-detected-row">
                            <GameSwatch gameKey={g.Key}/>
                            <div className="wizard-folder-main">
                                <div className="wizard-detected-name">{g.DisplayName}</div>
                                <div className="mono wizard-folder-path">
                                    {names === undefined ? 'checking...' : names.length === 0 ? 'no saved playsets yet' : names.join(', ')}
                                </div>
                            </div>
                        </div>
                    );
                })}
            </div>
        </>
    );
}

function PreferencesStep() {
    return (
        <>
            <div className="wizard-heading">Preferences</div>
            <div className="wizard-subheading">
                Preferences aren't built yet - Parallax Mod Manager will use sensible defaults until they are.
            </div>
        </>
    );
}

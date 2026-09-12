import './FirstRunWizard.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {BrowseForAnyGameInstall, BrowseForGameInstall, DetectGames, ListPlaysets} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {APP_NAME, wizardSteps} from '../data/mockData';
import {gameLogos} from '../data/gameLogos';
import {setManagedGames} from '../data/managedGames';

type LoadState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

const FOOTNOTES: Record<number, string> = {
    1: 'Only games you select here show up anywhere else in Parallax Mod Manager.',
    2: "This is where each game's mods actually live on disk.",
    3: "Playsets you've already saved locally are ready to use immediately.",
    4: "Preferences aren't built yet - sensible defaults are used for now.",
};

export function FirstRunWizard({onFinish}: { onFinish: () => void }) {
    const [step, setStep] = useState(1);
    const [state, setState] = useState<LoadState>({kind: 'loading'});
    const [managed, setManaged] = useState<Set<string>>(new Set());
    const [browsing, setBrowsing] = useState<Set<string>>(new Set());
    const [browseErrors, setBrowseErrors] = useState<Record<string, string>>({});
    const [playsetCounts, setPlaysetCounts] = useState<Record<string, string[]>>({});
    const [notice, setNotice] = useState('');
    const [browsingAny, setBrowsingAny] = useState(false);

    useEffect(() => {
        DetectGames()
            .then((games) => {
                setState({kind: 'ready', games});
                setManaged(new Set(games.filter((g) => g.Installed).map((g) => g.Key)));
            })
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
                .then((names) => setPlaysetCounts((prev) => ({...prev, [g.Key]: names ?? []})))
                .catch(() => setPlaysetCounts((prev) => ({...prev, [g.Key]: []})));
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [step, state]);

    function toggleManaged(key: string) {
        const next = new Set(managed);
        if (next.has(key)) next.delete(key); else next.add(key);
        setManaged(next);
    }

    async function browseFor(key: string) {
        setBrowsing((prev) => new Set(prev).add(key));
        setBrowseErrors((prev) => {
            const next = {...prev};
            delete next[key];
            return next;
        });
        try {
            const updated = await BrowseForGameInstall(key);
            if (state.kind === 'ready') {
                setState({kind: 'ready', games: state.games.map((g) => (g.Key === key ? updated : g))});
            }
            if (updated.Installed) {
                setManaged((prev) => new Set(prev).add(key));
            }
        } catch (err) {
            setBrowseErrors((prev) => ({...prev, [key]: String(err)}));
        } finally {
            setBrowsing((prev) => {
                const next = new Set(prev);
                next.delete(key);
                return next;
            });
        }
    }

    async function browseForAny() {
        setBrowsingAny(true);
        setNotice('');
        try {
            const updated = await BrowseForAnyGameInstall();
            if (!updated.Key) {
                return; // user cancelled the dialog
            }
            if (state.kind === 'ready') {
                setState({kind: 'ready', games: state.games.map((g) => (g.Key === updated.Key ? updated : g))});
            }
            if (updated.Installed) {
                setManaged((prev) => new Set(prev).add(updated.Key));
            }
        } catch (err) {
            setNotice(String(err));
        } finally {
            setBrowsingAny(false);
        }
    }

    function finish() {
        setManagedGames(Array.from(managed));
        onFinish();
    }

    function goToStep(next: number) {
        setNotice('');
        setStep(next);
    }

    function continueFromStep1() {
        if (managed.size === 0) {
            setNotice('Select at least one game to continue.');
            return;
        }
        goToStep(2);
    }

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
                    <div className="wizard-footnote">{FOOTNOTES[step]}</div>
                </div>
                <div className="wizard-main">
                    {notice && <div className="wizard-notice">{notice}</div>}
                    {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
                    {state.kind === 'loading' && <p className="status-page">Looking for installed games...</p>}
                    {state.kind === 'ready' && step === 1 && (
                        <FindGamesStep
                            games={games}
                            managed={managed}
                            onToggle={toggleManaged}
                            onBrowse={browseFor}
                            browsing={browsing}
                            browseErrors={browseErrors}
                        />
                    )}
                    {state.kind === 'ready' && step === 2 && <ModFoldersStep games={games}/>}
                    {state.kind === 'ready' && step === 3 && <ImportPlaysetsStep games={games} counts={playsetCounts}/>}
                    {state.kind === 'ready' && step === 4 && <PreferencesStep/>}

                    <div className="wizard-bottom">
                        {step === 1 && (
                            <span className="btn-ghost" onClick={browseForAny}>
                                {browsingAny ? 'Looking...' : 'Browse manually...'}
                            </span>
                        )}
                        <div className="spacer"/>
                        {step > 1 && <span className="wizard-back" onClick={() => goToStep(step - 1)}>Back</span>}
                        {step === 1 && <span className="btn-primary" onClick={continueFromStep1}>Continue</span>}
                        {step > 1 && step < 4 && <span className="btn-primary" onClick={() => goToStep(step + 1)}>Continue</span>}
                        {step === 4 && <span className="btn-primary" onClick={finish}>Finish</span>}
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

function FindGamesStep({games, managed, onToggle, onBrowse, browsing, browseErrors}: {
    games: library.DetectedGame[];
    managed: Set<string>;
    onToggle: (key: string) => void;
    onBrowse: (key: string) => void;
    browsing: Set<string>;
    browseErrors: Record<string, string>;
}) {
    return (
        <>
            <div className="wizard-heading">Games found</div>
            <div className="wizard-subheading">Check the ones you want Parallax Mod Manager to manage.</div>
            <div className="wizard-detected-list">
                {games.map((g) => (
                    <div key={g.Key} className="wizard-detected-item">
                        <div className={`wizard-detected-row ${g.Installed ? 'found' : ''}`}>
                            <span
                                className={`wizard-checkbox ${managed.has(g.Key) ? 'checked' : ''} ${g.Installed ? '' : 'disabled'}`}
                                onClick={() => g.Installed && onToggle(g.Key)}
                            />
                            <GameSwatch gameKey={g.Key}/>
                            <span className="wizard-detected-name">{g.DisplayName}</span>
                            {g.Installed && (
                                <span className="mono wizard-detected-mods">
                                    {g.ModCount} mod{g.ModCount === 1 ? '' : 's'} found
                                </span>
                            )}
                            {g.Installed ? (
                                <i
                                    className={`fa-solid fa-pen-to-square wizard-change-icon ${browsing.has(g.Key) ? 'busy' : ''}`}
                                    title={`Change ${g.DisplayName}'s install folder`}
                                    onClick={() => onBrowse(g.Key)}
                                />
                            ) : (
                                <span className="link-btn wizard-browse-btn" onClick={() => onBrowse(g.Key)}>
                                    {browsing.has(g.Key) ? 'Looking...' : 'Browse...'}
                                </span>
                            )}
                        </div>
                        {browseErrors[g.Key] && <div className="wizard-browse-error">{browseErrors[g.Key]}</div>}
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

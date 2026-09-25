import './FirstRunWizard.css';
import {Fragment, h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {
    BrowseForAnyGameInstall,
    BrowseForGameInstall,
    DetectGames,
    GameMedia,
    GetPreferences,
    ListPlaysets,
    SetPreferences,
} from '../../wailsjs/go/main/App';
import type {library, preferences} from '../../wailsjs/go/models';
import {APP_NAME, wizardSteps} from '../data/mockData';
import {accentTiers, extractAccent, type Accent} from '../data/accentColor';
import {Toggle} from '../components/Toggle';
import {notify} from '../data/notifications';

// Installed games first (so what's actually usable is easy to find), then
// alphabetically within each group - a stable sort keeps the registry's own
// alphabetical order for everything that ties.
function sortGames(games: library.DetectedGame[]): library.DetectedGame[] {
    return [...games].sort((a, b) => Number(b.Installed) - Number(a.Installed));
}

type LoadState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

const FOOTNOTES: Record<number, string> = {
    1: 'Only games you select here show up anywhere else in Parallax Mod Manager.',
    2: "This is where each game's mods actually live on disk.",
    3: "Playsets you've already saved locally are ready to use immediately.",
    4: 'These are ready to use with sensible defaults, and can be changed again any time from Settings.',
};

export function FirstRunWizard({onFinish}: { onFinish: () => void }) {
    const [step, setStep] = useState(1);
    const [state, setState] = useState<LoadState>({kind: 'loading'});
    const [managed, setManaged] = useState<Set<string>>(new Set());
    const [browsing, setBrowsing] = useState<Set<string>>(new Set());
    const [browseErrors, setBrowseErrors] = useState<Record<string, string>>({});
    const [playsetCounts, setPlaysetCounts] = useState<Record<string, string[]>>({});
    const [browsingAny, setBrowsingAny] = useState(false);
    const [logos, setLogos] = useState<Record<string, string>>({});
    const [accents, setAccents] = useState<Record<string, Accent>>({});

    useEffect(() => {
        DetectGames()
            .then((games) => {
                setState({kind: 'ready', games});
                // None pre-checked - managing a game is an explicit choice,
                // even for one that's already installed.
                for (const g of games) {
                    GameMedia('logo', g.ID)
                        .then((src) => {
                            if (!src) return;
                            setLogos((prev) => ({...prev, [g.ID]: src}));
                            extractAccent(src).then((accent) => {
                                if (accent) setAccents((prev) => ({...prev, [g.ID]: accent}));
                            });
                        })
                        .catch(() => undefined);
                }
            })
            .catch((err) => setState({kind: 'error', message: String(err)}));
    }, []);

    useEffect(() => {
        if (step !== 3 || state.kind !== 'ready') {
            return;
        }
        for (const g of state.games) {
            if (g.ID in playsetCounts) {
                continue;
            }
            ListPlaysets(g.ID)
                .then((names) => setPlaysetCounts((prev) => ({...prev, [g.ID]: names ?? []})))
                .catch(() => setPlaysetCounts((prev) => ({...prev, [g.ID]: []})));
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
                setState({kind: 'ready', games: state.games.map((g) => (g.ID === key ? updated : g))});
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
        try {
            const updated = await BrowseForAnyGameInstall();
            if (!updated.ID) {
                return; // user cancelled the dialog
            }
            if (state.kind === 'ready') {
                setState({kind: 'ready', games: state.games.map((g) => (g.ID === updated.ID ? updated : g))});
            }
            if (updated.Installed) {
                setManaged((prev) => new Set(prev).add(updated.ID));
            }
        } catch (err) {
            notify('error', String(err));
        } finally {
            setBrowsingAny(false);
        }
    }

    async function finish() {
        try {
            const current = await GetPreferences();
            await SetPreferences({...current, managedGames: Array.from(managed)});
        } catch {
            // Best effort - app.tsx's own games loader falls back to
            // showing every registered game if this never landed.
        } finally {
            onFinish();
        }
    }

    function goToStep(next: number) {
        setStep(next);
    }

    function continueFromStep1() {
        if (managed.size === 0) {
            notify('warning', 'Select at least one game to continue.');
            return;
        }
        goToStep(2);
    }

    const games = state.kind === 'ready' ? sortGames(state.games) : [];

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
                            logos={logos}
                            accents={accents}
                        />
                    )}
                    {state.kind === 'ready' && step === 2 && <ModFoldersStep games={games} logos={logos} accents={accents}/>}
                    {state.kind === 'ready' && step === 3 && <ImportPlaysetsStep games={games} counts={playsetCounts} logos={logos} accents={accents}/>}
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

function GameSwatch({src}: { src?: string }) {
    if (src) {
        return <img className="wizard-detected-swatch" src={src} alt=""/>;
    }
    return (
        <div className="wizard-detected-swatch fallback">
            <i className="fa-solid fa-gamepad"/>
        </div>
    );
}

function FindGamesStep({games, managed, onToggle, onBrowse, browsing, browseErrors, logos, accents}: {
    games: library.DetectedGame[];
    managed: Set<string>;
    onToggle: (key: string) => void;
    onBrowse: (key: string) => void;
    browsing: Set<string>;
    browseErrors: Record<string, string>;
    logos: Record<string, string>;
    accents: Record<string, Accent>;
}) {
    return (
        <>
            <div className="wizard-heading">Games found</div>
            <div className="wizard-subheading">Check the ones you want Parallax Mod Manager to manage.</div>
            <div className="wizard-detected-list">
                {games.map((g) => {
                    const accent = accents[g.ID];
                    const checked = managed.has(g.ID);
                    const tiers = accent && g.Installed ? accentTiers(accent.color) : null;
                    const tone = tiers ? (checked ? tiers.bright : tiers.base) : null;
                    const rowStyle = tone
                        ? {borderColor: tone, background: `${tone}${checked ? '33' : '1f'}`}
                        : undefined;
                    const checkboxStyle = tone
                        ? {borderColor: tone, background: checked ? tone : undefined}
                        : undefined;
                    return (
                        <div key={g.ID} className="wizard-detected-item">
                            <div className={`wizard-detected-row ${g.Installed ? 'found' : ''}`} style={rowStyle}>
                                <span
                                    className={`wizard-checkbox ${checked ? 'checked' : ''} ${g.Installed ? '' : 'disabled'}`}
                                    style={checkboxStyle}
                                    onClick={() => g.Installed && onToggle(g.ID)}
                                />
                                <GameSwatch src={logos[g.ID]}/>
                                <span className="wizard-detected-name">{g.DisplayName}</span>
                                {g.Installed && (
                                    <span className="mono wizard-detected-mods">
                                        {g.ModCount} mod{g.ModCount === 1 ? '' : 's'} found
                                    </span>
                                )}
                                {g.Installed ? (
                                    <i
                                        className={`fa-solid fa-pen-to-square wizard-change-icon ${browsing.has(g.ID) ? 'busy' : ''}`}
                                        title={`Change ${g.DisplayName}'s install folder`}
                                        onClick={() => onBrowse(g.ID)}
                                    />
                                ) : (
                                    <span className="link-btn wizard-browse-btn" onClick={() => onBrowse(g.ID)}>
                                        {browsing.has(g.ID) ? 'Looking...' : 'Browse...'}
                                    </span>
                                )}
                            </div>
                            {browseErrors[g.ID] && <div className="wizard-browse-error">{browseErrors[g.ID]}</div>}
                        </div>
                    );
                })}
            </div>
        </>
    );
}

function ModFoldersStep({games, logos, accents}: { games: library.DetectedGame[]; logos: Record<string, string>; accents: Record<string, Accent> }) {
    return (
        <>
            <div className="wizard-heading">Mod folders</div>
            <div className="wizard-subheading">Where Parallax Mod Manager looks for each game's installed mods.</div>
            <div className="wizard-detected-list">
                {games.map((g) => {
                    const accent = accents[g.ID];
                    const tiers = accent && g.Installed ? accentTiers(accent.color) : null;
                    const rowStyle = tiers
                        ? {borderColor: tiers.base, background: `${tiers.base}1f`}
                        : undefined;
                    return (
                        <div key={g.ID} className={`wizard-detected-row ${g.Installed ? 'found' : ''}`} style={rowStyle}>
                            <GameSwatch src={logos[g.ID]}/>
                            <div className="wizard-folder-main">
                                <div className="wizard-detected-name">{g.DisplayName}</div>
                                <div className="mono wizard-folder-path">{g.ModFolder}</div>
                            </div>
                        </div>
                    );
                })}
            </div>
        </>
    );
}

function ImportPlaysetsStep({games, counts, logos, accents}: { games: library.DetectedGame[]; counts: Record<string, string[]>; logos: Record<string, string>; accents: Record<string, Accent> }) {
    return (
        <>
            <div className="wizard-heading">Import playsets</div>
            <div className="wizard-subheading">Playsets you've already saved locally for each game.</div>
            <div className="wizard-detected-list">
                {games.map((g) => {
                    const names = counts[g.ID];
                    const accent = accents[g.ID];
                    const tiers = accent && g.Installed ? accentTiers(accent.color) : null;
                    const rowStyle = tiers
                        ? {borderColor: tiers.base, background: `${tiers.base}1f`}
                        : undefined;
                    return (
                        <div key={g.ID} className="wizard-detected-row" style={rowStyle}>
                            <GameSwatch src={logos[g.ID]}/>
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
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    function togglePref(key: 'scanForNewMods' | 'closeAfterLaunch' | 'warnOnPatchMismatch' | 'featureBrowseEnabled' | 'featureEditorEnabled' | 'featureLibraryEnabled') {
        if (!prefs) return;
        const next = {...prefs, [key]: !prefs[key]};
        setPrefs(next);
        SetPreferences(next).catch(() => setPrefs(prefs));
    }

    return (
        <>
            <div className="wizard-heading">Preferences</div>
            <div className="wizard-subheading">
                A few settings to start with - these can be changed again any time from Settings ›
                Manage games.
            </div>
            {prefs && (
                <div className="wizard-toggles">
                    <div className="wizard-toggle-row">
                        <span>Scan for new mods automatically</span>
                        <Toggle on={prefs.scanForNewMods} onClick={() => togglePref('scanForNewMods')}/>
                    </div>
                    <div className="wizard-toggle-row">
                        <span>Warn on patch mismatch</span>
                        <Toggle on={prefs.warnOnPatchMismatch} onClick={() => togglePref('warnOnPatchMismatch')}/>
                    </div>
                    <div className="wizard-toggle-row">
                        <span>Close manager after launch</span>
                        <Toggle on={prefs.closeAfterLaunch} onClick={() => togglePref('closeAfterLaunch')}/>
                    </div>
                </div>
            )}
            <div className="wizard-subheading wizard-subheading-second">
                Which parts of the app you want - each of these can be turned back on any time from
                Settings › Features.
            </div>
            {prefs && (
                <div className="wizard-toggles">
                    <div className="wizard-toggle-row">
                        <span>Library</span>
                        <Toggle on={prefs.featureLibraryEnabled} onClick={() => togglePref('featureLibraryEnabled')}/>
                    </div>
                    <div className="wizard-toggle-row">
                        <span>Editor</span>
                        <Toggle on={prefs.featureEditorEnabled} onClick={() => togglePref('featureEditorEnabled')}/>
                    </div>
                    <div className="wizard-toggle-row">
                        <span>Browse</span>
                        <Toggle on={prefs.featureBrowseEnabled} onClick={() => togglePref('featureBrowseEnabled')}/>
                    </div>
                </div>
            )}
        </>
    );
}

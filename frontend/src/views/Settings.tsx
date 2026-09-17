import './Settings.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DetectGames, BrowseForGameInstall, GetPreferences, SetPreferences} from '../../wailsjs/go/main/App';
import type {library, preferences} from '../../wailsjs/go/models';
import {GameLogo} from '../components/GameLogo';
import {Toggle} from '../components/Toggle';
import {settingsNav} from '../data/mockData';

type Section = 'profiles' | 'sort';

export function Settings({jumpToProfiles}: {
    // Incremented by app.tsx (the TopBar's own "+ Add game") to ask this
    // view to switch to the "Game profiles" panel - 0 (the default,
    // falsy) means no pending request, so this never fights the section
    // the user's own click already put them on.
    jumpToProfiles?: number;
}) {
    const [section, setSection] = useState<Section>('sort');

    useEffect(() => {
        if (jumpToProfiles) setSection('profiles');
    }, [jumpToProfiles]);

    return (
        <div className="settings">
            <div className="settings-nav">
                <div className="sidebar-label">SETTINGS</div>
                {settingsNav.map((s) => {
                    const clickable = s.key === 'profiles' || s.key === 'sort';
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

            {section === 'profiles' && <ProfilesPanel/>}
            {section === 'sort' && <SortRulesPanel/>}
        </div>
    );
}

type ProfilesState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

function ProfilesPanel() {
    const [state, setState] = useState<ProfilesState>({kind: 'loading'});
    const [browsing, setBrowsing] = useState<Set<string>>(new Set());
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

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Game profiles</div>
                <div className="settings-subtitle">Each game keeps its own paths, playsets and sort rules.</div>
            </div>
            {state.kind === 'loading' && <p className="status-page">Checking installed games...</p>}
            {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
            {state.kind === 'ready' && (
                <div className="profile-list">
                    {state.games.map((g) => (
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
                        </div>
                    ))}
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

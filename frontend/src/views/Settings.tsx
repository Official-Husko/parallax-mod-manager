import './Settings.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DetectGames, BrowseForGameInstall} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {GameLogo} from '../components/GameLogo';
import {Toggle} from '../components/Toggle';
import {overrides, profileToggles, settingsNav, sortLog, sortRules} from '../data/mockData';

type Section = 'profiles' | 'sort';

export function Settings() {
    const [section, setSection] = useState<Section>('sort');

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

    useEffect(() => {
        DetectGames()
            .then((games) => setState({kind: 'ready', games}))
            .catch((err) => setState({kind: 'error', message: String(err)}));
    }, []);

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
            <div className="profile-toggles">
                {profileToggles.map((t) => (
                    <div key={t.label} className="profile-toggle-row">
                        <span>{t.label}</span>
                        <Toggle on={t.on}/>
                    </div>
                ))}
            </div>
        </div>
    );
}

function SortRulesPanel() {
    return (
        <div className="settings-content split">
            <div className="settings-main">
                <div>
                    <div className="settings-title">Sort rules</div>
                    <div className="settings-subtitle">
                        Autosort applies these in order, top first. Every move it makes is attributed to
                        the rule responsible, so you can see why a mod ended up where it did.
                    </div>
                </div>
                <div className="sort-rules-list">
                    {sortRules.map((r) => (
                        <div key={r.name} className="sort-rule-row" style={{borderColor: r.border, background: r.bg}}>
                            <i className="fa-solid fa-grip-vertical drag-handle"/>
                            <span className="mono index">{r.i}</span>
                            <div className="sort-rule-main">
                                <div className="sort-rule-name">{r.name}</div>
                                <div className="sort-rule-desc">{r.desc}</div>
                            </div>
                            <span className="mono moves">{r.moves}</span>
                            <Toggle on={r.on}/>
                        </div>
                    ))}
                </div>
                <div className="sort-rule-actions">
                    <span className="btn-ghost">+ Custom rule</span>
                    <span className="btn-ghost">Import community ruleset</span>
                    <span className="btn-ghost inert" style={{border: 'none', background: 'none'}}>Reset to defaults</span>
                </div>
                <div className="overrides-block">
                    <div className="sidebar-label">MANUAL OVERRIDES · {overrides.length}</div>
                    {overrides.map((o) => (
                        <div key={o.text} className="override-row">
                            <span className="mono kind">{o.kind}</span>
                            <span className="text">{o.text}</span>
                            <span className="remove">remove</span>
                        </div>
                    ))}
                </div>
            </div>
            <div className="sort-log-rail">
                <div className="sidebar-label">LAST AUTOSORT · 11:04</div>
                <div className="sort-log-subtitle">17 of 62 mods moved. Each entry names the rule that moved it.</div>
                <div className="sort-log-list">
                    {sortLog.map((l) => (
                        <div key={l.name} className="sort-log-entry" style={{borderColor: l.edge}}>
                            <div className="sort-log-name">{l.name}</div>
                            <div className="mono sort-log-move">{l.move}</div>
                            <div className="sort-log-why">{l.why}</div>
                        </div>
                    ))}
                </div>
                <span className="btn-ghost" style={{textAlign: 'center', justifyContent: 'center'}}>Revert this autosort</span>
            </div>
        </div>
    );
}

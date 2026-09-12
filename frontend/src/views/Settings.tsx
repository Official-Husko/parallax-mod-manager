import './Settings.css';
import {h} from 'preact';
import {useState} from 'preact/hooks';
import {Toggle} from '../components/Toggle';
import {overrides, profileToggles, profiles, settingsNav, sortLog, sortRules} from '../data/mockData';

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

function ProfilesPanel() {
    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Game profiles</div>
                <div className="settings-subtitle">Each game keeps its own paths, playsets and sort rules.</div>
            </div>
            <div className="profile-list">
                {profiles.map((g) => (
                    <div key={g.name} className="profile-row" style={{borderColor: g.border, background: g.bg}}>
                        <div className="profile-swatch" style={{background: g.swatch}}/>
                        <div className="profile-main">
                            <div className="profile-name">{g.name}</div>
                            <div className="mono profile-path">{g.path}</div>
                        </div>
                        <span className="mono profile-state" style={{color: g.stateC}}>{g.state}</span>
                    </div>
                ))}
            </div>
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

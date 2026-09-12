import './Dlc.css';
import {h} from 'preact';
import {Toggle} from '../components/Toggle';
import {dlcPresets, dlcRows, dlcSelected, dlcTypes} from '../data/mockData';

export function Dlc() {
    return (
        <div className="dlc">
            <div className="dlc-sidebar">
                <div className="sidebar-label">TYPE</div>
                {dlcTypes.map((t) => (
                    <div key={t.label} className="dlc-type-row" style={{background: t.active ? '#1e2734' : 'transparent'}}>
                        <span style={{color: t.active ? 'var(--text-bright)' : 'var(--text-mid)'}}>{t.label}</span>
                        <span className="mono n">{t.n}</span>
                    </div>
                ))}
                <div className="sidebar-label" style={{marginTop: 18}}>PRESETS</div>
                {dlcPresets.map((p) => (
                    <div key={p.label} className="dlc-preset-row">
                        <span className="dot" style={{background: p.dot}}/>
                        <span style={{color: p.active ? 'var(--text-bright)' : 'var(--text-mid)'}}>{p.label}</span>
                    </div>
                ))}
                <div className="save-preset-wrap">
                    <span className="btn-ghost">Save current as preset</span>
                </div>
                <div className="sidebar-footer">
                    <div className="mono line">DLC state is stored per profile, alongside the load order.</div>
                </div>
            </div>

            <div className="dlc-main">
                <div className="dlc-toolbar">
                    <div className="search-box">
                        <i className="fa-solid fa-magnifying-glass"/>
                        <input placeholder="Search 29 packs..."/>
                    </div>
                    <span className="chip">Required by mods 6</span>
                    <span className="chip">Not owned 3</span>
                    <div className="spacer"/>
                    <span className="btn-ghost">Enable all owned</span>
                    <span className="btn-ghost">Disable all</span>
                </div>
                <div className="dlc-columns mono">
                    <span className="col-on">ON</span>
                    <span className="col-name">PACK</span>
                    <span className="col-type">TYPE</span>
                    <span className="col-year">RELEASED</span>
                    <span className="col-size">SIZE</span>
                    <span className="col-use">USED BY ACTIVE MODS</span>
                </div>
                <div className="dlc-rows">
                    {dlcRows.map((d) => (
                        <div key={d.name} className="dlc-row" style={{background: d.highlighted ? '#141b25' : 'transparent'}}>
                            <span className="col-on"><Toggle on={d.togBg === '#c4623a'}/></span>
                            <span className="col-name" style={{color: d.nameC}}>{d.name}</span>
                            <span className="col-type">{d.type}</span>
                            <span className="col-year mono">{d.year}</span>
                            <span className="col-size mono">{d.size}</span>
                            <span className="col-use mono" style={{color: d.useC}}>{d.use}</span>
                        </div>
                    ))}
                </div>
                <div className="dlc-footer">
                    <span className="mono">22 of 29 enabled · 3 not owned</span>
                    <span>Toggling a pack changes the multiplayer checksum</span>
                </div>
            </div>

            <div className="dlc-detail">
                <div>
                    <div className="sidebar-label">SELECTED</div>
                    <div className="dlc-detail-name">{dlcSelected.name}</div>
                    <div className="dlc-detail-meta">{dlcSelected.meta}</div>
                    <div className="dlc-detail-desc">{dlcSelected.description}</div>
                </div>
                <div>
                    <div className="sidebar-label">REQUIRED BY · {dlcSelected.requiredBy.length} ACTIVE MODS</div>
                    <div className="required-list">
                        {dlcSelected.requiredBy.map((r) => (
                            <span key={r.name}><span style={{color: r.c}}>●</span> {r.name} - {r.note}</span>
                        ))}
                    </div>
                </div>
                <div className="dlc-warning">
                    <div className="dlc-warning-label">IF YOU DISABLE THIS</div>
                    <div className="dlc-warning-body">{dlcSelected.warning}</div>
                </div>
                <div>
                    <div className="sidebar-label">NOT OWNED · {dlcSelected.notOwned.length}</div>
                    <div className="not-owned-list">
                        {dlcSelected.notOwned.map((n) => <span key={n}>{n}</span>)}
                    </div>
                </div>
                <div className="dlc-detail-bottom">
                    <div className="checksum-row mono">
                        <span>checksum with DLC</span><span style={{color: 'var(--green)'}}>{dlcSelected.checksum}</span>
                    </div>
                    <span className="btn-primary" style={{textAlign: 'center'}}>Apply to profile</span>
                </div>
            </div>
        </div>
    );
}

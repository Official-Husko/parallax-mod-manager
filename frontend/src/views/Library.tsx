import './Library.css';
import {h} from 'preact';
import {libCollections, libGames, libRows} from '../data/mockData';

export function Library() {
    return (
        <div className="library">
            <div className="library-sidebar">
                <div className="sidebar-label">GAMES</div>
                {libGames.map((g) => (
                    <div key={g.name} className="sidebar-game" style={{background: g.active ? '#1e2734' : 'transparent'}}>
                        <span className="swatch" style={{background: g.swatch}}/>
                        <span className="name" style={{color: g.active ? 'var(--text-bright)' : 'var(--text-mid)'}}>{g.name}</span>
                        <span className="mono n">{g.n}</span>
                    </div>
                ))}
                <div className="sidebar-label" style={{marginTop: 18}}>COLLECTIONS</div>
                {libCollections.map((c) => (
                    <div key={c.label} className="sidebar-collection">
                        <span>{c.label}</span><span className="mono n">{c.n}</span>
                    </div>
                ))}
                <div className="sidebar-footer">
                    <div className="mono line">2,440 mods · 61.4 GB on disk</div>
                    <div className="find-unused">Find unused · 380 GB free</div>
                </div>
            </div>
            <div className="library-main">
                <div className="library-toolbar">
                    <div className="search-box">
                        <i className="fa-solid fa-magnifying-glass"/>
                        <input placeholder="Search all games - name, author, tag, file path..."/>
                        <span className="kbd">⌘K</span>
                    </div>
                    <span className="chip">Outdated 47</span>
                    <span className="chip">Never played 612</span>
                    <span className="chip">Broken 3</span>
                    <div className="spacer"/>
                    <span className="sort-label">Table <i className="fa-solid fa-chevron-down"/></span>
                </div>
                <div className="library-columns mono">
                    <span className="col-check"/>
                    <span className="col-src">SRC</span>
                    <span className="col-name">NAME</span>
                    <span className="col-game">GAME</span>
                    <span className="col-ver">VERSION</span>
                    <span className="col-size">SIZE</span>
                    <span className="col-played">LAST PLAYED</span>
                    <span className="col-state">STATE</span>
                </div>
                <div className="library-rows">
                    {libRows.map((r) => (
                        <div key={r.name} className="library-row" style={{background: r.highlighted ? '#141b25' : 'transparent'}}>
                            <span className="col-check"><span className="checkbox"/></span>
                            <span className={`col-src src-badge ${r.src === 'W' ? 'badge-workshop' : 'badge-local'}`}>{r.src}</span>
                            <span className="col-name name">{r.name}</span>
                            <span className="col-game">
                                <span className="dot" style={{background: r.gameC}}/>
                                <span className="game-name">{r.game}</span>
                            </span>
                            <span className="col-ver mono">{r.ver}</span>
                            <span className="col-size mono">{r.size}</span>
                            <span className="col-played">{r.played}</span>
                            <span className="col-state mono" style={{color: r.stateC}}>{r.state}</span>
                        </div>
                    ))}
                </div>
                <div className="library-footer">
                    <span className="mono">2 selected · 1.4 GB</span>
                    <span className="link-btn amber">Add to playset</span>
                    <span className="link-btn" style={{color: 'var(--text-muted)'}}>Update</span>
                    <span className="link-btn" style={{color: 'var(--text-muted)'}}>Move to collection</span>
                    <span className="link-btn" style={{color: 'var(--red)'}}>Uninstall</span>
                </div>
            </div>
        </div>
    );
}

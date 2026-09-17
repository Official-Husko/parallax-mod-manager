import './PlaysetsModal.css';
import {h} from 'preact';
import {colorFromName} from '../data/nameColor';
import type {launcherdb} from '../../wailsjs/go/models';

export function PlaysetsModal({gameName, names, launcherPlaysets, onActivate, onNew, onImport, onClose}: {
    gameName: string;
    names: string[];
    // Real playsets found in the Paradox Launcher's own database
    // (launcher-v2.sqlite), read-only - see Workspace.tsx's own fetch and
    // docs/launcher-database.md. Empty when there's nothing to import (no
    // such file, a JSON-launcher-format game, or the real Launcher has
    // simply never been opened against this install) - the section below
    // only renders when this is non-empty.
    launcherPlaysets: launcherdb.Playset[];
    onActivate: (name: string) => void;
    onNew: () => void;
    onImport: (playset: launcherdb.Playset) => void;
    onClose: () => void;
}) {
    return (
        <div className="overlay" onClick={onClose}>
            <div className="playsets-modal" onClick={(e) => e.stopPropagation()}>
                <div className="playsets-modal-header">
                    <span className="title">Playsets</span>
                    <span className="mono count">{names.length} · {gameName}</span>
                    <div className="spacer"/>
                    <span className="btn-ghost inert">Import code</span>
                    <span className="btn-primary" onClick={onNew}>New</span>
                </div>
                <div className="playsets-modal-body">
                    {names.length === 0 && <div className="playsets-empty">No saved playsets yet - save one from the Workspace.</div>}
                    {names.map((name) => (
                        <div key={name} className="playset-row">
                            <div className="playset-row-edge" style={{background: colorFromName(name)}}/>
                            <div className="playset-row-main">
                                <div className="playset-row-head">
                                    <span className="name">{name}</span>
                                    <span className="mono state">SAVED</span>
                                </div>
                            </div>
                            <div className="playset-row-actions">
                                <span className="btn-ghost inert">Duplicate</span>
                                <span className="btn-ghost inert">Share <i className="fa-solid fa-arrow-up-right-from-square"/></span>
                                <span className="btn-ghost activate" onClick={() => onActivate(name)}>Activate</span>
                            </div>
                        </div>
                    ))}
                    {launcherPlaysets.length > 0 && (
                        <div className="playsets-import-section">
                            <div className="section-label">FROM THE PARADOX LAUNCHER</div>
                            {launcherPlaysets.map((p) => (
                                <div key={p.ID} className="playset-row">
                                    <div className="playset-row-edge" style={{background: colorFromName(p.Name)}}/>
                                    <div className="playset-row-main">
                                        <div className="playset-row-head">
                                            <span className="name">{p.Name}</span>
                                            <span className="mono state">{p.Mods.length} mods{p.IsActive ? ' · ACTIVE' : ''}</span>
                                        </div>
                                    </div>
                                    <div className="playset-row-actions">
                                        <span className="btn-ghost activate" onClick={() => onImport(p)}>Import</span>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                    <div className="playset-join-row">
                        <span>Join a friend's playset:</span>
                        <span className="mono code-input">PLLX-XXXX-XXXX</span>
                        <span className="link-btn amber">Resolve</span>
                    </div>
                </div>
            </div>
        </div>
    );
}

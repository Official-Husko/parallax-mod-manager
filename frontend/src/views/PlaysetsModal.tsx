import './PlaysetsModal.css';
import {h} from 'preact';

export function PlaysetsModal({gameName, names, onActivate, onNew, onClose}: {
    gameName: string;
    names: string[];
    onActivate: (name: string) => void;
    onNew: () => void;
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
                            <div className="playset-row-edge"/>
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

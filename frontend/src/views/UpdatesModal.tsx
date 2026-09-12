import './UpdatesModal.css';
import {h} from 'preact';
import {updates} from '../data/mockData';

export function UpdatesModal({onClose}: { onClose: () => void }) {
    return (
        <div className="overlay" onClick={onClose}>
            <div className="updates-modal" onClick={(e) => e.stopPropagation()}>
                <div className="updates-modal-header">
                    <span className="title">Updates</span>
                    <span className="mono checked">checked 4 min ago</span>
                    <div className="spacer"/>
                    <span className="btn-primary">Update 7 selected</span>
                    <i className="fa-solid fa-xmark close-btn" onClick={onClose}/>
                </div>
                <div className="updates-modal-body">
                    <div className="updates-warning">
                        Game updated to 1.16.4 on 8 Sep. 2 active mods have not been updated since.
                    </div>
                    {updates.map((u) => (
                        <div key={u.name} className="update-row">
                            <span className="checkbox" style={{borderColor: u.boxC, background: u.boxBg}}/>
                            <div className="update-main">
                                <div className="update-name">{u.name}</div>
                                <div className="mono update-note" style={{color: u.noteC}}>{u.note}</div>
                            </div>
                            <span className="mono update-versions">{u.from} &#8594; <span style={{color: 'var(--green)'}}>{u.to}</span></span>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}

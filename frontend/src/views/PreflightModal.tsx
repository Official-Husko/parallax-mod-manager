import './PreflightModal.css';
import {h} from 'preact';
import {preflight} from '../data/mockData';

export function PreflightModal({gameName, modCount, onFixConflicts, onLaunchAnyway, onClose}: {
    gameName: string;
    modCount: number;
    onFixConflicts: () => void;
    onLaunchAnyway: () => void;
    onClose: () => void;
}) {
    return (
        <div className="overlay" onClick={onClose}>
            <div className="preflight-dialog" onClick={(e) => e.stopPropagation()}>
                <div className="preflight-dialog-header">
                    <div className="title">Ready to launch?</div>
                    <div className="mono subtitle">{gameName} · {modCount} mods</div>
                </div>
                <div className="preflight-dialog-body">
                    {preflight.map((p) => (
                        <div key={p.title} className="preflight-row">
                            <i className={`fa-solid ${p.icon}`} style={{color: p.c}}/>
                            <div className="preflight-text">
                                <div className="preflight-title">{p.title}</div>
                                <div className="preflight-detail">{p.detail}</div>
                            </div>
                            {p.action && <span className="link-btn" style={{color: p.actionC}}>{p.action}</span>}
                        </div>
                    ))}
                </div>
                <div className="preflight-dialog-actions">
                    <span className="btn-ghost" onClick={onFixConflicts}>Fix conflicts first</span>
                    <span className="btn-primary" onClick={onLaunchAnyway}>Launch anyway</span>
                </div>
            </div>
        </div>
    );
}

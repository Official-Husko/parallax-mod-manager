import './PreflightModal.css';
import {h} from 'preact';
import type {PreflightItem} from '../data/preflight';

export function PreflightModal({gameName, modCount, items, onFixConflicts, onLaunchAnyway, onClose}: {
    gameName: string;
    modCount: number;
    items: PreflightItem[];
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
                    {items.map((p) => (
                        <div key={p.title} className="preflight-row">
                            <i className={`fa-solid ${p.icon}`} style={{color: p.color}}/>
                            <div className="preflight-text">
                                <div className="preflight-title">{p.title}</div>
                                <div className="preflight-detail">{p.detail}</div>
                            </div>
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

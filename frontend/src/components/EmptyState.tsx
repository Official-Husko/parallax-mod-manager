import './EmptyState.css';
import {h} from 'preact';

// A muted icon-and-text placeholder for a panel that has nothing to show
// yet - an Available/Active list with no mods, or a detail panel with
// nothing selected. Deliberately quiet (dim icon, muted text) rather than
// styled like an error or a call to action: an empty list is a normal,
// expected state here, not a problem.
export function EmptyState({icon, title, subtitle}: {
    icon: string; // a Font Awesome solid glyph name, e.g. "fa-inbox"
    title: string;
    subtitle?: string;
}) {
    return (
        <div className="empty-state">
            <i className={`fa-solid ${icon} empty-state-icon`}/>
            <div className="empty-state-title">{title}</div>
            {subtitle && <div className="empty-state-subtitle">{subtitle}</div>}
        </div>
    );
}

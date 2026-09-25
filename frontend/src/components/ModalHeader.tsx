import {h} from 'preact';
import type {ComponentChildren} from 'preact';

// The header bar shared by every modal (and the Conflict Resolver's own overlay panel, which
// is really the same kind of thing): an optional icon, a title, whatever badges or buttons a
// specific one needs before and after the spacer that pushes the rest to the far right, and
// usually a close button - see .modal-header in App.css for the shared layout this renders
// into. Was eight separate, near-identical hand-rolled headers before this.
export function ModalHeader({title, icon, overlay, onClose, closeTitle, headerClassName, children, after}: {
    title: string | h.JSX.Element;
    // A Font Awesome solid glyph name, shown before the title - most headers have none.
    icon?: string;
    // The two modals with no enclosing panel background above them already (their own box
    // sits right on the dark overlay) - see .modal-header.overlay in App.css.
    overlay?: boolean;
    // Omitted entirely for a header with no close button (Playsets' own is closed only by
    // clicking outside it or picking a playset).
    onClose?: () => void;
    // The close icon's own title attribute, e.g. "Close (Esc)" - most headers set none.
    closeTitle?: string;
    // An extra class for a file's own small residual spacing override (see
    // ConflictResolver.css's own .resolver-header rule).
    headerClassName?: string;
    // Rendered between the title and the spacer - a count, a status badge, anything short.
    children?: ComponentChildren;
    // Rendered between the spacer and the close button - a mode toggle, an action button.
    after?: ComponentChildren;
}) {
    return (
        <div className={`modal-header ${overlay ? 'overlay' : ''} ${headerClassName ?? ''}`}>
            {icon && <i className={`fa-solid ${icon}`}/>}
            <span className="title">{title}</span>
            {children}
            <div className="spacer"/>
            {after}
            {onClose && <i className="fa-solid fa-xmark close-btn" title={closeTitle} onClick={onClose}/>}
        </div>
    );
}

import './Tooltip.css';
import {h} from 'preact';
import type {ComponentChild} from 'preact';
import {useLayoutEffect, useRef} from 'preact/hooks';
import {hideTooltip, useTooltip} from '../data/tooltip';

const MARGIN = 8;
const GAP = 6;

// One instance lives in app.tsx next to <ContextMenu/> - see data/tooltip.ts.
// Placed under the hovered element (above it when there's no room below),
// centered on it and clamped inside the window. Never takes the pointer, so
// it can't steal the hover it belongs to.
export function Tooltip() {
    const tooltip = useTooltip();
    const ref = useRef<HTMLDivElement>(null);

    // Measure, then place, before the browser paints - the tooltip is
    // first rendered hidden at 0,0 purely so its real size is known.
    useLayoutEffect(() => {
        const box = ref.current;
        if (!tooltip || !box) return;
        const anchor = tooltip.el.getBoundingClientRect();
        const {width, height} = box.getBoundingClientRect();
        let left = anchor.left + anchor.width / 2 - width / 2;
        left = Math.min(Math.max(MARGIN, left), Math.max(MARGIN, window.innerWidth - width - MARGIN));
        let top = anchor.bottom + GAP;
        if (top + height > window.innerHeight - MARGIN) top = anchor.top - height - GAP;
        top = Math.max(MARGIN, top);
        box.style.left = `${left}px`;
        box.style.top = `${top}px`;
        box.style.visibility = 'visible';
    }, [tooltip]);

    // Whatever the tooltip was opened on can move or go away without a
    // mouseleave ever firing (a click, a scroll, a re-rendered list), so
    // dismiss on those too, and whenever the pointer is over something
    // that isn't the element the tooltip belongs to.
    useLayoutEffect(() => {
        if (!tooltip) return;
        const onOver = (e: MouseEvent) => {
            if (!tooltip.el.isConnected || !tooltip.el.contains(e.target as Node)) hideTooltip();
        };
        document.addEventListener('mouseover', onOver);
        document.addEventListener('mousedown', hideTooltip);
        document.addEventListener('keydown', hideTooltip);
        window.addEventListener('scroll', hideTooltip, true);
        window.addEventListener('blur', hideTooltip);
        window.addEventListener('resize', hideTooltip);
        return () => {
            document.removeEventListener('mouseover', onOver);
            document.removeEventListener('mousedown', hideTooltip);
            document.removeEventListener('keydown', hideTooltip);
            window.removeEventListener('scroll', hideTooltip, true);
            window.removeEventListener('blur', hideTooltip);
            window.removeEventListener('resize', hideTooltip);
        };
    }, [tooltip]);

    if (!tooltip) return null;
    return (
        <div className="tip" key={tooltip.id} ref={ref} style={{left: 0, top: 0, visibility: 'hidden'}}>
            {tooltip.content}
        </div>
    );
}

// TipItem is one entry in a tooltip's list, drawn the way a flag or badge
// looks in the list itself: its own colored icon (or any leading visual,
// e.g. a domain segment) on a faint wash of that same color, then a title
// and an optional explanation. `color` is any CSS color, normally a
// --flag-* token, so a tooltip never has its own idea of a flag's hue.
export function TipItem({icon, lead, color, title, children}: {
    icon?: string;
    lead?: ComponentChild;
    color: string;
    title: ComponentChild;
    children?: ComponentChild;
}) {
    return (
        <div className="tip-item" style={`--tip-color: ${color}`}>
            <span className="tip-item-lead">
                {lead ?? <i className={`fa-solid ${icon}`}/>}
            </span>
            <div className="tip-item-body">
                <div className="tip-item-title">{title}</div>
                {children != null && <div className="tip-item-detail">{children}</div>}
            </div>
        </div>
    );
}

// TipHeading is the small caps line above a tooltip's list.
export function TipHeading({children}: {children: ComponentChild}) {
    return <div className="tip-heading">{children}</div>;
}

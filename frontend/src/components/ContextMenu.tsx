import './ContextMenu.css';
import {h} from 'preact';
import {useLayoutEffect, useRef, useState} from 'preact/hooks';
import {closeContextMenu, useContextMenu} from '../data/contextMenu';

interface Placement {
    left: number;
    top: number;
    // First rendered invisibly, flush against the real cursor position,
    // purely to measure the menu's own real size (item count/labels vary
    // per call, so there's no fixed size to precompute) - the very next
    // layout effect corrects the position before the browser ever paints
    // it, so this never causes a visible flash at the wrong spot.
    measuring: boolean;
}

// One instance lives in app.tsx, above everything else - this app's own
// replacement for the webview's native right-click menu (see app.tsx's
// global 'contextmenu' suppression, which covers everywhere that doesn't
// open one of these). Closes on an outside click, Escape, scroll, or the
// window losing focus - the same dismissal behavior a native context menu
// itself has.
export function ContextMenu() {
    const menu = useContextMenu();
    const ref = useRef<HTMLDivElement>(null);
    const [placement, setPlacement] = useState<Placement | null>(null);

    useLayoutEffect(() => {
        if (!menu) {
            setPlacement(null);
            return;
        }
        setPlacement({left: menu.x, top: menu.y, measuring: true});
    }, [menu]);

    useLayoutEffect(() => {
        if (!menu || !placement?.measuring || !ref.current) return;
        const rect = ref.current.getBoundingClientRect();
        const margin = 6;
        let left = menu.x;
        let top = menu.y;
        if (left + rect.width > window.innerWidth - margin) left = Math.max(margin, window.innerWidth - rect.width - margin);
        if (top + rect.height > window.innerHeight - margin) top = Math.max(margin, window.innerHeight - rect.height - margin);
        setPlacement({left, top, measuring: false});
    }, [menu, placement]);

    useLayoutEffect(() => {
        if (!menu) return;
        const onPointerDown = (e: MouseEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) closeContextMenu();
        };
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') closeContextMenu();
        };
        // Any of these means the menu's position no longer makes sense
        // relative to whatever it was opened on - close rather than let
        // it drift or point at the wrong row.
        document.addEventListener('mousedown', onPointerDown);
        document.addEventListener('keydown', onKeyDown);
        window.addEventListener('scroll', closeContextMenu, true);
        window.addEventListener('blur', closeContextMenu);
        window.addEventListener('resize', closeContextMenu);
        return () => {
            document.removeEventListener('mousedown', onPointerDown);
            document.removeEventListener('keydown', onKeyDown);
            window.removeEventListener('scroll', closeContextMenu, true);
            window.removeEventListener('blur', closeContextMenu);
            window.removeEventListener('resize', closeContextMenu);
        };
    }, [menu]);

    if (!menu || !placement) {
        return null;
    }
    return (
        <div
            className="context-menu"
            ref={ref}
            style={{left: `${placement.left}px`, top: `${placement.top}px`, visibility: placement.measuring ? 'hidden' : 'visible'}}
        >
            {menu.items.map((item, i) => (
                <span
                    key={i}
                    className={`context-menu-item ${item.danger ? 'danger' : ''} ${item.disabled ? 'disabled' : ''} ${item.separatorBefore ? 'separator-before' : ''}`}
                    onClick={() => {
                        if (item.disabled) return;
                        closeContextMenu();
                        item.onClick();
                    }}
                >
                    {item.label}
                </span>
            ))}
        </div>
    );
}

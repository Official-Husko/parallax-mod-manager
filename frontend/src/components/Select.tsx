import './Select.css';
import {h} from 'preact';
import {useEffect, useLayoutEffect, useRef, useState} from 'preact/hooks';

export interface SelectOption {
    value: string;
    label: string;
    // A short secondary figure shown at the option's right edge, e.g. a size.
    hint?: string;
}

// The app's own dropdown, used wherever a native <select> would be: the
// webview draws a native select's popup with the system theme (a white box on
// this dark UI) and it can't be styled, so the menu is drawn here instead.
// Behaves like the native one - click or Space/Enter/arrows to open, arrows and
// Enter to choose, Escape or a click elsewhere to close - and flips upward when
// there is no room below.
export function Select({value, options, onChange, disabled, placeholder, className, title}: {
    value: string;
    options: SelectOption[];
    onChange: (value: string) => void;
    disabled?: boolean;
    // Shown in the trigger when `value` matches no option (e.g. an empty list).
    placeholder?: string;
    // Sizing hooks for the caller (max-width and the like) - applied to the root.
    className?: string;
    title?: string;
}) {
    const [open, setOpen] = useState(false);
    const [active, setActive] = useState(0);
    const root = useRef<HTMLDivElement>(null);
    const menu = useRef<HTMLDivElement>(null);
    const selectedIndex = options.findIndex((o) => o.value === value);
    const current = options[selectedIndex];

    function openMenu() {
        if (disabled || options.length === 0) return;
        setActive(Math.max(0, selectedIndex));
        setOpen(true);
    }

    function choose(index: number) {
        setOpen(false);
        const option = options[index];
        if (option && option.value !== value) onChange(option.value);
    }

    // Open upward when the menu would run off the bottom of the window and
    // there is more room above. Measured before paint, so it never flashes.
    useLayoutEffect(() => {
        const box = menu.current;
        const anchor = root.current;
        if (!open || !box || !anchor) return;
        const rect = anchor.getBoundingClientRect();
        const below = window.innerHeight - rect.bottom;
        box.classList.toggle('up', box.offsetHeight + 8 > below && rect.top > below);
    }, [open]);

    // Keep the highlighted option in view while arrowing through a long list.
    useEffect(() => {
        if (open) (menu.current?.children[active] as HTMLElement | undefined)?.scrollIntoView({block: 'nearest'});
    }, [open, active]);

    useEffect(() => {
        if (!open) return;
        const close = () => setOpen(false);
        const onDown = (e: MouseEvent) => {
            if (root.current && !root.current.contains(e.target as Node)) close();
        };
        document.addEventListener('mousedown', onDown);
        window.addEventListener('blur', close);
        window.addEventListener('resize', close);
        return () => {
            document.removeEventListener('mousedown', onDown);
            window.removeEventListener('blur', close);
            window.removeEventListener('resize', close);
        };
    }, [open]);

    function onKeyDown(e: KeyboardEvent) {
        if (disabled) return;
        if (!open) {
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                openMenu();
            }
            return;
        }
        if (e.key === 'Escape') {
            // Only closes the menu - not also whatever dialog the select sits in.
            e.preventDefault();
            e.stopPropagation();
            setOpen(false);
        } else if (e.key === 'ArrowDown') {
            e.preventDefault();
            setActive((i) => Math.min(options.length - 1, i + 1));
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            setActive((i) => Math.max(0, i - 1));
        } else if (e.key === 'Home') {
            e.preventDefault();
            setActive(0);
        } else if (e.key === 'End') {
            e.preventDefault();
            setActive(options.length - 1);
        } else if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            choose(active);
        } else if (e.key === 'Tab') {
            setOpen(false);
        }
    }

    return (
        <div className={`select ${open ? 'open' : ''} ${disabled ? 'disabled' : ''} ${className ?? ''}`} ref={root}>
            <button
                type="button"
                className="select-trigger"
                title={title}
                disabled={disabled}
                aria-haspopup="listbox"
                aria-expanded={open}
                onClick={() => (open ? setOpen(false) : openMenu())}
                onKeyDown={onKeyDown}
            >
                <span className="select-label">{current?.label ?? placeholder ?? ''}</span>
                {current?.hint && <span className="select-option-hint select-trigger-hint">{current.hint}</span>}
                <i className="fa-solid fa-chevron-down select-chevron"/>
            </button>
            {open && (
                <div className="select-menu" role="listbox" ref={menu}>
                    {options.map((o, i) => (
                        <div
                            key={o.value}
                            role="option"
                            aria-selected={o.value === value}
                            className={`select-option ${o.value === value ? 'selected' : ''} ${i === active ? 'active' : ''}`}
                            onMouseEnter={() => setActive(i)}
                            onClick={() => choose(i)}
                        >
                            <span className="select-option-label">{o.label}</span>
                            {o.hint && <span className="select-option-hint">{o.hint}</span>}
                            {o.value === value && <i className="fa-solid fa-check select-option-check"/>}
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

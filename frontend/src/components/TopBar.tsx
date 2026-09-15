import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {APP_NAME} from '../data/mockData';

export type ViewKey = 'library' | 'workspace' | 'dlc' | 'settings';

const NAV_ITEMS: { key: ViewKey; label: string }[] = [
    {key: 'library', label: 'Library'},
    {key: 'workspace', label: 'Workspace'},
    {key: 'dlc', label: 'DLC'},
    {key: 'settings', label: 'Settings'},
];

export function TopBar({view, onNavigate, gamePicker}: {
    view: ViewKey;
    onNavigate: (v: ViewKey) => void;
    gamePicker?: {
        gameLabel: string;
        // Optional - a view with no notion of "the active playset" (e.g.
        // DLC, which edits a saved playset by name from its own picker
        // instead) simply omits this rather than showing a fake one.
        playsetLabel?: string;
        // Opens the real playset switcher (Workspace's own PlaysetsModal -
        // this component has no idea what that even is, just a callback
        // to trigger it) - present exactly when playsetLabel is, so the
        // pill is only ever clickable where there's a real playset
        // concept to switch.
        onOpenPlaysetSwitcher?: () => void;
        games?: { ID: string; DisplayName: string }[];
        onSelectGame?: (id: string) => void;
    };
}) {
    const [open, setOpen] = useState(false);
    const boxRef = useRef<HTMLDivElement>(null);
    const switchable = !!gamePicker?.games && !!gamePicker.onSelectGame && gamePicker.games.length > 1;

    useEffect(() => {
        if (!open) return;
        const onClickAway = (e: MouseEvent) => {
            if (boxRef.current && !boxRef.current.contains(e.target as Node)) {
                setOpen(false);
            }
        };
        document.addEventListener('mousedown', onClickAway);
        return () => document.removeEventListener('mousedown', onClickAway);
    }, [open]);

    return (
        <div className="topbar">
            <div className="topbar-brand">
                <img className="topbar-icon" src="/favicon.png" alt=""/>
                <span className="topbar-name">{APP_NAME}</span>
            </div>
            {gamePicker && (
                <>
                    <div className="topbar-game-pill-wrap" ref={boxRef}>
                        <div
                            className={`topbar-game-pill ${switchable ? 'switchable' : ''}`}
                            onClick={() => switchable && setOpen((v) => !v)}
                        >
                            <span className="game-name">{gamePicker.gameLabel}</span>
                            {switchable && <i className="fa-solid fa-chevron-down"/>}
                        </div>
                        {open && switchable && (
                            <div className="topbar-game-dropdown">
                                {gamePicker.games!.map((g) => (
                                    <div
                                        key={g.ID}
                                        className={`topbar-game-option ${g.DisplayName === gamePicker.gameLabel ? 'active' : ''}`}
                                        onClick={() => {
                                            gamePicker.onSelectGame!(g.ID);
                                            setOpen(false);
                                        }}
                                    >
                                        {g.DisplayName}
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                    {gamePicker.playsetLabel && (
                        <div
                            className={`topbar-playset ${gamePicker.onOpenPlaysetSwitcher ? 'clickable' : ''}`}
                            onClick={gamePicker.onOpenPlaysetSwitcher}
                            title={gamePicker.onOpenPlaysetSwitcher ? 'Switch playset' : undefined}
                        >
                            <span className="playset-label">Playset</span>
                            <span className="playset-name">{gamePicker.playsetLabel}</span>
                            {gamePicker.onOpenPlaysetSwitcher && <i className="fa-solid fa-chevron-down"/>}
                        </div>
                    )}
                </>
            )}
            <div className="topbar-spacer"/>
            <nav className="topbar-nav">
                {NAV_ITEMS.map((item) => (
                    <span
                        key={item.key}
                        className={`topbar-nav-item ${view === item.key ? 'active' : ''}`}
                        onClick={() => onNavigate(item.key)}
                    >
                        {item.label}
                    </span>
                ))}
            </nav>
        </div>
    );
}

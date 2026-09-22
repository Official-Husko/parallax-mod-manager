import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {APP_NAME} from '../data/mockData';
import {colorFromName} from '../data/nameColor';
import {displayVersion} from '../data/versionCompat';

export type ViewKey = 'library' | 'workspace' | 'dlc' | 'editor' | 'settings';

const NAV_ITEMS: { key: ViewKey; label: string }[] = [
    {key: 'library', label: 'Library'},
    {key: 'workspace', label: 'Workspace'},
    {key: 'dlc', label: 'DLC'},
    {key: 'editor', label: 'Editor'},
    {key: 'settings', label: 'Settings'},
];

export function TopBar({view, onNavigate, gamePicker}: {
    view: ViewKey;
    onNavigate: (v: ViewKey) => void;
    gamePicker?: {
        gameLabel: string;
        // The real, currently-installed game version (e.g. "v4.4.6") -
        // see app.tsx's own GameVersion() fetch. Empty when it couldn't be
        // determined (not installed, or its launcher-settings.json simply
        // doesn't carry one) - shown as nothing at all in that case, never
        // a placeholder or an error.
        gameVersion?: string;
        // Every managed game's own version, keyed by ID - lets the
        // dropdown below show each game's real version next to its name,
        // not just the currently-selected one's.
        gameVersions?: Record<string, string>;
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
        // Navigates to Settings' "Manage games" panel - present exactly
        // when there's a real place for it to go. Shown as its own row in
        // the dropdown below, always available there (even with only one
        // or zero managed games) rather than only once there's already
        // more than one to switch between.
        onManageGames?: () => void;
    };
}) {
    const [open, setOpen] = useState(false);
    const boxRef = useRef<HTMLDivElement>(null);
    // The pill opens a dropdown as soon as there's anything real to do in
    // one - switch games (once there's a real list to switch within) or
    // manage which ones show up here at all - not gated on already
    // managing more than one game, so "Manage games" stays reachable from
    // a fresh install managing just one (or zero) games.
    const hasGameMenu = (!!gamePicker?.games && !!gamePicker.onSelectGame) || !!gamePicker?.onManageGames;

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
                            className={`topbar-game-pill ${hasGameMenu ? 'switchable' : ''}`}
                            onClick={() => hasGameMenu && setOpen((v) => !v)}
                        >
                            <span className="game-name">{gamePicker.gameLabel}</span>
                            {(gamePicker.gameVersion || hasGameMenu) && (
                                <span className="game-version-chevron">
                                    {gamePicker.gameVersion && <span className="game-version mono">{displayVersion(gamePicker.gameVersion)}</span>}
                                    {hasGameMenu && <i className="fa-solid fa-chevron-down"/>}
                                </span>
                            )}
                        </div>
                        {open && hasGameMenu && (
                            <div className="topbar-game-dropdown">
                                {gamePicker.games?.map((g) => {
                                    const version = gamePicker.gameVersions?.[g.ID];
                                    return (
                                        <div
                                            key={g.ID}
                                            className={`topbar-game-option ${g.DisplayName === gamePicker.gameLabel ? 'active' : ''}`}
                                            onClick={() => {
                                                gamePicker.onSelectGame!(g.ID);
                                                setOpen(false);
                                            }}
                                        >
                                            <span className="option-name">{g.DisplayName}</span>
                                            {version && <span className="option-version mono">{displayVersion(version)}</span>}
                                        </div>
                                    );
                                })}
                                {gamePicker.onManageGames && (
                                    <div
                                        className="topbar-game-option add"
                                        onClick={() => {
                                            gamePicker.onManageGames!();
                                            setOpen(false);
                                        }}
                                    >
                                        <i className="fa-solid fa-gear"/> Manage games
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                    {gamePicker.playsetLabel && (() => {
                        // "(unsaved)" is a real placeholder (app.tsx's own
                        // sentinel for "no playset chosen/named yet"), not
                        // an actual playset name - deriving a color from
                        // that literal string would be meaningless, so it
                        // keeps the plain default color instead.
                        const hasRealName = gamePicker.playsetLabel !== '(unsaved)';
                        const color = hasRealName ? colorFromName(gamePicker.playsetLabel) : undefined;
                        return (
                            <div
                                className={`topbar-playset ${gamePicker.onOpenPlaysetSwitcher ? 'clickable' : ''}`}
                                onClick={gamePicker.onOpenPlaysetSwitcher}
                                title={gamePicker.onOpenPlaysetSwitcher ? 'Switch playset' : undefined}
                            >
                                <span className="playset-label">Playset</span>
                                <span className="playset-name" style={color ? {color, borderBottomColor: color} : undefined}>
                                    {gamePicker.playsetLabel}
                                </span>
                                {gamePicker.onOpenPlaysetSwitcher && <i className="fa-solid fa-chevron-down"/>}
                            </div>
                        );
                    })()}
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

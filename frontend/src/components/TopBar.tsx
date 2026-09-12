import {h} from 'preact';
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
    gamePicker?: { gameLabel: string; profileLabel: string };
}) {
    return (
        <div className="topbar">
            <div className="topbar-dots">
                <span/><span/><span/>
            </div>
            <div className="topbar-divider"/>
            <div className="topbar-brand">
                <span className="topbar-icon"/>
                <span className="topbar-name">{APP_NAME}</span>
            </div>
            {gamePicker && (
                <>
                    <div className="topbar-game-pill">
                        <span className="game-name">{gamePicker.gameLabel}</span>
                        <i className="fa-solid fa-chevron-down"/>
                    </div>
                    <div className="topbar-profile">
                        <span className="profile-label">Profile</span>
                        <span className="profile-name">{gamePicker.profileLabel}</span>
                    </div>
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

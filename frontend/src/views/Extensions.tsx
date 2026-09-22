import './Extensions.css';
import {Fragment, h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {ClearLoversLabCredentials, LoversLabStatus, SaveLoversLabCredentials} from '../../wailsjs/go/main/App';
import type {library, main} from '../../wailsjs/go/models';
import {notify} from '../data/notifications';

// Browsing Extensions: additional, unofficial places to find mods beyond the Steam
// Workshop (already covered by Library/Workspace). LoversLab is the first placeholder
// source - see internal/credentials and loverslab_settings.go for the one real part of
// this page: signing in is genuine, encrypted the same way the Steam Web API key
// already is (Settings > Steam API), and saved for whenever browsing itself is built.
// The card grid below is example content only, clearly marked - nothing on it is a
// real LoversLab listing yet.

type ExampleMod = {
    id: string;
    title: string;
    author: string;
    category: string;
    updated: string;
    downloads: string;
    hue: number; // a placeholder thumbnail's own gradient hue - not a real preview image
};

const EXAMPLE_MODS: ExampleMod[] = [
    {id: '1', title: 'Ethos Overhaul', author: 'Kalyska', category: 'Species Pack', updated: '2 days ago', downloads: '18.2k', hue: 210},
    {id: '2', title: 'Expanded Narrative Events', author: 'Dunmore', category: 'Events', updated: '1 week ago', downloads: '9.4k', hue: 280},
    {id: '3', title: "Traveler's Wardrobe", author: 'Sylvaine', category: 'Portraits', updated: '3 weeks ago', downloads: '31.7k', hue: 340},
    {id: '4', title: 'Frontier Outposts', author: 'RustAndRuin', category: 'Buildings', updated: '5 days ago', downloads: '6.1k', hue: 25},
    {id: '5', title: 'Court Intrigue Continued', author: 'Marrow', category: 'Gameplay', updated: '2 weeks ago', downloads: '14.9k', hue: 150},
    {id: '6', title: 'Longer Nights Anthology', author: 'Hollowmere', category: 'Events', updated: '4 days ago', downloads: '22.3k', hue: 260},
];

export function Extensions({games, selectedGame}: {
    games: library.GameInfo[];
    selectedGame: string;
}) {
    const [status, setStatus] = useState<main.LoversLabStatus | null>(null);
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [busy, setBusy] = useState<'save' | 'clear' | null>(null);
    const [error, setError] = useState('');

    useEffect(() => {
        LoversLabStatus().then(setStatus).catch(() => undefined);
    }, []);

    async function save() {
        setBusy('save');
        setError('');
        try {
            const s = await SaveLoversLabCredentials(username, password);
            setStatus(s);
            setUsername('');
            setPassword('');
            notify('success', 'LoversLab sign-in saved (encrypted on this computer).');
        } catch (err) {
            setError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            setBusy(null);
        }
    }

    async function clearSignIn() {
        setBusy('clear');
        setError('');
        try {
            const s = await ClearLoversLabCredentials();
            setStatus(s);
            notify('info', 'LoversLab sign-in removed.');
        } catch (err) {
            setError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            setBusy(null);
        }
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;
    const canSave = !busy && username.trim() !== '' && password !== '';

    return (
        <div className="extensions">
            <div className="ext-sources">
                <div className="sidebar-label">SOURCES</div>
                <div className="ext-source-row active">
                    <i className="fa-solid fa-heart ext-source-icon loverslab"/>
                    <span>LoversLab</span>
                </div>
                <div className="ext-source-row disabled">
                    <i className="fa-solid fa-plus ext-source-icon"/>
                    <span>More later</span>
                </div>
                <div className="sidebar-footer">
                    <div className="mono line">
                        Unofficial places to find {gameName} mods, outside the Steam Workshop.
                    </div>
                </div>
            </div>

            <div className="extensions-main">
                <div className="ext-soon-banner">
                    <i className="fa-solid fa-flask"/>
                    <div>
                        <div className="ext-soon-title">Browsing LoversLab <span className="chip">Coming later</span></div>
                        <div>
                            Signing in below is real, and saved encrypted the same way a Steam Web API key
                            is (Settings &gt; Steam API) - browsing itself isn't built yet, so every card
                            below is an example, not a real listing.
                        </div>
                    </div>
                </div>

                <div className="ext-toolbar">
                    <div className="search-box">
                        <i className="fa-solid fa-magnifying-glass"/>
                        <input placeholder="Search LoversLab (example only)..." disabled/>
                    </div>
                </div>

                <div className="ext-grid">
                    {EXAMPLE_MODS.map((m) => (
                        <div key={m.id} className="ext-card">
                            <div
                                className="ext-card-thumb"
                                style={{backgroundImage: `linear-gradient(135deg, hsl(${m.hue} 45% 22%), hsl(${m.hue} 35% 12%))`}}
                            />
                            <div className="ext-card-body">
                                <div className="ext-card-title" title={m.title}>{m.title}</div>
                                <div className="ext-card-meta">by {m.author} · {m.category}</div>
                                <div className="ext-card-stats">
                                    <span><i className="fa-solid fa-download"/>{m.downloads}</span>
                                    <span><i className="fa-solid fa-clock"/>{m.updated}</span>
                                </div>
                            </div>
                        </div>
                    ))}
                </div>
                <div className="ext-example-tag"><i className="fa-solid fa-flask"/> Example cards, not real LoversLab content</div>
            </div>

            <div className="ext-account">
                <div className="sidebar-label">LOVERSLAB SIGN-IN</div>
                {!status ? (
                    <p className="status-page">Loading...</p>
                ) : (
                    <>
                        <div className={`ext-account-status ${status.SignedIn ? 'good' : status.Unreadable ? 'warn' : 'neutral'}`}>
                            <i className={`fa-solid ${status.SignedIn ? 'fa-circle-check' : status.Unreadable ? 'fa-triangle-exclamation' : 'fa-user'}`}/>
                            <span>
                                {status.SignedIn
                                    ? `Signed in as ${status.Username}.`
                                    : status.Unreadable
                                        ? 'A sign-in is saved but cannot be read on this computer - sign in again.'
                                        : 'Not signed in yet.'}
                            </span>
                        </div>

                        <label className="ext-account-field">
                            <span className="ext-account-label">Username or email</span>
                            <input
                                className="ext-account-input"
                                value={username}
                                disabled={busy !== null}
                                placeholder={status.SignedIn ? status.Username : 'Your LoversLab username or email'}
                                onInput={(e) => setUsername((e.target as HTMLInputElement).value)}
                            />
                        </label>
                        <label className="ext-account-field">
                            <span className="ext-account-label">Password</span>
                            <input
                                type="password"
                                autocomplete="off"
                                spellcheck={false}
                                className="ext-account-input"
                                value={password}
                                disabled={busy !== null}
                                placeholder={status.SignedIn ? 'Enter a new password to replace it' : 'Your LoversLab password'}
                                onInput={(e) => setPassword((e.target as HTMLInputElement).value)}
                                onKeyDown={(e) => e.key === 'Enter' && canSave && save()}
                            />
                        </label>

                        {error && <div className="ext-account-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}

                        <div className="ext-account-actions">
                            <button type="button" className="btn-primary" disabled={!canSave} onClick={save}>
                                {busy === 'save' ? 'Saving...' : status.SignedIn ? 'Update sign-in' : 'Sign in'}
                            </button>
                            {status.SignedIn && (
                                <button type="button" className="btn-ghost" disabled={busy !== null} onClick={clearSignIn}>
                                    {busy === 'clear' ? 'Removing...' : 'Sign out'}
                                </button>
                            )}
                        </div>

                        <div className="ext-account-note">
                            <i className="fa-solid fa-lock"/> {status.Protection}
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}

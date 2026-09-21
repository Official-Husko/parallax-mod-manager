import './SteamApiPanel.css';
import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {CheckSteamAPIKey, SaveSteamAPIKey, SetSteamAPIMode, SteamAPIStatus} from '../../wailsjs/go/main/App';
import type {main} from '../../wailsjs/go/models';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import {notify} from '../data/notifications';

type Mode = 'complete' | 'backup' | 'free';

const KEY_PAGE = 'https://steamcommunity.com/dev/apikey';

// How often the status is re-read while the panel is open: a key can run out of
// requests, or come back, while Workshop details are being fetched in the background.
const STATUS_REFRESH_MS = 5000;

const MODES: { mode: Mode; name: string; recommended?: boolean; desc: string }[] = [
    {
        mode: 'complete',
        name: 'Complete Steam API use',
        recommended: true,
        desc: 'Uses your key for every Workshop details request. If the key runs out of requests, the app '
            + 'goes back to the free API until Steam lets the key through again.',
    },
    {
        mode: 'backup',
        name: 'Backup Steam API use',
        desc: 'Uses the free API, and your key only when the free API fails or cannot return a mod - for '
            + 'example an unlisted mod such as 2780180614, which the free API reports as not found.',
    },
    {
        mode: 'free',
        name: 'Free API use only',
        desc: 'Never uses a key, only Steam\'s free access, which works for most mods. Choosing this deletes '
            + 'the saved key from the settings file and locks the key field.',
    },
];

function timeOf(unixSeconds: number): string {
    return new Date(unixSeconds * 1000).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
}

// statusLine says what the app is doing right now, in one sentence, with the icon and
// tone that go with it.
function statusLine(s: main.SteamAPIStatus, draft: Mode | null): { icon: string; tone: string; text: string } {
    switch (s.State) {
        case 'active':
            return {icon: 'fa-circle-check', tone: 'good', text: `Using your Steam API key (${s.Fingerprint}) - ${s.Mode === 'complete' ? 'for every request' : 'when the free API cannot answer'}.`};
        case 'exhausted':
            return {icon: 'fa-hourglass-half', tone: 'warn', text: `The key is out of requests for now - using the free API until about ${timeOf(s.ExhaustedUntil)}.`};
        case 'rejected':
            return {icon: 'fa-circle-xmark', tone: 'bad', text: 'Steam rejected the saved key - using the free API. Enter a new key, or choose Free API use only.'};
        case 'unreadable':
            return {icon: 'fa-triangle-exclamation', tone: 'warn', text: 'A key is saved but cannot be read on this computer (the file came from another one, or was changed) - using the free API. Enter the key again.'};
        default:
            if (draft && draft !== 'free' && !s.HasKey) {
                return {icon: 'fa-key', tone: 'warn', text: 'Enter a key below to start using it - until then the free API is used.'};
            }
            return {icon: 'fa-cloud', tone: 'neutral', text: 'Using the free API.'};
    }
}

export function SteamApiPanel() {
    const [status, setStatus] = useState<main.SteamAPIStatus | null>(null);
    // A mode chosen before there is a key to use it with. Nothing is saved until the key
    // is: the app keeps using the free API meanwhile.
    const [draft, setDraft] = useState<Mode | null>(null);
    const [key, setKey] = useState('');
    const [busy, setBusy] = useState<'save' | 'check' | 'mode' | null>(null);
    const [error, setError] = useState('');
    // Choosing Free while a key is saved asks first, since it deletes the key.
    const [confirmFree, setConfirmFree] = useState(false);
    const alive = useRef(true);

    function refresh() {
        SteamAPIStatus().then((s) => alive.current && setStatus(s)).catch(() => undefined);
    }

    useEffect(() => {
        alive.current = true;
        refresh();
        const timer = window.setInterval(refresh, STATUS_REFRESH_MS);
        const off = EventsOn('steam-api-changed', refresh);
        return () => {
            alive.current = false;
            window.clearInterval(timer);
            off();
        };
    }, []);

    if (!status) return <div className="settings-content single"/>;

    const chosen: Mode = draft ?? (status.Mode as Mode);
    const locked = chosen === 'free';
    const keyUsable = status.HasKey && status.State !== 'unreadable';
    const line = statusLine(status, draft);

    async function run<T>(kind: 'save' | 'check' | 'mode', work: () => Promise<T>): Promise<T | undefined> {
        setBusy(kind);
        setError('');
        try {
            return await work();
        } catch (err) {
            setError(String(err).replace(/^Error:\s*/, ''));
            return undefined;
        } finally {
            if (alive.current) setBusy(null);
            refresh();
        }
    }

    function pick(mode: Mode) {
        if (busy) return;
        setError('');
        if (mode === 'free') {
            if (status!.HasKey) {
                setConfirmFree(true);
            } else {
                setDraft(null);
                run('mode', () => SetSteamAPIMode('free'));
            }
            return;
        }
        setConfirmFree(false);
        if (keyUsable) {
            setDraft(null);
            run('mode', () => SetSteamAPIMode(mode));
        } else {
            setDraft(mode);
        }
    }

    async function deleteKeyAndUseFree() {
        setConfirmFree(false);
        setDraft(null);
        setKey('');
        const result = await run('mode', () => SetSteamAPIMode('free'));
        if (result) notify('info', 'Steam API key deleted - using the free API only.');
    }

    async function save() {
        const result = await run('save', () => SaveSteamAPIKey(key, chosen));
        if (result) {
            setKey('');
            setDraft(null);
            notify('success', 'Steam API key saved (encrypted on this computer). Workshop details are being fetched again.');
        }
    }

    async function check() {
        const result = await run('check', () => CheckSteamAPIKey());
        if (result) notify('success', 'Steam accepts the saved key.');
    }

    const canSave = !locked && key.trim() !== '' && !busy;

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Steam API</div>
                <div className="settings-subtitle">
                    Workshop titles, counts and update dates come from Steam. By default the app uses Steam's free
                    API, which works for most mods. A Steam Web API key of your own also lets it read the mods the
                    free API cannot return, such as unlisted ones. It is optional.
                </div>
            </div>

            <div className="settings-columns">
                <div className="settings-column">
                    <div className={`steam-status ${line.tone}`}>
                        <i className={`fa-solid ${line.icon}`}/>
                        <span>{line.text}</span>
                    </div>

                    <div className="mode-option-list">
                        {MODES.map((m) => {
                            const active = chosen === m.mode;
                            return (
                                <div key={m.mode} className={`mode-option ${active ? 'active' : ''} ${busy ? 'disabled' : ''}`} onClick={() => pick(m.mode)}>
                                    <i className={`fa-solid ${active ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${active ? 'on' : 'off'}`}/>
                                    <div className="mode-option-main">
                                        <div className="mode-option-name">
                                            {m.name}
                                            {m.recommended && <span className="mode-option-recommended">(Recommended)</span>}
                                            {m.mode === 'free' && <span className="chip">Default</span>}
                                        </div>
                                        <div className="mode-option-desc">{m.desc}</div>
                                    </div>
                                </div>
                            );
                        })}
                    </div>

                    {confirmFree && (
                        <div className="steam-confirm">
                            <span>Free API use only deletes your saved key ({status.Fingerprint}) from the settings file. You would have to enter it again to use it later.</span>
                            <span className="steam-confirm-actions">
                                <button className="btn-primary" onClick={deleteKeyAndUseFree}>Delete key and use free</button>
                                <button className="btn-ghost" onClick={() => setConfirmFree(false)}>Cancel</button>
                            </span>
                        </div>
                    )}
                </div>
                <div className="settings-column">
                    <div className={`steam-key ${locked ? 'locked' : ''}`}>
                        <label className="steam-key-label" htmlFor="steam-api-key">
                            <i className="fa-solid fa-key"/> Steam Web API key
                            <span className="link-btn" onClick={() => BrowserOpenURL(KEY_PAGE)}>Get a key</span>
                        </label>
                        <div className="steam-key-row">
                            <input
                                id="steam-api-key"
                                className="steam-key-input"
                                type="password"
                                autocomplete="off"
                                spellcheck={false}
                                value={key}
                                disabled={locked || busy !== null}
                                placeholder={locked ? 'Locked while Free API use only is chosen' : status.HasKey ? 'A key is saved - enter a new one to replace it' : '32 letters and numbers'}
                                onInput={(e) => setKey((e.target as HTMLInputElement).value)}
                                onKeyDown={(e) => e.key === 'Enter' && canSave && save()}
                            />
                            <button className="btn-primary" disabled={!canSave} onClick={save}>
                                {busy === 'save' ? 'Checking...' : 'Save key'}
                            </button>
                            {status.HasKey && !locked && (
                                <button className="btn-ghost" disabled={busy !== null || !keyUsable} onClick={check}>
                                    {busy === 'check' ? 'Checking...' : 'Check key'}
                                </button>
                            )}
                        </div>
                        {error && <div className="steam-key-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}
                        <div className="steam-key-note">
                            The key is checked with Steam before it is saved, so a wrong one is never stored.
                            {status.HasKey && <> Saved key: <span className="mono">{status.Fingerprint}</span> (a short fingerprint, not the key).</>}
                        </div>
                        <div className="steam-key-note">
                            <i className="fa-solid fa-lock"/> {status.Protection}
                        </div>
                    </div>

                    {(status.ItemsFromKey > 0 || status.ItemsFromFree > 0) && (
                        <div className="steam-counts">
                            Since the app started: {status.ItemsFromKey} Workshop items answered through your key, {status.ItemsFromFree} through the free API
                            {status.Rescued > 0 && <>, {status.Rescued} of them only the key could return</>}.
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

import './ToolsPanel.css';
import {Fragment, h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {ClearDeepLAPIKey, DeepLStatus, SaveDeepLAPIKey} from '../../wailsjs/go/main/App';
import type {app} from '../../wailsjs/go/models';
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime';
import {notify} from '../data/notifications';

const KEY_PAGE = 'https://www.deepl.com/en/your-account/keys';

type Tier = 'free' | 'pro';

// statusLine says what the app knows about the saved DeepL key right now,
// in one sentence, with the icon and tone that go with it - mirrors
// SteamApiPanel.tsx's own statusLine exactly.
function statusLine(s: app.DeepLStatus): { icon: string; tone: string; text: string } {
    if (s.Unreadable) {
        return {icon: 'fa-triangle-exclamation', tone: 'warn', text: 'A key is saved but cannot be read on this computer (the file came from another one, or was changed) - enter it again.'};
    }
    if (s.HasKey) {
        return {icon: 'fa-circle-check', tone: 'good', text: `Using your DeepL API key (${s.Fingerprint}), ${s.Tier} tier.`};
    }
    return {icon: 'fa-key', tone: 'neutral', text: 'No DeepL key saved yet - the DeepL service is unavailable in the Translate tab until one is.'};
}

export function ToolsPanel() {
    const [status, setStatus] = useState<app.DeepLStatus | null>(null);
    const [key, setKey] = useState('');
    const [tier, setTier] = useState<Tier>('free');
    const [busy, setBusy] = useState<'save' | 'clear' | null>(null);
    const [error, setError] = useState('');
    const [confirmClear, setConfirmClear] = useState(false);
    const alive = useRef(true);

    function refresh() {
        DeepLStatus().then((s) => {
            if (!alive.current) return;
            setStatus(s);
            setTier((s.Tier as Tier) || 'free');
        }).catch(() => undefined);
    }

    useEffect(() => {
        alive.current = true;
        refresh();
        return () => {
            alive.current = false;
        };
    }, []);

    if (!status) return <div className="settings-content single"/>;

    const line = statusLine(status);

    async function save() {
        setBusy('save');
        setError('');
        try {
            await SaveDeepLAPIKey(key, tier);
            setKey('');
            notify('success', 'DeepL API key saved (encrypted on this computer).');
        } catch (err) {
            setError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            if (alive.current) setBusy(null);
            refresh();
        }
    }

    async function clear() {
        setConfirmClear(false);
        setBusy('clear');
        setError('');
        try {
            await ClearDeepLAPIKey();
            notify('info', 'DeepL API key removed.');
        } catch (err) {
            setError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            if (alive.current) setBusy(null);
            refresh();
        }
    }

    const canSave = key.trim() !== '' && !busy;

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Tools</div>
                <div className="settings-subtitle">
                    Settings for tools that talk to a service outside this app. The auto-translation feature's
                    official DeepL option needs an API key of your own - the two other translation services it
                    offers are free and need nothing here.
                </div>
            </div>

            <div className="settings-columns">
                <div className="settings-column">
                    <div className={`tools-status ${line.tone}`}>
                        <i className={`fa-solid ${line.icon}`}/>
                        <span>{line.text}</span>
                    </div>

                    {status.HasKey && !confirmClear && (
                        <div className="editor-actions">
                            <button type="button" className="btn-ghost" disabled={busy !== null} onClick={() => setConfirmClear(true)}>
                                {busy === 'clear' ? 'Removing...' : 'Remove saved key'}
                            </button>
                        </div>
                    )}
                    {confirmClear && (
                        <div className="tools-confirm">
                            <span>Remove the saved DeepL key ({status.Fingerprint})? The DeepL option in the Translate tab becomes unavailable until you enter one again.</span>
                            <span className="tools-confirm-actions">
                                <button type="button" className="btn-primary" onClick={clear}>Remove key</button>
                                <button type="button" className="btn-ghost" onClick={() => setConfirmClear(false)}>Cancel</button>
                            </span>
                        </div>
                    )}
                </div>

                <div className="settings-column">
                    <div className="tools-key">
                        <label className="tools-key-label" htmlFor="deepl-api-key">
                            <i className="fa-solid fa-key"/> DeepL API key
                            <span className="link-btn" onClick={() => BrowserOpenURL(KEY_PAGE)}>Get a key</span>
                        </label>
                        <div className="tools-key-row">
                            <input
                                id="deepl-api-key"
                                className="tools-key-input"
                                type="password"
                                autocomplete="off"
                                spellcheck={false}
                                value={key}
                                disabled={busy !== null}
                                placeholder={status.HasKey ? 'A key is saved - enter a new one to replace it' : 'Paste your DeepL API key'}
                                onInput={(e) => setKey((e.target as HTMLInputElement).value)}
                                onKeyDown={(e) => e.key === 'Enter' && canSave && save()}
                            />
                            <select className="tools-tier-select" value={tier} disabled={busy !== null} onChange={(e) => setTier((e.target as HTMLSelectElement).value as Tier)}>
                                <option value="free">Free tier</option>
                                <option value="pro">Pro tier</option>
                            </select>
                            <button type="button" className="btn-primary" disabled={!canSave} onClick={save}>
                                {busy === 'save' ? 'Checking...' : 'Save key'}
                            </button>
                        </div>
                        {error && <div className="tools-key-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}
                        <div className="tools-key-note">
                            The key is checked with DeepL before it is saved, so a wrong one (or the wrong tier for
                            a real key) is never stored.
                            {status.HasKey && <> Saved key: <span className="mono">{status.Fingerprint}</span> (a short fingerprint, not the key).</>}
                        </div>
                        <div className="tools-key-note">
                            <i className="fa-solid fa-lock"/> {status.Protection}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}

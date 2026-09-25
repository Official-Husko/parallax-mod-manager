import {h} from 'preact';
import type {ComponentChildren} from 'preact';
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime';

// The "enter your own API key" card shared by the Tools (DeepL) and Steam API settings
// panels: a label with a "Get a key" link, the password input itself plus whatever else a
// specific key's row needs (a tier picker, a Check key button - passed as children,
// rendered right after the input), an error line, and the two note lines every one of these
// ends with (how it's checked before saving, and how it's protected on disk).
export function ApiKeyField({id, label, keyPageURL, value, placeholder, disabled, locked, error, note, protection, onInput, onEnter, children}: {
    id: string;
    label: string;
    keyPageURL: string;
    value: string;
    placeholder: string;
    disabled?: boolean;
    // Dims the whole field - for a key that cannot be used right now (Steam's own Free API
    // use only mode locks its field rather than hiding it).
    locked?: boolean;
    error?: string;
    // The "checked before saving..." sentence, including the saved-fingerprint clause a
    // caller adds when it has a key.
    note: ComponentChildren;
    protection: string;
    onInput: (value: string) => void;
    // Already gated by the caller (e.g. `() => canSave && save()`) - Enter always calls this,
    // the guard is the caller's own canSave check, same as clicking Save would be.
    onEnter: () => void;
    // Whatever comes after the input in its row - a tier select, a Save button, a Check button.
    children: ComponentChildren;
}) {
    return (
        <div className={`api-key-field ${locked ? 'locked' : ''}`}>
            <label className="api-key-label" htmlFor={id}>
                <i className="fa-solid fa-key"/> {label}
                <span className="link-btn" onClick={() => BrowserOpenURL(keyPageURL)}>Get a key</span>
            </label>
            <div className="api-key-row">
                <input
                    id={id}
                    className="api-key-input"
                    type="password"
                    autocomplete="off"
                    spellcheck={false}
                    value={value}
                    disabled={disabled}
                    placeholder={placeholder}
                    onInput={(e) => onInput((e.target as HTMLInputElement).value)}
                    onKeyDown={(e) => e.key === 'Enter' && onEnter()}
                />
                {children}
            </div>
            {error && <div className="api-key-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}
            <div className="api-key-note">{note}</div>
            <div className="api-key-note"><i className="fa-solid fa-lock"/> {protection}</div>
        </div>
    );
}

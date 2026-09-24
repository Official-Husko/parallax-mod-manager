import './Checkbox.css';
import {h} from 'preact';

// The app's own checkbox, used wherever a native <input type="checkbox"> would be: an unstyled
// native checkbox renders as a plain white/OS-themed box that clashes with this app's dark
// interface. Keeps a real <input type="checkbox"> for keyboard and screen-reader support, sized
// down to nothing and hidden, with a themed box drawn next to it that follows the input's own
// :checked/:focus-visible state through plain CSS sibling selectors - no click handling needed
// beyond the input's own onChange.
export function Checkbox({checked, onChange, disabled, id, title}: {
    checked: boolean;
    onChange: (checked: boolean) => void;
    disabled?: boolean;
    id?: string;
    title?: string;
}) {
    return (
        <label className={`checkbox ${disabled ? 'disabled' : ''}`} title={title}>
            <input
                type="checkbox"
                id={id}
                checked={checked}
                disabled={disabled}
                onChange={(e) => onChange((e.target as HTMLInputElement).checked)}
            />
            <span className="checkbox-box">&#10003;</span>
        </label>
    );
}

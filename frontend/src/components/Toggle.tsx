import {h} from 'preact';

export function Toggle({on}: { on: boolean }) {
    return (
        <span className={`toggle-pill ${on ? 'on' : 'off'}`}>
            <span className="toggle-knob"/>
        </span>
    );
}

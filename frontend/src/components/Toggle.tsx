import {h} from 'preact';

export function Toggle({on, onClick}: { on: boolean; onClick?: () => void }) {
    return (
        <span className={`toggle-pill ${on ? 'on' : 'off'} ${onClick ? 'clickable' : ''}`} onClick={onClick}>
            <span className="toggle-knob"/>
        </span>
    );
}

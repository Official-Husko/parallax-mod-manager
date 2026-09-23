import './Avatar.css';
import {h} from 'preact';
import {useState} from 'preact/hooks';
import {avatarColorFromName} from '../data/nameColor';

// Avatar: a real profile picture when one is given, otherwise the first
// letter of the name on a color hashed from the name itself (see
// avatarColorFromName) - same name always gets the same color, no lookup
// table to keep in sync. Falls back the same way if a real image URL is
// given but actually fails to load (a dead link, a since-deleted upload),
// rather than showing a broken-image icon. Used anywhere Browse shows a
// person - file authors, comment authors, and the signed-in account itself.
export function Avatar({name, url, size = 28}: {name: string; url?: string; size?: number}) {
    const [failed, setFailed] = useState(false);

    const style = {width: `${size}px`, height: `${size}px`, fontSize: `${Math.max(9, size * 0.42)}px`};

    if (url && !failed) {
        return (
            <img
                className="avatar avatar-img"
                style={style}
                src={url}
                alt={name}
                loading="lazy"
                onError={() => setFailed(true)}
            />
        );
    }

    const initial = name.trim().charAt(0).toUpperCase() || '?';
    return (
        <span className="avatar avatar-fallback" style={{...style, background: avatarColorFromName(name)}} title={name}>
            {initial}
        </span>
    );
}

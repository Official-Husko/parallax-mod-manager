import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {GameMedia} from '../../wailsjs/go/main/App';

// A per-game logo, fetched from the real embed-plus-override game media
// store (see internal/gamemedia) - not every game has art yet, so a
// plain-color fallback box (styled by className, same as the real image)
// is expected, not an error state.
export function GameLogo({gameId, className}: { gameId: string; className?: string }) {
    const [src, setSrc] = useState('');

    useEffect(() => {
        let cancelled = false;
        setSrc('');
        GameMedia('logo', gameId)
            .then((s) => { if (!cancelled) setSrc(s); })
            .catch(() => undefined);
        return () => { cancelled = true; };
    }, [gameId]);

    if (src) {
        return <img className={className} src={src} alt=""/>;
    }
    return <div className={`${className ?? ''} fallback`}/>;
}

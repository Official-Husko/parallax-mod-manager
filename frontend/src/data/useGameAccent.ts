import {useEffect, useState} from 'preact/hooks';
import {GameMedia} from '../../wailsjs/go/main/App';
import {extractAccent, type Accent} from './accentColor';

// useGameAccent resolves a game's real logo (if it has one yet) into a UI
// accent color via colorthief. Returns null while loading, while the game
// has no logo, or when extraction fails - callers should fall back to this
// app's default rust accent in every one of those cases, not guess.
export function useGameAccent(gameId: string): Accent | null {
    const [accent, setAccent] = useState<Accent | null>(null);

    useEffect(() => {
        let cancelled = false;
        setAccent(null);
        if (!gameId) {
            return;
        }
        GameMedia('logo', gameId)
            .then((src) => (src ? extractAccent(src) : null))
            .then((a) => { if (!cancelled) setAccent(a); })
            .catch(() => undefined);
        return () => { cancelled = true; };
    }, [gameId]);

    return accent;
}

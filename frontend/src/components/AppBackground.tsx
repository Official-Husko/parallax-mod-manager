import './AppBackground.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {backgroundsForGame} from '../data/gameBackgrounds';

export const DEFAULT_BACKGROUND_INTERVAL_SECONDS = 300;

function pickRandom(pool: string[], excluding: string): string {
    if (pool.length === 1) return pool[0];
    let next = pool[Math.floor(Math.random() * pool.length)];
    while (next === excluding) {
        next = pool[Math.floor(Math.random() * pool.length)];
    }
    return next;
}

// A slowly rotating, slightly darkened background image behind the whole
// app - one random image at a time from this game's own background pool
// (see data/gameBackgrounds.ts), cross-fading to a new random pick every
// intervalSeconds via two stacked, alternating layers (only one CSS
// transition property - opacity - so the fade is a true cross-fade, not a
// pop). A game with no background art yet (most of them, for now - only
// Stellaris ships any) renders nothing, same as disabled - #app's own
// plain --bg-app color shows through instead.
export function AppBackground({gameId, disabled, rotationPaused, intervalSeconds}: {
    gameId: string;
    disabled: boolean;
    rotationPaused: boolean;
    intervalSeconds: number;
}) {
    const [state, setState] = useState<{layers: [string, string]; active: 0 | 1}>({layers: ['', ''], active: 0});

    useEffect(() => {
        if (disabled) {
            setState({layers: ['', ''], active: 0});
            return;
        }
        const pool = backgroundsForGame(gameId);
        if (pool.length === 0) {
            setState({layers: ['', ''], active: 0});
            return;
        }

        function rotate() {
            setState((prev) => {
                const nextActive: 0 | 1 = prev.active === 0 ? 1 : 0;
                const next = pickRandom(pool, prev.layers[prev.active]);
                const layers = [...prev.layers] as [string, string];
                layers[nextActive] = next;
                return {layers, active: nextActive};
            });
        }

        rotate();
        if (rotationPaused) {
            return;
        }
        const ms = (intervalSeconds > 0 ? intervalSeconds : DEFAULT_BACKGROUND_INTERVAL_SECONDS) * 1000;
        const id = setInterval(rotate, ms);
        return () => clearInterval(id);
    }, [gameId, disabled, rotationPaused, intervalSeconds]);

    if (disabled || (!state.layers[0] && !state.layers[1])) {
        return null;
    }

    return (
        <div className="app-background">
            <div
                className="app-background-layer"
                style={{
                    // Quoted - many of these filenames contain literal
                    // spaces (Steam Workshop's own "NN - randomID.jpg"
                    // naming), which an unquoted url(...) can't contain.
                    backgroundImage: state.layers[0] ? `url("${state.layers[0]}")` : undefined,
                    opacity: state.active === 0 ? 1 : 0,
                }}
            />
            <div
                className="app-background-layer"
                style={{
                    backgroundImage: state.layers[1] ? `url("${state.layers[1]}")` : undefined,
                    opacity: state.active === 1 ? 1 : 0,
                }}
            />
            <div className="app-background-scrim"/>
        </div>
    );
}

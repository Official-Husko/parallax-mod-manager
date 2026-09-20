import './AppBackground.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {BackgroundImages} from '../../wailsjs/go/main/App';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {logEvent} from '../data/appLog';
import {createRotator} from '../data/backgroundRotation';

export const DEFAULT_BACKGROUND_INTERVAL_SECONDS = 300;

// How long an image may take to load before it is given up on (a hung connection
// would otherwise hold the whole rotation).
const PREFETCH_TIMEOUT_MS = 45_000;

// Images that have been prefetched and are being held, by address. Keeping the
// element alive is what keeps its decoded pixels in memory, so showing it later is
// instant; only the newest few are kept.
const held = new Map<string, HTMLImageElement>();
const HOLD_LIMIT = 3;

// prefetchImage loads and decodes one image ahead of when it is shown. Rejects if it
// cannot be loaded (offline, missing, not an image) or takes too long.
function prefetchImage(url: string): Promise<void> {
    const existing = held.get(url);
    if (existing && existing.complete && existing.naturalWidth > 0) return Promise.resolve();
    const img = new Image();
    img.decoding = 'async';
    img.src = url;
    held.set(url, img);
    let timer: number | undefined;
    const timeout = new Promise<never>((_, reject) => {
        timer = window.setTimeout(() => {
            img.src = '';
            reject(new Error('timed out'));
        }, PREFETCH_TIMEOUT_MS);
    });
    return Promise.race([img.decode(), timeout])
        .catch((err) => {
            held.delete(url);
            throw err;
        })
        .finally(() => window.clearTimeout(timer));
}

// release forgets the oldest held images beyond the limit, never `keep`.
function release(keep: string) {
    for (const url of held.keys()) {
        if (held.size <= HOLD_LIMIT) break;
        if (url !== keep) held.delete(url);
    }
}

// A slowly rotating, slightly darkened background image behind the whole app:
// one random image at a time from this game's pool, cross-fading to a new random
// pick every intervalSeconds via two stacked, alternating layers (only one CSS
// transition property - opacity - so the fade is a true cross-fade, not a pop).
//
// The pool is not part of the app. The backend lists it when the game is selected
// (see BackgroundImages): in online mode the images are streamed from GitHub, in
// offline mode - or when GitHub cannot be reached - they are the copies on disk. The
// next image is fetched and decoded PREFETCH_LEAD_SECONDS before it is due (see
// data/backgroundRotation.ts), so the swap never waits on the network. A game with
// no images renders nothing, same as disabled - #app's own plain --bg-app color
// shows through instead.
export function AppBackground({gameId, disabled, rotationPaused, intervalSeconds, source}: {
    gameId: string;
    disabled: boolean;
    rotationPaused: boolean;
    intervalSeconds: number;
    // "online" or "offline" - only here so a change of source reloads the pool.
    source: string;
}) {
    const [state, setState] = useState<{layers: [string, string]; active: 0 | 1}>({layers: ['', ''], active: 0});
    // Bumped when downloaded images are added or removed, so the pool is listed again.
    const [packsVersion, setPacksVersion] = useState(0);

    useEffect(() => {
        const off = EventsOn('background-packs-changed', () => setPacksVersion((v) => v + 1));
        return () => { off(); };
    }, []);

    useEffect(() => {
        const clear = () => setState({layers: ['', ''], active: 0});
        if (disabled || !gameId) {
            clear();
            return;
        }
        let cancelled = false;
        let stop = () => {};
        BackgroundImages(gameId)
            .then((pool) => {
                if (cancelled) return;
                if (pool.length === 0) {
                    clear();
                    return;
                }
                const seconds = intervalSeconds > 0 ? intervalSeconds : DEFAULT_BACKGROUND_INTERVAL_SECONDS;
                const rotator = createRotator({
                    pool,
                    intervalMs: rotationPaused ? 0 : seconds * 1000,
                    prefetch: prefetchImage,
                    show: (url) => {
                        release(url);
                        setState((prev) => {
                            const nextActive: 0 | 1 = prev.active === 0 ? 1 : 0;
                            const layers = [...prev.layers] as [string, string];
                            layers[nextActive] = url;
                            return {layers, active: nextActive};
                        });
                    },
                    onLoadError: (url, err) => logEvent('warn', 'Backgrounds', `couldn't load a background image (${String(err)}): ${url.slice(-80)}`),
                });
                rotator.start();
                stop = () => rotator.stop();
            })
            .catch((err) => {
                if (cancelled) return;
                logEvent('warn', 'Backgrounds', `couldn't list the background images: ${String(err)}`);
                clear();
            });
        return () => {
            cancelled = true;
            stop();
        };
    }, [gameId, disabled, rotationPaused, intervalSeconds, source, packsVersion]);

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

import './AppBackground.css';
import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {BackgroundImages, SetStaticBackground} from '../../wailsjs/go/main/App';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {logEvent} from '../data/appLog';
import {publishBackground, registerRandomBackground} from '../data/backgroundControl';
import {blurPixels, DEFAULT_BACKGROUND_BLUR, DEFAULT_BACKGROUND_DARKEN, scrimAlphas, useBackgroundLookPreview} from '../data/backgroundLook';
import {createRotator, imageNameFromUrl, imageOrigin} from '../data/backgroundRotation';

export const DEFAULT_BACKGROUND_INTERVAL_SECONDS = 300;

// How long an image may take to load before it is given up on (a hung connection
// would otherwise hold the whole rotation).
const PREFETCH_TIMEOUT_MS = 45_000;

// Images that have been prefetched and are being held, by address. Keeping the
// element alive is what keeps its decoded pixels in memory, so showing it later is
// instant; only the newest few are kept.
const held = new Map<string, HTMLImageElement>();
const HOLD_LIMIT = 3;
// How long each image took to load, for the activity log line when it is shown.
const loadMs = new Map<string, number>();

// prefetchImage loads and decodes one image ahead of when it is shown. Rejects if it
// cannot be loaded (offline, missing, not an image) or takes too long.
function prefetchImage(url: string): Promise<void> {
    const existing = held.get(url);
    if (existing && existing.complete && existing.naturalWidth > 0) return Promise.resolve();
    logEvent('debug', 'Backgrounds', `loading background '${imageNameFromUrl(url)}' (${imageOrigin(url)})`);
    const began = performance.now();
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
        .then(() => {
            loadMs.set(url, Math.round(performance.now() - began));
        })
        .catch((err) => {
            held.delete(url);
            throw err;
        })
        .finally(() => window.clearTimeout(timer));
}

// describeShown is the activity log line for an image that has just gone on
// screen: which one, how big it is, where it came from and how long it took to load.
function describeShown(url: string): string {
    const img = held.get(url);
    const parts: string[] = [];
    if (img && img.naturalWidth > 0) parts.push(`${img.naturalWidth}x${img.naturalHeight}`);
    parts.push(imageOrigin(url));
    const ms = loadMs.get(url);
    if (ms !== undefined) parts.push(`loaded in ${ms} ms`);
    return `showing background '${imageNameFromUrl(url)}' (${parts.join(', ')})`;
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
// In Static mode (rotationPaused) it shows one picture and never changes it by itself:
// the one saved for this game (staticImage), and whenever a picture goes on screen in
// that mode - the first pick, or "Random" in Settings - it is saved (SetStaticBackground),
// so the same picture loads every time. Changing a setting keeps the picture on screen
// rather than jumping to another; switching to Static freezes the one being shown.
//
// The pool is not part of the app. The backend lists it when the game is selected
// (see BackgroundImages): in online mode the images are streamed from GitHub, in
// offline mode - or when GitHub cannot be reached - they are the copies on disk. The
// next image is fetched and decoded PREFETCH_LEAD_SECONDS before it is due (see
// data/backgroundRotation.ts), so the swap never waits on the network. A game with
// no images renders nothing, same as disabled - #app's own plain --bg-app color
// shows through instead.
export function AppBackground({gameId, disabled, rotationPaused, intervalSeconds, source, blur = DEFAULT_BACKGROUND_BLUR, darken = DEFAULT_BACKGROUND_DARKEN, staticImage = '', onStaticSaved}: {
    gameId: string;
    disabled: boolean;
    rotationPaused: boolean;
    intervalSeconds: number;
    // "online" or "offline" - only here so a change of source reloads the pool.
    source: string;
    // How strongly the image is blurred and darkened, 0-100 (see data/backgroundLook.ts) - the
    // saved settings; a slider being dragged in Settings overrides them until it is let go.
    blur?: number;
    darken?: number;
    // The file name Static mode has saved for this game, '' when there is none yet.
    staticImage?: string;
    // Called after the static picture was saved, so the settings the app holds catch up.
    onStaticSaved?: () => void;
}) {
    const preview = useBackgroundLookPreview();
    // What is on screen (for which game), so a setting change or a switch of mode does not
    // jump to another picture; and the latest saved static name and callback, read when the
    // pool loads rather than restarting the rotation whenever they change.
    const shownRef = useRef<{gameId: string; url: string} | null>(null);
    const staticImageRef = useRef(staticImage);
    staticImageRef.current = staticImage;
    const onStaticSavedRef = useRef(onStaticSaved);
    onStaticSavedRef.current = onStaticSaved;
    const blurPx = blurPixels(preview?.blur ?? blur);
    const scrim = scrimAlphas(preview?.darken ?? darken);
    const [state, setState] = useState<{layers: [string, string]; active: 0 | 1}>({layers: ['', ''], active: 0});
    // Bumped when downloaded images are added or removed, so the pool is listed again.
    const [packsVersion, setPacksVersion] = useState(0);

    useEffect(() => {
        const off = EventsOn('background-packs-changed', () => setPacksVersion((v) => v + 1));
        return () => { off(); };
    }, []);

    useEffect(() => {
        const clear = () => {
            // Nothing is on screen any more, so what comes back is a fresh pick (or the saved
            // static one): remembering the old picture as "already showing" would leave it blank.
            shownRef.current = null;
            setState({layers: ['', ''], active: 0});
            publishBackground(null);
            registerRandomBackground(null);
        };
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
                // Start with the picture already on screen for this game (a setting changed, not
                // the game), else - in Static mode - the saved one, else a random one. Matched by
                // file name, which is the same online and on disk.
                const shown = shownRef.current?.gameId === gameId ? imageNameFromUrl(shownRef.current.url) : '';
                const wanted = shown || (rotationPaused ? staticImageRef.current : '');
                const first = wanted ? pool.find((url) => imageNameFromUrl(url) === wanted) : undefined;
                const rotator = createRotator({
                    pool,
                    intervalMs: rotationPaused ? 0 : seconds * 1000,
                    first,
                    prefetch: prefetchImage,
                    show: (url) => {
                        const name = imageNameFromUrl(url);
                        const previous = shownRef.current;
                        const alreadyOnScreen = previous?.gameId === gameId && previous.url === url;
                        shownRef.current = {gameId, url};
                        publishBackground({gameId, name, origin: imageOrigin(url)});
                        // Static mode keeps whatever it shows, so the same picture comes back.
                        if (rotationPaused && name !== staticImageRef.current) {
                            SetStaticBackground(gameId, name)
                                .then(() => onStaticSavedRef.current?.())
                                .catch((err) => logEvent('warn', 'Backgrounds', `couldn't save the static background '${name}': ${String(err)}`));
                        }
                        // Already the picture on screen (a setting changed): nothing to swap.
                        if (alreadyOnScreen) return;
                        logEvent('info', 'Backgrounds', describeShown(url));
                        release(url);
                        setState((prev) => {
                            const nextActive: 0 | 1 = prev.active === 0 ? 1 : 0;
                            const layers = [...prev.layers] as [string, string];
                            layers[nextActive] = url;
                            return {layers, active: nextActive};
                        });
                    },
                    onLoadError: (url, err) => logEvent('warn', 'Backgrounds', `couldn't load background '${imageNameFromUrl(url)}' (${imageOrigin(url)}): ${err instanceof Error ? err.message : String(err)}`),
                });
                rotator.start();
                // "Random" in Settings: another picture now (and, when rotating, a fresh wait).
                registerRandomBackground(pool.length > 1 ? () => rotator.next() : null);
                stop = () => {
                    rotator.stop();
                    registerRandomBackground(null);
                };
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
        <div
            className="app-background"
            style={{'--bg-blur': `${blurPx}px`, '--scrim-top': String(scrim.top), '--scrim-bottom': String(scrim.bottom)} as h.JSX.CSSProperties}
        >
            <div
                className={`app-background-layer ${blurPx > 0 ? 'blurred' : ''}`}
                style={{
                    // Quoted - many of these filenames contain literal
                    // spaces (Steam Workshop's own "NN - randomID.jpg"
                    // naming), which an unquoted url(...) can't contain.
                    backgroundImage: state.layers[0] ? `url("${state.layers[0]}")` : undefined,
                    opacity: state.active === 0 ? 1 : 0,
                }}
            />
            <div
                className={`app-background-layer ${blurPx > 0 ? 'blurred' : ''}`}
                style={{
                    backgroundImage: state.layers[1] ? `url("${state.layers[1]}")` : undefined,
                    opacity: state.active === 1 ? 1 : 0,
                }}
            />
            <div className="app-background-scrim"/>
        </div>
    );
}

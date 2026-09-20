import {useCallback, useEffect, useRef, useState} from 'preact/hooks';
import {GameStatus} from '../../wailsjs/go/main/App';

// Whether the game is running, as the backend sees it (internal/gameproc): asked
// again every few seconds, because the game may have been started from Steam or
// the Paradox Launcher rather than from here, and closes whenever the player
// quits it.

export interface GameRunning {
    running: boolean;
    pids: number[];
}

const NOT_RUNNING: GameRunning = {running: false, pids: []};

// How often to ask, normally and for a while after Play is pressed - the game
// takes a moment to appear, and that is when the button should change soonest.
export const POLL_IDLE_MS = 3000;
export const POLL_BOOST_MS = 1000;
export const BOOST_WINDOW_MS = 120000;

export function pollDelay(now: number, boostUntil: number): number {
    return now < boostUntil ? POLL_BOOST_MS : POLL_IDLE_MS;
}

function same(a: GameRunning, b: GameRunning): boolean {
    return a.running === b.running && a.pids.length === b.pids.length && a.pids.every((p, i) => p === b.pids[i]);
}

// Follows whether gameId's game is running. checkSoon asks again at once and
// keeps asking quickly for a couple of minutes - call it right after launching.
// Nothing is asked while the window is hidden, and it asks again the moment the
// window comes back.
export function useGameRunning(gameId: string): GameRunning & { checkSoon: () => void } {
    const [state, setState] = useState<GameRunning>(NOT_RUNNING);
    const boostUntil = useRef(0);
    const kick = useRef<() => void>(() => undefined);

    useEffect(() => {
        setState(NOT_RUNNING);
        if (!gameId) return;

        let cancelled = false;
        let inFlight = false;
        let timer: number | undefined;

        const tick = async () => {
            if (cancelled || inFlight) return;
            window.clearTimeout(timer);
            if (!document.hidden) {
                inFlight = true;
                try {
                    const st = await GameStatus(gameId);
                    const next = {running: st.Running, pids: st.PIDs ?? []};
                    if (!cancelled) setState((prev) => (same(prev, next) ? prev : next));
                } catch {
                    // Couldn't tell this time: keep what was last known.
                } finally {
                    inFlight = false;
                }
            }
            if (!cancelled) timer = window.setTimeout(tick, pollDelay(Date.now(), boostUntil.current));
        };

        kick.current = () => {
            window.clearTimeout(timer);
            void tick();
        };
        const onVisibility = () => {
            if (!document.hidden) kick.current();
        };
        document.addEventListener('visibilitychange', onVisibility);
        void tick();

        return () => {
            cancelled = true;
            window.clearTimeout(timer);
            document.removeEventListener('visibilitychange', onVisibility);
        };
    }, [gameId]);

    const checkSoon = useCallback(() => {
        boostUntil.current = Date.now() + BOOST_WINDOW_MS;
        kick.current();
    }, []);

    return {...state, checkSoon};
}

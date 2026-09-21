// The background rotation's brain, kept free of React and the DOM so its timing
// can be tested: pick a random image from the pool, show it once it has loaded,
// and PREFETCH_LEAD_SECONDS before each swap fetch and decode the next one, so the
// swap itself is instant instead of waiting on the network.

// How long before a swap the next image starts loading.
export const PREFETCH_LEAD_SECONDS = 30;

// How often a failed image is replaced by another pick before giving up on a step.
const MAX_ATTEMPTS = 3;

export interface Timers {
    // Runs fn after ms; returns a function that cancels it.
    after(ms: number, fn: () => void): () => void;
}

export const realTimers: Timers = {
    after(ms, fn) {
        const id = setTimeout(fn, ms);
        return () => clearTimeout(id);
    },
};

export interface RotatorOptions {
    // The images to choose from (addresses the webview can load).
    pool: string[];
    // Time between swaps, in ms; 0 (or less) shows one image and never rotates.
    intervalMs: number;
    // Loads and decodes an image; resolves when it is ready to be shown and
    // rejects when it cannot be loaded.
    prefetch: (url: string) => Promise<void>;
    // Puts a prefetched image on screen.
    show: (url: string) => void;
    // The image to start with instead of a random one - Static mode's saved picture, or the
    // one already on screen when a setting changed. Must be in the pool; if it is not, or
    // fails to load, a random one is used.
    first?: string;
    // Called when an image could not be loaded and another was tried instead.
    onLoadError?: (url: string, err: unknown) => void;
    leadMs?: number;
    random?: () => number;
    timers?: Timers;
}

export interface Rotator {
    start(): void;
    stop(): void;
    // Shows another random image now (never the one on screen while there is any other) and,
    // when rotating, starts the wait for the following swap over from here. Does nothing
    // before start or after stop.
    next(): void;
}

// pickRandom chooses from pool without repeating `not` (the image on screen) when
// there is any other choice.
export function pickRandom(pool: string[], not: Iterable<string>, random: () => number): string | undefined {
    const excluded = new Set(not);
    const candidates = pool.filter((url) => !excluded.has(url));
    const from = candidates.length > 0 ? candidates : pool;
    if (from.length === 0) return undefined;
    return from[Math.min(from.length - 1, Math.floor(random() * from.length))];
}

export function createRotator(opts: RotatorOptions): Rotator {
    const timers = opts.timers ?? realTimers;
    const random = opts.random ?? Math.random;
    const lead = opts.leadMs ?? PREFETCH_LEAD_SECONDS * 1000;
    let running = false;
    let generation = 0;
    let cancelTimer: (() => void) | null = null;
    let current: string | undefined;

    // load prefetches one image, replacing it with another pick when it fails (up
    // to MAX_ATTEMPTS). Resolves undefined when nothing could be loaded.
    async function load(gen: number, avoid: string[]): Promise<string | undefined> {
        const tried = [...avoid];
        for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt++) {
            const url = pickRandom(opts.pool, tried, random);
            if (url === undefined) return undefined;
            try {
                await opts.prefetch(url);
                return gen === generation ? url : undefined;
            } catch (err) {
                if (gen !== generation) return undefined;
                opts.onLoadError?.(url, err);
                tried.push(url);
            }
        }
        return undefined;
    }

    // cycle runs from the moment `current` went on screen: prefetch the next
    // image `lead` before the swap, swap when it is due (or as soon as the image
    // is ready if the network was slower than the lead), and go round again.
    async function cycle(gen: number) {
        if (opts.intervalMs <= 0 || opts.pool.length < 2) return;
        const wait = (ms: number) => new Promise<boolean>((resolve) => {
            cancelTimer = timers.after(Math.max(0, ms), () => resolve(true));
            // stop() drops the timer; this promise then simply never resolves.
        });
        const prefetchDelay = Math.max(0, opts.intervalMs - lead);
        const swapDelay = opts.intervalMs - prefetchDelay;

        await wait(prefetchDelay);
        if (gen !== generation) return;
        const loading = load(gen, current ? [current] : []);
        await wait(swapDelay);
        if (gen !== generation) return;
        const next = await loading;
        if (gen !== generation) return;
        if (next !== undefined) {
            opts.show(next);
            current = next;
        }
        void cycle(gen);
    }

    // loadFirst loads the requested starting image, or a random one when there is none, it is
    // not in the pool, or it cannot be loaded.
    async function loadFirst(gen: number): Promise<string | undefined> {
        if (opts.first !== undefined && opts.pool.includes(opts.first)) {
            try {
                await opts.prefetch(opts.first);
                return gen === generation ? opts.first : undefined;
            } catch (err) {
                if (gen !== generation) return undefined;
                opts.onLoadError?.(opts.first, err);
            }
        }
        return load(gen, opts.first !== undefined ? [opts.first] : []);
    }

    return {
        start() {
            if (running) return;
            running = true;
            const gen = ++generation;
            void (async () => {
                const first = await loadFirst(gen);
                if (gen !== generation || first === undefined) return;
                opts.show(first);
                current = first;
                void cycle(gen);
            })();
        },
        next() {
            if (!running) return;
            const gen = ++generation;
            cancelTimer?.();
            cancelTimer = null;
            void (async () => {
                const picked = await load(gen, current ? [current] : []);
                if (gen !== generation) return;
                // Only the picture already on screen was loadable (every other one failed): there
                // is nothing new to show.
                if (picked !== undefined && picked !== current) {
                    opts.show(picked);
                    current = picked;
                }
                // Carry on rotating, from now.
                void cycle(gen);
            })();
        },
        stop() {
            running = false;
            generation++;
            cancelTimer?.();
            cancelTimer = null;
        },
    };
}

// imageNameFromUrl is the file name an image address ends in, decoded ("100 - a
// b.jpg"), for the activity log - the address itself is long and full of escapes.
export function imageNameFromUrl(url: string): string {
    const path = url.startsWith('data:') ? 'inline image' : url.split(/[?#]/)[0];
    const last = path.split('/').filter(Boolean).pop() ?? url;
    try {
        return decodeURIComponent(last);
    } catch {
        return last;
    }
}

// imageOrigin says where an image address loads from, for the activity log: the
// copies on this computer (served by the app itself), or the host it is streamed from.
export function imageOrigin(url: string): string {
    if (url.startsWith('/backgrounds/')) return 'from disk';
    if (url.startsWith('data:')) return 'embedded';
    try {
        return `from ${new URL(url).host}`;
    } catch {
        return 'from an unknown address';
    }
}

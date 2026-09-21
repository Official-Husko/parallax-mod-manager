import {useEffect, useState} from 'preact/hooks';

// A live preview of the accent colour while one is being picked: the whole interface follows the
// colour picker as it moves, and the setting is saved when the picker is let go. A plain
// module-level store, like data/backgroundLook.ts, so the picker (in Settings) and the app shell
// need no shared parent. null means no preview: the saved setting applies.
let preview: string | null = null;
const listeners = new Set<(hex: string | null) => void>();

export function previewAccent(hex: string | null) {
    preview = hex;
    for (const listener of listeners) listener(hex);
}

export function useAccentPreview(): string | null {
    const [state, setState] = useState<string | null>(preview);
    useEffect(() => {
        listeners.add(setState);
        setState(preview);
        return () => { listeners.delete(setState); };
    }, []);
    return state;
}

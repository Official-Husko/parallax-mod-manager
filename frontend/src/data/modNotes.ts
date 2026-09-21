import {useEffect, useRef, useState} from 'preact/hooks';
import {ModNotes, SetModNote} from '../../wailsjs/go/main/App';
import {notify} from './notifications';

// Per-mod notes, the interface half: what a person writes about a mod ("crashes with X", "waiting
// for an update"), kept per game by the backend (internal/modnotes, modnotes.go).

// The longest a note can be - the backend's own limit (modnotes.MaxLength).
export const MAX_NOTE_LENGTH = 10000;

// tidy is what the backend makes of a note as typed, so the copy held here matches the saved one.
export function tidyNote(text: string): string {
    return text.replace(/\r\n?/g, '\n').trim();
}

export interface ModNotesControl {
    // The notes of the current game, by mod ID.
    notes: Map<string, string>;
    // Why the notes could not be read, when they could not. Editing is off then, so nothing
    // written can replace a file that could not be understood.
    error: string;
    // Saves a mod's note (an empty one removes it); false when it could not be saved.
    save(modId: string, modName: string, text: string): Promise<boolean>;
}

// useModNotes loads gameId's notes and saves changes to them. Saves go through one at a time
// in the order asked for, so a slow one can never land after a newer one.
export function useModNotes(gameId: string): ModNotesControl {
    const [notes, setNotes] = useState<Map<string, string>>(new Map());
    const [error, setError] = useState('');
    const gameRef = useRef(gameId);
    gameRef.current = gameId;
    const queue = useRef<Promise<unknown>>(Promise.resolve());
    const control = useRef<ModNotesControl | null>(null);

    async function reload(id: string) {
        try {
            const loaded = await ModNotes(id);
            if (gameRef.current !== id) return;
            setNotes(new Map(Object.entries(loaded ?? {})));
            setError('');
        } catch (err) {
            if (gameRef.current !== id) return;
            setNotes(new Map());
            setError(String(err));
            notify('error', `Couldn't read your mod notes: ${String(err)}`);
        }
    }

    useEffect(() => {
        setNotes(new Map());
        setError('');
        if (gameId) void reload(gameId);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [gameId]);

    if (control.current === null) {
        control.current = {
            notes,
            error,
            save(modId, modName, text) {
                const game = gameRef.current;
                const tidy = tidyNote(text);
                setNotes((prev) => {
                    const next = new Map(prev);
                    if (tidy) next.set(modId, tidy); else next.delete(modId);
                    return next;
                });
                const run = async (): Promise<boolean> => {
                    try {
                        await SetModNote(game, modId, modName, text);
                        return true;
                    } catch (err) {
                        notify('error', `Couldn't save the note for '${modName}': ${String(err)}`);
                        void reload(game);
                        return false;
                    }
                };
                const result = queue.current.then(run, run);
                queue.current = result;
                return result;
            },
        };
    }
    control.current.notes = notes;
    control.current.error = error;
    return control.current;
}

// noteMatches says whether a mod's note contains the search text (already lower case).
export function noteMatches(notes: Map<string, string>, modId: string, query: string): boolean {
    const note = notes.get(modId);
    return !!note && note.toLowerCase().includes(query);
}

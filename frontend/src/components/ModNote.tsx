import './ModNote.css';
import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {MAX_NOTE_LENGTH} from '../data/modNotes';
import {TipItem} from './Tooltip';

// How long typing has to pause before the note is saved on its own.
const SAVE_AFTER_MS = 800;

// How much of a note the tooltip on a list row shows.
const TIP_LIMIT = 600;

// The tooltip of a row's note icon: the note itself.
export function noteTip(text: string) {
    const shown = text.length > TIP_LIMIT ? `${text.slice(0, TIP_LIMIT).trimEnd()}...` : text;
    return (
        <TipItem icon="fa-note-sticky" color="var(--note-color)" title="Your note">
            <div className="tip-note">{shown}</div>
        </TipItem>
    );
}

// The Notes box on a mod's Overview tab: the person's own writing about it. Saves when typing
// pauses and when the box loses focus, and once more if the box goes away with something unsaved.
// Give it the mod's ID as its key, so it starts fresh for each mod.
export function ModNote({note, error, focusRequest, onFocused, onSave}: {
    note: string;
    // Why notes are switched off, or ''.
    error: string;
    // Set (to a number that changes per request) when the box should take focus.
    focusRequest: number;
    onFocused: () => void;
    onSave: (text: string) => Promise<boolean>;
}) {
    const [draft, setDraft] = useState(note);
    const [status, setStatus] = useState<'idle' | 'editing' | 'saving' | 'saved' | 'failed'>('idle');
    const box = useRef<HTMLTextAreaElement>(null);
    const draftRef = useRef(note);
    const dirty = useRef(false);
    const timer = useRef<number>();
    const saveRef = useRef(onSave);
    saveRef.current = onSave;

    // A change made elsewhere shows up unless something is being typed.
    useEffect(() => {
        if (!dirty.current) {
            setDraft(note);
            draftRef.current = note;
        }
    }, [note]);

    useEffect(() => {
        if (focusRequest && box.current) {
            box.current.focus();
            box.current.setSelectionRange(box.current.value.length, box.current.value.length);
            onFocused();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [focusRequest]);

    function flush() {
        window.clearTimeout(timer.current);
        if (!dirty.current) return;
        dirty.current = false;
        setStatus('saving');
        void saveRef.current(draftRef.current).then((ok) => setStatus(ok ? 'saved' : 'failed'));
    }

    useEffect(() => () => {
        window.clearTimeout(timer.current);
        if (dirty.current) {
            dirty.current = false;
            void saveRef.current(draftRef.current);
        }
    }, []);

    if (error) {
        return (
            <div className="section note-section">
                <div className="section-label">NOTES</div>
                <div className="note-error">
                    <i className="fa-solid fa-triangle-exclamation"/>
                    <span>Your notes could not be read, so they are switched off to keep them safe: {error}</span>
                </div>
            </div>
        );
    }

    const statusText = status === 'editing' ? 'Unsaved' : status === 'saving' ? 'Saving...' : status === 'saved' ? 'Saved' : status === 'failed' ? 'Not saved' : '';
    return (
        <div className="section note-section">
            <div className="section-label note-label">
                <span>NOTES</span>
                {statusText && <span className={`note-status ${status}`}>{statusText}</span>}
            </div>
            <textarea
                ref={box}
                className="note-input"
                rows={4}
                maxLength={MAX_NOTE_LENGTH}
                placeholder="Your own notes about this mod. Kept only on this computer."
                value={draft}
                onInput={(e) => {
                    const value = (e.target as HTMLTextAreaElement).value;
                    setDraft(value);
                    draftRef.current = value;
                    dirty.current = true;
                    setStatus('editing');
                    window.clearTimeout(timer.current);
                    timer.current = window.setTimeout(flush, SAVE_AFTER_MS);
                }}
                onBlur={flush}
            />
        </div>
    );
}

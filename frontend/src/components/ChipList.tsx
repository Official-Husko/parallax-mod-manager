import './ChipList.css';
import {h} from 'preact';
import {useState} from 'preact/hooks';
import {addItems} from '../data/editorDraft';

// A list of short text items shown as chips, with a box to add more: a mod's tags, its
// dependencies, its replace paths. Enter or a comma adds what is typed (several can be pasted at
// once), leaving the box adds it too, and the x on a chip removes it. The add box itself always
// reads as a real "+ Add X" action, matching the mockup's own dashed pill (see .chip-list-input's
// :placeholder-shown styling) - not a plain, easy-to-miss text field with an example placeholder;
// hint (if given) explains what kind of thing it wants as a tooltip instead, so that recognizable
// label stays constant whether or not anything's already been added.
export function ChipList({items, onChange, addLabel, hint, mono, suggestions, flagged, flagTitle, disabled, id}: {
    items: string[];
    onChange: (items: string[]) => void;
    // Always shown as the add box's own placeholder, e.g. "+ Add tag" - not just while empty.
    addLabel: string;
    // An example of what to type, e.g. "Gameplay, Graphics, Fixes..." - a tooltip on the add box,
    // not competing with addLabel for the same space.
    hint?: string;
    // Draw the chips in the monospace font (paths).
    mono?: boolean;
    // Names offered while typing.
    suggestions?: string[];
    // Items to draw as a warning, with the tooltip flagTitle.
    flagged?: Set<string>;
    flagTitle?: string;
    disabled?: boolean;
    id?: string;
}) {
    const [text, setText] = useState('');
    const listId = suggestions && suggestions.length > 0 && id ? `${id}-suggestions` : undefined;

    function commit(value: string) {
        if (value.trim() === '') {
            setText('');
            return;
        }
        onChange(addItems(items, value));
        setText('');
    }

    return (
        <div className={`chip-list ${disabled ? 'disabled' : ''}`}>
            {items.map((item) => (
                <span key={item} className={`chip-item ${mono ? 'mono' : ''} ${flagged?.has(item) ? 'flagged' : ''}`} title={flagged?.has(item) ? flagTitle : undefined}>
                    {flagged?.has(item) && <span className="chip-item-flag">!</span>}
                    <span className="chip-item-text">{item}</span>
                    {!disabled && (
                        <span className="chip-item-remove" onClick={() => onChange(items.filter((x) => x !== item))}>&times;</span>
                    )}
                </span>
            ))}
            {!disabled && (
                <input
                    id={id}
                    className={`chip-list-input ${mono ? 'mono' : ''}`}
                    list={listId}
                    value={text}
                    placeholder={addLabel}
                    title={hint}
                    spellcheck={false}
                    onInput={(e) => {
                        const value = (e.target as HTMLInputElement).value;
                        if (value.endsWith(',')) commit(value);
                        else setText(value);
                    }}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                            e.preventDefault();
                            commit((e.target as HTMLInputElement).value);
                        } else if (e.key === 'Backspace' && text === '' && items.length > 0) {
                            onChange(items.slice(0, -1));
                        }
                    }}
                    onBlur={(e) => commit((e.target as HTMLInputElement).value)}
                />
            )}
            {listId && (
                <datalist id={listId}>
                    {suggestions!.filter((s) => !items.includes(s)).map((s) => <option key={s} value={s}/>)}
                </datalist>
            )}
        </div>
    );
}

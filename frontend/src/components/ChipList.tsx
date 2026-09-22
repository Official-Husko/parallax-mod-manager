import './ChipList.css';
import {h} from 'preact';
import {useState} from 'preact/hooks';
import {addItems} from '../data/editorDraft';

// A list of short text items shown as chips, with a box to add more: a mod's tags, its
// dependencies, its replace paths. Enter or a comma adds what is typed (several can be pasted at
// once), leaving the box adds it too, and the x on a chip removes it.
export function ChipList({items, onChange, placeholder, mono, suggestions, flagged, flagTitle, disabled, id}: {
    items: string[];
    onChange: (items: string[]) => void;
    placeholder?: string;
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
                    {flagged?.has(item) && <i className="fa-solid fa-triangle-exclamation"/>}
                    <span className="chip-item-text">{item}</span>
                    {!disabled && (
                        <i className="fa-solid fa-xmark chip-item-remove" onClick={() => onChange(items.filter((x) => x !== item))}/>
                    )}
                </span>
            ))}
            {!disabled && (
                <input
                    id={id}
                    className={`chip-list-input ${mono ? 'mono' : ''}`}
                    list={listId}
                    value={text}
                    placeholder={items.length === 0 ? placeholder : 'Add another...'}
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

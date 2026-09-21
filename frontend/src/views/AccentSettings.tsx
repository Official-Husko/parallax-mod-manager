import './AccentSettings.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {GameMedia, GetPreferences, ListGames} from '../../wailsjs/go/main/App';
import type {preferences} from '../../wailsjs/go/models';
import {extractPalette} from '../data/accentColor';
import {ensureVisible, normalizeAccentMode, normalizeHex} from '../data/accentPick';
import type {AccentMode, PaletteColor} from '../data/accentPick';
import {previewAccent} from '../data/accentPreview';
import {notify} from '../data/notifications';
import {patchPreferences} from '../data/preferencesPatch';

// The colour to start a custom accent from when there is none yet: the app's own.
const APP_DEFAULT_COLOR = '#c4623a';

// Settings > Appearance > Accent colour: where the interface's main colour comes from - each game's
// own (taken from its icon with colorthief, the default), one custom colour for every game, or the
// app's own - and the main colours of the current game's icon to pick from.
export function AccentSettings({onPreferencesChanged}: {onPreferencesChanged?: () => void}) {
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [game, setGame] = useState<{id: string; name: string} | null>(null);
    // The current game's icon colours; null while they are being worked out.
    const [palette, setPalette] = useState<PaletteColor[] | null>(null);
    // The hex box's own text, committed (validated and saved) when it is left.
    const [hexInput, setHexInput] = useState('');

    useEffect(() => {
        let cancelled = false;
        Promise.all([GetPreferences(), ListGames()]).then(([p, games]) => {
            if (cancelled) return;
            setPrefs(p);
            setHexInput(p.accentColor ?? '');
            const g = games.find((x) => x.ID === p.lastSelectedGame) ?? games[0];
            if (g) setGame({id: g.ID, name: g.DisplayName});
        }).catch(() => undefined);
        // Leaving the panel ends any preview; what was let go of is saved by then.
        return () => { cancelled = true; previewAccent(null); };
    }, []);

    useEffect(() => {
        let cancelled = false;
        setPalette(null);
        if (!game) return;
        GameMedia('logo', game.id)
            .then((src) => (src ? extractPalette(src) : []))
            .then((colors) => { if (!cancelled) setPalette(colors); })
            .catch(() => { if (!cancelled) setPalette([]); });
        return () => { cancelled = true; };
    }, [game?.id]);

    if (!prefs) {
        return null;
    }

    const mode: AccentMode = normalizeAccentMode(prefs.accentMode);
    const custom = normalizeHex(prefs.accentColor ?? '');

    // Saves the mode and colour together, changing nothing else in the settings.
    function save(nextMode: AccentMode, nextColor: string) {
        previewAccent(null);
        setHexInput(nextColor);
        patchPreferences({accentMode: nextMode, accentColor: nextColor})
            .then((saved) => { setPrefs(saved); onPreferencesChanged?.(); })
            .catch((err) => notify('error', `Couldn't save the accent colour: ${String(err)}`));
    }

    function chooseMode(next: AccentMode) {
        if (next === mode) return;
        if (next === 'custom') {
            // Start from the colour already chosen, else the game's main colour, else the app's own.
            save('custom', custom || palette?.[0]?.hex || APP_DEFAULT_COLOR);
        } else {
            save(next, custom);
        }
    }

    // Takes the box's own text, which is what was typed even if the last keystroke has not
    // been rendered yet.
    function commitHex(text: string) {
        const hex = normalizeHex(text);
        if (!hex) {
            setHexInput(custom);
            return;
        }
        if (hex !== custom || mode !== 'custom') save('custom', hex);
        else setHexInput(hex);
    }

    // What the interface really uses for the custom colour: a lighter shade when it is too dark
    // to read (see ensureVisible).
    const chosen = normalizeHex(hexInput) || custom;
    const shade = chosen ? ensureVisible(chosen) : '';

    const note = mode === 'custom'
        ? 'This colour is used for every game.'
        : mode === 'default'
            ? 'The app\'s own colour for every game.'
            : 'Each game gets its own accent, taken from the main colours of its icon. Pick one of the colours below to use it for every game instead.';

    return (
        <>
            <div className="appearance-group-label">ACCENT COLOUR</div>
            <div className="profile-toggles">
                <div className="profile-toggle-row">
                    <span>Accent colour</span>
                    <span className="source-toggle">
                        <span className={mode === 'game' ? 'active' : ''} onClick={() => chooseMode('game')}>
                            <i className="fa-solid fa-gamepad"/> Game
                        </span>
                        <span className={mode === 'custom' ? 'active' : ''} onClick={() => chooseMode('custom')}>
                            <i className="fa-solid fa-palette"/> Custom
                        </span>
                        <span className={mode === 'default' ? 'active' : ''} onClick={() => chooseMode('default')}>
                            <i className="fa-solid fa-rotate-left"/> Default
                        </span>
                    </span>
                </div>
                <div className="appearance-source-note"><span>{note}</span></div>
                {mode === 'custom' && (
                    <div className="profile-toggle-row">
                        <span>Colour</span>
                        <span className="accent-pick">
                            <input
                                type="color"
                                className="accent-color-input"
                                value={normalizeHex(hexInput) || custom || APP_DEFAULT_COLOR}
                                onInput={(e) => { const v = (e.target as HTMLInputElement).value; setHexInput(v); previewAccent(v); }}
                                onChange={(e) => save('custom', normalizeHex((e.target as HTMLInputElement).value))}
                            />
                            <input
                                type="text"
                                className="accent-hex-input mono"
                                value={hexInput}
                                maxLength={7}
                                spellcheck={false}
                                placeholder="#c4623a"
                                onInput={(e) => setHexInput((e.target as HTMLInputElement).value)}
                                onBlur={(e) => commitHex((e.target as HTMLInputElement).value)}
                                onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
                            />
                        </span>
                    </div>
                )}
                {mode === 'custom' && shade && shade !== chosen && (
                    <div className="appearance-source-note accent-shade">
                        <span>
                            <i className="fa-solid fa-circle-half-stroke"/> This colour is too dark to read on the app's dark background, so the
                            interface uses a lighter shade of it, <span className="accent-shade-swatch" style={{background: shade}}/> <span className="mono">{shade}</span>. Your colour stays saved as it is.
                        </span>
                    </div>
                )}
                {game && (
                    <div className="profile-toggle-row">
                        <span>From {game.name}'s icon</span>
                        <span className="accent-chips">
                            {palette === null && <span className="accent-chips-note">Looking at the icon...</span>}
                            {palette !== null && palette.length === 0 && <span className="accent-chips-note">No colours found in this game's icon.</span>}
                            {palette?.map((c) => (
                                <button
                                    key={c.hex}
                                    type="button"
                                    className={`accent-chip ${mode === 'custom' && custom === c.hex ? 'active' : ''}`}
                                    style={{background: c.hex}}
                                    title={`${c.role.replace(/([a-z])([A-Z])/g, '$1 $2')} - ${c.hex}. Use it for every game.`}
                                    onClick={() => save('custom', c.hex)}
                                />
                            ))}
                        </span>
                    </div>
                )}
            </div>
            <div className="appearance-group-label">BACKGROUND</div>
        </>
    );
}

import {GetPreferences, SetPreferences} from '../../wailsjs/go/main/App';
import type {preferences} from '../../wailsjs/go/models';

// Changes some settings without touching the others.
//
// SetPreferences saves the whole settings object, so a component that holds its own copy and
// writes it back also writes back every setting it did not mean to change - as they were when
// it last looked. A long-lived copy (the workspace view, the app shell) is old by the time it
// writes: a slider moved in Settings an hour earlier, saved, then undone by the next playset
// load. This reads the current settings first and puts only the change on top of them.
//
// update is the change, or a function of the current settings that returns it (for a change
// to one entry of a per-game map, which must be built on the current map, not an old one).
// Changes are made one at a time, in the order asked for. Resolves to the settings as saved.
let queue: Promise<unknown> = Promise.resolve();

export function patchPreferences(
    update: Partial<preferences.Preferences> | ((current: preferences.Preferences) => Partial<preferences.Preferences>),
): Promise<preferences.Preferences> {
    const run = async () => {
        const current = await GetPreferences();
        const change = typeof update === 'function' ? update(current) : update;
        const next = {...current, ...change} as preferences.Preferences;
        await SetPreferences(next);
        return next;
    };
    const result = queue.then(run, run);
    queue = result.catch(() => undefined);
    return result;
}

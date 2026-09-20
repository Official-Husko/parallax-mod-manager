import type {library} from '../../wailsjs/go/models';
import {displayVersion} from './versionCompat';

// How many changed mods to name before summarising the rest as "+N more" -
// a big modlist update can touch dozens, and a notification isn't the place
// to list them all (the resolver's per-key notes name the exact ones).
const maxNamedMods = 3;

// True when a generated patch exists and something about it is worth telling
// the user over: covered conflicts whose sources changed, conflicts it doesn't
// cover, or covered keys that no longer conflict. The quick scan summary that
// arrives first has no Patch field at all (it's computed with the conflicts),
// so undefined means "not known yet", never "fine".
export function patchNeedsAttention(p: library.PatchSummary | null | undefined): p is library.PatchSummary {
    return !!p && p.Exists && (p.Changed > 0 || p.New > 0 || p.Obsolete > 0 || p.GameChanged);
}

function plural(n: number, one: string, many: string): string {
    return `${n} ${n === 1 ? one : many}`;
}

// One plain-language sentence saying what's wrong with the patch, shared by
// the Workspace notification and the resolver banner so they can never
// disagree about it.
export function describePatchStatus(p: library.PatchSummary): string {
    const parts: string[] = [];
    if (p.Changed > 0) {
        let text = `${plural(p.Changed, 'conflict', 'conflicts')} changed since it was made`;
        const mods = p.ChangedMods ?? [];
        if (mods.length > 0) {
            const shown = mods.slice(0, maxNamedMods).join(', ');
            text += mods.length > maxNamedMods ? ` (${shown} +${mods.length - maxNamedMods} more)` : ` (${shown})`;
        }
        parts.push(text);
    }
    if (p.New > 0) {
        parts.push(`${plural(p.New, 'new conflict', 'new conflicts')} it doesn't cover`);
    }
    if (p.Obsolete > 0) {
        parts.push(`${plural(p.Obsolete, 'patched key', 'patched keys')} no longer ${p.Obsolete === 1 ? 'conflicts' : 'conflict'}`);
    }
    if (p.GameChanged) {
        parts.push(`it was made for ${displayVersion(p.GeneratedForVersion)} and the game is now ${displayVersion(p.GameVersion)}`);
    }
    return `Your generated patch is out of date: ${parts.join(', ')}.`;
}

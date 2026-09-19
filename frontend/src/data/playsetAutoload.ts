import type {preferences} from '../../wailsjs/go/models';

// Mirrors preferences.Preferences' own PlaysetAutoloadModes/
// PlaysetAutoloadCustom fields - stored as plain map[string]string on the
// Go side (see that type's own comment for why), so there's no generated
// binding to import here. Shared between Settings' Playsets panel (which
// writes these) and Workspace (which reads them on every game switch), so
// the two can never disagree about what an unset or "off"/"last"/"custom"
// value actually means.
export type PlaysetAutoloadMode = 'off' | 'last' | 'custom';

export function playsetAutoloadModeFor(prefs: preferences.Preferences | null | undefined, gameId: string): PlaysetAutoloadMode {
    const mode = prefs?.playsetAutoloadModes?.[gameId];
    return mode === 'off' || mode === 'custom' ? mode : 'last';
}

// Resolves which playset name (if any) should be auto-loaded for gameId
// right now, given mode. Doesn't check whether that name still actually
// exists - the caller (Workspace's mount effect) re-verifies against a
// fresh ListPlaysets result first, since a remembered/pinned name can go
// stale (renamed or deleted) independently of this preference.
export function playsetAutoloadTarget(prefs: preferences.Preferences | null | undefined, gameId: string): string | undefined {
    const mode = playsetAutoloadModeFor(prefs, gameId);
    if (mode === 'off') return undefined;
    if (mode === 'custom') return prefs?.playsetAutoloadCustom?.[gameId] || undefined;
    return prefs?.lastActivePlaysets?.[gameId] || undefined;
}

// Compares a mod's own declared supported_version wildcard pattern
// against the real, currently-installed game version (Workspace.tsx's own
// gameVersion, fetched once per game via GameVersion() - see
// internal/game.GameConfig.GameVersion) - a bare string comparison, so
// this stays a plain client-side function rather than a round trip to the
// Go backend for something this cheap.
//
// Confirmed wildcard format against real, currently-installed Stellaris
// mods (both from mods_registry.json/launcher-v2.sqlite and read directly
// out of real descriptor.mod files): "*" (any version), "v4.0.*" and
// "4.0.*" (major.minor pinned, patch wildcarded - the leading "v" is
// present in some real mods and absent in others for the exact same
// pattern), "v*.*.*" (every segment wildcarded), "v4.*" (only major
// pinned). A pattern segment matches the current version's corresponding
// segment exactly, or short-circuits as compatible the moment it hits a
// "*" - matching how a real, independently-verified compatibility checker
// for this same file format behaves, not a guess.

export interface VersionCompatibility {
    // Only meaningful when known is true.
    compatible: boolean;
    // false when either string is empty, or the pattern pins a segment
    // (e.g. a patch level) the current version doesn't have enough
    // segments to confirm or deny - deliberately distinct from
    // compatible: false, since "can't tell" must never be shown as
    // "known bad."
    known: boolean;
}

const VERSION_SEGMENT_PATTERN = /\d+|\*/g;

function versionSegments(v: string): string[] {
    return v.trim().toLowerCase().replace(/^v/, '').match(VERSION_SEGMENT_PATTERN) ?? [];
}

// displayVersion strips a leading "v"/"V" for display - confirmed some
// real mods declare supported_version as "v4.4.*" and others as "4.4.*"
// for the exact same pattern (see this file's own top comment), which
// looks inconsistent shown side by side in the same list unless
// normalized to one form. The real installed game version (from
// GameVersion()) always carries the "v" prefix, so this applies there
// too wherever it's shown alongside a mod's own version.
export function displayVersion(v: string): string {
    return v.replace(/^v/i, '');
}

export function checkVersionCompatibility(supportedVersion: string, currentVersion: string): VersionCompatibility {
    const required = versionSegments(supportedVersion);
    const current = versionSegments(currentVersion);
    if (required.length === 0 || current.length === 0) {
        return {compatible: false, known: false};
    }
    for (let i = 0; i < required.length; i++) {
        if (required[i] === '*') {
            return {compatible: true, known: true};
        }
        if (i >= current.length) {
            return {compatible: false, known: false};
        }
        if (required[i] !== current[i]) {
            return {compatible: false, known: true};
        }
    }
    return {compatible: true, known: true};
}

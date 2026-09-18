// Legacy read path for which registered games the user chose to manage.
// This used to be the only place that state lived (localStorage, written
// once by the first-run wizard); it now lives in the backend as
// preferences.Preferences.ManagedGames, kept in sync live from the Manage
// Games settings panel. app.tsx reads this once, on first load after an
// upgrade, to migrate an existing selection into the backend preference -
// see its loadGames(). Nothing writes to this key anymore.
const MANAGED_GAMES_KEY = 'parallax-managed-games';

export function getLegacyManagedGames(): string[] | null {
    try {
        const raw = localStorage.getItem(MANAGED_GAMES_KEY);
        if (!raw) {
            return null;
        }
        const parsed = JSON.parse(raw);
        return Array.isArray(parsed) ? parsed : null;
    } catch {
        return null;
    }
}

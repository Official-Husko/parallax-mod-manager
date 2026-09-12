// Which registered games the user has chosen for Parallax Mod Manager to
// actually manage - picked once in the first-run wizard, changeable there
// again later. A frontend-only preference (no backend settings store exists
// yet) so it lives in localStorage, same as the onboarding flag.
const MANAGED_GAMES_KEY = 'parallax-managed-games';

export function getManagedGames(): string[] | null {
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

export function setManagedGames(keys: string[]) {
    try {
        localStorage.setItem(MANAGED_GAMES_KEY, JSON.stringify(keys));
    } catch {
        // Private browsing / blocked storage - the selection just won't
        // persist across restarts, which is a harmless degradation here.
    }
}

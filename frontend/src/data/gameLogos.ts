import stellarisLogo from '../assets/game_media/logo/stellaris.png';

// Per-game logo art, keyed by the same GameConfig.Key the backend uses.
// Only what's actually been dropped into assets/game_media/logo/ is listed
// here - a game without an entry falls back to a plain color swatch.
export const gameLogos: Record<string, string> = {
    stellaris: stellarisLogo,
};

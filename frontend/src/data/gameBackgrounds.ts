// Every background image bundled under assets/game_media/background/<gameID>/
// (frontend-only art, separate from the backend's own embed-plus-override
// internal/gamemedia store - that one resolves a single logo per game, not
// a whole rotating pool) - grouped by the game ID folder it lives in. A
// game with no folder here just has an empty pool; callers treat that as
// "no background art yet," not an error - see components/AppBackground.tsx.
const modules = import.meta.glob<string>(
    '../assets/game_media/background/*/*.{jpg,jpeg,png,webp,JPG,JPEG,PNG,WEBP}',
    {eager: true, as: 'url'},
);

const byGame = new Map<string, string[]>();
for (const [path, url] of Object.entries(modules)) {
    const gameId = path.match(/background\/([^/]+)\//)?.[1];
    if (!gameId) continue;
    const pool = byGame.get(gameId);
    if (pool) pool.push(url); else byGame.set(gameId, [url]);
}

export function backgroundsForGame(gameId: string): string[] {
    return byGame.get(gameId) ?? [];
}

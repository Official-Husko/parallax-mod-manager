import './Library.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DetectGames, ModSizes, ScanGame} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {GameLogo} from '../components/GameLogo';
import {libCollections} from '../data/mockData';

type LoadState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; games: library.DetectedGame[] };

type Row = {
    modId: string;
    name: string;
    version: string;
    source: string;
    gameId: string;
    gameName: string;
};

export function Library() {
    const [state, setState] = useState<LoadState>({kind: 'loading'});
    const [rows, setRows] = useState<Row[]>([]);
    const [sizes, setSizes] = useState<Record<string, number>>({});
    const [selectedGame, setSelectedGame] = useState('');
    const [search, setSearch] = useState('');

    useEffect(() => {
        let cancelled = false;
        DetectGames()
            .then((games) => {
                if (cancelled) return;
                setState({kind: 'ready', games});
                // Each game's mod list (fast - just descriptor reads) and its
                // real on-disk sizes (a separate, independent call) are
                // fetched per game rather than awaited all at once, so the
                // table starts filling in as soon as the first game
                // responds instead of waiting on the slowest one.
                for (const g of games) {
                    ScanGame(g.ID, '')
                        .then((summary) => {
                            if (cancelled) return;
                            const gameRows = summary.Mods.map((m): Row => ({
                                modId: m.ID, name: m.Name, version: m.Version, source: m.Source,
                                gameId: g.ID, gameName: g.DisplayName,
                            }));
                            setRows((prev) => [...prev.filter((r) => r.gameId !== g.ID), ...gameRows]);
                        })
                        .catch(() => undefined);
                    ModSizes(g.ID)
                        .then((s) => { if (!cancelled) setSizes((prev) => ({...prev, ...s})); })
                        .catch(() => undefined);
                }
            })
            .catch((err) => { if (!cancelled) setState({kind: 'error', message: String(err)}); });
        return () => { cancelled = true; };
    }, []);

    const games = state.kind === 'ready' ? state.games : [];
    const totalMods = games.reduce((sum, g) => sum + g.ModCount, 0);

    const visibleRows = rows
        .filter((r) => !selectedGame || r.gameId === selectedGame)
        .filter((r) => !search.trim() || r.name.toLowerCase().includes(search.toLowerCase()));

    return (
        <div className="library">
            <div className="library-sidebar">
                <div className="sidebar-label">GAMES</div>
                {state.kind === 'loading' && <p className="status-page">Checking games...</p>}
                {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
                {state.kind === 'ready' && (
                    <>
                        <div
                            className="sidebar-game"
                            style={{background: selectedGame === '' ? '#1e2734' : 'transparent', cursor: 'pointer'}}
                            onClick={() => setSelectedGame('')}
                        >
                            <span className="swatch all-games"><i className="fa-solid fa-layer-group"/></span>
                            <span className="name" style={{color: selectedGame === '' ? 'var(--text-bright)' : 'var(--text-mid)'}}>All games</span>
                            <span className="mono n">{totalMods}</span>
                        </div>
                        {games.map((g) => (
                            <div
                                key={g.ID}
                                className="sidebar-game"
                                style={{background: selectedGame === g.ID ? '#1e2734' : 'transparent', cursor: 'pointer'}}
                                onClick={() => setSelectedGame(g.ID)}
                            >
                                <GameLogo gameId={g.ID} className="swatch"/>
                                <span className="name" style={{color: selectedGame === g.ID ? 'var(--text-bright)' : 'var(--text-mid)'}}>{g.DisplayName}</span>
                                <span className="mono n">{g.ModCount}</span>
                            </div>
                        ))}
                    </>
                )}
                <div className="sidebar-label" style={{marginTop: 18}}>COLLECTIONS</div>
                {libCollections.map((c) => (
                    <div key={c.label} className="sidebar-collection">
                        <span>{c.label}</span><span className="mono n">{c.n}</span>
                    </div>
                ))}
                <div className="sidebar-footer">
                    <div className="mono line">{totalMods} mods across {games.length} game{games.length === 1 ? '' : 's'}</div>
                    <div className="find-unused">Find unused</div>
                </div>
            </div>
            <div className="library-main">
                <div className="library-toolbar">
                    <div className="search-box">
                        <i className="fa-solid fa-magnifying-glass"/>
                        <input
                            placeholder="Search by name..."
                            value={search}
                            onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                        />
                        <span className="kbd">⌘K</span>
                    </div>
                    <div className="spacer"/>
                    <span className="sort-label">Table <i className="fa-solid fa-chevron-down"/></span>
                </div>
                <div className="library-columns mono">
                    <span className="col-check"/>
                    <span className="col-src">SRC</span>
                    <span className="col-name">NAME</span>
                    <span className="col-game">GAME</span>
                    <span className="col-ver">VERSION</span>
                    <span className="col-size">SIZE</span>
                    <span className="col-played">LAST PLAYED</span>
                    <span className="col-state">STATE</span>
                </div>
                <div className="library-rows">
                    {visibleRows.map((r) => (
                        <div key={`${r.gameId}:${r.modId}`} className="library-row">
                            <span className="col-check"><span className="checkbox"/></span>
                            <span className={`col-src src-badge ${r.source === 'workshop' ? 'badge-workshop' : 'badge-local'}`}>
                                {r.source === 'workshop' ? 'W' : 'L'}
                            </span>
                            <span className="col-name name">{r.name}</span>
                            <span className="col-game">
                                <span className="game-name">{r.gameName}</span>
                            </span>
                            <span className="col-ver mono">{r.version || '-'}</span>
                            <span className="col-size mono">{r.modId in sizes ? formatBytes(sizes[r.modId]) : '-'}</span>
                            <span className="col-played">-</span>
                            <span className="col-state mono">-</span>
                        </div>
                    ))}
                    {state.kind === 'ready' && visibleRows.length === 0 && (
                        <p className="status-page">No mods found.</p>
                    )}
                </div>
                <div className="library-footer">
                    <span className="mono">{visibleRows.length} shown</span>
                    <span className="link-btn" style={{color: 'var(--text-muted)'}}>Add to playset</span>
                    <span className="link-btn" style={{color: 'var(--text-muted)'}}>Move to collection</span>
                    <span className="link-btn" style={{color: 'var(--red)'}}>Uninstall</span>
                </div>
            </div>
        </div>
    );
}

function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = n / 1024;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value.toFixed(value < 10 ? 2 : 1)} ${units[unit]}`;
}

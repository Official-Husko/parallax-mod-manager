import './BackgroundDownloadModal.css';
import {Fragment, h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {
    BackgroundCatalog,
    CancelBackgroundDownload,
    ListGames,
    OpenPath,
    RemoveBackgroundPack,
    StartBackgroundDownload,
} from '../../wailsjs/go/main/App';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import type {app} from '../../wailsjs/go/models';
import {EmptyState} from '../components/EmptyState';
import {ModalHeader} from '../components/ModalHeader';
import {formatBytes} from '../data/format';
import {notify} from '../data/notifications';

// One snapshot of a running download - backgrounds.Progress on the Go side. Written
// out here because events are not part of the generated bindings.
interface DownloadProgress {
    State: 'running' | 'done' | 'cancelled';
    GameID: string;
    File: string;
    FilesDone: number;
    FilesTotal: number;
    BytesDone: number;
    BytesTotal: number;
    Skipped: number;
    Failed: number;
    BytesPerSecond: number;
}

type Phase = 'loading' | 'choose' | 'downloading' | 'finished' | 'stopped';

function formatDuration(seconds: number): string {
    if (!Number.isFinite(seconds) || seconds < 0) return '';
    if (seconds < 60) return `${Math.max(1, Math.round(seconds))}s`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ${Math.round(seconds % 60)}s`;
    return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

// The window for getting the background images onto this computer, opened from
// Settings > Appearance. In "switch" mode it is the step after choosing Offline:
// pick which games to download, watch it happen, and only then does the app go
// offline - declining leaves it online. In "manage" mode (already offline) it tops
// up or removes downloads and never changes the source.
//
// onClose says whether to switch to offline; it is always false in manage mode.
export function BackgroundDownloadModal({mode, onClose}: {
    mode: 'switch' | 'manage';
    onClose: (result: { useOffline: boolean }) => void;
}) {
    const [phase, setPhase] = useState<Phase>('loading');
    const [catalog, setCatalog] = useState<app.BackgroundCatalog | null>(null);
    const [names, setNames] = useState<Record<string, string>>({});
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [progress, setProgress] = useState<DownloadProgress | null>(null);
    const [confirmRemove, setConfirmRemove] = useState('');
    const phaseRef = useRef<Phase>('loading');
    phaseRef.current = phase;

    async function load(refresh: boolean, keepSelection: boolean) {
        setPhase('loading');
        const cat = await BackgroundCatalog(refresh);
        setCatalog(cat);
        setSelected((prev) => {
            const wanted = (p: app.BackgroundPack) => p.Files > 0 && p.MissingFiles > 0;
            if (keepSelection) return new Set(cat.Packs.filter((p) => wanted(p) && prev.has(p.GameID)).map((p) => p.GameID));
            return new Set(cat.Packs.filter(wanted).map((p) => p.GameID));
        });
        setPhase('choose');
    }

    useEffect(() => {
        ListGames()
            .then((games) => setNames(Object.fromEntries(games.map((g) => [g.ID, g.DisplayName]))))
            .catch(() => undefined);
        load(false, false).catch((err) => {
            notify('error', `Couldn't read the background list: ${String(err)}`);
            onClose({useOffline: false});
        });
        const off = EventsOn('background-download', (p: DownloadProgress) => {
            setProgress(p);
            if (p.State === 'done') {
                setPhase('finished');
                BackgroundCatalog(false).then(setCatalog).catch(() => undefined);
            } else if (p.State === 'cancelled') {
                setPhase('stopped');
                BackgroundCatalog(false).then(setCatalog).catch(() => undefined);
            }
        });
        return () => { off(); };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Escape and a click outside decline - except while downloading, where the Stop
    // button is the only way out.
    function decline() {
        if (phaseRef.current === 'downloading') return;
        onClose({useOffline: false});
    }
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') decline(); };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const packs = catalog?.Packs ?? [];
    const nameOf = (id: string) => names[id] ?? id;
    const chosen = packs.filter((p) => selected.has(p.GameID));
    const toDownloadFiles = chosen.reduce((n, p) => n + p.MissingFiles, 0);
    const toDownloadBytes = chosen.reduce((n, p) => n + p.MissingBytes, 0);
    const haveLocal = packs.some((p) => p.LocalFiles > 0);

    async function start() {
        const ids = chosen.filter((p) => p.MissingFiles > 0).map((p) => p.GameID);
        setProgress(null);
        try {
            await StartBackgroundDownload(ids);
            setPhase('downloading');
        } catch (err) {
            notify('error', `Couldn't start the download: ${String(err)}`);
        }
    }

    async function removePack(id: string) {
        setConfirmRemove('');
        try {
            await RemoveBackgroundPack(id);
            await load(false, true);
        } catch (err) {
            notify('error', `Couldn't remove those images: ${String(err)}`);
        }
    }

    function toggle(id: string) {
        setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id); else next.add(id);
            return next;
        });
    }

    function renderProgress() {
        const p = progress;
        const total = p?.BytesTotal ?? 0;
        const done = p?.BytesDone ?? 0;
        const pct = total > 0 ? Math.min(100, (done / total) * 100) : 0;
        const speed = p?.BytesPerSecond ?? 0;
        const eta = speed > 0 ? formatDuration((total - done) / speed) : '';
        return (
            <div className="bgdl-progress">
                <div className="bgdl-bar"><div className="bgdl-bar-fill" style={{width: `${pct}%`}}/></div>
                <div className="bgdl-progress-line">
                    <span className="mono">{Math.round(pct)}%</span>
                    <span>{p?.FilesDone ?? 0} / {p?.FilesTotal ?? toDownloadFiles} images</span>
                    <span className="mono">{formatBytes(done)} of {formatBytes(total || toDownloadBytes)}</span>
                    {speed > 0 && <span className="mono">{formatBytes(speed)}/s{eta ? ` · ${eta} left` : ''}</span>}
                </div>
                <div className="bgdl-now">
                    {p?.File
                        ? <><i className="fa-solid fa-image"/> <span>{nameOf(p.GameID)}</span> <span className="mono bgdl-now-file">{p.File}</span></>
                        : <span>Starting...</span>}
                </div>
                <div className="bgdl-counts">
                    {(p?.Skipped ?? 0) > 0 && <span>{p!.Skipped} already on disk</span>}
                    {(p?.Failed ?? 0) > 0 && <span className="bad">{p!.Failed} failed</span>}
                </div>
            </div>
        );
    }

    let body;
    if (phase === 'loading') {
        body = <EmptyState icon="fa-spinner fa-spin" title="Looking up the backgrounds..." subtitle={catalog?.Source ? `Reading the list from ${catalog.Source}` : undefined}/>;
    } else if (phase === 'downloading') {
        body = renderProgress();
    } else if (phase === 'finished' || phase === 'stopped') {
        const p = progress;
        body = (
            <div className="bgdl-result">
                <i className={`fa-solid ${phase === 'finished' ? 'fa-circle-check ok' : 'fa-circle-pause warn'}`}/>
                <div className="bgdl-result-title">
                    {phase === 'finished' ? 'Backgrounds downloaded' : 'Download stopped'}
                </div>
                <div className="bgdl-result-detail">
                    {p ? `${p.FilesDone} of ${p.FilesTotal} images (${formatBytes(p.BytesDone)})` : ''}
                    {p && p.Skipped > 0 ? `, ${p.Skipped} were already on disk` : ''}
                    {p && p.Failed > 0 ? `, ${p.Failed} failed - run it again to retry them` : ''}
                    {phase === 'stopped' ? '. What was finished is kept.' : '.'}
                </div>
            </div>
        );
    } else {
        body = (
            <>
                {catalog?.RemoteError && (
                    <div className="bgdl-note warn">
                        <i className="fa-solid fa-cloud-slash"/>
                        <span>Couldn't reach GitHub ({catalog.RemoteError}) - showing only what is already on this computer.</span>
                        <span className="link-btn amber" onClick={() => load(true, true).catch(() => undefined)}>Retry</span>
                    </div>
                )}
                {packs.length === 0 ? (
                    <EmptyState
                        icon="fa-images"
                        title="No backgrounds published yet"
                        subtitle={`Nothing is published at ${catalog?.Source ?? 'the repository'} yet. You can still put your own images in the folder below.`}
                    />
                ) : (
                    <div className="bgdl-packs">
                        {packs.map((p) => {
                            const published = p.Files > 0;
                            const complete = published && p.MissingFiles === 0;
                            return (
                                <div key={p.GameID} className={`bgdl-pack ${published ? '' : 'own'}`}>
                                    <input
                                        type="checkbox"
                                        checked={selected.has(p.GameID)}
                                        disabled={!published || complete}
                                        onChange={() => toggle(p.GameID)}
                                    />
                                    <div className="bgdl-pack-main">
                                        <div className="bgdl-pack-name">{nameOf(p.GameID)}</div>
                                        <div className="mono bgdl-pack-meta">
                                            {published
                                                ? `${p.Files} images · ~${formatBytes(p.Bytes)}`
                                                : 'your own images (not published)'}
                                        </div>
                                    </div>
                                    <div className="bgdl-pack-state">
                                        {p.LocalFiles > 0 && (
                                            <span className={`bgdl-chip ${complete ? 'ok' : 'part'}`}>
                                                <i className={`fa-solid ${complete ? 'fa-circle-check' : 'fa-circle-half-stroke'}`}/>
                                                {complete
                                                    ? `on disk · ${formatBytes(p.LocalBytes)}`
                                                    : `${p.LocalFiles} on disk · ${formatBytes(p.LocalBytes)}`}
                                            </span>
                                        )}
                                        {published && !complete && p.MissingFiles > 0 && (
                                            <span className="mono bgdl-pack-todo">{formatBytes(p.MissingBytes)} to download</span>
                                        )}
                                        {p.LocalFiles > 0 && (
                                            confirmRemove === p.GameID
                                                ? (
                                                    <span className="bgdl-confirm">
                                                        Delete them?
                                                        <span className="link-btn amber" onClick={() => removePack(p.GameID)}>Delete</span>
                                                        <span className="link-btn" onClick={() => setConfirmRemove('')}>Keep</span>
                                                    </span>
                                                )
                                                : <span className="bgdl-remove" onClick={() => setConfirmRemove(p.GameID)}>Remove</span>
                                        )}
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                )}
                {catalog?.Folder && (
                    <div className="bgdl-folder">
                        <span>Stored in</span>
                        <span className="mono bgdl-folder-path" title={catalog.Folder}>{catalog.Folder}</span>
                        <span className="bgdl-remove" onClick={() => OpenPath(catalog.Folder).catch(() => undefined)}>Open</span>
                    </div>
                )}
                <div className="bgdl-hint">
                    You can also add your own: put images in a folder named after a game's id inside it.
                </div>
            </>
        );
    }

    let footer;
    if (phase === 'downloading') {
        footer = <span className="btn-ghost" onClick={() => CancelBackgroundDownload()}><i className="fa-solid fa-stop"/> Stop</span>;
    } else if (phase === 'finished') {
        footer = <span className="btn-primary" onClick={() => onClose({useOffline: mode === 'switch'})}>{mode === 'switch' ? 'Use offline' : 'Done'}</span>;
    } else if (phase === 'stopped') {
        footer = (
            <>
                <span className="btn-ghost" onClick={() => onClose({useOffline: false})}>{mode === 'switch' ? 'Stay online' : 'Close'}</span>
                <span className="btn-primary" onClick={start}>Resume</span>
            </>
        );
    } else if (phase === 'choose') {
        footer = (
            <>
                <span className="bgdl-total mono">
                    {chosen.length === 0 ? 'nothing selected'
                        : toDownloadFiles === 0 ? 'nothing new to download'
                            : `${chosen.length} ${chosen.length === 1 ? 'game' : 'games'} · ${toDownloadFiles} images · ~${formatBytes(toDownloadBytes)}`}
                </span>
                <span className="btn-ghost" onClick={() => onClose({useOffline: false})}>{mode === 'switch' ? 'Stay online' : 'Close'}</span>
                {toDownloadFiles > 0 && (
                    <span className="btn-primary" onClick={start}>
                        {mode === 'switch' ? `Download ~${formatBytes(toDownloadBytes)} and go offline` : `Download ~${formatBytes(toDownloadBytes)}`}
                    </span>
                )}
                {toDownloadFiles === 0 && mode === 'switch' && (
                    <span className={`btn-primary ${haveLocal ? '' : 'inert'}`} onClick={haveLocal ? () => onClose({useOffline: true}) : undefined}>
                        Use offline
                    </span>
                )}
            </>
        );
    }

    return (
        <div className="overlay" onClick={decline}>
            <div className="bgdl-modal" onClick={(e) => e.stopPropagation()}>
                <ModalHeader
                    title={mode === 'switch' ? 'Go offline: download backgrounds' : 'Downloaded backgrounds'}
                    icon="fa-cloud-arrow-down" overlay headerClassName="bgdl-header"
                    onClose={phase !== 'downloading' ? decline : undefined} closeTitle="Close (Esc)"
                />
                {phase === 'choose' && mode === 'switch' && (
                    <p className="bgdl-intro">
                        Offline mode uses only images stored on this computer, so nothing is fetched while you use the
                        app. Choose which games to download; you can stay online instead.
                    </p>
                )}
                <div className="bgdl-body">{body}</div>
                <div className="bgdl-footer">{footer}</div>
            </div>
        </div>
    );
}

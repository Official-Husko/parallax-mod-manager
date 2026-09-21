import './BackupPanel.css';
import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {
    BackupOverview,
    BackupStatus,
    BrowseForBackupFolder,
    GetPreferences,
    ListBackups,
    OpenBackupFolder,
    OpenPath,
    SetBackupFolder,
    SetBackupMode,
} from '../../wailsjs/go/main/App';
import type {backup, main, preferences} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {backupReasonLabel} from '../data/backups';
import {formatBytes, timeAgo} from '../data/format';
import {notify} from '../data/notifications';
import {GamePickerChips, useManagedGamePicker} from './GamePicker';

type Mode = 'atrisk' | 'all' | 'off';

const MODES: { mode: Mode; name: string; recommended?: boolean; desc: string }[] = [
    {
        mode: 'atrisk',
        name: 'Deleted and private mods',
        recommended: true,
        desc: 'When a Steam Workshop mod turns out to be deleted or made private, its files are copied at once, '
            + 'before Steam removes them. Costs space only for mods that are really in danger.',
    },
    {
        mode: 'all',
        name: 'Every Workshop mod',
        desc: 'Keeps a copy of every installed Workshop mod and refreshes it when the mod changes. The only '
            + 'way to be certain nothing is lost, and it uses the most space.',
    },
    {
        mode: 'off',
        name: 'Off',
        desc: 'Nothing is backed up on its own. A single mod can still be backed up by hand from its right-click menu.',
    },
];

// How often the status is re-read while the panel is open: copies run in the background.
const REFRESH_MS = 4000;

export function BackupPanel() {
    const [status, setStatus] = useState<main.BackupStatus | null>(null);
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [overview, setOverview] = useState<main.BackupOverview | null>(null);
    const [entries, setEntries] = useState<backup.Entry[]>([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    // Choosing "every mod" shows what it would copy before it starts.
    const [confirmAll, setConfirmAll] = useState(false);
    const alive = useRef(true);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);
    const {state, managedGames, selectedGameId, setSelectedGameId} = useManagedGamePicker(prefs);

    function refreshStatus() {
        BackupStatus().then((s) => alive.current && setStatus(s)).catch(() => undefined);
    }

    function refreshGame(gameId: string) {
        if (!gameId) return;
        BackupOverview(gameId).then((o) => alive.current && setOverview(o)).catch(() => undefined);
        ListBackups(gameId).then((l) => alive.current && setEntries(l)).catch(() => undefined);
    }

    useEffect(() => {
        alive.current = true;
        refreshStatus();
        return () => { alive.current = false; };
    }, []);

    useEffect(() => {
        setOverview(null);
        setEntries([]);
        refreshGame(selectedGameId);
        const timer = window.setInterval(() => { refreshStatus(); refreshGame(selectedGameId); }, REFRESH_MS);
        const off = EventsOn('backups-changed', () => { refreshStatus(); refreshGame(selectedGameId); });
        return () => {
            window.clearInterval(timer);
            off();
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGameId]);

    if (!status) return <div className="settings-content single"/>;

    async function run(work: () => Promise<main.BackupStatus>) {
        setBusy(true);
        setError('');
        try {
            const s = await work();
            if (alive.current) setStatus(s);
        } catch (err) {
            setError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            if (alive.current) setBusy(false);
            refreshStatus();
            refreshGame(selectedGameId);
        }
    }

    function pick(mode: Mode) {
        if (busy || mode === status!.Mode) return;
        setConfirmAll(false);
        if (mode === 'all') {
            setConfirmAll(true);
            return;
        }
        run(() => SetBackupMode(mode));
    }

    async function chooseFolder() {
        await run(() => BrowseForBackupFolder());
    }

    const usingDefault = status.CustomPath === '';
    const willNeed = overview ? Math.max(0, overview.WorkshopBytes - overview.BackedUpBytes) : 0;
    const notEnoughRoom = confirmAll && status.FreeBytes > 0 && willNeed > status.FreeBytes;

    return (
        <div className="settings-content wide">
            <div>
                <div className="settings-title">Backup</div>
                <div className="settings-subtitle">
                    When a Steam Workshop mod is deleted, Steam removes its files from your computer on its own
                    schedule and cannot be asked to wait. So the app checks the Workshop regularly and copies a mod
                    the moment it finds out the mod is deleted or private, while the files are still there. The
                    copies are plain 1:1 folders you can put straight back.
                </div>
            </div>

            <div className="mode-option-list backup-modes">
                {MODES.map((m) => {
                    const active = status.Mode === m.mode;
                    return (
                        <div key={m.mode} className={`mode-option ${active ? 'active' : ''} ${busy ? 'disabled' : ''}`} onClick={() => pick(m.mode)}>
                            <i className={`fa-solid ${active ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${active ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">
                                    {m.name}
                                    {m.recommended && <span className="mode-option-recommended">(Recommended)</span>}
                                </div>
                                <div className="mode-option-desc">{m.desc}</div>
                            </div>
                        </div>
                    );
                })}
            </div>

            {confirmAll && (
                <div className="backup-confirm">
                    <span>
                        {overview
                            ? <>This game has {overview.WorkshopMods} Workshop mods ({formatBytes(overview.WorkshopBytes)}); {overview.BackedUpMods > 0 ? `${formatBytes(willNeed)} more would be copied now. ` : 'all of it would be copied now. '}</>
                            : <>Every installed Workshop mod would be copied. </>}
                        Other games add their own. {status.FreeBytes > 0 && <>{formatBytes(status.FreeBytes)} is free in the backup folder.</>}
                        {notEnoughRoom && <strong className="backup-warn"> That is more than the free space: choose another folder first.</strong>}
                    </span>
                    <span className="backup-confirm-actions">
                        <button className="btn-primary" disabled={busy} onClick={() => { setConfirmAll(false); run(() => SetBackupMode('all')); }}>Back up every Workshop mod</button>
                        <button className="btn-ghost" onClick={() => setConfirmAll(false)}>Cancel</button>
                    </span>
                </div>
            )}

            <div className="backup-folder">
                <div className="backup-folder-label"><i className="fa-solid fa-folder-open"/> Backup folder</div>
                <div className="backup-folder-row">
                    <span className="backup-path mono" title={status.Root}>{status.Root || 'No folder'}</span>
                    <button className="btn-ghost" disabled={busy} onClick={chooseFolder}>Change...</button>
                    {!usingDefault && <button className="btn-ghost" disabled={busy} onClick={() => run(() => SetBackupFolder(''))}>Use default</button>}
                    <button className="btn-ghost" disabled={!status.Root} onClick={() => OpenPath(status.Root).catch((e) => notify('error', String(e)))}>Open</button>
                </div>
                <div className="backup-note">
                    {usingDefault ? 'The default folder, in your home folder. ' : 'A folder you chose. '}
                    Each game gets its own folder inside, named by its id: <span className="mono">{'<folder>/<game id>/mods/<item id>'}</span>.
                    Backups already made stay where they are if you change the folder.
                    {status.FreeBytes > 0 && <> {formatBytes(status.FreeBytes)} free.</>}
                </div>
                {error && <div className="backup-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}
            </div>

            <div className="backup-compress">
                <div className="mode-option disabled">
                    <i className="fa-solid fa-square mode-option-radio off"/>
                    <div className="mode-option-main">
                        <div className="mode-option-name">Compress backups <span className="chip">Planned</span></div>
                        <div className="mode-option-desc">
                            Store backups in a compressed archive (7-Zip at maximum compression, or similar) to save space.
                            Still being investigated: backups are plain folders for now.
                        </div>
                    </div>
                </div>
            </div>

            <div className="backup-games">
                {state.kind === 'loading' && <p className="status-page">Checking installed games...</p>}
                {state.kind === 'error' && <p className="status-page error">{state.message}</p>}
                {managedGames.length > 0 && (
                    <GamePickerChips games={managedGames} selectedGameId={selectedGameId} onSelect={setSelectedGameId}/>
                )}
                {selectedGameId && (
                    <div className="backup-summary">
                        {overview
                            ? <>
                                {overview.BackedUpMods} {overview.BackedUpMods === 1 ? 'mod' : 'mods'} backed up ({formatBytes(overview.BackedUpBytes)}) of {overview.WorkshopMods} installed Workshop {overview.WorkshopMods === 1 ? 'mod' : 'mods'} ({formatBytes(overview.WorkshopBytes)}).
                                {overview.AtRiskMods > 0 && <> {overview.AtRiskMods} deleted or private.</>}
                            </>
                            : 'Counting...'}
                        {status.Running && <span className="backup-running"><i className="fa-solid fa-spinner fa-spin"/> Backing up...</span>}
                        {overview && overview.BackedUpMods > 0 && (
                            <span className="link-btn" onClick={() => OpenBackupFolder(selectedGameId).catch((e) => notify('error', String(e)))}>Open this game's folder</span>
                        )}
                    </div>
                )}
                {selectedGameId && entries.length === 0 && overview && (
                    <p className="status-page">No backups for this game yet. That is normal while no mod has been deleted.</p>
                )}
                {entries.length > 0 && (
                    <div className="backup-list">
                        {entries.map((e) => (
                            <div key={e.RemoteFileID} className="backup-row">
                                <span className="backup-name" title={e.Name}>{e.Name || e.RemoteFileID}</span>
                                <span className={`chip backup-reason ${e.Reason}`}>{backupReasonLabel(e.Reason)}</span>
                                {!e.Complete && <span className="chip backup-incomplete" title={`${e.Missing} files were removed while copying`}>Incomplete</span>}
                                <span className="backup-meta mono">{formatBytes(e.Size)}</span>
                                <span className="backup-meta mono" title={new Date(e.BackedUpAt * 1000).toLocaleString()}>{timeAgo(e.BackedUpAt)}</span>
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}

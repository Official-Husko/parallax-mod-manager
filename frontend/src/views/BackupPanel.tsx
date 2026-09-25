import './BackupPanel.css';
import {Fragment, h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {
    BackupOverview,
    BackupStatus,
    BrowseForBackupFolder,
    DeleteBackups,
    GetPreferences,
    ListBackups,
    OpenBackupFolder,
    OpenPath,
    PlanBackupCleanup,
    SetBackupFolder,
    SetBackupLimits,
    SetBackupMode,
} from '../../wailsjs/go/main/App';
import type {app, backup, preferences} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {backupReasonLabel} from '../data/backups';
import {formatBytes, timeAgo} from '../data/format';
import {notify} from '../data/notifications';
import {Select} from '../components/Select';
import {Toggle} from '../components/Toggle';
import {ManagedGamePicker, useManagedGamePicker} from './GamePicker';

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

// Sizes are entered as a number and a unit (1024-based, like every size shown in the app).
type Unit = 'MB' | 'GB' | 'TB';
const UNIT_BYTES: Record<Unit, number> = {MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4};
const UNIT_OPTIONS = (['MB', 'GB', 'TB'] as Unit[]).map((u) => ({value: u, label: u}));

// splitBytes shows a size in the largest unit that keeps it 1 or more ("50 GB", "1.5 TB").
function splitBytes(bytes: number): {value: string; unit: Unit} {
    const unit: Unit = bytes >= UNIT_BYTES.TB ? 'TB' : bytes >= UNIT_BYTES.GB ? 'GB' : 'MB';
    const v = bytes / UNIT_BYTES[unit];
    return {value: String(Math.round(v * 100) / 100), unit};
}

// SizeField is a number and a unit that commit together: on Enter, on leaving the field
// or on choosing another unit - not on every keystroke, so typing "1024" is one change.
function SizeField({bytes, disabled, onCommit}: {bytes: number; disabled?: boolean; onCommit: (bytes: number) => Promise<boolean>}) {
    const [text, setText] = useState('');
    const [unit, setUnit] = useState<Unit>('GB');
    // Shown again whenever the saved size changes (another panel, a rejected value).
    useEffect(() => {
        const s = splitBytes(bytes);
        setText(s.value);
        setUnit(s.unit);
    }, [bytes]);

    function revert() {
        const s = splitBytes(bytes);
        setText(s.value);
        setUnit(s.unit);
    }

    // A value that is not a number, or that the backend refuses, is put back to the saved one.
    async function commit(nextText: string, nextUnit: Unit) {
        const n = Number(nextText);
        if (nextText.trim() === '' || !Number.isFinite(n) || n < 0) {
            revert();
            return;
        }
        const next = Math.round(n * UNIT_BYTES[nextUnit]);
        if (next === bytes) return;
        if (!(await onCommit(next))) revert();
    }

    return (
        <span className={`backup-size ${disabled ? 'disabled' : ''}`}>
            <input
                className="backup-size-input mono"
                type="number"
                min="0"
                step="any"
                value={text}
                disabled={disabled}
                onInput={(e) => setText((e.target as HTMLInputElement).value)}
                onBlur={(e) => commit((e.target as HTMLInputElement).value, unit)}
                onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
            />
            <Select
                className="backup-size-unit"
                value={unit}
                options={UNIT_OPTIONS}
                disabled={disabled}
                onChange={(u) => { setUnit(u as Unit); commit(text, u as Unit); }}
            />
        </span>
    );
}

// What a clean-up asks before it deletes: the plan is shown first.
interface CleanupPlan {
    kind: 'installed' | 'unavailable';
    entries: backup.Entry[];
    bytes: number;
}

export function BackupPanel() {
    const [status, setStatus] = useState<app.BackupStatus | null>(null);
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [overview, setOverview] = useState<app.BackupOverview | null>(null);
    const [entries, setEntries] = useState<backup.Entry[]>([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    // Choosing "every mod" shows what it would copy before it starts.
    const [confirmAll, setConfirmAll] = useState(false);
    // A clean-up that has been planned and is waiting for a yes, and the backup row whose
    // delete is being confirmed.
    const [plan, setPlan] = useState<CleanupPlan | null>(null);
    const [planning, setPlanning] = useState<'installed' | 'unavailable' | null>(null);
    const [cleanupError, setCleanupError] = useState('');
    const [rowDelete, setRowDelete] = useState('');
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

    async function run(work: () => Promise<app.BackupStatus>): Promise<boolean> {
        setBusy(true);
        setError('');
        let ok = true;
        try {
            const s = await work();
            if (alive.current) setStatus(s);
        } catch (err) {
            ok = false;
            setError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            if (alive.current) setBusy(false);
            refreshStatus();
            refreshGame(selectedGameId);
        }
        return ok;
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

    function setLimits(next: {limitEnabled?: boolean; limitBytes?: number; keepFreeEnabled?: boolean; keepFreeBytes?: number}): Promise<boolean> {
        const s = status!;
        return run(() => SetBackupLimits(
            next.limitEnabled ?? s.LimitEnabled,
            next.limitBytes ?? s.LimitBytes,
            next.keepFreeEnabled ?? s.KeepFreeEnabled,
            next.keepFreeBytes ?? s.KeepFreeBytes,
        ));
    }

    async function planCleanup(kind: 'installed' | 'unavailable') {
        setCleanupError('');
        setPlan(null);
        setPlanning(kind);
        try {
            const p = await PlanBackupCleanup(selectedGameId, kind);
            if (alive.current) setPlan({kind, entries: p.Entries, bytes: p.Bytes});
        } catch (err) {
            if (alive.current) setCleanupError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            if (alive.current) setPlanning(null);
        }
    }

    async function deleteBackups(kind: string, ids: string[]) {
        setCleanupError('');
        try {
            const r = await DeleteBackups(selectedGameId, kind, ids);
            notify('success', r.Deleted === 0
                ? 'Nothing was deleted: those backups are no longer candidates.'
                : `Deleted ${r.Deleted} ${r.Deleted === 1 ? 'backup' : 'backups'} and freed ${formatBytes(r.Bytes)}.`);
        } catch (err) {
            setCleanupError(String(err).replace(/^Error:\s*/, ''));
        } finally {
            if (alive.current) {
                setPlan(null);
                setRowDelete('');
            }
            refreshStatus();
            refreshGame(selectedGameId);
        }
    }

    const usingDefault = status.CustomPath === '';
    const willNeed = overview ? Math.max(0, overview.WorkshopBytes - overview.BackedUpBytes) : 0;
    const notEnoughRoom = confirmAll && status.FreeBytes > 0 && willNeed > status.FreeBytes;

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Backup</div>
                <div className="settings-subtitle">
                    When a Steam Workshop mod is deleted, Steam removes its files from your computer on its own
                    schedule and cannot be asked to wait. So the app checks the Workshop regularly and copies a mod
                    the moment it finds out the mod is deleted or private, while the files are still there. The
                    copies are plain 1:1 folders you can put straight back.
                </div>
            </div>

            <div className="settings-columns">
                <div className="settings-column">
                    {status.LimitState !== '' && (
                        <div className="backup-banner">
                            <i className="fa-solid fa-triangle-exclamation"/>
                            <div>
                                <div className="backup-banner-title">Backups are paused</div>
                                <div>
                                    {status.LimitState === 'size'
                                        ? <>The backup size limit ({formatBytes(status.LimitBytes)}) is reached, so no more mods are being backed up. Raise the limit below, or delete backups to make room.</>
                                        : <>Only {formatBytes(status.FreeBytes)} is free on the drive holding the backup folder, and you asked to keep {formatBytes(status.KeepFreeBytes)} free, so no more mods are being backed up. Free some space, lower the amount kept free, or delete backups.</>}
                                </div>
                            </div>
                        </div>
                    )}

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
                            {usingDefault ? 'The default folder, inside the app\'s settings folder (on the same drive, so choose another if that one is small). ' : 'A folder you chose. '}
                            Each game gets its own folder inside, named by its id: <span className="mono">{'<folder>/<game id>/mods/<item id>'}</span>.
                            Backups already made stay where they are if you change the folder.
                            {status.FreeBytes > 0 && <> {formatBytes(status.FreeBytes)} free.</>}
                        </div>
                        {error && <div className="backup-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}
                    </div>

                    <div className="backup-limits">
                        <div className="backup-limits-title"><i className="fa-solid fa-gauge-high"/> Limits</div>
                        <div className="profile-toggle-row backup-limit-row">
                            <span>
                                Limit the total size of all backups
                                <span className="backup-limit-hint">When it is reached no more mods are backed up, and you are told.</span>
                            </span>
                            <span className="backup-limit-controls">
                                <SizeField bytes={status.LimitBytes} disabled={!status.LimitEnabled || busy} onCommit={(b) => setLimits({limitBytes: b})}/>
                                <Toggle on={status.LimitEnabled} onClick={busy ? undefined : () => setLimits({limitEnabled: !status.LimitEnabled})}/>
                            </span>
                        </div>
                        <div className="backup-usage">
                            <div className="backup-usage-track">
                                <div
                                    className={`backup-usage-fill ${status.LimitState === 'size' ? 'full' : ''}`}
                                    style={{width: `${status.LimitEnabled && status.LimitBytes > 0 ? Math.min(100, (status.UsedBytes / status.LimitBytes) * 100) : 0}%`}}
                                />
                            </div>
                            <span className="mono">
                                {formatBytes(status.UsedBytes)} used{status.LimitEnabled ? ` of ${formatBytes(status.LimitBytes)}` : ' (no limit)'}
                            </span>
                        </div>
                        <div className="profile-toggle-row backup-limit-row">
                            <span>
                                Keep free space on the backup drive
                                <span className="backup-limit-hint">No mod is backed up if that would leave less than this free.{status.FreeBytes > 0 && <> {formatBytes(status.FreeBytes)} is free now.</>}</span>
                            </span>
                            <span className="backup-limit-controls">
                                <SizeField bytes={status.KeepFreeBytes} disabled={!status.KeepFreeEnabled || busy} onCommit={(b) => setLimits({keepFreeBytes: b})}/>
                                <Toggle on={status.KeepFreeEnabled} onClick={busy ? undefined : () => setLimits({keepFreeEnabled: !status.KeepFreeEnabled})}/>
                            </span>
                        </div>
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
                        <div className="mode-option disabled">
                            <i className="fa-solid fa-square mode-option-radio off"/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">Upload deleted mods to an archive <span className="chip">Coming soon</span></div>
                                <div className="mode-option-desc">
                                    Automatically send a copy of a mod that was deleted from the Workshop to a public archive, so it is
                                    not lost for everyone. Not built yet: nothing is uploaded.
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
                <div className="settings-column">
                    <div className="backup-games">
                        <ManagedGamePicker state={state} managedGames={managedGames} selectedGameId={selectedGameId} onSelect={setSelectedGameId}/>
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
                        {selectedGameId && (
                            <div className="backup-cleanup">
                                <div className="backup-limits-title"><i className="fa-solid fa-broom"/> Free up space</div>
                                <div className="backup-cleanup-row">
                                    <div>
                                        <div className="backup-cleanup-name">Delete backups of mods still in the Workshop folder</div>
                                        <div className="backup-limit-hint">Those mods are installed and still on the Workshop, so the copy is not needed yet.</div>
                                    </div>
                                    <button className="btn-ghost" disabled={planning !== null || busy} onClick={() => planCleanup('installed')}>
                                        {planning === 'installed' ? 'Checking...' : 'Review...'}
                                    </button>
                                </div>
                                <div className="backup-cleanup-row danger">
                                    <div>
                                        <div className="backup-cleanup-name">Delete backups of unavailable mods <span className="backup-notrec">(Not recommended)</span></div>
                                        <div className="backup-limit-hint">Mods that are deleted or private on the Workshop. These backups may be the only copies left.</div>
                                    </div>
                                    <button className="btn-ghost" disabled={planning !== null || busy} onClick={() => planCleanup('unavailable')}>
                                        {planning === 'unavailable' ? 'Checking...' : 'Review...'}
                                    </button>
                                </div>
                                {plan && (
                                    <div className={`backup-confirm ${plan.kind === 'unavailable' ? 'danger' : ''}`}>
                                        {plan.entries.length === 0 ? (
                                            <span>
                                                {plan.kind === 'installed'
                                                    ? 'Nothing to delete: no backup is of a mod that is installed and still available on the Workshop.'
                                                    : 'Nothing to delete: no backup is of a mod known to be deleted or private.'}
                                            </span>
                                        ) : (
                                            <>
                                                <span>
                                                    {plan.kind === 'unavailable' && <strong className="backup-warn">These may be the only copies of mods that are gone from the Workshop, and this cannot be undone. </strong>}
                                                    {plan.entries.length} {plan.entries.length === 1 ? 'backup' : 'backups'} ({formatBytes(plan.bytes)}) would be deleted:
                                                </span>
                                                <span className="backup-plan-names">
                                                    {plan.entries.slice(0, 6).map((e) => e.Name || e.RemoteFileID).join(', ')}
                                                    {plan.entries.length > 6 && ` and ${plan.entries.length - 6} more`}
                                                </span>
                                                <span className="backup-confirm-actions">
                                                    <button
                                                        className={plan.kind === 'unavailable' ? 'btn-danger' : 'btn-primary'}
                                                        onClick={() => deleteBackups(plan.kind, plan.entries.map((e) => e.RemoteFileID))}
                                                    >
                                                        {plan.kind === 'unavailable' ? 'Delete the only copies' : `Delete ${plan.entries.length} ${plan.entries.length === 1 ? 'backup' : 'backups'}`}
                                                    </button>
                                                    <button className="btn-ghost" onClick={() => setPlan(null)}>Cancel</button>
                                                </span>
                                            </>
                                        )}
                                        {plan.entries.length === 0 && <span className="backup-confirm-actions"><button className="btn-ghost" onClick={() => setPlan(null)}>OK</button></span>}
                                    </div>
                                )}
                                {cleanupError && <div className="backup-error"><i className="fa-solid fa-circle-exclamation"/> {cleanupError}</div>}
                            </div>
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
                                        {rowDelete === e.RemoteFileID ? (
                                            <span className="backup-row-confirm">
                                                Delete this backup?
                                                <button className="btn-danger" onClick={() => deleteBackups('selected', [e.RemoteFileID])}>Delete</button>
                                                <button className="btn-ghost" onClick={() => setRowDelete('')}>Cancel</button>
                                            </span>
                                        ) : (
                                            <i className="fa-solid fa-trash-can backup-row-delete" title="Delete this backup" onClick={() => setRowDelete(e.RemoteFileID)}/>
                                        )}
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
}

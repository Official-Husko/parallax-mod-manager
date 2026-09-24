import {Fragment, h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {CancelPublish, ListModFiles, PreviewModFile, PublishModToWorkshop, SteamAccountInfo} from '../../wailsjs/go/main/App';
import type {app, library} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {Avatar} from '../components/Avatar';
import {FileTree} from '../components/FileTree';

// The Publish tab: uploading a mod to the Steam Workshop as a new item, or pushing an update to
// one already there, with a line-by-line log of what the app and Steam did - see
// internal/workshop and docs/workshop-upload.md for the real mechanism (talking to this
// computer's own, already-running Steam client the same way the game itself does, via
// steam_appid.txt and Steamworks' flat API). There is no separate "sign in to Steam" step of
// this app's own - it uses whichever Steam account is already logged into the local client.

// WorkshopPublishProgress mirrors app.WorkshopPublishProgress's own JSON shape exactly - it is
// only ever an event payload (see PublishModToWorkshop's own "workshop-publish-progress" doc
// comment), never a bound method's return type, so Wails generates no TypeScript type for it.
interface WorkshopPublishProgress {
    Stage: string;
    Message: string;
    Processed: number;
    Total: number;
    PublishedFileID: string;
}

// workshopURL builds the real Workshop page for a published item - the same URL "done"'s own log
// line already links to.
function workshopURL(id: string): string {
    return `https://steamcommunity.com/sharedfiles/filedetails/?id=${id}`;
}

type LogTone = 'info' | 'success' | 'warn' | 'error';

interface LogLine {
    time: string;
    tone: LogTone;
    text: string;
}

function timestamp(): string {
    return new Date().toLocaleTimeString([], {hour: '2-digit', minute: '2-digit', second: '2-digit'});
}

function formatBytes(n: number): string {
    if (n <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    let value = n;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value.toFixed(unit === 0 || value >= 10 ? 0 : 1)} ${units[unit]}`;
}

// describeStage turns one raw progress event into the log line shown for it, so the wording
// lives in one place rather than scattered through the event handler below. null means the
// stage isn't worth its own log line.
function describeStage(p: WorkshopPublishProgress): { tone: LogTone; text: string } | null {
    switch (p.Stage) {
        case 'staging':
            return {tone: 'info', text: p.Message ? `Preparing files to upload (${p.Message})...` : 'Preparing files to upload...'};
        case 'opening':
            return {tone: 'info', text: 'Connecting to Steam...'};
        case 'initialized':
            return {tone: 'info', text: 'Connected to Steam.'};
        case 'creating':
            return {tone: 'info', text: 'Creating a new Workshop item...'};
        case 'created':
            return {tone: 'info', text: `Workshop item created (id ${p.PublishedFileID}).`};
        case 'updating':
            return {tone: 'info', text: 'Preparing the update...'};
        case 'uploading':
            return {
                tone: 'info',
                text: p.Total > 0 ? `Uploading to Steam... (${formatBytes(p.Processed)} / ${formatBytes(p.Total)})` : 'Uploading to Steam...',
            };
        case 'done':
            return {tone: 'success', text: `Published. View it at steamcommunity.com/sharedfiles/filedetails/?id=${p.PublishedFileID}`};
        case 'error':
            return {tone: 'error', text: p.Message || 'Something went wrong.'};
        default:
            return null;
    }
}

export function EditorPublish({gameId, mod}: { gameId: string; mod: library.ModSummary }) {
    const hasRemote = !!mod.RemoteFileID;
    const [changeNote, setChangeNote] = useState('');
    const [visibility, setVisibility] = useState<'public' | 'friendsOnly' | 'unlisted' | 'private'>('private');
    const [publishing, setPublishing] = useState(false);
    const [confirmingCancel, setConfirmingCancel] = useState(false);
    const [log, setLog] = useState<LogLine[]>([]);
    const [error, setError] = useState('');
    const [progress, setProgress] = useState<{ processed: number; total: number } | null>(null);
    const [files, setFiles] = useState<library.ModFiles | null>(null);
    const [filesError, setFilesError] = useState('');
    // Which relative paths the person chose to leave out - see
    // components/FileTree's own SelectionProps for exactly what excluding a
    // folder does to everything under it.
    const [excluded, setExcluded] = useState<Set<string>>(new Set());
    const [selectedPath, setSelectedPath] = useState('');
    const [preview, setPreview] = useState<app.FilePreview | null>(null);
    const [account, setAccount] = useState<app.SteamAccountInfo | null>(null);
    const [accountError, setAccountError] = useState('');
    const requestIdRef = useRef('');

    // A different mod selected - this tab's own log/draft/exclusions belong to
    // whichever mod was open when they were set, never carried over to a
    // different one.
    useEffect(() => {
        setLog([]);
        setError('');
        setChangeNote('');
        setExcluded(new Set());
        setFiles(null);
        setFilesError('');
        setSelectedPath('');
        setPreview(null);
        ListModFiles(gameId, mod.ID)
            .then(setFiles)
            .catch((err) => setFilesError(String(err)));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [mod.ID]);

    useEffect(() => {
        setAccount(null);
        setAccountError('');
        SteamAccountInfo(gameId)
            .then(setAccount)
            .catch((err) => setAccountError(String(err)));
    }, [gameId]);

    function toggleExcluded(relPath: string) {
        setExcluded((prev) => {
            const next = new Set(prev);
            if (next.has(relPath)) next.delete(relPath); else next.add(relPath);
            return next;
        });
    }

    function selectFile(relPath: string) {
        setSelectedPath(relPath);
        setPreview(null);
        PreviewModFile(gameId, mod.ID, relPath).then(setPreview).catch(() => setPreview(null));
    }

    useEffect(() => {
        const off = EventsOn('workshop-publish-progress', (eventGameId: string, p: WorkshopPublishProgress) => {
            if (eventGameId !== gameId) return;
            if (p.Stage === 'uploading') setProgress({processed: p.Processed, total: p.Total});
            const line = describeStage(p);
            if (!line) return;
            setLog((prev) => [...prev, {time: timestamp(), tone: line.tone, text: line.text}]);
        });
        return () => off();
    }, [gameId]);

    function publish() {
        const requestId = `publish-${Date.now()}-${Math.random().toString(36).slice(2)}`;
        requestIdRef.current = requestId;
        setPublishing(true);
        setError('');
        setProgress(null);
        setLog([{
            time: timestamp(),
            tone: 'info',
            text: hasRemote ? `Updating Workshop item ${mod.RemoteFileID}...` : 'Starting a new Workshop upload...',
        }]);
        PublishModToWorkshop(gameId, mod.ID, requestId, {
            ItemID: mod.RemoteFileID,
            Title: mod.Name,
            Description: mod.ShortDescription,
            ChangeNote: changeNote,
            Visibility: visibility,
            ExcludePaths: Array.from(excluded),
        } as unknown as app.WorkshopPublishRequest)
            .catch((err) => {
                const text = String(err);
                setError(text);
                setLog((prev) => [...prev, {time: timestamp(), tone: 'error', text}]);
            })
            .finally(() => {
                setPublishing(false);
                setProgress(null);
                requestIdRef.current = '';
            });
    }

    function confirmCancel() {
        setConfirmingCancel(false);
        if (requestIdRef.current) CancelPublish(requestIdRef.current);
    }

    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-card">
                    <div className="editor-card-title">STEAM ACCOUNT</div>
                    {account ? (
                        <div className="publish-account-row">
                            <Avatar name={account.PersonaName || '?'} size={32}/>
                            <div>
                                <div className="publish-account-name">{account.PersonaName || 'Signed in'}</div>
                                <div className="editor-muted">Signed in to the Steam client. Parallax uses that session.</div>
                            </div>
                        </div>
                    ) : (
                        <div className="editor-muted">
                            {accountError
                                ? "Couldn't reach Steam - make sure it's open and you're logged in before publishing."
                                : "Uses this computer's own, already-running Steam client - the same one every Workshop upload goes through."}
                        </div>
                    )}
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">DESTINATION</div>
                    <div className="editor-muted" style={{marginBottom: 9}}>Decided automatically by whether this mod already has a Workshop item of its own.</div>
                    <div className={`mode-option ${!hasRemote ? 'active' : 'disabled'}`} style={{cursor: 'default'}}>
                        <i className={`fa-solid ${!hasRemote ? 'fa-circle-dot on' : 'fa-circle off'} mode-option-radio`}/>
                        <div className="mode-option-main">
                            <div className="mode-option-name">Upload as a new Workshop item</div>
                            <div className="mode-option-desc">Creates a brand new Workshop page for this mod.</div>
                        </div>
                    </div>
                    <div className={`mode-option ${hasRemote ? 'active' : 'disabled'}`} style={{cursor: hasRemote ? 'default' : 'default', justifyContent: 'space-between'}}>
                        <i className={`fa-solid ${hasRemote ? 'fa-circle-dot on' : 'fa-circle off'} mode-option-radio`}/>
                        <div className="mode-option-main">
                            <div className="mode-option-name">Update the existing item</div>
                            <div className="mode-option-desc">
                                {hasRemote
                                    ? <>Pushes changes to Workshop item <span className="mono">{mod.RemoteFileID}</span>, this mod's own.</>
                                    : 'This mod has no Workshop item of its own yet - publishing creates one.'}
                            </div>
                        </div>
                        {hasRemote && (
                            <a className="link-btn" href={workshopURL(mod.RemoteFileID)} target="_blank" rel="noreferrer" onClick={(e) => e.stopPropagation()}>
                                Workshop page <i className="fa-solid fa-arrow-up-right-from-square"/>
                            </a>
                        )}
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">DETAILS</div>
                    <label className="editor-field">
                        <span className="editor-label">Change note</span>
                        <textarea
                            className="editor-input"
                            rows={3}
                            disabled={publishing}
                            value={changeNote}
                            onInput={(e) => setChangeNote((e.target as HTMLTextAreaElement).value)}
                            placeholder="What changed in this update? Shown to subscribers on the Workshop page."
                        />
                    </label>
                    <div className="editor-field-row">
                        <label className="editor-field">
                            <span className="editor-label">Visibility</span>
                            <select
                                className="editor-input"
                                disabled={publishing}
                                value={visibility}
                                onChange={(e) => setVisibility((e.target as HTMLSelectElement).value as typeof visibility)}
                            >
                                <option value="public">Public</option>
                                <option value="friendsOnly">Friends only</option>
                                <option value="unlisted">Unlisted</option>
                                <option value="private">Private</option>
                            </select>
                        </label>
                        <label className="editor-field">
                            <span className="editor-label">Tags</span>
                            <input className="editor-input" disabled value={mod.Tags.join(', ') || 'None'}/>
                        </label>
                    </div>
                    <div className="editor-actions">
                        {publishing && (
                            <button type="button" className="btn-ghost" onClick={() => setConfirmingCancel(true)}>Cancel</button>
                        )}
                        <button type="button" className="btn-primary" disabled={publishing} onClick={publish}>
                            {publishing
                                ? <><i className="fa-solid fa-spinner fa-spin"/> Publishing...</>
                                : <><i className="fa-solid fa-cloud-arrow-up"/> Publish</>}
                        </button>
                    </div>
                    {confirmingCancel && (
                        <div className="editor-alert warn">
                            <i className="fa-solid fa-triangle-exclamation editor-alert-icon"/>
                            <div className="editor-alert-body">
                                <div className="editor-alert-title">Cancel this upload?</div>
                                <div className="editor-alert-text">
                                    Steam has no clean way to stop mid-upload - cancelling may leave the Workshop item
                                    partially updated. Check its Workshop page afterward if you're unsure.
                                </div>
                                <div className="editor-alert-actions">
                                    <button type="button" className="btn-ghost" onClick={() => setConfirmingCancel(false)}>Keep going</button>
                                    <button type="button" className="btn-primary" onClick={confirmCancel}>Cancel upload</button>
                                </div>
                            </div>
                        </div>
                    )}
                    {error && (
                        <div className="editor-alert bad">
                            <i className="fa-solid fa-circle-xmark editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{error}</div></div>
                        </div>
                    )}
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">
                        FILES TO UPLOAD{excluded.size > 0 && <span className="editor-card-count mono"> &middot; {excluded.size} excluded</span>}
                    </div>
                    {filesError && (
                        <div className="editor-alert bad">
                            <i className="fa-solid fa-circle-xmark editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{filesError}</div></div>
                        </div>
                    )}
                    {!filesError && !files && <div className="editor-muted">Loading this mod's files...</div>}
                    {!filesError && files && files.Entries.length === 0 && (
                        <div className="editor-muted">This mod has no files of its own yet.</div>
                    )}
                    {!filesError && files && files.Entries.length > 0 && (
                        <>
                            <div className="editor-muted" style={{marginBottom: 6}}>
                                Untick a file or folder to leave it out of the upload - it stays on your computer either way.
                                Click a file to preview it.
                            </div>
                            <div className="publish-file-layout">
                                <div className="editor-file-picker publish-file-picker">
                                    <FileTree
                                        entries={files.Entries}
                                        selection={{excluded, onToggle: (relPath) => toggleExcluded(relPath)}}
                                        onSelectFile={selectFile}
                                        selectedPath={selectedPath}
                                    />
                                </div>
                                <div className="publish-file-preview">
                                    {!selectedPath && <div className="editor-muted">Select a file to preview it.</div>}
                                    {selectedPath && !preview && <div className="editor-muted">Loading preview...</div>}
                                    {selectedPath && preview && preview.Kind === 'image' && (
                                        <>
                                            <img className="publish-file-preview-img" src={preview.DataURI} alt={selectedPath}/>
                                            <div className="mono editor-hint">{selectedPath.split('/').pop()}</div>
                                            <div className="editor-hint">{preview.Width} x {preview.Height} &middot; {formatBytes(preview.Bytes)}</div>
                                        </>
                                    )}
                                    {selectedPath && preview && preview.Kind === 'none' && (
                                        <>
                                            <div className="publish-file-preview-none"><i className="fa-solid fa-file"/></div>
                                            <div className="editor-hint">No preview for this file type.</div>
                                        </>
                                    )}
                                </div>
                            </div>
                            {files.Truncated && (
                                <div className="editor-muted" style={{marginTop: 6}}>
                                    Showing the first {files.Entries.length.toLocaleString()} files - this mod has more than that.
                                </div>
                            )}
                        </>
                    )}
                </div>

                <div className="editor-card editor-changes">
                    <div className="editor-card-title">UPLOAD</div>
                    {progress && progress.total > 0 && (
                        <>
                            <div className="editor-progress-header-row">
                                <span className="mono">{formatBytes(progress.processed)} / {formatBytes(progress.total)}</span>
                                <span className="mono">{Math.round((progress.processed / progress.total) * 100)}%</span>
                            </div>
                            <div className="editor-progress-bar">
                                <div className="editor-progress-fill" style={{width: `${Math.min(100, (progress.processed / progress.total) * 100)}%`}}/>
                            </div>
                        </>
                    )}
                    {log.length === 0 ? (
                        <div className="editor-muted">Nothing uploaded yet. Each step and Steam's own answer appear here as they happen.</div>
                    ) : (
                        <div className="editor-upload-log">
                            {log.map((line, i) => (
                                <div key={i} className={`editor-log-line ${line.tone}`}>
                                    <span className="editor-log-time mono">{line.time}</span>
                                    <span className="editor-log-text">{line.text}</span>
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

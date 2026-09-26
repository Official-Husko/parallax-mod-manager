import {Fragment, h} from 'preact';
import {useCallback, useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {CancelPublish, GetPreferences, ListModFiles, OpenWorkshopPage, PreviewModFile, PublishModToWorkshop, SetPreferences, SteamAccountInfo, WorkshopDetails} from '../../wailsjs/go/main/App';
import type {app, library, preferences} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {Avatar} from '../components/Avatar';
import {FILE_ROW_HEIGHT, FileTreeRows, buildFileTreeRows} from '../components/FileTree';
import {Select} from '../components/Select';
import {useVirtualWindow} from '../data/useVirtualWindow';

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

// isEffectivelyExcluded mirrors components/FileTree's own cascading rule: a file is excluded
// either directly, or because some ancestor folder is - matching exactly what would actually be
// left out of the upload.
function isEffectivelyExcluded(relPath: string, excluded: Set<string>): boolean {
    if (excluded.has(relPath)) return true;
    const parts = relPath.split('/');
    for (let i = 1; i < parts.length; i++) {
        if (excluded.has(parts.slice(0, i).join('/'))) return true;
    }
    return false;
}

// The Visibility field's own options, for the shared Select component - see
// components/Select.tsx (the app's own dropdown; a native <select> draws its
// popup with the webview's own system theme, a white box on this dark UI,
// and can't be restyled at all).
const VISIBILITY_OPTIONS = [
    {value: 'public', label: 'Public'},
    {value: 'friendsOnly', label: 'Friends only'},
    {value: 'unlisted', label: 'Unlisted'},
    {value: 'private', label: 'Private'},
];

type LogTone = 'info' | 'success' | 'warn' | 'error';

interface LogLine {
    time: string;
    tone: LogTone;
    text: string;
}

function timestamp(): string {
    return new Date().toLocaleTimeString([], {hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false});
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

// formatBytesPair renders "142.1 / 208.9 MB" - one unit shown once, at the end, picked from the
// larger (total) value, rather than formatBytes called on each side separately (which would
// print that unit twice, once per side, and could even pick a different one for each).
function formatBytesPair(processed: number, total: number): string {
    const units = ['B', 'KB', 'MB', 'GB'];
    let scale = 1;
    let unit = 0;
    while (total / scale >= 1024 && unit < units.length - 1) {
        scale *= 1024;
        unit++;
    }
    const fmt = (n: number) => (n / scale).toFixed(unit === 0 ? 0 : 1);
    return `${fmt(processed)} / ${fmt(total)} ${units[unit]}`;
}

// describeStage turns one raw progress event into the log line shown for it, so the wording
// lives in one place rather than scattered through the event handler below. null means the
// stage isn't worth its own log line.
function describeStage(p: WorkshopPublishProgress): { tone: LogTone; text: string } | null {
    switch (p.Stage) {
        case 'staging':
            return {tone: 'info', text: p.Message ? `Preparing files to upload (${p.Message})...` : 'Preparing files to upload...'};
        case 'staged':
            return {tone: 'info', text: `Staged ${p.Processed.toLocaleString()} file${p.Processed === 1 ? '' : 's'} to a temporary copy`};
        case 'opening':
            return {tone: 'info', text: 'Connecting to Steam...'};
        case 'initialized':
            return {tone: 'info', text: 'Connected to Steam'};
        case 'creating':
            return {tone: 'info', text: 'Creating a new Workshop item...'};
        case 'created':
            return {tone: 'info', text: `Workshop item created (id ${p.PublishedFileID})`};
        case 'updating':
            return {tone: 'info', text: `Updating item ${p.PublishedFileID}`};
        case 'uploading':
            // Fires repeatedly as bytes stream, not once - live feedback belongs to the
            // progress bar (already fed straight from Processed/Total in the event handler
            // below), not a new log line every tick. loggedUploading gates this to the first
            // one only.
            return {tone: 'info', text: 'Uploading...'};
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
    // The real owner's own SteamID64 for a mod that already has a
    // remote_file_id, from the exact same Workshop details this app already
    // fetches to show subscriber counts and changelogs elsewhere (a cache
    // hit here, not a fresh fetch) - null while unknown, "" if the lookup
    // came back but genuinely found no matching entry. See ownerMismatch
    // below for what this is actually for.
    const [remoteCreator, setRemoteCreator] = useState<string | null>(null);
    // Only for the one-time "Allow / No thanks" prompt below, on this person's first-ever
    // publish - see toolMarkPrompt and internal/toolmark's own doc comment for the feature
    // itself. Fetched once, not per-mod - it's an app-wide setting, not this tab's own draft.
    const [prefs, setPrefs] = useState<preferences.Preferences | null>(null);
    const [toolMarkPrompt, setToolMarkPrompt] = useState(false);
    const requestIdRef = useRef('');
    // 'uploading' fires repeatedly as bytes stream (it drives the live progress bar via
    // setProgress below) - this gates its own log line to the first occurrence only, so the
    // log doesn't fill with one "Uploading..." entry per tick.
    const loggedUploadingRef = useRef(false);
    const logBoxRef = useRef<HTMLDivElement>(null);

    // The log box has its own capped height (see .editor-upload-log) so a long publish never
    // grows the card without bound - this keeps the newest line in view as it grows instead of
    // leaving it scrolled to whatever position it was at when the box first appeared.
    useEffect(() => {
        const box = logBoxRef.current;
        if (box) box.scrollTop = box.scrollHeight;
    }, [log.length]);

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

    // Who actually owns this mod's existing Workshop item, if it has one -
    // Steam itself would refuse an update from any other account anyway,
    // but this catches it before a doomed attempt ever starts (see
    // ownerMismatch below). Skipped entirely for a mod with no
    // remote_file_id - there's nothing to own yet, it can only ever be
    // published as a new item.
    useEffect(() => {
        setRemoteCreator(null);
        if (!hasRemote) return;
        let cancelled = false;
        WorkshopDetails(gameId)
            .then((list) => {
                if (cancelled) return;
                setRemoteCreator(list.find((d) => d.ID === mod.RemoteFileID)?.Creator ?? '');
            })
            .catch(() => undefined); // stays null - never asserted a mismatch on a failed lookup
        return () => { cancelled = true; };
    }, [gameId, mod.RemoteFileID, hasRemote]);

    useEffect(() => {
        GetPreferences().then(setPrefs).catch(() => undefined);
    }, []);

    // True only once both this account's own SteamID and the item's real
    // owner are both actually known and non-empty, and they disagree -
    // never on incomplete data (still loading, or either lookup failed),
    // since the cost of a false positive (blocking a publish that would
    // have worked) is worse than occasionally letting Steam's own real
    // rejection be the one to catch it instead.
    const ownerMismatch = hasRemote && !!account?.SteamID && !!remoteCreator && account.SteamID !== remoteCreator;

    // Stable across renders (see the empty/narrow deps below) specifically so FileTree - wrapped
    // in memo() for exactly this - can skip its own expensive re-render (tens of thousands of
    // rows for a big mod) on a render this component does for an unrelated reason, e.g. a
    // running publish's own progress ticking.
    const toggleExcluded = useCallback((relPath: string) => {
        setExcluded((prev) => {
            const next = new Set(prev);
            if (next.has(relPath)) next.delete(relPath); else next.add(relPath);
            return next;
        });
    }, []);

    const selectFile = useCallback((relPath: string) => {
        setSelectedPath(relPath);
        setPreview(null);
        PreviewModFile(gameId, mod.ID, relPath).then(setPreview).catch(() => setPreview(null));
    }, [gameId, mod.ID]);

    const fileSelection = useMemo(() => ({excluded, onToggle: toggleExcluded}), [excluded, toggleExcluded]);

    // See FileTree.tsx's own comment on buildFileTreeRows/FileTreeRows: windowed the same way,
    // for the same reason (a mod's file list can run into the thousands of entries).
    const fileRows = useMemo(
        () => files ? buildFileTreeRows(files.Entries, fileSelection, selectFile, selectedPath) : [],
        [files, fileSelection, selectFile, selectedPath],
    );
    const fileWindow = useVirtualWindow<HTMLDivElement>(fileRows.length, FILE_ROW_HEIGHT, 15);

    // FILES TO UPLOAD's own "N of Total" badge - walking every entry against isEffectivelyExcluded
    // is real work for a big mod (tens of thousands of files), so this only redoes it when the
    // file list or the exclusion set actually changes, not on every render (selecting a file to
    // preview, or a running publish's own progress ticking, re-renders this component far more
    // often than either of those two actually change).
    const uploadCounts = useMemo(() => {
        if (!files) return null;
        const realFiles = files.Entries.filter((e) => !e.IsDir);
        const included = realFiles.filter((e) => !isEffectivelyExcluded(e.RelPath, excluded)).length;
        return {included, total: realFiles.length};
    }, [files, excluded]);

    useEffect(() => {
        const off = EventsOn('workshop-publish-progress', (eventGameId: string, p: WorkshopPublishProgress) => {
            if (eventGameId !== gameId) return;
            if (p.Stage === 'uploading') {
                setProgress({processed: p.Processed, total: p.Total});
                if (loggedUploadingRef.current) return;
                loggedUploadingRef.current = true;
            }
            const line = describeStage(p);
            if (!line) return;
            setLog((prev) => [...prev, {time: timestamp(), tone: line.tone, text: line.text}]);
        });
        return () => off();
    }, [gameId]);

    function publish() {
        // Belt and braces alongside the disabled button above - Publish
        // should be unreachable already, but never send a doomed update to
        // Steam on its say-so alone.
        if (ownerMismatch) return;
        // First-ever publish (prefs loaded and never yet asked): hold off starting the real
        // upload and ask instead - answerToolMarkPrompt below calls doPublish() itself once
        // answered, so nothing is lost, this click just takes one extra step the first time.
        if (prefs && !prefs.toolMarkPromptShown) {
            setToolMarkPrompt(true);
            return;
        }
        doPublish();
    }

    function answerToolMarkPrompt(allow: boolean) {
        setToolMarkPrompt(false);
        if (!prefs) return;
        const next = {...prefs, shareToolMark: allow, toolMarkPromptShown: true};
        setPrefs(next);
        SetPreferences(next).catch(() => undefined); // never blocks the publish the person just asked for
        doPublish();
    }

    function doPublish() {
        const requestId = `publish-${Date.now()}-${Math.random().toString(36).slice(2)}`;
        requestIdRef.current = requestId;
        loggedUploadingRef.current = false;
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
            .then((result) => {
                // Never blocks or fails the publish itself - the upload already succeeded by
                // this point regardless of whether opening the page works (Steam not
                // installed, no default browser configured, and so on).
                if (result?.PublishedFileID) OpenWorkshopPage(result.PublishedFileID).catch(() => undefined);
            })
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
                            <Avatar name={account.PersonaName || '?'} url={account.AvatarDataURI || undefined} size={32}/>
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
                    <div className="publish-destination-row">
                        <span className="publish-destination-glyph">{hasRemote ? '↻' : '+'}</span>
                        <div className="publish-destination-main">
                            <div className="publish-destination-name">{hasRemote ? 'Update existing item' : 'Upload as a new item'}</div>
                            {hasRemote && <div className="mono publish-destination-sub">remote_file_id {mod.RemoteFileID}</div>}
                        </div>
                        {hasRemote && (
                            <span className="link-btn" onClick={() => OpenWorkshopPage(mod.RemoteFileID).catch(() => undefined)}>
                                Workshop page <i className="fa-solid fa-arrow-up-right-from-square"/>
                            </span>
                        )}
                    </div>
                    <div className="editor-muted">Chosen automatically. A mod without a remote_file_id is published as a new item.</div>
                    {ownerMismatch && (
                        <div className="editor-alert bad">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body">
                                <div className="editor-alert-title">This account can't update this item</div>
                                <div className="editor-alert-text">
                                    This Workshop item belongs to a different Steam account than the one signed
                                    in right now ({account?.PersonaName || 'this one'}) - Steam only lets an
                                    item's own owner push updates to it, so this would fail. Signed-in account:{' '}
                                    <span className="mono">{account?.SteamID}</span>, item's own owner:{' '}
                                    <span className="mono">{remoteCreator}</span>.
                                </div>
                            </div>
                        </div>
                    )}
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
                            <Select
                                value={visibility}
                                options={VISIBILITY_OPTIONS}
                                onChange={(v) => setVisibility(v as typeof visibility)}
                                disabled={publishing}
                            />
                            <span className="editor-hint">Default. Switch to Public when you're ready.</span>
                        </label>
                        <label className="editor-field">
                            <span className="editor-label">Version</span>
                            <input className="editor-input mono" disabled value={mod.Version || '-'}/>
                            <span className="editor-hint">From descriptor</span>
                        </label>
                    </div>
                    <div className="editor-field">
                        <span className="editor-label">Tags</span>
                        {mod.Tags.length > 0 ? (
                            <div className="publish-tags">
                                {mod.Tags.map((tag) => <span key={tag} className="chip">{tag}</span>)}
                            </div>
                        ) : (
                            <span className="editor-muted">None</span>
                        )}
                        <span className="editor-hint">Edit tags on the Edit tab.</span>
                    </div>
                    {!publishing && (
                        <div className="editor-actions">
                            <button
                                type="button"
                                className={`btn-primary ${ownerMismatch ? 'inert' : ''}`}
                                disabled={ownerMismatch}
                                title={ownerMismatch ? "This account doesn't own this Workshop item" : undefined}
                                onClick={publish}
                            >
                                Publish
                            </button>
                        </div>
                    )}
                    {toolMarkPrompt && (
                        <div className="editor-alert info">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body">
                                <div className="editor-alert-title">Note Parallax in this mod?</div>
                                <div className="editor-alert-text">
                                    Add a small PARALLAX_TOOLS.md saying this mod was made with Parallax Mod
                                    Manager, with a link back to the project - it's how other modders find
                                    out about it. Off unless you say yes, and you can change your mind anytime
                                    in Settings &rsaquo; Advanced. This only asks once.
                                </div>
                                <div className="editor-alert-actions">
                                    <button type="button" className="btn-ghost" onClick={() => answerToolMarkPrompt(false)}>No thanks</button>
                                    <button type="button" className="btn-primary" onClick={() => answerToolMarkPrompt(true)}>Allow</button>
                                </div>
                            </div>
                        </div>
                    )}
                    {error && (
                        <div className="editor-alert bad">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{error}</div></div>
                        </div>
                    )}
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">
                        FILES TO UPLOAD
                        {uploadCounts && <span className="editor-card-count mono">{uploadCounts.included} of {uploadCounts.total}</span>}
                    </div>
                    {filesError && (
                        <div className="editor-alert bad">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{filesError}</div></div>
                        </div>
                    )}
                    {!filesError && !files && <div className="editor-muted">Loading this mod's files...</div>}
                    {!filesError && files && files.Entries.length === 0 && (
                        <div className="editor-muted">This mod has no files of its own yet.</div>
                    )}
                    {!filesError && files && files.Entries.length > 0 && (
                        <>
                            <div className="publish-file-layout">
                                <div className="editor-file-picker publish-file-picker" ref={fileWindow.ref} onScroll={fileWindow.onScroll}>
                                    <FileTreeRows rows={fileRows} first={fileWindow.first} last={fileWindow.last}/>
                                </div>
                                <div className="publish-file-preview">
                                    {!selectedPath && <div className="editor-muted">Select a file to preview it.</div>}
                                    {selectedPath && !preview && <div className="editor-muted">Loading preview...</div>}
                                    {selectedPath && preview && preview.Kind === 'image' && (
                                        <>
                                            <img className="publish-file-preview-img" src={preview.DataURI} alt={selectedPath}/>
                                            <div className="mono editor-hint">{selectedPath.split('/').pop()}</div>
                                            <div className="editor-hint">
                                                {preview.Width} x {preview.Height} &middot; {formatBytes(preview.Bytes)}
                                                {selectedPath.split('/').pop() === 'thumbnail.png' ? '. Shown as the Workshop preview image.' : ''}
                                            </div>
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
                            <div className="editor-muted">Unticked files are left out of the upload copy. Nothing is deleted.</div>
                            {files.Truncated && (
                                <div className="editor-muted">
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
                                <span className="mono">{formatBytesPair(progress.processed, progress.total)}</span>
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
                        <div className="editor-upload-log" ref={logBoxRef}>
                            {log.map((line, i) => (
                                <div key={i} className={`editor-log-line ${line.tone}`}>
                                    <span className="editor-log-time mono">{line.time}</span>
                                    <span className="editor-log-text">{line.text}</span>
                                </div>
                            ))}
                        </div>
                    )}
                    {publishing && (
                        <div className="editor-actions">
                            <button type="button" className="btn-ghost" onClick={() => setConfirmingCancel(true)}>Cancel</button>
                            <span className="editor-actions-spacer"/>
                            <button type="button" className="btn-primary inert" disabled>Publishing...</button>
                        </div>
                    )}
                    {confirmingCancel && (
                        <div className="editor-alert warn">
                            <span className="editor-alert-icon"/>
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
                </div>
            </div>
        </div>
    );
}

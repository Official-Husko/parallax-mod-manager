import {h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {
    ModEditInfo,
    ModThumbnail,
    PickThumbnailFile,
    PreviewModEdit,
    PreviewThumbnail,
    SaveModEdit,
    UndoModEdit,
} from '../../wailsjs/go/main/App';
import type {library, main} from '../../wailsjs/go/models';
import {ChipList} from '../components/ChipList';
import {Checkbox} from '../components/Checkbox';
import {diffText} from '../data/editorDiff';
import {draftFromSaved, isChanged, toEdit, unknownDependencies} from '../data/editorDraft';
import type {Draft} from '../data/editorDraft';
import {formatBytes, timeAgo} from '../data/format';
import {notify} from '../data/notifications';
import {checkVersionCompatibility, displayVersion} from '../data/versionCompat';

// The Edit tab: one mod's name, versions, tags, dependencies, replace paths and thumbnail. Every
// change is previewed - the files it would write, line by line - before anything is saved.
export function EditorEdit({gameId, gameVersion, mod, installedNames, initialDraft, onDraft, onSaved}: {
    gameId: string;
    gameVersion: string;
    mod: library.ModSummary;
    // The names of every installed mod, for the dependency suggestions.
    installedNames: string[];
    // The unsaved draft this mod had when it was last looked at, if any.
    initialDraft: Draft | null;
    // Reports the draft after each change (null once there is nothing unsaved).
    onDraft: (draft: Draft | null) => void;
    // Called after a save or an undo, so the mod list can refresh.
    onSaved: () => void;
}) {
    const [info, setInfo] = useState<main.EditInfo | null>(null);
    const [loadError, setLoadError] = useState('');
    const [draft, setDraftState] = useState<Draft | null>(initialDraft);
    const [thumb, setThumb] = useState<string | null>(null);
    const [newThumb, setNewThumb] = useState<main.ThumbnailPreview | null>(null);
    const [preview, setPreview] = useState<main.EditPreview | null>(null);
    const [busy, setBusy] = useState(false);
    // Bumped after a save or undo, so what is on disk is read again.
    const [reload, setReload] = useState(0);

    function setDraft(next: Draft | null) {
        setDraftState(next);
        onDraft(next);
    }

    useEffect(() => {
        let cancelled = false;
        setLoadError('');
        ModEditInfo(gameId, mod.ID)
            .then((i) => {
                if (cancelled) return;
                setInfo(i);
                setDraftState((prev) => prev ?? draftFromSaved(i.Fields));
            })
            .catch((err) => { if (!cancelled) setLoadError(String(err)); });
        ModThumbnail(gameId, mod.ID)
            .then((src) => { if (!cancelled) setThumb(src); })
            .catch(() => { if (!cancelled) setThumb(''); });
        return () => { cancelled = true; };
    }, [gameId, mod.ID, reload]);

    // Resolves the chosen picture into a resized preview - both right after it is picked, and
    // again for a draft (with a picture already chosen) restored by switching back to this mod.
    // The single effect covers both: choosing a picture only ever changes thumbnailFrom (see
    // chooseThumbnail below), so there is exactly one PreviewThumbnail call per picture chosen,
    // not two - resizing a large source image is real, avoidable work to duplicate.
    useEffect(() => {
        let cancelled = false;
        if (!draft?.thumbnailFrom) {
            setNewThumb(null);
            return;
        }
        PreviewThumbnail(draft.thumbnailFrom)
            .then((t) => { if (!cancelled) setNewThumb(t); })
            .catch((err) => {
                if (cancelled) return;
                setNewThumb(null);
                notify('error', String(err));
            });
        return () => { cancelled = true; };
    }, [draft?.thumbnailFrom]);

    const changed = !!info && !!draft && isChanged(draft, info.Fields);

    // What saving would do, worked out a moment after the last change.
    useEffect(() => {
        if (!info || !info.Editable || !draft || !changed) {
            setPreview(null);
            return;
        }
        let cancelled = false;
        const timer = window.setTimeout(() => {
            PreviewModEdit(gameId, mod.ID, toEdit(draft) as unknown as main.ModEdit)
                .then((p) => { if (!cancelled) setPreview(p); })
                .catch((err) => { if (!cancelled) setPreview({Files: [], Problems: [String(err)], Warnings: [], Nothing: true} as unknown as main.EditPreview); });
        }, 300);
        return () => { cancelled = true; window.clearTimeout(timer); };
    }, [info, draft, changed]);

    const installed = useMemo(() => new Set(installedNames), [installedNames]);

    if (loadError) {
        return <div className="editor-empty"><i className="fa-solid fa-triangle-exclamation"/><p>{loadError}</p></div>;
    }
    if (!info || !draft) {
        return <div className="editor-empty"><i className="fa-solid fa-spinner fa-spin"/></div>;
    }

    const readOnly = !info.Editable;
    const problems = preview?.Problems ?? [];
    const canSave = info.Editable && changed && !busy && problems.length === 0 && preview !== null && !preview.Nothing;
    const unknownDeps = new Set(unknownDependencies(draft.dependencies, installed));
    const compat = draft.supportedVersion && gameVersion ? checkVersionCompatibility(draft.supportedVersion.trim(), gameVersion) : null;

    function change(patch: Partial<Draft>) {
        if (readOnly || !draft) return;
        setDraft({...draft, ...patch});
    }

    async function chooseThumbnail() {
        try {
            const path = await PickThumbnailFile();
            if (!path) return;
            // The effect above resolves it into newThumb (and reports an error if the picture
            // cannot be used) - nothing further to do here.
            change({thumbnailFrom: path});
        } catch (err) {
            notify('error', String(err));
        }
    }

    async function save() {
        if (!draft) return;
        setBusy(true);
        try {
            const result = await SaveModEdit(gameId, mod.ID, toEdit(draft) as unknown as main.ModEdit);
            const names = (result.Files ?? []).map((p) => p.split(/[\\/]/).pop());
            notify('success', `Saved '${draft.name.trim() || mod.Name}': ${names.join(', ') || 'nothing needed changing'}.`);
            setDraftState(null);
            onDraft(null);
            setNewThumb(null);
            setReload((n) => n + 1);
            onSaved();
        } catch (err) {
            notify('error', `Couldn't save: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    async function undo() {
        setBusy(true);
        try {
            const result = await UndoModEdit(gameId, mod.ID);
            const names = (result.Files ?? []).map((p) => p.split(/[\\/]/).pop());
            notify('success', `Put back ${names.join(', ')} as they were before the last save.`);
            setDraftState(null);
            onDraft(null);
            setNewThumb(null);
            setReload((n) => n + 1);
            onSaved();
        } catch (err) {
            notify('error', String(err));
        } finally {
            setBusy(false);
        }
    }

    function revert() {
        setDraftState(null);
        onDraft(null);
        setNewThumb(null);
        setReload((n) => n + 1);
    }

    return (
        <div className="editor-columns">
            <div className="editor-column">
                {readOnly && (
                    <div className="editor-readonly">
                        <i className="fa-solid fa-lock"/>
                        <div>
                            <div className="editor-readonly-title">This mod can't be edited here</div>
                            <div>{info.Reason}</div>
                        </div>
                    </div>
                )}

                <div className="editor-card">
                    <div className="editor-card-title">THUMBNAIL</div>
                    <div className="editor-thumb-row">
                        <div className={`editor-thumb ${newThumb ? 'changed' : ''}`}>
                            {newThumb
                                ? <img src={newThumb.DataURI} alt="New thumbnail"/>
                                : thumb
                                    ? <img src={thumb} alt="Current thumbnail"/>
                                    : <span className="editor-thumb-none">{thumb === null ? <i className="fa-solid fa-spinner fa-spin"/> : 'No thumbnail'}</span>}
                        </div>
                        <div className="editor-thumb-side">
                            {newThumb ? (
                                <div className="editor-thumb-note">
                                    New picture: {newThumb.Width} x {newThumb.Height}, {formatBytes(newThumb.Bytes)}
                                    {newThumb.Resized && <> (scaled down from {newThumb.SourceWidth} x {newThumb.SourceHeight})</>}. Saved as thumbnail.png.
                                </div>
                            ) : (
                                <div className="editor-thumb-note">
                                    {thumb ? 'The picture the mod has now.' : 'This mod has no thumbnail. Choose a picture to add one.'}
                                    {' '}Pictures larger than 512 pixels are scaled down when saved.
                                </div>
                            )}
                            <div className="editor-thumb-buttons">
                                <button type="button" className="btn-ghost" disabled={readOnly || busy} onClick={chooseThumbnail}>
                                    <i className="fa-solid fa-image"/> Change...
                                </button>
                                {newThumb && (
                                    <button type="button" className="btn-ghost" disabled={busy} onClick={() => change({thumbnailFrom: ''})}>Keep the current one</button>
                                )}
                            </div>
                        </div>
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">DESCRIPTOR</div>
                    <label className="editor-field">
                        <span className="editor-label">Name</span>
                        <input
                            className={`editor-input ${draft.name.trim() === '' ? 'invalid' : ''}`}
                            value={draft.name}
                            disabled={readOnly}
                            onInput={(e) => change({name: (e.target as HTMLInputElement).value})}
                        />
                        {draft.name.trim() === '' && <span className="editor-hint bad">A mod needs a name.</span>}
                    </label>
                    <div className="editor-field-row">
                        <label className="editor-field">
                            <span className="editor-label">Version</span>
                            <input className="editor-input mono" value={draft.version} disabled={readOnly} placeholder="1.0" onInput={(e) => change({version: (e.target as HTMLInputElement).value})}/>
                        </label>
                        <label className="editor-field">
                            <span className="editor-label">Made for game version</span>
                            <input className="editor-input mono" value={draft.supportedVersion} disabled={readOnly} placeholder="v4.*" onInput={(e) => change({supportedVersion: (e.target as HTMLInputElement).value})}/>
                            {compat && compat.known && (
                                <span className={`editor-hint ${compat.compatible ? 'good' : 'warn'}`}>
                                    {compat.compatible
                                        ? `Matches the installed game (${displayVersion(gameVersion)}).`
                                        : `Built for another version than the installed ${displayVersion(gameVersion)}.`}
                                </span>
                            )}
                        </label>
                    </div>
                    <div className="editor-field">
                        <span className="editor-label">Tags</span>
                        <ChipList id="editor-tags" items={draft.tags} onChange={(tags) => change({tags})} disabled={readOnly} placeholder="Gameplay, Graphics, Fixes..."/>
                    </div>
                    <div className="editor-field">
                        <span className="editor-label">Needs these mods (by name)</span>
                        <ChipList
                            id="editor-deps"
                            items={draft.dependencies}
                            onChange={(dependencies) => change({dependencies})}
                            disabled={readOnly}
                            placeholder="Add a mod's exact name..."
                            suggestions={installedNames}
                            flagged={unknownDeps}
                            flagTitle="No installed mod has this name"
                        />
                    </div>
                    <div className="editor-field">
                        <span className="editor-label">Replaces these folders of the game and other mods</span>
                        <ChipList id="editor-replace" items={draft.replacePaths} onChange={(replacePaths) => change({replacePaths})} disabled={readOnly} mono placeholder="common/buildings"/>
                    </div>
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">WHAT WILL CHANGE</div>

                    {!readOnly && info.CanCreateDescriptor && (
                        <label className="editor-check">
                            <Checkbox checked={draft.createDescriptor} disabled={readOnly} onChange={(createDescriptor) => change({createDescriptor})}/>
                            <span>
                                Also create a <span className="mono">descriptor.mod</span> in the mod's folder. This mod only has the file the game reads;
                                one in the folder is what publishing to the Workshop will use.
                            </span>
                        </label>
                    )}

                    {readOnly && <p className="editor-muted">Nothing can be saved for this mod.</p>}
                    {!readOnly && !changed && <p className="editor-muted">No changes yet. Edit a field or choose a new thumbnail and the files it would write show up here.</p>}

                    {problems.map((p) => <div key={p} className="editor-problem bad"><i className="fa-solid fa-circle-xmark"/> {p}</div>)}
                    {(preview?.Warnings ?? []).map((w) => <div key={w} className="editor-problem warn"><i className="fa-solid fa-triangle-exclamation"/> {w}</div>)}

                    {(preview?.Files ?? []).map((f) => <FileChange key={f.Path} file={f}/>)}

                    {draft.thumbnailFrom && newThumb && (
                        <div className="editor-file">
                            <div className="editor-file-head">
                                <span className="mono">thumbnail.png</span>
                                <span className="editor-file-tag">{thumb ? 'replaced' : 'new file'}</span>
                            </div>
                            <div className="editor-file-note">
                                {newThumb.Width} x {newThumb.Height}, {formatBytes(newThumb.Bytes)}.
                            </div>
                        </div>
                    )}

                    {!readOnly && (
                        <div className="editor-actions">
                            <button type="button" className="btn-primary" disabled={!canSave} onClick={save}>{busy ? 'Saving...' : 'Save changes'}</button>
                            <button type="button" className="btn-ghost" disabled={!changed || busy} onClick={revert}>Revert</button>
                            <span className="editor-actions-spacer"/>
                            <button
                                type="button"
                                className="btn-ghost"
                                disabled={info.HistoryCount === 0 || busy}
                                title={info.HistoryCount > 0 ? `Put the files back as they were before the last save (${timeAgo(info.LastSavedAt)}). ${info.HistoryCount} saves are kept.` : 'Nothing has been saved for this mod yet.'}
                                onClick={undo}
                            >
                                <i className="fa-solid fa-rotate-left"/> Undo last save{info.HistoryCount > 0 ? ` (${info.HistoryCount})` : ''}
                            </button>
                        </div>
                    )}
                    {info.Files.length > 0 && (
                        <div className="editor-files-note">
                            A save changes {info.Files.map((f) => f.Path.split(/[\\/]/).pop()).join(' and ')}
                            {' '}and keeps the earlier {info.Files.length === 1 ? 'version' : 'versions'} in the app's settings folder, never in the mod's own folder.
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

// One file's part of the preview: which file it is and its changed lines.
function FileChange({file}: {file: main.EditPreviewFile}) {
    const name = file.Path.split(/[\\/]/).pop() ?? file.Path;
    const diff = useMemo(() => (file.Changed ? diffText(file.Before, file.After) : null), [file.Before, file.After, file.Changed]);
    const what = file.Kind === 'stub' ? 'the file the game reads' : "in the mod's folder";
    return (
        <div className="editor-file">
            <div className="editor-file-head">
                <span className="mono">{name}</span>
                <span className="editor-file-what">{what}</span>
                {file.Create && <span className="editor-file-tag">new file</span>}
                {!file.Changed && <span className="editor-file-tag same">unchanged</span>}
                {diff && <span className="editor-file-count mono"><span className="add">+{diff.added}</span> <span className="del">-{diff.removed}</span></span>}
            </div>
            {diff && (
                <pre className="editor-diff">
                    {diff.rows.map((row, i) => (
                        <div key={i} className={`editor-diff-row ${row.kind}`}>
                            <span className="editor-diff-sign">{row.kind === 'add' ? '+' : row.kind === 'del' ? '-' : row.kind === 'gap' ? '' : ' '}</span>
                            <span className="editor-diff-text">{row.kind === 'gap' ? '...' : row.text}</span>
                        </div>
                    ))}
                </pre>
            )}
        </div>
    );
}

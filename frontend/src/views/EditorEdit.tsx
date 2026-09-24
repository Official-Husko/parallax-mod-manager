import {Fragment, h} from 'preact';
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
import type {app, library, modedit} from '../../wailsjs/go/models';
import {ChipList} from '../components/ChipList';
import {Checkbox} from '../components/Checkbox';
import {diffText} from '../data/editorDiff';
import {draftFromSaved, isChanged, toEdit, unknownDependencies} from '../data/editorDraft';
import type {Draft} from '../data/editorDraft';
import {formatBytes, timeAgo} from '../data/format';
import {notify} from '../data/notifications';
import {checkVersionCompatibility, displayVersion} from '../data/versionCompat';

// versionString renders one of VersionBumpSuggestion's Options the same way modedit.Version's
// own String() does in Go - Major.Minor, or Major.Minor.Patch once the mod's version has ever
// had a third number.
function versionString(v: modedit.Version): string {
    return v.HadPatch ? `${v.Major}.${v.Minor}.${v.Patch}` : `${v.Major}.${v.Minor}`;
}

const BUMP_KINDS = ['patch', 'minor', 'major'] as const;
const BUMP_LABELS: Record<string, string> = {patch: 'Patch', minor: 'Minor', major: 'Major'};

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
    const [info, setInfo] = useState<app.EditInfo | null>(null);
    const [loadError, setLoadError] = useState('');
    const [draft, setDraftState] = useState<Draft | null>(initialDraft);
    const [thumb, setThumb] = useState<string | null>(null);
    // The current thumbnail's own real pixel size, decoded client-side from thumb (ModThumbnail
    // returns image bytes, not dimensions) - just for "WHAT WILL CHANGE"'s own "512x384, was
    // 1024x768" note; the THUMBNAIL card above has its own, separate "current" description that
    // doesn't need this.
    const [thumbDims, setThumbDims] = useState<{width: number; height: number} | null>(null);
    const [newThumb, setNewThumb] = useState<app.ThumbnailPreview | null>(null);
    const [preview, setPreview] = useState<app.EditPreview | null>(null);
    const [busy, setBusy] = useState(false);
    // Bumped after a save or undo, so what is on disk is read again.
    const [reload, setReload] = useState(0);
    // Set by "Continue anyway" for a mod that is Overridable (a Steam Workshop or Paradox
    // Launcher mod) - unlocks editing for this visit only, never persisted. Naturally resets to
    // false on every mod switch, since Editor.tsx remounts this component per mod.
    const [forced, setForced] = useState(false);

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

    useEffect(() => {
        if (!thumb) { setThumbDims(null); return; }
        let cancelled = false;
        const img = new Image();
        img.onload = () => { if (!cancelled) setThumbDims({width: img.naturalWidth, height: img.naturalHeight}); };
        img.onerror = () => { if (!cancelled) setThumbDims(null); };
        img.src = thumb;
        return () => { cancelled = true; };
    }, [thumb]);

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
    // Editable outright, or Overridable and the person has said Continue anyway for this visit -
    // see modedit.go's editTarget.effectiveEditable, which this mirrors.
    const effectivelyEditable = !!info && (info.Editable || (info.Overridable && forced));

    // What saving would do, worked out a moment after the last change.
    useEffect(() => {
        if (!info || !effectivelyEditable || !draft || !changed) {
            setPreview(null);
            return;
        }
        let cancelled = false;
        const timer = window.setTimeout(() => {
            PreviewModEdit(gameId, mod.ID, {...toEdit(draft), Force: forced} as unknown as app.ModEdit)
                .then((p) => { if (!cancelled) setPreview(p); })
                .catch((err) => { if (!cancelled) setPreview({Files: [], Problems: [String(err)], Warnings: [], Nothing: true} as unknown as app.EditPreview); });
        }, 300);
        return () => { cancelled = true; window.clearTimeout(timer); };
    }, [info, draft, changed, effectivelyEditable, forced]);

    const installed = useMemo(() => new Set(installedNames), [installedNames]);

    if (loadError) {
        return <div className="editor-empty"><i className="fa-solid fa-triangle-exclamation"/><p>{loadError}</p></div>;
    }
    if (!info || !draft) {
        return <div className="editor-empty"><i className="fa-solid fa-spinner fa-spin"/></div>;
    }

    const readOnly = !effectivelyEditable;
    const problems = preview?.Problems ?? [];
    const canSave = effectivelyEditable && changed && !busy && problems.length === 0 && preview !== null && !preview.Nothing;
    const unknownDeps = new Set(unknownDependencies(draft.dependencies, installed));
    const compat = draft.supportedVersion && gameVersion ? checkVersionCompatibility(draft.supportedVersion.trim(), gameVersion) : null;

    function change(patch: Partial<Draft>) {
        if (readOnly || !draft) return;
        const next = {...draft, ...patch};
        setDraftState(next);
        // Report "nothing unsaved" once the draft is back to exactly what is saved - ticking a
        // checkbox (or typing into a field) and then undoing just that one change must never
        // leave the mod list's dot lit for a net-zero edit.
        onDraft(isChanged(next, info.Fields) ? next : null);
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
            const result = await SaveModEdit(gameId, mod.ID, {...toEdit(draft), Force: forced} as unknown as app.ModEdit);
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
        if (!info) return;
        // Nothing on disk changed, so there is no need to ask the backend again - going straight
        // back to a fresh draftFromSaved(info.Fields) (rather than nulling the draft and waiting
        // for a reload to rebuild it) means the fields never disappear behind the "loading"
        // spinner for a frame, which is what caused the flicker.
        setDraftState(draftFromSaved(info.Fields));
        onDraft(null);
        setNewThumb(null);
    }

    return (
        <div className="editor-columns">
            <div className="editor-column">
                {!info.Editable && !forced && (
                    <div className="editor-alert warn">
                        <span className="editor-alert-icon"/>
                        <div className="editor-alert-body">
                            <div className="editor-alert-title">This mod can't be edited here</div>
                            <div className="editor-alert-text">{info.Reason}</div>
                            {info.Overridable && (
                                <div className="editor-alert-actions">
                                    <button type="button" className="btn-ghost" onClick={() => setForced(true)}>Continue anyway</button>
                                </div>
                            )}
                        </div>
                    </div>
                )}
                {!info.Editable && forced && (
                    <div className="editor-alert warn">
                        <span className="editor-alert-icon"/>
                        <div className="editor-alert-body">
                            <div className="editor-alert-title">Editing anyway</div>
                            <div className="editor-alert-text">{info.Reason}</div>
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
                                    <span className="mono">{draft.thumbnailFrom.split(/[\\/]/).pop()}</span>
                                    {' '}&middot;{' '}
                                    {newThumb.Resized
                                        ? <>{newThumb.SourceWidth} x {newThumb.SourceHeight}, scaled down to {newThumb.Width} x {newThumb.Height}</>
                                        : <>{newThumb.Width} x {newThumb.Height}</>}
                                    {' '}&middot; {formatBytes(newThumb.Bytes)}
                                </div>
                            ) : (
                                <div className="editor-thumb-note">
                                    {thumb ? 'The picture the mod has now.' : 'This mod has no thumbnail. Choose a picture to add one.'}
                                    {' '}Pictures larger than 512 pixels are scaled down when saved.
                                </div>
                            )}
                            <div className="editor-thumb-buttons">
                                <button type="button" className="btn-ghost" disabled={readOnly || busy} onClick={chooseThumbnail}>Choose image...</button>
                                {newThumb && (
                                    <button type="button" className="btn-ghost" disabled={busy} onClick={() => change({thumbnailFrom: ''})}>Keep current</button>
                                )}
                            </div>
                        </div>
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">DESCRIPTOR</div>
                    <div className="editor-field-row">
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
                        <label className="editor-field">
                            <span className="editor-label">Version</span>
                            <input className="editor-input mono" value={draft.version} disabled={readOnly} placeholder="1.0" onInput={(e) => change({version: (e.target as HTMLInputElement).value})}/>
                            {draft.version !== info.Fields.Version && <span className="editor-hint">Was {info.Fields.Version || '(none)'}</span>}
                        </label>
                    </div>

                    {info.VersionBump && (
                        <div className="editor-version-bump">
                            <div className="editor-version-bump-head">
                                <span className="editor-version-bump-title">Version bump</span>
                                <span className="editor-version-bump-new">NEW</span>
                            </div>
                            <div className="editor-version-bump-pills">
                                {BUMP_KINDS.map((kind) => {
                                    const optionVersion = versionString(info.VersionBump!.Options[kind]);
                                    const selected = draft.version === optionVersion;
                                    const suggested = info.VersionBump!.Suggested === kind;
                                    return (
                                        <span
                                            key={kind}
                                            className={`chip ${selected ? 'chip-active' : suggested ? 'chip-suggested' : ''}`}
                                            style={{cursor: readOnly ? 'default' : 'pointer', fontWeight: selected ? 600 : undefined}}
                                            onClick={() => change({version: optionVersion})}
                                        >
                                            {BUMP_LABELS[kind]} &middot; {optionVersion}{selected ? ' ✓' : ''}
                                        </span>
                                    );
                                })}
                            </div>
                            <span className="editor-hint">Suggested {BUMP_LABELS[info.VersionBump.Suggested].toLowerCase()}: {info.VersionBump.Reason}</span>
                        </div>
                    )}

                    <div className="editor-field-row">
                        <label className="editor-field">
                            <span className="editor-label">Made for game version</span>
                            <input className="editor-input mono" value={draft.supportedVersion} disabled={readOnly} placeholder="v4.*" onInput={(e) => change({supportedVersion: (e.target as HTMLInputElement).value})}/>
                            {compat && compat.known && (
                                <span className={`editor-hint ${compat.compatible ? 'good' : 'warn'}`}>
                                    {compat.compatible
                                        ? `Matches installed ${displayVersion(gameVersion)}.`
                                        : `Built for another version than the installed ${displayVersion(gameVersion)}.`}
                                </span>
                            )}
                        </label>
                        <label className="editor-field">
                            <span className="editor-label">Folder</span>
                            <input className="editor-input mono" value={info.FolderName} disabled title="Rename by duplicating - see the New tab."/>
                            <span className="editor-hint">Rename by duplicating</span>
                        </label>
                    </div>
                    <div className="editor-field">
                        <span className="editor-label">Tags</span>
                        <ChipList id="editor-tags" items={draft.tags} onChange={(tags) => change({tags})} disabled={readOnly} placeholder="Gameplay, Graphics, Fixes..."/>
                    </div>
                    <div className="editor-field">
                        <span className="editor-label">Dependencies</span>
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
                        {unknownDeps.size > 0 && (
                            <span className="editor-hint warn">
                                No installed mod is named {Array.from(unknownDeps).map((d) => `"${d}"`).join(', ')}.
                            </span>
                        )}
                    </div>
                    <div className="editor-field">
                        <span className="editor-label">Replace paths</span>
                        <ChipList id="editor-replace" items={draft.replacePaths} onChange={(replacePaths) => change({replacePaths})} disabled={readOnly} mono placeholder="common/buildings"/>
                    </div>
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">
                        WHAT WILL CHANGE
                        {preview && !preview.Nothing && <span className="editor-card-count mono">{(preview.Files?.length ?? 0) + (draft.thumbnailFrom && newThumb ? 1 : 0)} files</span>}
                    </div>

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

                    {unknownDeps.size > 0 && (
                        <div className="editor-alert warn">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body">
                                <div className="editor-alert-title">Dependency not installed</div>
                                <div className="editor-alert-text">
                                    Saving is allowed. Players without {Array.from(unknownDeps).join(', ')} will see {unknownDeps.size === 1 ? 'it' : 'them'} flagged in their load order.
                                </div>
                            </div>
                        </div>
                    )}

                    {problems.map((p) => (
                        <div key={p} className="editor-alert bad">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{p}</div></div>
                        </div>
                    ))}
                    {(preview?.Warnings ?? []).map((w) => (
                        <div key={w} className="editor-alert warn">
                            <span className="editor-alert-icon"/>
                            <div className="editor-alert-body"><div className="editor-alert-text">{w}</div></div>
                        </div>
                    ))}

                    {(preview?.Files ?? []).map((f) => <FileChange key={f.Path} file={f}/>)}

                    {draft.thumbnailFrom && newThumb && (
                        <div className="editor-file">
                            <div className="editor-file-head">
                                <span className="mono">thumbnail.png</span>
                                <span className={`editor-file-tag ${thumb ? 'changed' : ''}`}>{thumb ? 'changed' : 'new'}</span>
                                <span className="editor-file-count mono"/>
                                <span className="editor-file-chevron">&#9656;</span>
                            </div>
                            <div className="editor-file-note">
                                {newThumb.Width} x {newThumb.Height}{thumbDims ? `, was ${thumbDims.width} x ${thumbDims.height}` : ''}.
                            </div>
                        </div>
                    )}

                    {!readOnly && (
                        <div className="editor-actions divided">
                            <button type="button" className="btn-ghost" disabled={!changed || busy} onClick={revert}>Revert</button>
                            <button
                                type="button"
                                className="btn-ghost"
                                disabled={info.HistoryCount === 0 || busy}
                                title={info.HistoryCount > 0 ? `Put the files back as they were before the last save (${timeAgo(info.LastSavedAt)}). ${info.HistoryCount} saves are kept.` : 'Nothing has been saved for this mod yet.'}
                                onClick={undo}
                            >
                                &#8630; Undo last save
                            </button>
                            <span className="editor-actions-spacer"/>
                            <button type="button" className="btn-primary" disabled={!canSave} onClick={save}>{busy ? 'Saving...' : 'Save'}</button>
                        </div>
                    )}
                    {info.HistoryCount > 0 && (
                        <div className="editor-files-note">
                            Undo keeps the last {info.MaxHistoryCount} saves. Last saved {timeAgo(info.LastSavedAt)}.
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

// One file's part of the preview: which file it is and its changed lines. Exported for the New
// tab (EditorNew.tsx), which shows the same shape of preview for a mod being created or
// duplicated.
export function FileChange({file}: {file: app.EditPreviewFile}) {
    const name = file.Path.split(/[\\/]/).pop() ?? file.Path;
    const diff = useMemo(() => (file.Changed ? diffText(file.Before, file.After) : null), [file.Before, file.After, file.Changed]);
    const what = file.Kind === 'stub' ? 'the file the game reads' : "in the mod's folder";
    // Each row starts open - the mockup shows a mix of open/collapsed rows, but that is one
    // static screenshot's own illustrative variety, not a "collapse every row but the first"
    // rule worth hard-coding; a real diff is exactly what "what will change" is here for.
    const [open, setOpen] = useState(true);
    return (
        <div className="editor-file">
            <div className="editor-file-head" onClick={diff ? () => setOpen((o) => !o) : undefined} style={{cursor: diff ? 'pointer' : undefined}}>
                <span className="mono">{name}</span>
                <span className="editor-file-what">{what}</span>
                {file.Create && <span className="editor-file-tag">new</span>}
                {!file.Create && file.Changed && <span className="editor-file-tag changed">changed</span>}
                {!file.Changed && <span className="editor-file-tag same">unchanged</span>}
                {diff && (
                    <span className="editor-file-count mono">
                        <span className="add">+{diff.added}</span>
                        {diff.removed > 0 && <span className="del">-{diff.removed}</span>}
                    </span>
                )}
                {diff && <span className="editor-file-chevron">{open ? '▾' : '▸'}</span>}
            </div>
            {diff && open && (
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

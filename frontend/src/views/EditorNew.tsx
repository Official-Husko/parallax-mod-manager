import {Fragment, h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {
    CancelDuplicate,
    CreateMod,
    DuplicateMod,
    NewModLocations,
    PreviewDuplicateMod,
    PreviewNewMod,
} from '../../wailsjs/go/main/App';
import type {app, library} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {ChipList} from '../components/ChipList';
import {Select} from '../components/Select';
import type {SelectOption} from '../components/Select';
import {formatBytes} from '../data/format';
import {notify} from '../data/notifications';
import {FileChange} from './EditorEdit';

// A "duplicate-progress" event's shape - not a Wails-bound method's own parameter or return
// type, so it never gets a generated model (see newmod.go's DuplicateProgress).
interface DuplicateProgressEvent {
    RequestID: string;
    File: string;
    Done: number;
    Total: number;
}

type Mode = 'create' | 'duplicate';

// The New tab: create a brand-new mod from scratch, or - when a mod is already selected -
// duplicate it into a brand-new, independent one instead (a safe way to "edit" a Steam Workshop
// mod without ever touching Steam's own copy). Neither ever feeds the Edit tab's own "Undo last
// save" history - see newmod.go's own comment on why.
export function EditorNew({gameId, selected, onCreated}: {
    gameId: string;
    selected: library.ModSummary | null;
    onCreated: (name: string) => void;
}) {
    const canDuplicate = !!selected && !selected.GeneratedPatch;
    const [mode, setMode] = useState<Mode>('create');
    const [locations, setLocations] = useState<app.NewModLocation[]>([]);
    const [location, setLocation] = useState('');

    const [name, setName] = useState('');
    const [version, setVersion] = useState('');
    const [supportedVersion, setSupportedVersion] = useState('');
    const [tags, setTags] = useState<string[]>([]);

    const [dupName, setDupName] = useState('');

    const [preview, setPreview] = useState<app.EditPreview | null>(null);
    const [dupPreview, setDupPreview] = useState<app.DuplicatePreview | null>(null);
    const [busy, setBusy] = useState(false);
    const [confirmingBig, setConfirmingBig] = useState(false);
    const [progress, setProgress] = useState<DuplicateProgressEvent | null>(null);
    const requestRef = useRef<string | null>(null);

    useEffect(() => {
        setMode('create');
        setName('');
        setVersion('');
        setSupportedVersion('');
        setTags([]);
        setDupName(selected ? `${selected.Name} Copy` : '');
    }, [gameId, selected?.ID]);

    useEffect(() => {
        NewModLocations(gameId)
            .then((found) => {
                setLocations(found);
                const def = found.find((l) => l.Default) ?? found[0];
                setLocation(def?.Path ?? '');
            })
            .catch((err) => notify('error', String(err)));
    }, [gameId]);

    const locationOptions: SelectOption[] = useMemo(
        () => locations.map((l) => ({value: l.Path, label: l.Label, hint: l.Default ? 'default' : undefined})),
        [locations],
    );

    // Create form's live preview.
    useEffect(() => {
        if (mode !== 'create' || !location || !name.trim()) {
            setPreview(null);
            return;
        }
        let cancelled = false;
        const timer = window.setTimeout(() => {
            PreviewNewMod(gameId, {Fields: {Name: name, Version: version, SupportedVersion: supportedVersion, Tags: tags, Dependencies: [], ReplacePaths: []}, Location: location} as unknown as app.NewModRequest)
                .then((p) => { if (!cancelled) setPreview(p); })
                .catch((err) => { if (!cancelled) setPreview({Files: [], Problems: [String(err)], Warnings: [], Nothing: true} as unknown as app.EditPreview); });
        }, 300);
        return () => { cancelled = true; window.clearTimeout(timer); };
    }, [mode, gameId, location, name, version, supportedVersion, tags]);

    // Duplicate form's live preview.
    useEffect(() => {
        if (mode !== 'duplicate' || !selected || !location || !dupName.trim()) {
            setDupPreview(null);
            return;
        }
        let cancelled = false;
        const timer = window.setTimeout(() => {
            PreviewDuplicateMod(gameId, selected.ID, {Name: dupName, Location: location})
                .then((p) => { if (!cancelled) setDupPreview(p); })
                .catch((err) => { if (!cancelled) setDupPreview({Files: [], Problems: [String(err)], Warnings: [], Nothing: true, SourceFiles: 0, SourceBytes: 0, Big: false, FreeAtTarget: -1} as unknown as app.DuplicatePreview); });
        }, 300);
        return () => { cancelled = true; window.clearTimeout(timer); };
    }, [mode, gameId, selected, location, dupName]);

    async function create() {
        setBusy(true);
        try {
            const result = await CreateMod(gameId, {Fields: {Name: name, Version: version, SupportedVersion: supportedVersion, Tags: tags, Dependencies: [], ReplacePaths: []}, Location: location} as unknown as app.NewModRequest);
            const names = (result.Files ?? []).map((p) => p.split(/[\\/]/).pop());
            notify('success', `Created '${name.trim()}': ${names.join(', ')}.`);
            onCreated(name.trim());
        } catch (err) {
            notify('error', `Couldn't create the mod: ${String(err)}`);
        } finally {
            setBusy(false);
        }
    }

    function startDuplicate() {
        if (dupPreview?.Big) {
            setConfirmingBig(true);
            return;
        }
        void runDuplicate();
    }

    async function runDuplicate() {
        setConfirmingBig(false);
        if (!selected) return;
        const requestId = `dup-${Date.now()}-${Math.random().toString(36).slice(2)}`;
        requestRef.current = requestId;
        setProgress({RequestID: requestId, File: '', Done: 0, Total: dupPreview?.SourceBytes ?? 0});
        setBusy(true);
        const off = EventsOn('duplicate-progress', (p: DuplicateProgressEvent) => {
            if (p.RequestID === requestId) setProgress(p);
        });
        try {
            const result = await DuplicateMod(gameId, selected.ID, requestId, {Name: dupName, Location: location});
            const names = (result.Files ?? []).map((p) => p.split(/[\\/]/).pop());
            notify('success', `Duplicated '${selected.Name}' as '${dupName.trim()}': ${names.join(', ')}.`);
            onCreated(dupName.trim());
        } catch (err) {
            notify(String(err).includes('cancelled') ? 'info' : 'error', String(err));
        } finally {
            off();
            requestRef.current = null;
            setProgress(null);
            setBusy(false);
        }
    }

    function cancelDuplicate() {
        if (requestRef.current) CancelDuplicate(requestRef.current);
    }

    const createProblems = preview?.Problems ?? [];
    const canCreate = !!location && name.trim() !== '' && !busy && preview !== null && !preview.Nothing && createProblems.length === 0;

    const dupProblems = dupPreview?.Problems ?? [];
    const canStartDuplicate = !!location && dupName.trim() !== '' && !busy && dupPreview !== null && !dupPreview.Nothing && dupProblems.length === 0;

    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-card">
                    <div className="editor-card-title">MODE</div>
                    <div className="mode-option-list">
                        <div className={`mode-option ${mode === 'create' ? 'active' : ''}`} onClick={() => setMode('create')}>
                            <i className={`fa-solid ${mode === 'create' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'create' ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">Create a new mod</div>
                                <div className="mode-option-desc">Start from a template. Files are written to the folder you pick below.</div>
                            </div>
                        </div>
                        <div
                            className={`mode-option ${mode === 'duplicate' ? 'active' : ''} ${!canDuplicate ? 'disabled' : ''}`}
                            onClick={() => canDuplicate && setMode('duplicate')}
                        >
                            <i className={`fa-solid ${mode === 'duplicate' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'duplicate' ? 'on' : 'off'}`}/>
                            <div className="mode-option-main">
                                <div className="mode-option-name">Duplicate a mod</div>
                                <div className="mode-option-desc">
                                    {canDuplicate ? <>Copy '{selected!.Name}' under a new name.</> : 'Copy an existing mod under a new name. Select a mod in the list to enable this.'}
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">LOCATION</div>
                    <div className="editor-field">
                        <span className="editor-label">Where the mod's own folder goes</span>
                        <Select value={location} options={locationOptions} onChange={setLocation} placeholder="Choose a location..."/>
                    </div>
                </div>

                {mode === 'create' ? (
                    <div className="editor-card">
                        <div className="editor-card-title">DESCRIPTOR</div>
                        <label className="editor-field">
                            <span className="editor-label">Name</span>
                            <input className={`editor-input ${name.trim() === '' ? 'invalid' : ''}`} value={name} onInput={(e) => setName((e.target as HTMLInputElement).value)}/>
                            {name.trim() === '' && <span className="editor-hint bad">A mod needs a name.</span>}
                        </label>
                        <div className="editor-field-row">
                            <label className="editor-field">
                                <span className="editor-label">Version</span>
                                <input className="editor-input mono" value={version} placeholder="1.0" onInput={(e) => setVersion((e.target as HTMLInputElement).value)}/>
                            </label>
                            <label className="editor-field">
                                <span className="editor-label">Made for game version</span>
                                <input className="editor-input mono" value={supportedVersion} placeholder="v4.*" onInput={(e) => setSupportedVersion((e.target as HTMLInputElement).value)}/>
                            </label>
                        </div>
                        <div className="editor-field">
                            <span className="editor-label">Tags</span>
                            <ChipList id="new-mod-tags" items={tags} onChange={setTags} placeholder="Gameplay, Graphics, Fixes..."/>
                        </div>
                    </div>
                ) : (
                    <div className="editor-card">
                        <div className="editor-card-title">DESCRIPTOR</div>
                        <label className="editor-field">
                            <span className="editor-label">Name</span>
                            <input className={`editor-input ${dupName.trim() === '' ? 'invalid' : ''}`} value={dupName} onInput={(e) => setDupName((e.target as HTMLInputElement).value)}/>
                            {dupName.trim() === '' && <span className="editor-hint bad">A mod needs a name.</span>}
                        </label>
                        <p className="editor-muted">Version, tags, dependencies and replace paths are kept exactly as they are on {selected?.Name}.</p>
                    </div>
                )}
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">WHAT WILL BE CREATED</div>

                    {mode === 'create' && (
                        <>
                            {!name.trim() && <p className="editor-muted">Give the mod a name to see what would be created.</p>}
                            {createProblems.map((p) => (
                                <div key={p} className="editor-alert bad">
                                    <i className="fa-solid fa-circle-xmark editor-alert-icon"/>
                                    <div className="editor-alert-body"><div className="editor-alert-text">{p}</div></div>
                                </div>
                            ))}
                            {(preview?.Warnings ?? []).map((w) => (
                                <div key={w} className="editor-alert warn">
                                    <i className="fa-solid fa-triangle-exclamation editor-alert-icon"/>
                                    <div className="editor-alert-body"><div className="editor-alert-text">{w}</div></div>
                                </div>
                            ))}
                            {(preview?.Files ?? []).map((f) => <FileChange key={f.Path} file={f}/>)}
                            <div className="editor-actions">
                                <button type="button" className="btn-primary" disabled={!canCreate} onClick={create}>{busy ? 'Creating...' : 'Create mod'}</button>
                            </div>
                        </>
                    )}

                    {mode === 'duplicate' && selected && (
                        <>
                            {!dupName.trim() && <p className="editor-muted">Give the copy a name to see what would be created.</p>}
                            {dupProblems.map((p) => (
                                <div key={p} className="editor-alert bad">
                                    <i className="fa-solid fa-circle-xmark editor-alert-icon"/>
                                    <div className="editor-alert-body"><div className="editor-alert-text">{p}</div></div>
                                </div>
                            ))}
                            {(dupPreview?.Warnings ?? []).map((w) => (
                                <div key={w} className="editor-alert warn">
                                    <i className="fa-solid fa-triangle-exclamation editor-alert-icon"/>
                                    <div className="editor-alert-body"><div className="editor-alert-text">{w}</div></div>
                                </div>
                            ))}
                            {(dupPreview?.Files ?? []).map((f) => <FileChange key={f.Path} file={f}/>)}
                            {dupPreview && dupPreview.SourceFiles > 0 && (
                                <div className="editor-file-note">
                                    Copies {dupPreview.SourceFiles} file{dupPreview.SourceFiles === 1 ? '' : 's'} ({formatBytes(dupPreview.SourceBytes)}) into the new mod's own folder.
                                    {dupPreview.FreeAtTarget >= 0 && ` ${formatBytes(dupPreview.FreeAtTarget)} free at the target location.`}
                                </div>
                            )}
                            <div className="editor-actions">
                                <button type="button" className="btn-primary" disabled={!canStartDuplicate} onClick={startDuplicate}>Duplicate</button>
                            </div>
                        </>
                    )}
                </div>
            </div>

            {confirmingBig && dupPreview && (
                <div className="overlay">
                    <div className="editor-warn-screen">
                        <i className="fa-solid fa-triangle-exclamation"/>
                        <div className="editor-warn-title">This might take a while</div>
                        <p>
                            This mod is {formatBytes(dupPreview.SourceBytes)} across {dupPreview.SourceFiles} files - duplicating it may take a while.
                            Make sure the target location has enough free space
                            {dupPreview.FreeAtTarget >= 0 && ` (${formatBytes(dupPreview.FreeAtTarget)} free there now)`}.
                        </p>
                        <div className="editor-warn-actions">
                            <button type="button" className="btn-ghost" onClick={() => setConfirmingBig(false)}>Cancel</button>
                            <button type="button" className="btn-primary" onClick={() => void runDuplicate()}>Continue</button>
                        </div>
                    </div>
                </div>
            )}

            {progress && (
                <div className="overlay">
                    <div className="editor-progress-screen">
                        <div className="editor-warn-title">Duplicating {selected?.Name}...</div>
                        <div className="editor-progress-bar">
                            <div className="editor-progress-fill" style={{width: `${progress.Total > 0 ? Math.min(100, (progress.Done / progress.Total) * 100) : 0}%`}}/>
                        </div>
                        <div className="editor-progress-file mono">{progress.File || 'Starting...'}</div>
                        <div className="editor-warn-actions">
                            <button type="button" className="btn-ghost" onClick={cancelDuplicate}>Cancel</button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

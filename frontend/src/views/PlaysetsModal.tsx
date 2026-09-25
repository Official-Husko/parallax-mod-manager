import './PlaysetsModal.css';
import {Fragment, h} from 'preact';
import {useState} from 'preact/hooks';
import {colorFromName} from '../data/nameColor';
import {ModalHeader} from '../components/ModalHeader';
import type {launcherdb} from '../../wailsjs/go/models';

export function PlaysetsModal({gameName, names, launcherPlaysets, onActivate, onNew, onImport, onRename, onDelete, onClose}: {
    gameName: string;
    names: string[];
    // Real playsets found in the Paradox Launcher's own database
    // (launcher-v2.sqlite), read-only - see Workspace.tsx's own fetch and
    // docs/launcher-database.md. Empty when there's nothing to import (no
    // such file, a JSON-launcher-format game, or the real Launcher has
    // simply never been opened against this install) - the section below
    // only renders when this is non-empty.
    launcherPlaysets: launcherdb.Playset[];
    onActivate: (name: string) => void;
    onNew: () => void;
    onImport: (playset: launcherdb.Playset) => void;
    // Rename / delete a saved playset. Each resolves to null on success, or a
    // message to show on that playset's row (a name already taken, say) -
    // this modal never has to know how either is done.
    onRename: (oldName: string, newName: string) => Promise<string | null>;
    onDelete: (name: string) => Promise<string | null>;
    onClose: () => void;
}) {
    // At most one row is being edited or confirmed at a time, so one piece of
    // state each rather than one per row.
    const [renaming, setRenaming] = useState<string | null>(null);
    const [draft, setDraft] = useState('');
    const [confirmingDelete, setConfirmingDelete] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);
    const [rowError, setRowError] = useState<{ name: string; text: string } | null>(null);

    function startRename(name: string) {
        setConfirmingDelete(null);
        setRowError(null);
        setRenaming(name);
        setDraft(name);
    }

    async function commitRename(name: string) {
        const next = draft.trim();
        if (next === name) {
            setRenaming(null);
            return;
        }
        setBusy(true);
        const problem = await onRename(name, next);
        setBusy(false);
        if (problem) {
            setRowError({name, text: problem});
        } else {
            setRenaming(null);
            setRowError(null);
        }
    }

    async function commitDelete(name: string) {
        setBusy(true);
        const problem = await onDelete(name);
        setBusy(false);
        if (problem) {
            setRowError({name, text: problem});
        } else {
            setConfirmingDelete(null);
            setRowError(null);
        }
    }

    return (
        <div className="overlay" onClick={onClose}>
            <div className="playsets-modal" onClick={(e) => e.stopPropagation()}>
                <ModalHeader
                    title="Playsets"
                    after={<>
                        <span className="btn-ghost inert">Import code</span>
                        <span className="btn-primary" onClick={onNew}>New</span>
                    </>}
                >
                    <span className="mono count">{names.length} · {gameName}</span>
                </ModalHeader>
                <div className="playsets-modal-body">
                    {names.length === 0 && <div className="playsets-empty">No saved playsets yet - save one from the Workspace.</div>}
                    {names.map((name) => (
                        <div key={name} className="playset-row">
                            <div className="playset-row-edge" style={{background: colorFromName(name)}}/>
                            <div className="playset-row-main">
                                <div className="playset-row-head">
                                    {renaming === name ? (
                                        <input
                                            className="playset-rename-input"
                                            value={draft}
                                            autoFocus
                                            disabled={busy}
                                            onInput={(e) => { setDraft((e.target as HTMLInputElement).value); setRowError(null); }}
                                            onKeyDown={(e) => {
                                                if (e.key === 'Enter') commitRename(name);
                                                if (e.key === 'Escape') { setRenaming(null); setRowError(null); }
                                            }}
                                        />
                                    ) : (
                                        <span className="name">{name}</span>
                                    )}
                                    {renaming !== name && <span className="mono state">SAVED</span>}
                                </div>
                                {rowError?.name === name && <div className="playset-row-error">{rowError.text}</div>}
                            </div>
                            {renaming === name && (
                                <div className="playset-row-actions">
                                    <span className={`btn-ghost activate ${busy ? 'inert' : ''}`} onClick={busy ? undefined : () => commitRename(name)}>Save name</span>
                                    <span className="btn-ghost" onClick={() => { setRenaming(null); setRowError(null); }}>Cancel</span>
                                </div>
                            )}
                            {confirmingDelete === name && (
                                <div className="playset-row-actions">
                                    <span className="playset-delete-ask">Delete this playset?</span>
                                    <span className={`btn-ghost danger ${busy ? 'inert' : ''}`} onClick={busy ? undefined : () => commitDelete(name)}>Delete</span>
                                    <span className="btn-ghost" onClick={() => { setConfirmingDelete(null); setRowError(null); }}>Keep</span>
                                </div>
                            )}
                            {renaming !== name && confirmingDelete !== name && (
                                <div className="playset-row-actions">
                                    <span className="btn-ghost icon" title="Rename this playset" onClick={() => startRename(name)}>
                                        <i className="fa-solid fa-pen"/>
                                    </span>
                                    <span className="btn-ghost icon" title="Delete this playset" onClick={() => { setRenaming(null); setRowError(null); setConfirmingDelete(name); }}>
                                        <i className="fa-regular fa-trash-can"/>
                                    </span>
                                    <span className="btn-ghost inert">Duplicate</span>
                                    <span className="btn-ghost inert">Share <i className="fa-solid fa-arrow-up-right-from-square"/></span>
                                    <span className="btn-ghost activate" onClick={() => onActivate(name)}>Activate</span>
                                </div>
                            )}
                        </div>
                    ))}
                    {launcherPlaysets.length > 0 && (
                        <div className="playsets-import-section">
                            <div className="section-label">FROM THE PARADOX LAUNCHER</div>
                            {launcherPlaysets.map((p) => (
                                <div key={p.ID} className="playset-row">
                                    <div className="playset-row-edge" style={{background: colorFromName(p.Name)}}/>
                                    <div className="playset-row-main">
                                        <div className="playset-row-head">
                                            <span className="name">{p.Name}</span>
                                            <span className="mono state">{p.Mods.length} mods{p.IsActive ? ' · ACTIVE' : ''}</span>
                                        </div>
                                    </div>
                                    <div className="playset-row-actions">
                                        <span className="btn-ghost activate" onClick={() => onImport(p)}>Import</span>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                    <div className="playset-join-row">
                        <span>Join a friend's playset:</span>
                        <span className="mono code-input">PLLX-XXXX-XXXX</span>
                        <span className="link-btn amber">Resolve</span>
                    </div>
                </div>
            </div>
        </div>
    );
}

import './PurgeEmptyModal.css';
import {h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {FindEmptyMods, PurgeMods} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {Checkbox} from '../components/Checkbox';
import {ModalHeader} from '../components/ModalHeader';
import {notify} from '../data/notifications';

// PurgeEmptyModal reviews every local mod with no usable content (missing
// or empty content folder - see library.FindEmptyMods) before deleting
// anything: every candidate starts checked (included), the user can
// uncheck any they want to keep, and nothing is touched until they
// explicitly confirm. Only ever deletes a mod's small descriptor file,
// never its content folder - see library.PurgeMods.
export function PurgeEmptyModal({gameId, onClose, onPurged}: {
    gameId: string;
    onClose: () => void;
    onPurged: (result: library.PurgeResult) => void;
}) {
    const [candidates, setCandidates] = useState<library.EmptyModCandidate[] | null>(null);
    const [loadError, setLoadError] = useState('');
    const [excluded, setExcluded] = useState<Set<string>>(new Set());
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        let cancelled = false;
        FindEmptyMods(gameId)
            .then((found) => { if (!cancelled) setCandidates(found); })
            .catch((err) => { if (!cancelled) setLoadError(String(err)); });
        return () => { cancelled = true; };
    }, [gameId]);

    const selectedIds = useMemo(
        () => (candidates ?? []).filter((c) => !excluded.has(c.ID)).map((c) => c.ID),
        [candidates, excluded],
    );

    function toggle(id: string) {
        setExcluded((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id); else next.add(id);
            return next;
        });
    }

    async function handleDelete() {
        if (selectedIds.length === 0 || busy) return;
        setBusy(true);
        try {
            const result = await PurgeMods(gameId, selectedIds);
            onPurged(result);
            onClose();
        } catch (err) {
            notify('error', `Couldn't delete the selected mods: ${String(err)}`);
            setBusy(false);
        }
    }

    return (
        <div className="overlay" onClick={onClose}>
            <div className="purge-modal" onClick={(e) => e.stopPropagation()}>
                <ModalHeader title="Purge empty mods" onClose={onClose}>
                    {candidates && <span className="mono count">{candidates.length} found</span>}
                </ModalHeader>
                <p className="purge-modal-intro">
                    These local mods have no real content - their folder is either missing or empty. Deleting
                    one only removes its small descriptor file here; nothing else on disk is touched. Uncheck
                    any you want to keep.
                </p>
                <div className="purge-modal-body">
                    {loadError && <p className="status-page error">{loadError}</p>}
                    {!loadError && candidates === null && <p className="purge-empty">Checking your mods...</p>}
                    {!loadError && candidates?.length === 0 && (
                        <p className="purge-empty">No empty or broken local mods found.</p>
                    )}
                    {candidates?.map((c) => (
                        <div key={c.ID} className={`purge-row ${excluded.has(c.ID) ? 'excluded' : ''}`}>
                            <Checkbox checked={!excluded.has(c.ID)} onChange={() => toggle(c.ID)}/>
                            <div className="purge-row-main">
                                <div className="purge-row-name">{c.Name}</div>
                                <div className="purge-row-reason">{c.Reason}</div>
                            </div>
                        </div>
                    ))}
                </div>
                {candidates !== null && candidates.length > 0 && (
                    <div className="modal-footer purge-modal-footer">
                        <span className="note">{selectedIds.length} of {candidates.length} selected</span>
                        <span className="btn-ghost" onClick={onClose}>Cancel</span>
                        <button className="btn-danger" disabled={selectedIds.length === 0 || busy} onClick={handleDelete}>
                            {busy ? 'Deleting...' : `Delete ${selectedIds.length} mod${selectedIds.length === 1 ? '' : 's'}`}
                        </button>
                    </div>
                )}
            </div>
        </div>
    );
}

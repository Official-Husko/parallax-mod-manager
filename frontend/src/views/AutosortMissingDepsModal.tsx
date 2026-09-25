import './AutosortModals.css';
import {h} from 'preact';
import {ModalHeader} from '../components/ModalHeader';
import {FLAG} from '../data/flags';
import type {MissingDependency} from '../data/autosort';

// Shown when Workspace's own "Autosort" action finds at least one mod name
// a currently-active mod declares as a dependency but that isn't itself
// active - see data/autosort.ts's own findMissingActiveDependencies.
// Autosort's dependency rule can only reorder mods already in the load
// order, never add to it, so sorting without asking first would silently
// leave these gaps in place. Never touches the load order on its own -
// onLoadAndSort/onSortAnyway both come from Workspace, which does the
// actual mutation.
export function AutosortMissingDepsModal({missing, onClose, onLoadAndSort, onSortAnyway}: {
    missing: MissingDependency[];
    onClose: () => void;
    onLoadAndSort: () => void;
    onSortAnyway: () => void;
}) {
    const them = missing.length === 1 ? 'it' : 'them';
    return (
        <div className="overlay" onClick={onClose}>
            <div className="autosort-modal" onClick={(e) => e.stopPropagation()}>
                <ModalHeader title="Missing dependencies" onClose={onClose}>
                    <span className="mono count">{missing.length} found</span>
                </ModalHeader>
                <p className="autosort-modal-intro">
                    {missing.length} mod{missing.length === 1 ? '' : 's'} your active load order depends on{' '}
                    {missing.length === 1 ? "isn't" : "aren't"} active yet - Autosort can only reorder what's
                    already loaded, not add to it. Load {them} first, or sort without {them}?
                </p>
                <div className="autosort-modal-body">
                    {missing.map((m) => (
                        <div key={m.id} className="autosort-dep-row">
                            <i className={`fa-solid ${FLAG.dependency.icon}`}/>
                            <span className="autosort-dep-name">{m.name}</span>
                        </div>
                    ))}
                </div>
                <div className="modal-footer">
                    <span className="btn-ghost" onClick={onSortAnyway}>Autosort without {them}</span>
                    <button className="btn-primary" onClick={onLoadAndSort}>Load {them} & Autosort</button>
                </div>
            </div>
        </div>
    );
}

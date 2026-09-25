import './AutosortModals.css';
import {h} from 'preact';
import {ModalHeader} from '../components/ModalHeader';
import {FLAG} from '../data/flags';

// Shown after Autosort actually runs (see Workspace's own handleAutosort),
// when at least one currently-active mod declares a dependency that
// doesn't match anything this project has scanned at all - not just
// inactive like AutosortMissingDepsModal's own list, genuinely not
// installed. Purely informational: there's no local mod record to add,
// so the only real action is going and getting it - no invented Steam
// Workshop link, since nothing here identifies which Workshop item (if
// any) actually is the missing dependency.
export function AutosortUnresolvedDepsModal({names, onClose}: {
    names: string[];
    onClose: () => void;
}) {
    const them = names.length === 1 ? 'it' : 'them';
    return (
        <div className="overlay" onClick={onClose}>
            <div className="autosort-modal" onClick={(e) => e.stopPropagation()}>
                <ModalHeader title="Dependencies not found" onClose={onClose}>
                    <span className="mono count">{names.length} missing</span>
                </ModalHeader>
                <p className="autosort-modal-intro">
                    {names.length} declared dependenc{names.length === 1 ? 'y' : 'ies'} of your active mods{' '}
                    {names.length === 1 ? "wasn't" : "weren't"} found among your installed mods at all - not
                    just inactive, not downloaded. It's recommended to install {them} from Steam Workshop.
                </p>
                <div className="autosort-modal-body">
                    {names.map((name) => (
                        <div key={name} className="autosort-dep-row unresolved">
                            <i className={`fa-solid ${FLAG.dependency.icon}`}/>
                            <span className="autosort-dep-name">{name}</span>
                        </div>
                    ))}
                </div>
                <div className="modal-footer">
                    <button className="btn-primary" onClick={onClose}>Got it</button>
                </div>
            </div>
        </div>
    );
}

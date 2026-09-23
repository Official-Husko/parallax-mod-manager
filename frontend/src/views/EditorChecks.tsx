import {h} from 'preact';
import {useState} from 'preact/hooks';
import {CheckMod} from '../../wailsjs/go/main/App';
import type {app, library, modcheck} from '../../wailsjs/go/models';

// The Checks tab: problems worth knowing about before publishing or sharing a mod - files and
// script keys it overwrites in the base game, syntax errors in its own script files, a
// descriptor that won't parse right, a dependency that can't be found. See internal/modcheck.
// Checking a mod's files (and, the first time for a game, the whole base game) is real work, so
// it runs on request rather than automatically every time this tab opens.
//
// Laid out the same way Workspace's own PRE-FLIGHT list reads a load order: one plain row per
// check, with the count baked into the row's own sentence rather than a separate badge. Each
// category keeps one identity (its own icon and, once it has a problem, its own color - see
// CATEGORIES.color) that FINDINGS reuses for that category's own rows below, so a finding is
// visibly traceable back to the check it came from instead of the two panels reading as
// unrelated lists. "Clean" is always green and "not checked/skipped" is always dim, in CHECKS
// only - a finding is, by definition, never either of those.

type CategoryKey = 'base_game' | 'syntax' | 'descriptor' | 'dependency';
type Status = 'idle' | 'skipped' | 'warn' | 'good';

const CATEGORIES: { key: CategoryKey; icon: string; color: string; detail: string; text: (status: Status, n: number) => string }[] = [
    {
        key: 'base_game',
        icon: 'fa-chess-board',
        color: 'var(--amber)',
        detail: 'Files and script keys this mod overwrites that belong to the game itself, not another mod - the base-game half of what the Conflict Resolver finds between mods.',
        text: (status, n) => {
            if (status === 'idle') return 'Conflicts with the base game - not checked yet';
            if (status === 'skipped') return "Conflicts with the base game - couldn't check, the game wasn't found";
            if (status === 'warn') return `Overwrites ${n} thing${n === 1 ? '' : 's'} from the base game`;
            return 'No conflicts with the base game';
        },
    },
    {
        key: 'syntax',
        icon: 'fa-code',
        // A syntax problem is a parse failure, not a maybe - it gets the same red EditorEdit's
        // own Problems list uses for a hard error, not the amber the other categories share.
        color: 'var(--red)',
        detail: "This mod's own Clausewitz script files that fail to parse: which file, which line, and what looked wrong.",
        text: (status, n) => {
            if (status === 'idle') return 'Syntax errors - not checked yet';
            if (status === 'warn') return `${n} syntax problem${n === 1 ? '' : 's'} found`;
            return 'No syntax errors';
        },
    },
    {
        key: 'descriptor',
        icon: 'fa-file-circle-exclamation',
        color: 'var(--blue)',
        detail: 'Things wrong with the descriptor itself: no supported_version, a version that is not shaped like the game expects, a picture= that points at a file that does not exist.',
        text: (status, n) => {
            if (status === 'idle') return 'Descriptor problems - not checked yet';
            if (status === 'warn') return `${n} descriptor problem${n === 1 ? '' : 's'} found`;
            return 'Descriptor looks fine';
        },
    },
    {
        key: 'dependency',
        icon: 'fa-link-slash',
        // Same purple Workspace's own FLAGS column uses for a dependency issue there
        // (--flag-dependency) - one color for "a dependency problem" everywhere in the app.
        color: 'var(--flag-dependency)',
        detail: "A dependency this mod declares that matches no mod you have installed - the same matching the Editor's own dependency field already flags as you type.",
        text: (status, n) => {
            if (status === 'idle') return 'Dependencies - not checked yet';
            if (status === 'warn') return `${n} dependency${n === 1 ? '' : ' dependencies'} not found`;
            return 'Every dependency is installed';
        },
    },
];

const SEVERITY_LABEL: Record<string, string> = {error: 'Error', warn: 'Warning', info: 'Info'};

// checkColor is CHECKS' own row color: green once a category comes back clean, dim before it's
// checked (or, for base_game, when the game couldn't be found), otherwise that category's own
// color (CATEGORIES[].color) - never a generic "problem" amber for every category alike.
function checkColor(category: { color: string }, status: Status): string {
    if (status === 'good') return 'var(--green)';
    if (status === 'idle' || status === 'skipped') return 'var(--text-dim)';
    return category.color;
}

function categoryOf(key: string) {
    return CATEGORIES.find((c) => c.key === key);
}

export function EditorChecks({gameId, mod, installedNames, initialResult, onResult}: {
    gameId: string;
    mod: library.ModSummary;
    // Every other installed mod's name, for the dependency check - the same list the Edit tab
    // already gets from the Editor's own mod list.
    installedNames: string[];
    // A cached result from earlier this session, if this mod has one - see Editor.tsx's own
    // checkResults map. Editor.tsx also gives this component a key={mod.ID}, so a mod switch
    // remounts it fresh rather than needing an effect here to reset stale local state; the
    // result itself survives that remount only because it is re-supplied right back through
    // this same prop.
    initialResult: app.CheckResult | null;
    // Reports every new result (or its clearing) so Editor.tsx can cache it per mod - called
    // right alongside setResult below, never on its own.
    onResult: (result: app.CheckResult | null) => void;
}) {
    const [result, setResultState] = useState<app.CheckResult | null>(initialResult);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    function setResult(r: app.CheckResult | null) {
        setResultState(r);
        onResult(r);
    }

    function run() {
        setBusy(true);
        setError('');
        CheckMod(gameId, mod.ID, installedNames)
            .then(setResult)
            .catch((err) => setError(String(err)))
            .finally(() => setBusy(false));
    }

    const findings = result?.Findings ?? [];
    const countFor = (key: CategoryKey) => findings.filter((f) => f.Category === key).length;

    function statusOf(key: CategoryKey): Status {
        if (!result) return 'idle';
        if (key === 'base_game' && !result.BaseGameChecked) return 'skipped';
        return countFor(key) > 0 ? 'warn' : 'good';
    }

    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-card">
                    <div className="editor-card-title">CHECKS</div>
                    <div className="check-list">
                        {CATEGORIES.map((c) => {
                            const status = statusOf(c.key);
                            return (
                                <div key={c.key} className="check-row" title={c.detail}>
                                    <i className={`fa-duotone ${c.icon}`} style={{color: checkColor(c, status)}}/>
                                    <span>{c.text(status, countFor(c.key))}</span>
                                </div>
                            );
                        })}
                    </div>
                    <div className="editor-actions">
                        <button type="button" className="btn-primary" disabled={busy} onClick={run}>
                            {busy ? 'Checking...' : result ? 'Check again' : 'Check now'}
                        </button>
                    </div>
                    {busy && (
                        <p className="editor-muted">
                            Reading this mod's files, and the base game's own files the first time this game is checked.
                        </p>
                    )}
                    {error && <div className="editor-problem bad"><i className="fa-solid fa-circle-xmark"/> {error}</div>}
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">
                        FINDINGS{findings.length > 0 && <span className="editor-card-count mono"> · {findings.length}</span>}
                    </div>
                    {!result && !busy && <p className="editor-muted">Nothing has been checked yet. Click Check now.</p>}
                    {busy && <p className="editor-muted">Checking...</p>}
                    {result && findings.length === 0 && <p className="editor-muted">No problems found.</p>}
                    {findings.length > 0 && (
                        <div className="editor-findings">
                            {findings.map((f: modcheck.Finding, i) => {
                                const cat = categoryOf(f.Category);
                                const color = cat?.color ?? 'var(--text-dim)';
                                const icon = cat?.icon ?? 'fa-circle-info';
                                return (
                                    <div key={i} className="editor-finding-row" style={{borderLeftColor: color}}>
                                        <i title={SEVERITY_LABEL[f.Severity] ?? 'Info'} className={`fa-duotone ${icon}`} style={{color}}/>
                                        <div className="editor-finding-body">
                                            <span className="mono editor-finding-file">{f.File}{f.Line ? `:${f.Line}` : ''}</span>
                                            <span className="editor-finding-message">{f.Message}</span>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

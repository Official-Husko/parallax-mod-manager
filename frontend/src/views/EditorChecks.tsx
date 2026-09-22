import {h} from 'preact';
import type {library} from '../../wailsjs/go/models';

// The Checks tab: problems worth knowing about before publishing a mod - files it overwrites in
// the base game, syntax errors in its own script files, a descriptor that will not parse, a
// dependency that cannot be found. Not built yet - laid out here as it will look, with example
// findings clearly marked as an example. Nothing here scans anything.

type Severity = 'error' | 'warn' | 'info';

const PLANNED: { icon: string; title: string; desc: string }[] = [
    {
        icon: 'fa-chess-board',
        title: 'Conflicts with the base game',
        desc: 'Files and script keys this mod overwrites that belong to the game itself, not another mod - the base-game half of what the Conflict Resolver already finds between mods.',
    },
    {
        icon: 'fa-code',
        title: 'Syntax errors',
        desc: "This mod's own Clausewitz script files that fail to parse: which file, which line, and what looked wrong. Scanning already finds these; this collects them here per mod instead of only the activity log.",
    },
    {
        icon: 'fa-file-circle-exclamation',
        title: 'Descriptor problems',
        desc: 'Things wrong with the descriptor itself: no supported_version, a version that is not shaped like the game expects, a picture= that points at a file that does not exist.',
    },
    {
        icon: 'fa-link-slash',
        title: 'Dependencies not found',
        desc: "A dependency this mod declares that matches no mod you have installed - the same matching the Editor's own dependency field already flags as you type.",
    },
];

const EXAMPLE_FINDINGS: { severity: Severity; file: string; line?: number; message: string }[] = [
    {severity: 'error', file: 'common/buildings/00_buildings.txt', line: 214, message: "Unexpected token '}' - a block was not closed."},
    {severity: 'warn', file: 'common/buildings/00_buildings.txt', message: 'Overwrites a base-game building definition (building_capital) instead of extending it.'},
    {severity: 'warn', file: 'descriptor.mod', message: 'supported_version is not set, so the game cannot tell if this mod matches its own version.'},
    {severity: 'info', file: 'descriptor.mod', message: "Declares a dependency on 'Ethos Overhaul', which is not currently installed."},
];

const SEVERITY_STYLE: Record<Severity, { icon: string; label: string }> = {
    error: {icon: 'fa-circle-xmark', label: 'Error'},
    warn: {icon: 'fa-triangle-exclamation', label: 'Warning'},
    info: {icon: 'fa-circle-info', label: 'Info'},
};

export function EditorChecks({mod}: { mod: library.ModSummary }) {
    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-soon-banner">
                    <i className="fa-solid fa-shield-halved"/>
                    <div>
                        <div className="editor-soon-title">Checks for {mod.Name} <span className="chip">Coming later</span></div>
                        <div>Problems worth fixing before you publish or share this mod. Nothing is scanned yet - this is a preview of what will be checked.</div>
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">PLANNED CHECKS</div>
                    <div className="mode-option-list tiled">
                        {PLANNED.map((p) => (
                            <div key={p.title} className="mode-option disabled">
                                <i className={`fa-solid ${p.icon} editor-check-icon`}/>
                                <div className="mode-option-main">
                                    <div className="mode-option-name">{p.title} <span className="chip">Planned</span></div>
                                    <div className="mode-option-desc">{p.desc}</div>
                                </div>
                            </div>
                        ))}
                    </div>
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">FINDINGS</div>
                    <div className="editor-muted">Nothing has been checked yet. Example of what a result will look like:</div>
                    <div className="editor-findings">
                        <div className="editor-findings-head">
                            <span/>
                            <span>File</span>
                            <span>Message</span>
                        </div>
                        {EXAMPLE_FINDINGS.map((f, i) => {
                            const style = SEVERITY_STYLE[f.severity];
                            return (
                                <div key={i} className={`editor-finding-row ${f.severity}`}>
                                    <span title={style.label}><i className={`fa-solid ${style.icon}`}/></span>
                                    <span className="mono">{f.file}{f.line ? `:${f.line}` : ''}</span>
                                    <span>{f.message}</span>
                                </div>
                            );
                        })}
                    </div>
                    <div className="editor-example-tag"><i className="fa-solid fa-flask"/> Example findings, not a real check</div>
                </div>
            </div>
        </div>
    );
}

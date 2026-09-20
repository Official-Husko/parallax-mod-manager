import {h} from 'preact';
import type {ComponentChild} from 'preact';
import {FLAG} from '../data/flags';
import type {ModConflictInfo} from '../data/flags';
import {domains} from '../data/mockData';
import {DOMAIN_NAMES} from '../data/domainOverlap';
import type {DomainState} from '../data/domainOverlap';
import {displayVersion} from '../data/versionCompat';
import {TipHeading, TipItem} from './Tooltip';

// The rich tooltip bodies for the Active/Available lists' colored markers -
// the FLAGS column and the CEGILM domain bars. Each entry is drawn with the
// same icon, color and shape it has in the list (see data/flags.ts and the
// .segment rules in views/Workspace.css), so the tooltip reads as a caption
// of the thing you are pointing at rather than a plain text box.

// What one mod is flagged for. A field is present only when that flag
// applies to the mod.
export interface ModFlags {
    version?: {supported: string; game: string; ignored: boolean};
    conflict?: ModConflictInfo;
    dependency?: {missing: string[]; misordered: string[]};
}

// Names shown in a detail line: the first few, then how many more.
const NAME_LIMIT = 4;

function Names({names}: {names: string[]}) {
    const shown = names.slice(0, NAME_LIMIT);
    const rest = names.length - shown.length;
    return (
        <span>
            {shown.map((n, i) => (
                <span key={n}>{i > 0 && ', '}<span className="tip-name">{n}</span></span>
            ))}
            {rest > 0 && <span className="tip-more"> +{rest} more</span>}
        </span>
    );
}

// The tooltip for a mod's own flags - one entry per flag that applies.
// null when none does, so hovering a clean row shows nothing at all.
export function modFlagsTip(flags: ModFlags): ComponentChild | null {
    const {version, conflict, dependency} = flags;
    if (!version && !conflict && !dependency) return null;
    return (
        <div className="tip-list">
            {version && (
                <TipItem
                    icon={FLAG.version.icon}
                    color={version.ignored ? 'var(--flag-ignored)' : FLAG.version.color}
                    title={version.ignored ? 'Version mismatch (ignored)' : 'Version mismatch'}
                >
                    Built for {displayVersion(version.supported)} - you have {displayVersion(version.game)}.
                    {version.ignored && <div>Right-click to stop ignoring it.</div>}
                </TipItem>
            )}
            {conflict && (
                <TipItem icon={FLAG.conflict.icon} color={FLAG.conflict.color} title="Hard conflict">
                    {conflict.keys} contested {conflict.keys === 1 ? 'key' : 'keys'} shared with <Names names={conflict.others}/>.
                    <div>Pick winners in the Conflict Resolver.</div>
                </TipItem>
            )}
            {dependency && (
                <TipItem icon={FLAG.dependency.icon} color={FLAG.dependency.color} title="Dependency issue">
                    {dependency.missing.length > 0 && <div>Not in the load order: <Names names={dependency.missing}/></div>}
                    {dependency.misordered.length > 0 && <div>Loads before what it needs: <Names names={dependency.misordered}/></div>}
                    <div>Autosort can fix the order.</div>
                </TipItem>
            )}
        </div>
    );
}

// The FLAGS column header's tooltip: what every flag means.
export function flagLegendTip(): ComponentChild {
    return (
        <div className="tip-list">
            <TipHeading>Flags</TipHeading>
            <TipItem icon={FLAG.version.icon} color={FLAG.version.color} title="Version mismatch">
                Built for a different game version than the one installed.
            </TipItem>
            <TipItem icon={FLAG.conflict.icon} color={FLAG.conflict.color} title="Hard conflict">
                Defines a key another active mod also defines.
            </TipItem>
            <TipItem icon={FLAG.dependency.icon} color={FLAG.dependency.color} title="Dependency issue">
                A required mod is missing, not active, or loads too late.
            </TipItem>
        </div>
    );
}

// The three colors a domain bar can be, as the bar itself is drawn.
const STATE_STYLE: Record<DomainState, {color: string; title: string; detail: string}> = {
    overwritten: {color: 'var(--red)', title: 'Overwritten', detail: 'Every contested key it defines here is won by another mod.'},
    partial: {color: 'var(--amber)', title: 'Partly overwritten', detail: 'Wins some contested keys here and loses others.'},
    clean: {color: '#2f3b49', title: 'Clean', detail: 'No contested keys here, or it wins every one.'},
};

// One domain bar's tooltip: the bar itself, which domain it is, and what its
// color says about this mod in it.
export function domainTip(letter: string, state: DomainState): ComponentChild {
    const style = STATE_STYLE[state];
    return (
        <TipItem
            lead={<span className={`segment l-${letter} ${state}`}/>}
            color={style.color}
            title={`${DOMAIN_NAMES[letter]} - ${style.title.toLowerCase()}`}
        >
            {style.detail}
        </TipItem>
    );
}

// The CEGILM header's tooltip: which letter is which folder, and the
// color key.
export function domainLegendTip(): ComponentChild {
    return (
        <div className="tip-list">
            <TipHeading>Domains</TipHeading>
            <div className="tip-domain-grid">
                {domains.map((d) => (
                    <div key={d} className="tip-domain">
                        <span className={`segment l-${d} clean`}/>
                        <span>{DOMAIN_NAMES[d]}</span>
                    </div>
                ))}
            </div>
            <TipHeading>Colors</TipHeading>
            {(['overwritten', 'partial', 'clean'] as DomainState[]).map((state) => (
                <TipItem key={state} lead={<span className={`segment ${state}`}/>} color={STATE_STYLE[state].color} title={STATE_STYLE[state].title}>
                    {STATE_STYLE[state].detail}
                </TipItem>
            ))}
        </div>
    );
}

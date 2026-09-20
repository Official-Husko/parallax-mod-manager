import {h} from 'preact';
import {useMemo} from 'preact/hooks';
import {
    CHANGE_KIND_ORDER,
    CHANGE_KINDS,
    summarizeChanges,
    useModUpdates,
} from '../data/modUpdates';
import {tip} from '../data/tooltip';
import {TipItem} from './Tooltip';

// The sidebar's UPDATES card: what happened to this game's mods since the app
// last started (updated, changed, removed, deleted from the Workshop), as one
// headline and a count per kind, with the button that opens the full list. The
// data is data/modUpdates.ts', shared with that window.
export function UpdatesCard({gameId, onReview}: { gameId: string; onReview: () => void }) {
    const state = useModUpdates(gameId);
    const report = state.report;
    const summary = useMemo(() => summarizeChanges(report), [report]);
    const busy = state.status === 'checking';

    let icon = 'fa-circle-check';
    let tone = 'ok';
    let title = 'No changes since last start';
    if (!report) {
        if (state.status === 'error') {
            icon = 'fa-triangle-exclamation';
            tone = 'bad';
            title = "Couldn't check for updates";
        } else {
            icon = 'fa-spinner fa-spin';
            tone = '';
            title = 'Checking your mods...';
        }
    } else if (report.Unreadable) {
        icon = 'fa-triangle-exclamation';
        tone = 'warn';
        title = "Couldn't read your mods";
    } else if (report.BaselineAt === 0) {
        icon = 'fa-clock-rotate-left';
        tone = '';
        title = 'Tracking starts now';
    } else if (summary.fresh > 0) {
        icon = 'fa-bell';
        tone = 'warn';
        title = `${summary.fresh} ${summary.fresh === 1 ? 'mod' : 'mods'} changed since last start`;
    } else if (summary.standing > 0) {
        icon = 'fa-cloud-slash';
        tone = 'warn';
        title = `${summary.standing} gone from the Workshop`;
    }
    if (busy && report) icon = 'fa-spinner fa-spin';

    return (
        <div className="updates-card">
            <div className="updates-card-head">
                <span className={`updates-card-title ${tone}`}>
                    <i className={`fa-solid ${icon}`}/>
                    {title}
                </span>
                <span className="link-btn amber" onClick={onReview}>Review</span>
            </div>
            {summary.fresh > 0 && (
                <span className="updates-card-chips">
                    {CHANGE_KIND_ORDER.filter((kind) => summary.counts[kind] > 0).map((kind) => {
                        const meta = CHANGE_KINDS[kind];
                        const n = summary.counts[kind];
                        return (
                            <span
                                key={kind}
                                className="updates-chip"
                                style={`--chip-color: ${meta.color}`}
                                {...tip(() => (
                                    <TipItem icon={meta.icon} color={meta.color} title={meta.label}>
                                        {n} {n === 1 ? 'mod' : 'mods'} {meta.plural} since you last started.
                                    </TipItem>
                                ))}
                            >
                                <i className={`fa-solid ${meta.icon}`}/>{n}
                            </span>
                        );
                    })}
                </span>
            )}
        </div>
    );
}

import './UpdatesModal.css';
import {h} from 'preact';
import type {modupdates} from '../../wailsjs/go/models';
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime';
import {EmptyState} from '../components/EmptyState';
import {SourceBadge} from '../components/SourceBadge';
import {TipItem} from '../components/Tooltip';
import {formatBytes, timeAgo} from '../data/format';
import {
    CHANGE_KIND_ORDER,
    CHANGE_KINDS,
    type ChangeKind,
    checkModUpdates,
    markModUpdatesSeen,
    summarizeChanges,
    useModUpdates,
} from '../data/modUpdates';
import {tip} from '../data/tooltip';
import {displayVersion} from '../data/versionCompat';

function shortDate(unixSeconds: number): string {
    return new Date(unixSeconds * 1000).toLocaleString(undefined, {day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit'});
}

function signedBytes(n: number): string {
    return `${n < 0 ? '-' : '+'}${formatBytes(Math.abs(n))}`;
}

// What happened to the files of a mod, in a few words - "3 files added, +1.2 MB".
function filesNote(c: modupdates.Change): string {
    if (!c.FilesChanged) return '';
    const parts: string[] = [];
    if (c.FilesDelta > 0) parts.push(`${c.FilesDelta} ${c.FilesDelta === 1 ? 'file' : 'files'} added`);
    else if (c.FilesDelta < 0) parts.push(`${-c.FilesDelta} ${c.FilesDelta === -1 ? 'file' : 'files'} removed`);
    else parts.push('files rewritten');
    if (c.SizeDelta !== 0) parts.push(signedBytes(c.SizeDelta));
    return parts.join(', ');
}

// The line under a mod's name: what happened, in plain words.
function noteFor(c: modupdates.Change): string {
    switch (c.Kind as ChangeKind) {
        case 'updated': {
            const parts: string[] = [];
            if (c.WorkshopUpdated > 0) parts.push(`Workshop page updated ${timeAgo(c.WorkshopUpdated)}`);
            const files = filesNote(c);
            if (files) parts.push(files);
            else if (c.WorkshopUpdated > 0) parts.push("files unchanged so far - Steam may not have downloaded it yet, or only the page changed");
            return parts.join(' · ');
        }
        case 'changed':
            return filesNote(c);
        case 'removed':
            return c.Source === 'workshop' ? 'No longer installed - unsubscribed, or its Workshop files were removed' : 'No longer installed';
        case 'deleted':
            return c.New
                ? 'Deleted from the Workshop since you last started - it keeps working, but will not be updated again'
                : `Gone from the Workshop${c.GoneSince > 0 ? ` since ${shortDate(c.GoneSince)}` : ''} - it will not be updated again`;
    }
    return '';
}

function ChangeRow({change}: { change: modupdates.Change }) {
    const meta = CHANGE_KINDS[change.Kind as ChangeKind];
    const versionChanged = change.FromVersion !== change.ToVersion;
    const note = noteFor(change);
    return (
        <div className={`update-row ${change.New ? '' : 'standing'}`}>
            <span className="update-kind" style={`--kind-color: ${meta.color}`}>
                <i className={`fa-solid ${meta.icon}`}/>
            </span>
            <div className="update-main">
                <div className="update-name">
                    <SourceBadge source={change.Source} name={change.Name}/>
                    <span className="update-name-text">{change.Name}</span>
                    {!change.New && <span className="update-standing">still</span>}
                </div>
                {note && <div className="update-note">{note}</div>}
            </div>
            {change.Kind === 'updated' && versionChanged && (
                <span className="mono update-versions">
                    {change.FromVersion ? displayVersion(change.FromVersion) : '-'} &#8594;{' '}
                    <span style={{color: 'var(--green)'}}>{change.ToVersion ? displayVersion(change.ToVersion) : '-'}</span>
                </span>
            )}
            {change.RemoteFileID && (
                <i
                    className="fa-solid fa-up-right-from-square update-link"
                    onClick={() => BrowserOpenURL(`https://steamcommunity.com/sharedfiles/filedetails/?id=${change.RemoteFileID}`)}
                    {...tip(() => (
                        <TipItem icon="fa-up-right-from-square" color="var(--blue)" title="Open the Workshop page">
                            See the mod's page and its change notes on Steam.
                        </TipItem>
                    ))}
                />
            )}
        </div>
    );
}

// The window behind the sidebar's UPDATES card: what happened to this game's mods
// since the app last started, from the report data/modUpdates.ts holds.
export function UpdatesModal({gameId, gameName, onClose}: {
    gameId: string;
    gameName: string;
    onClose: () => void;
}) {
    const state = useModUpdates(gameId);
    const report = state.report;
    const summary = summarizeChanges(report);
    const checking = state.status === 'checking';

    const grouped = CHANGE_KIND_ORDER.map((kind) => ({
        kind,
        // What is new first, then what was already the case.
        items: (report?.Changes ?? [])
            .filter((c) => c.Kind === kind)
            .sort((a, b) => Number(b.New) - Number(a.New)),
    })).filter((g) => g.items.length > 0);

    let body;
    if (!report) {
        body = state.status === 'error'
            ? <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't check your mods" subtitle={state.error}/>
            : <EmptyState icon="fa-spinner fa-spin" title="Checking your mods..." subtitle="Comparing them with how they looked when you last started."/>;
    } else if (report.Unreadable) {
        body = (
            <EmptyState
                icon="fa-triangle-exclamation"
                tone="error"
                title="Couldn't read your mods"
                subtitle={`No mods were found for ${gameName}, though there were some last time - is a drive disconnected? Nothing was compared, so nothing is reported as removed.`}
            />
        );
    } else {
        body = (
            <>
                {state.status === 'error' && <div className="updates-note error">Couldn't check just now: {state.error}</div>}
                {!report.WorkshopChecked && report.WorkshopError && (
                    <div className="updates-note warn">
                        <i className="fa-solid fa-cloud-slash"/>
                        <span>Couldn't reach Steam ({report.WorkshopError}). Workshop updates and deletions weren't checked; changes to the files below still are.</span>
                    </div>
                )}
                {report.BaselineAt === 0 && (
                    <div className="updates-note info">
                        <i className="fa-solid fa-circle-info"/>
                        <span>
                            This is the first time {gameName}'s mods were recorded. From the next startup this lists which were
                            updated, changed, removed, or deleted from the Steam Workshop since you last ran Parallax.
                        </span>
                    </div>
                )}
                {report.BaselineAt > 0 && grouped.length === 0 && (
                    <EmptyState
                        icon="fa-circle-check"
                        title="Nothing has changed"
                        subtitle={`None of ${report.ModsChecked} ${report.ModsChecked === 1 ? 'mod was' : 'mods were'} updated, changed, removed or deleted from the Workshop since ${shortDate(report.BaselineAt)}.`}
                    />
                )}
                {grouped.map(({kind, items}) => {
                    const meta = CHANGE_KINDS[kind];
                    return (
                        <div key={kind} className="updates-group">
                            <div className="updates-group-header" style={`--kind-color: ${meta.color}`}>
                                <i className={`fa-solid ${meta.icon}`}/>
                                <span>{meta.label}</span>
                                <span className="mono updates-group-count">{items.length}</span>
                            </div>
                            {items.map((c) => <ChangeRow key={c.ModID} change={c}/>)}
                        </div>
                    );
                })}
            </>
        );
    }

    return (
        <div className="overlay" onClick={onClose}>
            <div className="updates-modal" onClick={(e) => e.stopPropagation()}>
                <div className="updates-modal-header">
                    <span className="title">Updates</span>
                    <span className="mono checked">
                        {checking ? 'checking...'
                            : report ? `checked ${timeAgo(report.CheckedAt)}${report.BaselineAt > 0 ? ` · since ${shortDate(report.BaselineAt)}` : ''}`
                                : ''}
                    </span>
                    <div className="spacer"/>
                    <span
                        className={`btn-ghost ${checking ? 'inert' : ''}`}
                        onClick={checking ? undefined : () => checkModUpdates(gameId, true)}
                        {...tip(() => (
                            <TipItem icon="fa-arrows-rotate" color="var(--blue)" title="Check again">
                                Ask Steam again and re-read the mod folders. Still compares with your last startup.
                            </TipItem>
                        ))}
                    >
                        <i className={`fa-solid fa-arrows-rotate ${checking ? 'fa-spin' : ''}`}/> Check again
                    </span>
                    {summary.fresh > 0 && (
                        <span
                            className="btn-primary"
                            onClick={() => markModUpdatesSeen(gameId)}
                            {...tip(() => (
                                <TipItem icon="fa-check" color="var(--green)" title="Mark all seen">
                                    Clear this list. Later checks only report what changes from now on.
                                </TipItem>
                            ))}
                        >
                            Mark all seen
                        </span>
                    )}
                    <i className="fa-solid fa-xmark close-btn" onClick={onClose}/>
                </div>
                <div className="updates-modal-body">{body}</div>
            </div>
        </div>
    );
}

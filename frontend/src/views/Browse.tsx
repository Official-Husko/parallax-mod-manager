import './Browse.css';
import {Fragment, h} from 'preact';
import type {JSX} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {
    CancelLoversLabInstall,
    ClearLoversLabCredentials,
    ListGames,
    LoversLabCategories,
    LoversLabChangelog,
    LoversLabComments,
    LoversLabDownloadDialog,
    LoversLabFileDetail,
    LoversLabFiles,
    LoversLabInstall,
    LoversLabInstalledMods,
    LoversLabPostComment,
    LoversLabProfile,
    LoversLabStatus,
    SaveLoversLabCredentials,
    UninstallLoversLabMod,
} from '../../wailsjs/go/main/App';
import type {app, library, loverslab} from '../../wailsjs/go/models';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import {Avatar} from '../components/Avatar';
import {Checkbox} from '../components/Checkbox';
import {EmptyState} from '../components/EmptyState';
import {GameLogo} from '../components/GameLogo';
import {openContextMenu} from '../data/contextMenu';
import {CARD_CATEGORIES, mockExtrasFor} from '../data/browseMockData';
import {checkLoversLabNotifications, loversLabNotificationsURL, useLoversLabUnreadCount} from '../data/loversLabNotifications';
import {colorFromName} from '../data/nameColor';
import {checkLoversLabUpdates, useModUpdates} from '../data/modUpdates';
import {notify} from '../data/notifications';

// A "loverslab-install-progress" event's shape - not a Wails-bound method's own
// parameter or return type, so it never gets a generated model (see
// loverslabinstall.go's LoversLabInstallProgress, and EditorNew.tsx's own
// DuplicateProgressEvent for the same reason).
interface LoversLabInstallProgressEvent {
    RequestID: string;
    Stage: string; // "downloading" | "extracting"
    Done: number;
    Total: number; // -1 when unknown
    // Which of a possibly multi-file batch install (the Files tab's own checked
    // selection) this event belongs to - FileIndex is 1-based, FileCount is always
    // at least 1.
    FileName: string;
    FileIndex: number;
    FileCount: number;
}

// Browse: additional, unofficial places to find mods beyond the Steam Workshop (already
// covered by Library/Workspace). LoversLab is the first real source - see
// internal/loverslab, loverslab.go and docs/loverslab.md for how it actually works and
// why it's built the way it is. Signing in is real, saved encrypted the same way the
// Steam Web API key already is (Settings > Steam API), and the sidebar's games, the file
// grid and changelogs below are the real thing straight off loverslab.com, not example
// data. The sidebar is "All" (every Paradox game's mods together) followed by whichever
// specific games currently have their own real subcategory there (loverslab.go's own
// LoversLabCategories/buildBrowseSidebar) - LoversLab does not split most Paradox games
// out individually the way it does Skyrim, Fallout, and so on, so "All" is the only way
// to reach a game with no subcategory of its own. A card opens the file's real page in
// the system browser rather than embedding a foreign site's content here - this app
// browses and links out, it doesn't mirror pages.

type CategoryState =
    | { kind: 'idle' }
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; categories: loverslab.Category[] };

type FilesState =
    | { kind: 'idle' }
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; files: app.LoversLabFileSummary[]; totalPages: number };

type ChangelogState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; entries: loverslab.ChangelogEntry[] };

type DetailState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; detail: loverslab.FileDetail };

// Comments load as an infinite scroll, not page-number pagination: 'ready'
// always holds every post fetched so far (page 1 through page, appended in
// order), and loadingMore is true while a further page is being fetched to
// append - see loadMoreComments.
type CommentsState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; posts: loverslab.Post[]; page: number; totalPages: number; hasTopic: boolean; loadingMore: boolean };

// mergeTopicAuthor keeps whatever topic-author name is already known once a later
// page's own fetch returns none of its own - LoversLabCommentList.TopicAuthor is
// only ever populated from a page 1 fetch (that's the only page which ever actually
// sees the topic's own opening post), so moving to page 2+ must not forget it.
function mergeTopicAuthor(existing: string, fetched: string): string {
    return fetched || existing;
}

// The detail overlay's four tabs, Nexus/Steam Workshop/Thunderstore-style: an
// overview (description, screenshots stay outside the tabs - see the mockup this
// redesign follows), the file's own downloadable files (pick one to install), a
// changelog (split out on its own rather than folded into the overview), and
// comments (read and, per Phase 1, write).
type DetailTab = 'overview' | 'files' | 'changelog' | 'comments';

// The Files tab's own list of what's downloadable for the open file - loaded
// eagerly, the moment a mod's detail is opened, the same as the other tabs.
type FilesTabState =
    | { kind: 'idle' }
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; downloads: loverslab.FileDownload[] };

type InstalledModsState =
    | { kind: 'idle' }
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; mods: app.LoversLabInstalledMod[] };

// Installing (from the Files tab's own checked selection and its "Download &
// install" button) and uninstalling (from the header or the Files tab) share this
// one piece of state - both are shown the same way, inline at the top of the tab
// body, never a second popup on top of the detail modal itself. installing can
// carry more than one download at once - every file the person had ticked when
// they clicked the button, installed together as one batch (see
// internal/app/loverslabinstall.go's own LoversLabInstall).
type InstallState =
    | { kind: 'idle' }
    | { kind: 'installing'; downloads: loverslab.FileDownload[]; progress: LoversLabInstallProgressEvent | null }
    | { kind: 'uninstall-confirm' }
    | { kind: 'uninstalling' }
    | { kind: 'error'; message: string };

function errorText(err: unknown): string {
    return String(err).replace(/^Error:\s*/, '');
}

// normalizeGameName lets a LoversLab category name match this app's own
// registered game name despite either one's numeral style - confirmed live:
// this app calls it "Crusader Kings III", LoversLab's own category is named
// "Crusader Kings 3", and a plain case-insensitive comparison alone missed
// that entirely (Stellaris/Victoria 3 have no such mismatch, since neither
// numbers nor romanizes in its own name either way).
function normalizeGameName(name: string): string {
    return name.trim()
        .replace(/\bIV\b/gi, '4')
        .replace(/\bIII\b/gi, '3')
        .replace(/\bII\b/gi, '2')
        .toLowerCase();
}

// pageButtons is up to max consecutive page numbers centered on current,
// clamped to [1, total] - LoversLabFiles already takes any page directly, this
// just exposes jumping straight to one instead of only stepping one at a time.
function pageButtons(current: number, total: number, max = 7): number[] {
    if (total <= max) return Array.from({length: total}, (_, i) => i + 1);
    let start = Math.max(1, current - Math.floor(max / 2));
    let end = start + max - 1;
    if (end > total) {
        end = total;
        start = end - max + 1;
    }
    return Array.from({length: end - start + 1}, (_, i) => start + i);
}

// formatUpdated turns FileDetail.DateModified's real ISO 8601 timestamp into the
// same "Month Day, Year" style the mock Submitted/Published dates already use, so
// all three read consistently in the detail overlay's dates grid. Falls back to
// the raw string if it somehow doesn't parse, rather than showing nothing.
function formatUpdated(iso: string): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleDateString(undefined, {month: 'long', day: 'numeric', year: 'numeric'});
}

// parseSizeToBytes turns a FileDownload.Size display string (e.g. "2.43 MB", exactly
// as LoversLab's own download dialog shows it) back into a real byte count, so the
// Files tab can add several selected files' sizes together. Returns 0 for anything
// that doesn't match - a size that can't be parsed just doesn't count toward the
// total rather than throwing off the sum with a guess.
function parseSizeToBytes(size: string): number {
    const m = size.trim().match(/^([\d.]+)\s*(B|KB|MB|GB|TB)$/i);
    if (!m) return 0;
    const units: Record<string, number> = {b: 1, kb: 1024, mb: 1024 ** 2, gb: 1024 ** 3, tb: 1024 ** 4};
    return parseFloat(m[1]) * (units[m[2].toLowerCase()] ?? 0);
}

// formatBytes is parseSizeToBytes' own inverse, for showing the selected files'
// combined size back in the same style LoversLab's own dialog uses.
function formatBytes(bytes: number): string {
    if (bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let value = bytes;
    let i = 0;
    while (value >= 1024 && i < units.length - 1) {
        value /= 1024;
        i++;
    }
    const decimals = i === 0 ? 0 : value < 10 ? 2 : value < 100 ? 1 : 0;
    return `${value.toFixed(decimals)} ${units[i]}`;
}

// Browsing (the card grid) and the Installed section (a plain row list) are two
// different kinds of data - loverslab.FileSummary and app.LoversLabInstalledMod -
// shown through the exact same switchable view, never two separate card-grid/
// row-list implementations. BrowseListItem is what both get normalized into before
// reaching BrowseItemsView below; ViewMode is the one shared choice ("cards" or
// "tree") both sections' own toggle sets.
type ViewMode = 'cards' | 'tree';

interface BrowseListItem {
    id: number;
    title: string;
    thumbnailURL: string;
    // A short genre/category badge shown over the thumbnail's top-left corner and,
    // in list view, right before the title - mock for now (see browseMockData.ts;
    // real cards have no such field yet).
    tag?: string;
    tagColor?: string;
    // Which small circular badge to overlay on the thumbnail's top-right corner, so
    // installed state reads at a glance while just browsing - real, from installedIds/
    // ContentMissing, never mock.
    stateIcon?: 'installed' | 'update' | 'missing' | null;
    authorName?: string;
    authorAvatarURL?: string;
    // The line right under the title - an author for a browsing card, or a missing-
    // content warning for an installed mod - shown the same way in either view.
    lineOne: string;
    lineOneWarn?: boolean;
    onLineOneClick?: () => void;
    // A small trailing stat - "Updated ..." for browsing, an install date for
    // installed.
    lineTwo?: string;
    onClick?: () => void;
    onContextMenu?: (e: MouseEvent) => void;
    // Replaces this item's whole content (e.g. the Installed list's inline
    // "Uninstall this mod?" confirm) - shown the same way in either view.
    overrideContent?: JSX.Element;
}

// ViewModeToggle is the two-icon switch ("cards"/"tree") both the browsing grid and
// the Installed section show in their own toolbar - one small shared control, not a
// pair of near-identical ones.
function ViewModeToggle({mode, onChange}: {mode: ViewMode; onChange: (m: ViewMode) => void}) {
    return (
        <div className="browse-view-toggle">
            <span className={`browse-view-toggle-btn ${mode === 'cards' ? 'active' : ''}`} onClick={() => onChange('cards')}>
                <i className="fa-solid fa-grip"/> Cards
            </span>
            <span className={`browse-view-toggle-btn ${mode === 'tree' ? 'active' : ''}`} onClick={() => onChange('tree')}>
                <i className="fa-solid fa-list"/> List
            </span>
        </div>
    );
}

// BrowseItemsView is the one shared system for showing a list of mods, as either a
// card grid (Steam Workshop/Nexus-style) or a compact row list - reused as-is by
// both the browsing grid and the Installed section, switched by ViewModeToggle
// above. Kept generic (BrowseListItem, not either backend type directly) so this
// rendering is never duplicated for the two different kinds of data it shows.
function BrowseItemsView({items, viewMode, emptyIcon, emptyMessage}: {
    items: BrowseListItem[];
    viewMode: ViewMode;
    emptyIcon: string;
    emptyMessage: string;
}) {
    if (items.length === 0) {
        return <EmptyState icon={emptyIcon} title={emptyMessage}/>;
    }

    if (viewMode === 'tree') {
        return (
            <div className="browse-tree-list">
                {items.map((it) => (
                    <div
                        key={it.id}
                        className="browse-tree-row"
                        onClick={it.overrideContent ? undefined : it.onClick}
                        onContextMenu={it.onContextMenu}
                    >
                        {it.overrideContent ?? (
                            <>
                                <div className="browse-tree-thumb">
                                    {it.thumbnailURL ? (
                                        <>
                                            <div className="browse-card-thumb-backdrop" style={{backgroundImage: `url(${it.thumbnailURL})`}}/>
                                            <img className="browse-card-thumb-fg" src={it.thumbnailURL} alt=""/>
                                        </>
                                    ) : (
                                        <i className="fa-solid fa-image"/>
                                    )}
                                </div>
                                <div className="browse-tree-main">
                                    <div className="browse-tree-titlerow">
                                        {it.tag && <span className="browse-tag" style={{background: it.tagColor}}>{it.tag}</span>}
                                        <span className="browse-tree-name" title={it.title}>{it.title}</span>
                                    </div>
                                    {it.lineOne && (
                                        <div
                                            className={`browse-tree-sub ${it.lineOneWarn ? 'warn' : ''} ${it.onLineOneClick ? 'clickable' : ''}`}
                                            onClick={it.onLineOneClick ? (e) => { e.stopPropagation(); it.onLineOneClick!(); } : undefined}
                                        >
                                            {it.lineOneWarn && <i className="fa-solid fa-triangle-exclamation"/>}
                                            {it.authorName && <Avatar name={it.authorName} url={it.authorAvatarURL} size={16}/>}
                                            {it.lineOne}
                                        </div>
                                    )}
                                </div>
                                {it.lineTwo && <div className="browse-tree-meta mono">{it.lineTwo}</div>}
                                <StateBadge state={it.stateIcon} className="browse-tree-state"/>
                            </>
                        )}
                    </div>
                ))}
            </div>
        );
    }

    return (
        <div className="browse-grid">
            {items.map((it) => (
                <div
                    key={it.id}
                    className="browse-card"
                    onClick={it.overrideContent ? undefined : it.onClick}
                    onContextMenu={it.onContextMenu}
                >
                    {it.overrideContent ?? (
                        <>
                            <div className={it.thumbnailURL ? 'browse-card-thumb' : 'browse-card-thumb placeholder'}>
                                {it.thumbnailURL ? (
                                    <>
                                        <div className="browse-card-thumb-backdrop" style={{backgroundImage: `url(${it.thumbnailURL})`}}/>
                                        <img className="browse-card-thumb-fg" src={it.thumbnailURL} alt=""/>
                                    </>
                                ) : (
                                    <i className="fa-solid fa-image"/>
                                )}
                                {it.tag && <span className="browse-tag browse-card-tag" style={{background: it.tagColor}}>{it.tag}</span>}
                                <StateBadge state={it.stateIcon} className="browse-card-state"/>
                            </div>
                            <div className="browse-card-body">
                                <div className="browse-card-title" title={it.title}>{it.title}</div>
                                <div className="browse-card-footer">
                                    {it.authorName && <Avatar name={it.authorName} url={it.authorAvatarURL} size={18}/>}
                                    {it.lineOne && (
                                        <span
                                            className={`browse-card-meta ${it.lineOneWarn ? 'warn' : ''} ${it.onLineOneClick ? 'clickable' : ''}`}
                                            onClick={it.onLineOneClick ? (e) => { e.stopPropagation(); it.onLineOneClick!(); } : undefined}
                                        >
                                            {it.lineOneWarn && <i className="fa-solid fa-triangle-exclamation"/>} {it.lineOne}
                                        </span>
                                    )}
                                    {it.lineTwo && <span className="browse-card-updated">{it.lineTwo}</span>}
                                </div>
                            </div>
                        </>
                    )}
                </div>
            ))}
        </div>
    );
}

// DescriptionRunView renders one loverslab.DescriptionRun - a real line break
// (originally a <br>) as an actual <br/> (a literal "\n" in JSX text has no visual
// effect at all - browsers collapse it, the same as any other whitespace in
// normal text flow), otherwise the run's own text wrapped in whichever of
// bold/italic/underline/link it carries. A link opens in the system browser like
// every other external link in this app, never navigating away from it in place.
function DescriptionRunView({run}: {run: loverslab.DescriptionRun}) {
    if (run.Text === '\n') {
        return <br/>;
    }
    // A real inline emoticon (see loverslab.DescriptionRun.EmoteURL) - kept at
    // icon size and inline with the surrounding text, unlike a real embedded
    // image (DescriptionBlock.ImageURL), which is photo-sized and its own
    // block. Conflating the two used to render every emoji at full
    // description-image size.
    if (run.EmoteURL) {
        return <img className="browse-emote" src={run.EmoteURL} alt={run.EmoteAlt} title={run.EmoteAlt} loading="lazy"/>;
    }
    let node: JSX.Element | string = run.Text;
    if (run.LinkURL) {
        const url = run.LinkURL;
        node = <span className="browse-description-link" onClick={() => BrowserOpenURL(url)}>{node}</span>;
    }
    if (run.Underline) node = <u>{node}</u>;
    if (run.Italic) node = <em>{node}</em>;
    if (run.Bold) node = <strong>{node}</strong>;
    return <>{node}</>;
}

// DescriptionBlocksView renders a loverslab.DescriptionBlock[] - shared by the
// Overview tab's own description and the Changelog tab's per-version notes,
// since both are the exact same real, parsed rich text (see
// internal/loverslab/detail.go's parseDescriptionBlocks) and deserve the same
// treatment: real headings/blockquotes/list items/dividers, not just flat
// paragraphs, including literal Markdown syntax some authors paste directly
// into the editor as plain text rather than using its own formatting toolbar
// (confirmed live against a real file's whole description written that way).
function DescriptionBlocksView({blocks}: {blocks: loverslab.DescriptionBlock[]}) {
    return (
        <>
            {blocks.map((block, i) => {
                if (block.ImageURL) {
                    return <img key={i} className="browse-description-image" src={block.ImageURL} alt="" loading="lazy"/>;
                }
                if (block.Divider) {
                    return <hr key={i} className="browse-description-divider"/>;
                }
                if (block.QuotedAuthor) {
                    return (
                        <blockquote key={i} className="browse-forum-quote">
                            <div className="browse-forum-quote-author">{block.QuotedAuthor} said:</div>
                            <DescriptionBlocksView blocks={block.QuotedBlocks ?? []}/>
                        </blockquote>
                    );
                }
                if (block.EmbedURL) {
                    const url = block.EmbedURL;
                    return (
                        <div key={i} className="browse-embed-reference" onClick={() => BrowserOpenURL(url)}>
                            <i className="fa-solid fa-arrow-up-right-from-square"/> View embedded LoversLab post
                        </div>
                    );
                }
                const content = block.Runs.map((run, j) => <DescriptionRunView key={j} run={run}/>);
                if (block.Heading > 0) {
                    const Tag = `h${Math.min(block.Heading, 6)}` as unknown as 'h1';
                    return <Tag key={i} className="browse-description-heading">{content}</Tag>;
                }
                if (block.Quote) {
                    return <blockquote key={i} className="browse-description-quote">{content}</blockquote>;
                }
                if (block.ListItem) {
                    return (
                        <div key={i} className="browse-description-listitem">
                            <span className="browse-description-bullet">•</span>
                            <span>{content}</span>
                        </div>
                    );
                }
                return <p key={i}>{content}</p>;
            })}
        </>
    );
}

// StateBadge is the small circular install-state indicator overlaid on a card's
// thumbnail (and shown plainly in list view) - installed (a real, tracked
// LoversLab install) or missing (installed but its files are gone from disk,
// ContentMissing) - both real. null/undefined (not yet installed) renders nothing,
// rather than a badge for an absence.
const STATE_BADGE: Record<'installed' | 'update' | 'missing', {icon: string; title: string}> = {
    installed: {icon: 'fa-check', title: 'Installed'},
    update: {icon: 'fa-arrow-up', title: 'An update is available'},
    missing: {icon: 'fa-triangle-exclamation', title: "Installed, but its files can't be found"},
};

function StateBadge({state, className}: {state?: 'installed' | 'update' | 'missing' | null; className: string}) {
    if (!state) return null;
    const {icon, title} = STATE_BADGE[state];
    return (
        <span className={`browse-state-badge ${className} ${state}`} title={title}>
            <i className={`fa-solid ${icon}`}/>
        </span>
    );
}

export function Browse({games, selectedGame, onOpenInWorkspace}: {
    games: library.GameInfo[];
    selectedGame: string;
    // Requirements (in the detail overlay) needs to send the person to Workspace
    // with the matching mod selected there - app.tsx owns the actual view switch
    // and the pending-selection state Workspace itself resolves, since neither is
    // Browse's own to hold.
    onOpenInWorkspace: (modName: string) => void;
}) {
    const unreadNotifications = useLoversLabUnreadCount();
    const [status, setStatus] = useState<app.LoversLabStatus | null>(null);
    // The account's own real display name/profile link/avatar (see
    // LoversLabProfile) - distinct from status.Username, which is only ever
    // whatever was actually typed to sign in (an email works just as well as a
    // username there). null while not yet fetched or not signed in; the
    // identity row falls back to status.Username until this resolves.
    const [profile, setProfile] = useState<app.LoversLabAccountProfile | null>(null);
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [busy, setBusy] = useState<'save' | 'clear' | null>(null);
    const [error, setError] = useState('');
    // The sign-in fields are always shown while signed out (there's nothing else
    // to show instead), but hidden by default once signed in - matching the
    // mockup's own cleaner signed-in card - revealed on demand by "Change
    // sign-in" for the real, existing "replace the saved credentials" flow.
    const [showSignInFields, setShowSignInFields] = useState(false);

    const [categoryState, setCategoryState] = useState<CategoryState>({kind: 'idle'});
    const [selectedCategory, setSelectedCategory] = useState<loverslab.Category | null>(null);
    const [page, setPage] = useState(1);
    const [filesState, setFilesState] = useState<FilesState>({kind: 'idle'});
    const [search, setSearch] = useState('');
    // The one shared choice (see BrowseItemsView) both the browsing grid and the
    // Installed section switch between - the same view either way, not a separate
    // toggle each remembers on its own.
    const [viewMode, setViewMode] = useState<ViewMode>('cards');
    // The sidebar's own CATEGORIES filter (see browseMockData.ts's CARD_CATEGORIES) -
    // mock for now, same as the tag itself, but the filtering is real: every file
    // deterministically gets one of these, so picking one really does narrow
    // whichever list (browsing or Installed) is showing. null = no filter, and
    // clicking the same one again clears it.
    const [selectedTagFilter, setSelectedTagFilter] = useState<string | null>(null);

    // The "Installed" section: a new place in the sidebar, alongside GAMES, to see and
    // uninstall everything installed from LoversLab for the selected game.
    const [viewingInstalled, setViewingInstalled] = useState(false);
    const [installedSearch, setInstalledSearch] = useState('');
    // Real, not decorative - toggles which end of InstalledAt the list starts
    // from, the same "Installed date" sort the mockup shows as a dropdown, just
    // exposed as a click-to-flip control since there is only ever the one axis
    // to sort installed mods by right now.
    const [installedNewestFirst, setInstalledNewestFirst] = useState(true);
    const [installedState, setInstalledState] = useState<InstalledModsState>({kind: 'idle'});
    const [confirmUninstallId, setConfirmUninstallId] = useState<number | null>(null);
    // Which open file ids are installed - kept up to date alongside installedState,
    // read by the detail modal to decide whether to offer Uninstall or Install/Update.
    const [installedIds, setInstalledIds] = useState<Set<number>>(new Set());

    const [detailFor, setDetailFor] = useState<loverslab.FileSummary | null>(null);
    const [detailTab, setDetailTab] = useState<DetailTab>('overview');
    // Which screenshot the persistent left-hand media pane currently shows full-size -
    // independent of detailTab, since the mockup this follows keeps the media pane
    // visible across every tab rather than scoping it to just one.
    const [screenshotIndex, setScreenshotIndex] = useState(0);
    const [detailState, setDetailState] = useState<DetailState | null>(null);
    const [changelogState, setChangelogState] = useState<ChangelogState | null>(null);
    const [commentsState, setCommentsState] = useState<CommentsState | null>(null);
    // Who started the open file's support topic - real data (see LoversLabCommentList.
    // TopicAuthor), only ever refreshed by a page 1 fetch, kept across later pages of
    // the same topic rather than lost once the page moves on.
    const [topicAuthor, setTopicAuthor] = useState('');
    const [commentDraft, setCommentDraft] = useState('');
    const [postingComment, setPostingComment] = useState(false);
    const [filesTabState, setFilesTabState] = useState<FilesTabState>({kind: 'idle'});
    // Which of filesTabState.downloads are currently checked, by index - reset
    // whenever a different mod's detail opens/closes, or once an install started
    // from this selection finishes (success or failure), so a stale tick from a
    // previous file never carries over.
    const [selectedFileIndexes, setSelectedFileIndexes] = useState<Set<number>>(new Set());

    const [installState, setInstallState] = useState<InstallState>({kind: 'idle'});
    const installRequestRef = useRef<string | null>(null);

    useEffect(() => {
        LoversLabStatus().then(setStatus).catch(() => undefined);
    }, []);

    // The account's own real display name/profile/avatar - fetched once per
    // sign-in (a real request, unlike LoversLabStatus itself), cleared again on
    // sign-out so a stale name never lingers into a different account's sign-in.
    useEffect(() => {
        if (!status?.SignedIn) {
            setProfile(null);
            return;
        }
        let cancelled = false;
        LoversLabProfile().then((p) => { if (!cancelled) setProfile(p); }).catch(() => undefined);
        return () => { cancelled = true; };
    }, [status?.SignedIn]);

    // The real sidebar, loaded once signed in - whether that sign-in just happened below
    // or was already saved from a previous run, status.SignedIn ends up true either way.
    // Its first entry is always "All" (every Paradox game's mods together - see
    // loverslab.go's LoversLabCategories), auto-selected here so signing in lands
    // straight on a useful file grid instead of an extra, redundant click.
    useEffect(() => {
        if (!status?.SignedIn) {
            setCategoryState({kind: 'idle'});
            return;
        }
        let cancelled = false;
        setCategoryState({kind: 'loading'});
        LoversLabCategories()
            .then((categories) => {
                if (cancelled) return;
                const list = categories ?? [];
                setCategoryState({kind: 'ready', categories: list});
                if (list.length > 0) setSelectedCategory(list[0]);
            })
            .catch((err) => { if (!cancelled) setCategoryState({kind: 'error', message: errorText(err)}); });
        return () => { cancelled = true; };
    }, [status?.SignedIn]);

    // What's installed from LoversLab for the selected game - kept live alongside the
    // categories above (same trigger), independent of whether the Installed section is
    // currently being looked at, since the detail modal also needs installedIds to know
    // whether to offer Uninstall or Install/Update for whatever file is open.
    async function refreshInstalled() {
        if (!status?.SignedIn || !selectedGame) return;
        setInstalledState((prev) => (prev.kind === 'ready' ? prev : {kind: 'loading'}));
        try {
            const mods = await LoversLabInstalledMods(selectedGame);
            const list = mods ?? [];
            setInstalledState({kind: 'ready', mods: list});
            setInstalledIds(new Set(list.map((m) => m.FileID)));
        } catch (err) {
            setInstalledState({kind: 'error', message: errorText(err)});
        }
    }

    useEffect(() => {
        if (!status?.SignedIn || !selectedGame) {
            setInstalledState({kind: 'idle'});
            setInstalledIds(new Set());
            return;
        }
        void refreshInstalled();
    }, [status?.SignedIn, selectedGame]);

    useEffect(() => {
        if (!selectedCategory) {
            setFilesState({kind: 'idle'});
            return;
        }
        let cancelled = false;
        setFilesState({kind: 'loading'});
        LoversLabFiles(selectedCategory.URL, page)
            .then((result) => { if (!cancelled) setFilesState({kind: 'ready', files: result.Files ?? [], totalPages: result.TotalPages || 1}); })
            .catch((err) => { if (!cancelled) setFilesState({kind: 'error', message: errorText(err)}); });
        return () => { cancelled = true; };
    }, [selectedCategory, page]);

    function selectCategory(cat: loverslab.Category) {
        setViewingInstalled(false);
        setSelectedCategory(cat);
        setPage(1);
        setSearch('');
        // A CATEGORIES filter picked while browsing one game is about that
        // game's own files - carrying it over silently narrowed a
        // freshly-selected game's real, full page of results down to
        // whichever small slice happened to still match, which is exactly
        // what looked like "only a few mods" for an otherwise perfectly
        // real 25-per-page response.
        setSelectedTagFilter(null);
    }

    function selectInstalled() {
        setViewingInstalled(true);
        setConfirmUninstallId(null);
        void refreshInstalled();
    }

    async function save() {
        setBusy('save');
        setError('');
        try {
            const s = await SaveLoversLabCredentials(username, password);
            setStatus(s);
            setUsername('');
            setPassword('');
            setShowSignInFields(false);
            notify('success', 'LoversLab sign-in saved (encrypted on this computer).');
            // A fresh sign-in is exactly when a LoversLab update check first becomes
            // possible - don't make signing in and then waiting up to
            // loversLabCheckIntervalHours the only way to see it.
            void checkLoversLabUpdates(selectedGame);
            void checkLoversLabNotifications();
        } catch (err) {
            setError(errorText(err));
        } finally {
            setBusy(null);
        }
    }

    async function clearSignIn() {
        setBusy('clear');
        setError('');
        try {
            const s = await ClearLoversLabCredentials();
            setStatus(s);
            setSelectedCategory(null);
            notify('info', 'LoversLab sign-in removed.');
        } catch (err) {
            setError(errorText(err));
        } finally {
            setBusy(null);
        }
    }

    // Opening a card fetches its detail, changelog and first page of comments together -
    // three independent requests shown as three independent sections below, so a slow or
    // failed one (a file with no changelog, or no support topic at all) never blocks the
    // others from showing up.
    // Opening a card fetches everything the overlay can show - detail,
    // changelog, and the downloads list - together, up front, rather than
    // waiting for whichever tab a person happens to click first. Each is its
    // own independent request/state, so a slow or failed one (a file with no
    // changelog, or no support topic at all) never blocks the others from
    // showing up, and each tab's own label can show a real count immediately.
    function openDetail(file: loverslab.FileSummary) {
        setDetailFor(file);
        setDetailTab('overview');
        setScreenshotIndex(0);
        setInstallState({kind: 'idle'});
        setSelectedFileIndexes(new Set());

        setDetailState({kind: 'loading'});
        LoversLabFileDetail(file.URL)
            .then((detail) => setDetailState({kind: 'ready', detail}))
            .catch((err) => setDetailState({kind: 'error', message: errorText(err)}));

        setChangelogState({kind: 'loading'});
        LoversLabChangelog(file.URL)
            .then((entries) => setChangelogState({kind: 'ready', entries: entries ?? []}))
            .catch((err) => setChangelogState({kind: 'error', message: errorText(err)}));

        setFilesTabState({kind: 'loading'});
        LoversLabDownloadDialog(file.URL)
            .then((downloads) => setFilesTabState({kind: 'ready', downloads: downloads ?? []}))
            .catch((err) => setFilesTabState({kind: 'error', message: errorText(err)}));
    }

    useEffect(() => {
        if (!detailFor) {
            setCommentsState(null);
            setTopicAuthor('');
            return;
        }
        let cancelled = false;
        setCommentsState({kind: 'loading'});
        LoversLabComments(detailFor.URL, 1)
            .then((result) => {
                if (cancelled) return;
                setCommentsState({kind: 'ready', posts: result.Posts ?? [], page: 1, totalPages: result.TotalPages || 1, hasTopic: result.HasTopic, loadingMore: false});
                setTopicAuthor((prev) => mergeTopicAuthor(prev, result.TopicAuthor));
            })
            .catch((err) => { if (!cancelled) setCommentsState({kind: 'error', message: errorText(err)}); });
        return () => { cancelled = true; };
    }, [detailFor]);

    // Infinite scroll's own "fetch the next page and append it" - triggered by
    // scrolling near the bottom of the Comments tab (see the tab body's own
    // onScroll below) or by the fallback "Load more" button, whichever the
    // person actually uses. A no-op while already loading or once every page
    // is already in hand, so a fast scroll can't fire this twice for the same
    // next page.
    async function loadMoreComments() {
        if (!detailFor || commentsState?.kind !== 'ready') return;
        if (commentsState.loadingMore || commentsState.page >= commentsState.totalPages) return;
        const nextPage = commentsState.page + 1;
        setCommentsState((prev) => prev?.kind === 'ready' ? {...prev, loadingMore: true} : prev);
        try {
            const result = await LoversLabComments(detailFor.URL, nextPage);
            setCommentsState((prev) => prev?.kind === 'ready' ? {
                ...prev,
                posts: [...prev.posts, ...(result.Posts ?? [])],
                page: nextPage,
                totalPages: result.TotalPages || prev.totalPages,
                loadingMore: false,
            } : prev);
            setTopicAuthor((prev) => mergeTopicAuthor(prev, result.TopicAuthor));
        } catch (err) {
            notify('error', errorText(err));
            setCommentsState((prev) => prev?.kind === 'ready' ? {...prev, loadingMore: false} : prev);
        }
    }

    // Only relevant while the Comments tab is the one actually showing -
    // .browse-detail-tab-body is shared by all 4 tabs, so scrolling near the
    // bottom of, say, a long Overview description must never trigger this.
    function onDetailTabBodyScroll(e: Event) {
        if (detailTab !== 'comments') return;
        const el = e.currentTarget as HTMLDivElement;
        if (el.scrollTop + el.clientHeight >= el.scrollHeight - 300) {
            void loadMoreComments();
        }
    }

    // Opening a mod straight from the Installed list: only FileID/Title/FileURL are
    // known there (see LoversLabInstalledMod) - the rest (author, screenshots,
    // description) loads the same way it does for a card from the browsing grid.
    function openInstalledDetail(m: app.LoversLabInstalledMod) {
        openDetail({ID: m.FileID, Title: m.Title, URL: m.FileURL, Author: '', AuthorURL: '', Updated: '', ThumbnailURL: ''} as loverslab.FileSummary);
    }

    // Posting a reply: always reloads back to just page 1 afterward rather than
    // trying to splice the new reply into whatever's already been scrolled into
    // view - simpler, and correct regardless of how many pages had already
    // been loaded when the person posted.
    async function postComment() {
        if (!detailFor) return;
        const content = commentDraft.trim();
        if (!content) return;
        setPostingComment(true);
        try {
            await LoversLabPostComment(detailFor.URL, content);
            setCommentDraft('');
            notify('success', 'Comment posted.');
            const result = await LoversLabComments(detailFor.URL, 1);
            setCommentsState({kind: 'ready', posts: result.Posts ?? [], page: 1, totalPages: result.TotalPages || 1, hasTopic: result.HasTopic, loadingMore: false});
            setTopicAuthor((prev) => mergeTopicAuthor(prev, result.TopicAuthor));
        } catch (err) {
            notify('error', errorText(err));
        } finally {
            setPostingComment(false);
        }
    }

    function closeDetail() {
        // An install or uninstall in progress keeps the modal open - closing partway
        // through would leave no way to see it finish, cancel it, or find out whether
        // it succeeded.
        if (installState.kind === 'installing' || installState.kind === 'uninstalling') return;
        setDetailFor(null);
        setDetailState(null);
        setChangelogState(null);
        setCommentsState(null);
        setCommentDraft('');
        setFilesTabState({kind: 'idle'});
        setInstallState({kind: 'idle'});
        setSelectedFileIndexes(new Set());
    }

    // Installs every file in downloads together as one batch - the Files tab's own
    // checked selection, whatever size (one file, or several picked together, e.g. a
    // main archive plus an addon zip) - see internal/app/loverslabinstall.go's own
    // LoversLabInstall for how they land in the same mod folder.
    async function runInstall(downloads: loverslab.FileDownload[]) {
        if (!detailFor || downloads.length === 0) return;
        const requestId = `ll-install-${Date.now()}-${Math.random().toString(36).slice(2)}`;
        installRequestRef.current = requestId;
        setInstallState({kind: 'installing', downloads, progress: null});
        const off = EventsOn('loverslab-install-progress', (p: LoversLabInstallProgressEvent) => {
            if (p.RequestID === requestId) setInstallState({kind: 'installing', downloads, progress: p});
        });
        try {
            const dateModified = detailState?.kind === 'ready' ? detailState.detail.DateModified : '';
            await LoversLabInstall(selectedGame, requestId, detailFor, dateModified, downloads);
            notify('success', downloads.length === 1 ? `Installed '${detailFor.Title}'.` : `Installed '${detailFor.Title}' (${downloads.length} files).`);
            setInstallState({kind: 'idle'});
            setSelectedFileIndexes(new Set());
            // This mod's own tracked install just changed - a fresh check confirms it no
            // longer shows as outdated right away, rather than until the next scheduled one.
            void checkLoversLabUpdates(selectedGame);
            void refreshInstalled();
        } catch (err) {
            const message = errorText(err);
            notify(message.includes('cancelled') ? 'info' : 'error', message);
            setInstallState({kind: 'idle'});
        } finally {
            off();
            installRequestRef.current = null;
        }
    }

    function cancelInstall() {
        if (installRequestRef.current) CancelLoversLabInstall(installRequestRef.current);
    }

    function toggleFileSelected(i: number) {
        setSelectedFileIndexes((prev) => {
            const next = new Set(prev);
            if (next.has(i)) next.delete(i); else next.add(i);
            return next;
        });
    }

    function toggleSelectAllFiles(total: number) {
        setSelectedFileIndexes((prev) =>
            prev.size >= total ? new Set() : new Set(Array.from({length: total}, (_, i) => i)));
    }

    // Uninstalling the file currently open in the detail modal (header or Files tab) -
    // shares installState with the install flow above, so both show the same way,
    // inline at the top of the tab body.
    async function uninstallCurrent() {
        if (!detailFor) return;
        setInstallState({kind: 'uninstalling'});
        try {
            await UninstallLoversLabMod(selectedGame, detailFor.ID);
            notify('success', `Uninstalled '${detailFor.Title}'.`);
            setInstallState({kind: 'idle'});
            void refreshInstalled();
        } catch (err) {
            notify('error', errorText(err));
            setInstallState({kind: 'idle'});
        }
    }

    // Uninstalling from the Installed list itself, after its own inline "Uninstall
    // this mod?" confirm (set by the row's right-click menu) - see
    // confirmUninstallId below.
    async function uninstallFromList(fileId: number, title: string) {
        setConfirmUninstallId(null);
        try {
            await UninstallLoversLabMod(selectedGame, fileId);
            notify('success', `Uninstalled '${title}'.`);
            void refreshInstalled();
        } catch (err) {
            notify('error', errorText(err));
        }
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;
    // A LoversLab category's own name ("Crusader Kings 2", "Stellaris", ...)
    // matched against this app's own registered games, so the sidebar can show
    // each one's real logo (GameLogo) instead of a plain color swatch - "All"
    // (every Paradox game's mods together) has no single match, and keeps the
    // plain swatch on purpose, since it isn't really one specific game.
    // The GAMES sidebar's own logo matching needs every game this app knows
    // about, not just the ones set up as "managed" in Settings (the games
    // prop above) - LoversLab's own category list includes a game's section
    // regardless of whether this app itself is managing it yet, and there's
    // no reason its real logo shouldn't show just because of that.
    const [allGames, setAllGames] = useState<library.GameInfo[]>([]);
    useEffect(() => {
        ListGames().then(setAllGames).catch(() => undefined);
    }, []);
    const gameIdByName = useMemo(() => {
        const m = new Map<string, string>();
        for (const g of allGames) m.set(normalizeGameName(g.DisplayName), g.ID);
        return m;
    }, [allGames]);
    const canSave = !busy && username.trim() !== '' && password !== '';
    const isInstalled = detailFor ? installedIds.has(detailFor.ID) : false;
    const missingFilesCount = installedState.kind === 'ready' ? installedState.mods.filter((m) => m.ContentMissing).length : 0;
    // Which LoversLab file IDs have a real update waiting - the same report
    // Settings' own Updates window reads (data/modUpdates.ts), narrowed to the
    // LoversLab-sourced changes it already merges in (see checkLoversLabUpdates)
    // and their own "loverslab_<fileID>" ModID convention (internal/mod's
    // LoversLabFilePrefix) unwound back to a plain numeric ID.
    const modUpdatesState = useModUpdates(selectedGame);
    const updateAvailableIds = useMemo(() => {
        const s = new Set<number>();
        for (const c of modUpdatesState.report?.Changes ?? []) {
            if (c.Source !== 'loverslab') continue;
            const m = /^loverslab_(\d+)$/.exec(c.ModID);
            if (m) s.add(Number(m[1]));
        }
        return s;
    }, [modUpdatesState.report]);
    const updatesAvailableCount = installedState.kind === 'ready'
        ? installedState.mods.filter((m) => updateAvailableIds.has(m.FileID)).length
        : 0;
    // The mock extras (see browseMockData.ts) for whichever mod's detail is open -
    // null while nothing is open, since every one of these fields is only ever
    // rendered inside the detail overlay.
    const extras = detailFor ? mockExtrasFor(detailFor.ID) : null;
    const screenshots = (detailState?.kind === 'ready' ? detailState.detail.Screenshots : []) ?? [];
    const descriptionBlocks = (detailState?.kind === 'ready' ? detailState.detail.DescriptionBlocks : []) ?? [];
    const heroShot = screenshots.length > 0 ? screenshots[Math.min(screenshotIndex, screenshots.length - 1)] : null;
    // The hero display always prefers the real full-size image - Screenshots'
    // own URL/ThumbnailURL are genuinely separate stored files (not one derived
    // from the other), so showing ThumbnailURL here was rendering a real,
    // deliberately smaller/blurrier image where the real one was already known.
    const heroURL = heroShot ? (heroShot.URL || heroShot.ThumbnailURL) : (detailFor?.ThumbnailURL || '');

    const files = filesState.kind === 'ready' ? filesState.files : [];
    const visibleFiles = useMemo(() => {
        const q = search.trim().toLowerCase();
        if (!q) return files;
        return files.filter((f) => f.Title.toLowerCase().includes(q) || f.Author.toLowerCase().includes(q));
    }, [files, search]);

    // Normalizing into BrowseListItem is what lets the browsing grid and the
    // Installed section below share one rendering (BrowseItemsView) instead of
    // each having its own card-grid and row-list markup.
    const browsingItems = useMemo<BrowseListItem[]>(() => visibleFiles.map((f) => {
        const extras = mockExtrasFor(f.ID);
        // RealUpdated/Views/AuthorAvatarURL are this app's own opportunistic
        // cache (internal/loverslabmeta) - real, but only ever present for a
        // file this app has separately, already opened its own detail view
        // for at some point; the listing page itself has none of these three
        // at all (confirmed live), so a file never opened yet just shows
        // none of them, same as before this cache existed. f.Updated (the
        // listing's own display string) is kept as a fallback, though the
        // real site stopped rendering it there entirely.
        const dateText = f.RealUpdated ? formatUpdated(f.RealUpdated) : f.Updated;
        const viewsText = f.Views > 0 ? `${f.Views.toLocaleString()} views` : '';
        return {
            id: f.ID,
            title: f.Title,
            thumbnailURL: f.ThumbnailURL,
            tag: extras.tag,
            tagColor: extras.tagColor,
            stateIcon: updateAvailableIds.has(f.ID) ? 'update' : installedIds.has(f.ID) ? 'installed' : null,
            authorName: f.Author,
            authorAvatarURL: f.AuthorAvatarURL,
            lineOne: f.Author,
            onLineOneClick: f.AuthorURL ? () => BrowserOpenURL(f.AuthorURL) : undefined,
            lineTwo: [dateText, viewsText].filter(Boolean).join(' · '),
            onClick: () => openDetail(f),
        };
    }), [visibleFiles, installedIds, updateAvailableIds]);

    const installedItems = useMemo<BrowseListItem[]>(() => {
        if (installedState.kind !== 'ready') return [];
        const mods = [...installedState.mods].sort((a, b) =>
            installedNewestFirst ? b.InstalledAt - a.InstalledAt : a.InstalledAt - b.InstalledAt);
        return mods.map((m) => ({
            id: m.FileID,
            title: m.Title,
            thumbnailURL: '',
            tag: mockExtrasFor(m.FileID).tag,
            tagColor: mockExtrasFor(m.FileID).tagColor,
            stateIcon: m.ContentMissing ? 'missing' : updateAvailableIds.has(m.FileID) ? 'update' : 'installed',
            lineOne: m.ContentMissing ? 'Files not found on disk' : '',
            lineOneWarn: m.ContentMissing,
            lineTwo: m.InstalledAt ? new Date(m.InstalledAt * 1000).toLocaleDateString() : undefined,
            onClick: confirmUninstallId === m.FileID ? undefined : () => openInstalledDetail(m),
            onContextMenu: (e: MouseEvent) => openContextMenu(e, [
                {label: 'Open on LoversLab', onClick: () => BrowserOpenURL(m.FileURL)},
                {label: 'Uninstall', danger: true, separatorBefore: true, onClick: () => setConfirmUninstallId(m.FileID)},
            ]),
            overrideContent: confirmUninstallId === m.FileID ? (
                <div className="browse-item-confirm">
                    <span>Uninstall this mod? This deletes its files from your mod folder.</span>
                    <div className="browse-item-confirm-actions">
                        <span className="btn-ghost danger" onClick={(e: MouseEvent) => { e.stopPropagation(); void uninstallFromList(m.FileID, m.Title); }}>Uninstall</span>
                        <span className="btn-ghost" onClick={(e: MouseEvent) => { e.stopPropagation(); setConfirmUninstallId(null); }}>Keep</span>
                    </div>
                </div>
            ) : undefined,
        }));
    }, [installedState, confirmUninstallId, updateAvailableIds, installedNewestFirst]);

    const visibleBrowsingItems = useMemo(
        () => selectedTagFilter ? browsingItems.filter((it) => it.tag === selectedTagFilter) : browsingItems,
        [browsingItems, selectedTagFilter],
    );
    const visibleInstalledItems = useMemo(() => {
        const q = installedSearch.trim().toLowerCase();
        return installedItems.filter((it) =>
            (!selectedTagFilter || it.tag === selectedTagFilter) &&
            (!q || it.title.toLowerCase().includes(q)));
    }, [installedItems, selectedTagFilter, installedSearch]);
    // Categories' own counts reflect whichever list is actually on screen right
    // now, not a fixed site-wide total this app has no way to know.
    const categoryCounts = useMemo(() => {
        const counts = new Map<string, number>();
        for (const it of viewingInstalled ? installedItems : browsingItems) {
            if (it.tag) counts.set(it.tag, (counts.get(it.tag) ?? 0) + 1);
        }
        return counts;
    }, [viewingInstalled, installedItems, browsingItems]);

    return (
        <div className="browse">
            <div className="browse-sources">
                <div className="sidebar-label">SOURCES</div>
                <div className="browse-source-row active">
                    <span className="browse-source-swatch loverslab">LL</span>
                    <span className="browse-source-name">LoversLab</span>
                    <span className={`browse-source-dot ${status?.SignedIn ? 'good' : 'neutral'}`}/>
                </div>
                {status?.SignedIn && (
                    <>
                        <div className="sidebar-label">INSTALLED</div>
                        <div className="browse-games">
                            <div
                                className={`browse-category-row depth-0 ${viewingInstalled ? 'active' : ''}`}
                                onClick={selectInstalled}
                            >
                                <i className="fa-solid fa-circle-check browse-installed-icon"/>
                                <span className="cat-name">Installed mods</span>
                                <span className="mono cat-count">{installedIds.size}</span>
                            </div>
                        </div>

                        <div className="sidebar-label games-label">GAMES</div>
                        <div className="browse-games">
                            {categoryState.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                            {categoryState.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load games" subtitle={categoryState.message}/>}
                            {categoryState.kind === 'ready' && categoryState.categories.map((cat) => {
                                const matchedGameId = gameIdByName.get(normalizeGameName(cat.Name));
                                const isSelected = selectedCategory?.URL === cat.URL;
                                return (
                                    <Fragment key={cat.URL}>
                                        <div
                                            className={`browse-category-row depth-${cat.Depth} ${isSelected ? 'active' : ''}`}
                                            onClick={() => selectCategory(cat)}
                                        >
                                            {matchedGameId ? (
                                                <GameLogo gameId={matchedGameId} className="browse-category-icon"/>
                                            ) : cat.Name.toLowerCase() === 'all' ? (
                                                <span className="browse-category-icon fallback"><i className="fa-solid fa-layer-group"/></span>
                                            ) : (
                                                <span className="browse-category-swatch" style={{background: colorFromName(cat.Name)}}/>
                                            )}
                                            <span className="cat-name" title={cat.Name}>{cat.Name}</span>
                                            <span className="mono cat-count">{cat.Files}</span>
                                        </div>
                                        {/* CATEGORIES lives right under whichever game is actually
                                            selected, not as its own always-shown section - it's a
                                            filter on that game's own file list, not a site-wide one. */}
                                        {isSelected && (
                                            <div className="browse-tagfilter-list">
                                                {CARD_CATEGORIES.map((tag) => (
                                                    <div
                                                        key={tag}
                                                        className={`browse-tagfilter-row ${selectedTagFilter === tag ? 'active' : ''}`}
                                                        onClick={() => setSelectedTagFilter((t) => t === tag ? null : tag)}
                                                    >
                                                        <span className="cat-name">{tag}</span>
                                                        <span className="mono cat-count">{categoryCounts.get(tag) ?? 0}</span>
                                                    </div>
                                                ))}
                                            </div>
                                        )}
                                    </Fragment>
                                );
                            })}
                            {categoryState.kind === 'ready' && categoryState.categories.length === 0 && (
                                <EmptyState icon="fa-gamepad" title="No games found" subtitle="LoversLab's Paradox Games section could not be found."/>
                            )}
                        </div>
                    </>
                )}

                <div className="sidebar-footer">
                    <div className="mono line">
                        Unofficial places to find {gameName} mods, outside the Steam Workshop.
                    </div>
                </div>
            </div>

            <div className="browse-main">
                {status?.SignedIn && viewingInstalled ? (
                    <>
                        <div className="browse-toolbar column">
                            <div className="browse-toolbar-row">
                                <div className="search-box">
                                    <i className="fa-solid fa-magnifying-glass"/>
                                    <input
                                        placeholder="Search installed mods..."
                                        value={installedSearch}
                                        onInput={(e) => setInstalledSearch((e.target as HTMLInputElement).value)}
                                    />
                                </div>
                                <span
                                    className="browse-sort-control"
                                    title="Click to flip the order"
                                    onClick={() => setInstalledNewestFirst((v) => !v)}
                                >
                                    Installed date {installedNewestFirst ? '▾' : '▴'}
                                </span>
                            </div>
                            <div className="browse-toolbar-row">
                                <span className="mono browse-installed-stats">
                                    {installedIds.size} installed &middot; {updatesAvailableCount} update{updatesAvailableCount === 1 ? '' : 's'} &middot; {missingFilesCount} missing file{missingFilesCount === 1 ? '' : 's'}
                                </span>
                                <div className="spacer"/>
                                <ViewModeToggle mode={viewMode} onChange={setViewMode}/>
                            </div>
                        </div>
                        {installedState.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                        {installedState.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load your installed mods" subtitle={installedState.message}/>}
                        {installedState.kind === 'ready' && (
                            <BrowseItemsView
                                items={visibleInstalledItems}
                                viewMode={viewMode}
                                emptyIcon={installedSearch || selectedTagFilter ? 'fa-magnifying-glass' : 'fa-box-open'}
                                emptyMessage={
                                    installedSearch || selectedTagFilter
                                        ? 'No installed mods match that.'
                                        : 'Nothing installed from LoversLab yet for this game.'
                                }
                            />
                        )}
                    </>
                ) : !status?.SignedIn ? (
                    <EmptyState icon="fa-lock" title="Sign in to browse LoversLab" subtitle="Enter a username or email and a password on the right to see your Paradox games' real Downloads sections."/>
                ) : !selectedCategory ? (
                    <EmptyState icon="fa-gamepad" title="Pick a game" subtitle="Choose a game from the sidebar to see what's in it."/>
                ) : (
                    <>
                        <div className="browse-toolbar column">
                            <div className="browse-toolbar-row">
                                <div className="search-box">
                                    <i className="fa-solid fa-magnifying-glass"/>
                                    <input
                                        placeholder={`Search ${selectedCategory.Name}...`}
                                        value={search}
                                        onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                                    />
                                </div>
                            </div>
                            <div className="browse-toolbar-row">
                                {filesState.kind === 'ready' && (
                                    <span className="mono browse-installed-stats">Page {page} of {filesState.totalPages}</span>
                                )}
                                <div className="spacer"/>
                                {filesState.kind === 'ready' && filesState.totalPages > 1 && (
                                    <div className="browse-pager">
                                        <i
                                            className={`fa-solid fa-chevron-left ${page <= 1 ? 'inert' : ''}`}
                                            onClick={() => page > 1 && setPage(page - 1)}
                                        />
                                        {pageButtons(page, filesState.totalPages).map((n) => (
                                            <span
                                                key={n}
                                                className={`browse-pager-page mono ${n === page ? 'active' : ''}`}
                                                onClick={() => setPage(n)}
                                            >
                                                {n}
                                            </span>
                                        ))}
                                        <i
                                            className={`fa-solid fa-chevron-right ${page >= filesState.totalPages ? 'inert' : ''}`}
                                            onClick={() => page < filesState.totalPages && setPage(page + 1)}
                                        />
                                    </div>
                                )}
                                <ViewModeToggle mode={viewMode} onChange={setViewMode}/>
                            </div>
                        </div>

                        {filesState.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                        {filesState.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load this category" subtitle={filesState.message}/>}
                        {filesState.kind === 'ready' && (
                            <BrowseItemsView
                                items={visibleBrowsingItems}
                                viewMode={viewMode}
                                emptyIcon={search ? 'fa-magnifying-glass' : 'fa-box-open'}
                                emptyMessage={search ? 'No files match that search on this page.' : 'No files here.'}
                            />
                        )}
                    </>
                )}
            </div>

            <div className="browse-account">
                <div className="sidebar-label">SITE LOGINS</div>
                {!status ? (
                    <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>
                ) : (
                    <div className="browse-account-card">
                        <div className="browse-account-brand">
                            <span className="browse-source-swatch loverslab">LL</span>
                            <span className="browse-account-brand-name">LoversLab</span>
                            <span className={`browse-account-state ${status.SignedIn ? 'good' : 'neutral'} mono`}>
                                {status.SignedIn ? 'SIGNED IN' : 'SIGNED OUT'}
                            </span>
                        </div>

                        {status.SignedIn ? (
                            <>
                                <div className="browse-account-identity">
                                    <Avatar name={profile?.Username || status.Username} url={profile?.AvatarURL} size={34}/>
                                    <div className="browse-account-identity-text">
                                        <div
                                            className={profile?.ProfileURL ? 'browse-account-username clickable' : 'browse-account-username'}
                                            title={profile?.ProfileURL ? 'Open your profile' : undefined}
                                            onClick={() => profile?.ProfileURL && BrowserOpenURL(profile.ProfileURL)}
                                        >
                                            {profile?.Username || status.Username}
                                        </div>
                                    </div>
                                    <span
                                        className="browse-notifications-bell"
                                        title={unreadNotifications > 0
                                            ? `${unreadNotifications} unread on LoversLab - open notifications`
                                            : 'No unread LoversLab notifications - open notifications'}
                                        onClick={() => BrowserOpenURL(loversLabNotificationsURL())}
                                    >
                                        <i className="fa-solid fa-bell"/>
                                        {unreadNotifications > 0 && <span className="browse-notifications-badge">{unreadNotifications}</span>}
                                    </span>
                                </div>
                                <div className="browse-account-actions">
                                    <button type="button" className="btn-ghost" onClick={() => setShowSignInFields((v) => !v)}>
                                        {showSignInFields ? 'Cancel' : 'Change sign-in'}
                                    </button>
                                    <button type="button" className="btn-ghost" disabled={busy !== null} onClick={clearSignIn}>
                                        {busy === 'clear' ? 'Signing out...' : 'Sign out'}
                                    </button>
                                </div>
                            </>
                        ) : (
                            <div className={`browse-account-hint ${status.Unreadable ? 'warn' : ''}`}>
                                {status.Unreadable
                                    ? 'A sign-in is saved but cannot be read on this computer - sign in again.'
                                    : 'Browsing works signed out. Downloads and comments need an account.'}
                            </div>
                        )}

                        {(!status.SignedIn || showSignInFields) && (
                            <>
                                <label className="browse-account-field">
                                    <span className="browse-account-label">Email or username</span>
                                    <input
                                        className="browse-account-input"
                                        value={username}
                                        disabled={busy !== null}
                                        placeholder={status.SignedIn ? status.Username : 'you@example.com'}
                                        onInput={(e) => setUsername((e.target as HTMLInputElement).value)}
                                    />
                                </label>
                                <label className="browse-account-field">
                                    <span className="browse-account-label">Password</span>
                                    <input
                                        type="password"
                                        autocomplete="off"
                                        spellcheck={false}
                                        className="browse-account-input"
                                        value={password}
                                        disabled={busy !== null}
                                        placeholder={status.SignedIn ? 'Enter a new password to replace it' : '••••••••••'}
                                        onInput={(e) => setPassword((e.target as HTMLInputElement).value)}
                                        onKeyDown={(e) => e.key === 'Enter' && canSave && save()}
                                    />
                                </label>

                                {error && <div className="browse-account-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}

                                <button type="button" className="btn-primary browse-account-submit" disabled={!canSave} onClick={save}>
                                    {busy === 'save' ? 'Saving...' : status.SignedIn ? 'Update sign-in' : 'Log in'}
                                </button>
                            </>
                        )}

                        {/* The real Protection sentence (status.Protection) is accurate but long -
                            this card is compact, so a short, fixed label stands in for it here;
                            the full explanation still shows in Settings' own Steam API panel,
                            which has the room for it. */}
                        <div className="browse-account-note">
                            <i className="fa-solid fa-lock"/> Securely Encrypted
                        </div>
                    </div>
                )}

                {status?.SignedIn && missingFilesCount > 0 && (
                    <div className="browse-account-alert">
                        <div className="browse-account-alert-title mono">FILES MISSING &middot; {missingFilesCount}</div>
                        <div className="browse-account-alert-body">
                            {missingFilesCount === 1 ? 'One installed mod' : `${missingFilesCount} installed mods`} can't be found on
                            disk any more. Open Installed in the sidebar to reinstall or forget {missingFilesCount === 1 ? 'it' : 'them'}.
                        </div>
                    </div>
                )}
            </div>

            {detailFor && (
                <div className="overlay" onClick={closeDetail}>
                    <div className="browse-detail-modal" onClick={(e) => e.stopPropagation()}>
                        <div className="browse-detail-header">
                            <span className="title" title={detailFor.Title}>{detailFor.Title}</span>
                            <div className="spacer"/>
                            {isInstalled && installState.kind === 'idle' && (
                                <span className="btn-ghost danger browse-header-action" onClick={() => setInstallState({kind: 'uninstall-confirm'})}>
                                    <i className="fa-solid fa-trash-can"/> Uninstall
                                </span>
                            )}
                            <i className="fa-solid fa-up-right-from-square" title="Open on LoversLab" onClick={() => BrowserOpenURL(detailFor.URL)}/>
                            <i
                                className={`fa-solid fa-xmark close-btn ${(installState.kind === 'installing' || installState.kind === 'uninstalling') ? 'inert' : ''}`}
                                title={(installState.kind === 'installing' || installState.kind === 'uninstalling') ? 'Wait for this to finish first' : undefined}
                                onClick={closeDetail}
                            />
                        </div>
                        <div className="browse-detail-content">
                            <div className="browse-detail-left">
                                <div className="browse-detail-media">
                                    {heroURL ? (
                                        <div
                                            className="browse-detail-hero"
                                            title="Open full size"
                                            onClick={() => BrowserOpenURL(heroURL)}
                                        >
                                            <div className="browse-card-thumb-backdrop" style={{backgroundImage: `url(${heroURL})`}}/>
                                            <img className="browse-card-thumb-fg" src={heroURL} alt=""/>
                                            {screenshots.length > 1 && (
                                                <>
                                                    <span
                                                        className="browse-detail-hero-nav prev"
                                                        onClick={(e) => { e.stopPropagation(); setScreenshotIndex((i) => (i - 1 + screenshots.length) % screenshots.length); }}
                                                    >
                                                        <i className="fa-solid fa-chevron-left"/>
                                                    </span>
                                                    <span
                                                        className="browse-detail-hero-nav next"
                                                        onClick={(e) => { e.stopPropagation(); setScreenshotIndex((i) => (i + 1) % screenshots.length); }}
                                                    >
                                                        <i className="fa-solid fa-chevron-right"/>
                                                    </span>
                                                    <span className="browse-detail-hero-count mono">{screenshotIndex + 1} / {screenshots.length}</span>
                                                </>
                                            )}
                                        </div>
                                    ) : (
                                        <div className="browse-detail-hero placeholder"><i className="fa-solid fa-image"/></div>
                                    )}
                                    {screenshots.length > 1 && (
                                        <div className="browse-detail-thumbstrip">
                                            {screenshots.map((s, i) => (
                                                <div
                                                    key={i}
                                                    className={`browse-detail-thumbstrip-item ${i === screenshotIndex ? 'active' : ''}`}
                                                    onClick={() => setScreenshotIndex(i)}
                                                >
                                                    <div className="browse-card-thumb-backdrop" style={{backgroundImage: `url(${s.ThumbnailURL || s.URL})`}}/>
                                                    <img className="browse-card-thumb-fg" src={s.ThumbnailURL || s.URL} alt=""/>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </div>
                                <div className="browse-detail-tabs">
                                    <span className={`browse-detail-tab ${detailTab === 'overview' ? 'active' : ''}`} onClick={() => setDetailTab('overview')}>
                                        Overview
                                    </span>
                                    <span className={`browse-detail-tab ${detailTab === 'files' ? 'active' : ''}`} onClick={() => setDetailTab('files')}>
                                        Files{filesTabState.kind === 'ready' ? ` (${filesTabState.downloads.length})` : ''}
                                    </span>
                                    <span className={`browse-detail-tab ${detailTab === 'changelog' ? 'active' : ''}`} onClick={() => setDetailTab('changelog')}>
                                        Changelog{changelogState?.kind === 'ready' ? ` (${changelogState.entries.length})` : ''}
                                    </span>
                                    <span className={`browse-detail-tab ${detailTab === 'comments' ? 'active' : ''}`} onClick={() => setDetailTab('comments')}>
                                        Comments{commentsState?.kind === 'ready' && commentsState.hasTopic ? ` (${commentsState.posts.length})` : ''}
                                    </span>
                                </div>
                                <div className="browse-detail-tab-body" onScroll={onDetailTabBodyScroll}>
                                    {installState.kind !== 'idle' && (
                                        <div className="browse-install-panel">
                                            {installState.kind === 'error' && (
                                                <div className="browse-install-error">
                                                    <i className="fa-solid fa-circle-exclamation"/> {installState.message}
                                                    <span className="browse-install-dismiss" onClick={() => setInstallState({kind: 'idle'})}>Dismiss</span>
                                                </div>
                                            )}
                                            {installState.kind === 'installing' && (
                                                <>
                                                    <div className="browse-install-label">
                                                        {installState.progress?.Stage === 'extracting' ? 'Extracting' : 'Downloading'}
                                                        {installState.progress?.FileName ? ` ${installState.progress.FileName}` : '...'}
                                                        {installState.progress && installState.progress.FileCount > 1
                                                            ? ` (${installState.progress.FileIndex} of ${installState.progress.FileCount})`
                                                            : ''}
                                                    </div>
                                                    <div className="browse-install-bar">
                                                        <div
                                                            className={`browse-install-bar-fill ${!installState.progress || installState.progress.Total <= 0 ? 'indeterminate' : ''}`}
                                                            style={
                                                                installState.progress && installState.progress.Total > 0
                                                                    ? {width: `${Math.min(100, (installState.progress.Done / installState.progress.Total) * 100)}%`}
                                                                    : undefined
                                                            }
                                                        />
                                                    </div>
                                                    <div className="browse-install-actions">
                                                        <button type="button" className="btn-ghost" onClick={cancelInstall}>Cancel</button>
                                                    </div>
                                                </>
                                            )}
                                            {installState.kind === 'uninstall-confirm' && (
                                                <>
                                                    <div className="browse-install-label">
                                                        Uninstall <strong>{detailFor.Title}</strong>? This deletes its files from your mod folder.
                                                    </div>
                                                    <div className="browse-install-actions">
                                                        <button type="button" className="btn-danger" onClick={uninstallCurrent}>Uninstall</button>
                                                        <button type="button" className="btn-ghost" onClick={() => setInstallState({kind: 'idle'})}>Cancel</button>
                                                    </div>
                                                </>
                                            )}
                                            {installState.kind === 'uninstalling' && (
                                                <div className="browse-install-label">Uninstalling...</div>
                                            )}
                                        </div>
                                    )}

                                    {detailTab === 'overview' && (
                                        <>
                                            {detailState?.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                                            {detailState?.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load this mod's page" subtitle={detailState.message}/>}
                                            {detailState?.kind === 'ready' && descriptionBlocks.length > 0 && (
                                                <div className="browse-detail-description rich">
                                                    <DescriptionBlocksView blocks={descriptionBlocks}/>
                                                </div>
                                            )}
                                            {detailState?.kind === 'ready' && descriptionBlocks.length === 0 && detailState.detail.Description && (
                                                <div className="browse-detail-description">{detailState.detail.Description}</div>
                                            )}
                                            {extras && extras.features.length > 0 && (
                                                <>
                                                    <div className="browse-detail-section-label">FEATURES</div>
                                                    <ul className="browse-detail-features">
                                                        {extras.features.map((f, i) => <li key={i}>{f}</li>)}
                                                    </ul>
                                                </>
                                            )}
                                        </>
                                    )}

                                    {detailTab === 'changelog' && (
                                        <>
                                            {changelogState?.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                                            {changelogState?.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load the changelog" subtitle={changelogState.message}/>}
                                            {changelogState?.kind === 'ready' && changelogState.entries.length === 0 && (
                                                <EmptyState icon="fa-clock-rotate-left" title="No changelog" subtitle="The author hasn't written release notes for this file."/>
                                            )}
                                            {changelogState?.kind === 'ready' && changelogState.entries.map((entry, i) => (
                                                <div key={i} className="browse-changelog-entry" style={{borderLeftColor: colorFromName(entry.Version || String(i))}}>
                                                    <div className="browse-changelog-version">
                                                        <span className="mono">{entry.Version}</span>
                                                        {entry.Released && <span className="browse-changelog-released">{entry.Released}</span>}
                                                    </div>
                                                    {entry.DescriptionBlocks && entry.DescriptionBlocks.length > 0 ? (
                                                        <div className="browse-changelog-description rich">
                                                            <DescriptionBlocksView blocks={entry.DescriptionBlocks}/>
                                                        </div>
                                                    ) : entry.Description && (
                                                        <div className="browse-changelog-description">{entry.Description}</div>
                                                    )}
                                                </div>
                                            ))}
                                        </>
                                    )}

                                    {detailTab === 'files' && (
                                        <>
                                            {filesTabState.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Finding downloadable files..."/>}
                                            {filesTabState.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't find the downloadable files" subtitle={filesTabState.message}/>}
                                            {filesTabState.kind === 'ready' && filesTabState.downloads.length === 0 && (
                                                <EmptyState icon="fa-file-circle-question" title="No downloadable files" subtitle="None were found for this mod."/>
                                            )}
                                            {filesTabState.kind === 'ready' && filesTabState.downloads.length > 0 && (() => {
                                                const downloads = filesTabState.downloads;
                                                const selectedCount = selectedFileIndexes.size;
                                                const totalBytes = downloads.reduce(
                                                    (sum, d, i) => selectedFileIndexes.has(i) ? sum + parseSizeToBytes(d.Size) : sum, 0);
                                                const busy = installState.kind !== 'idle';
                                                return (
                                                    <>
                                                        <div className="browse-files-header">
                                                            <div className="browse-files-title">Choose files to install</div>
                                                            <div className="browse-files-count">
                                                                This mod ships {downloads.length} download{downloads.length === 1 ? '' : 's'}. Pick any combination.
                                                            </div>
                                                        </div>
                                                        {downloads.map((d, i) => (
                                                            <label key={i} className={`browse-file-row ${selectedFileIndexes.has(i) ? 'selected' : ''}`}>
                                                                <Checkbox
                                                                    checked={selectedFileIndexes.has(i)}
                                                                    onChange={() => toggleFileSelected(i)}
                                                                    disabled={busy}
                                                                />
                                                                <span className="browse-file-name" title={d.Name}>{d.Name}</span>
                                                                {(d.Size || d.Posted) && (
                                                                    <span className="browse-file-meta">{[d.Size, d.Posted].filter(Boolean).join(' · ')}</span>
                                                                )}
                                                            </label>
                                                        ))}
                                                        <div className="browse-files-footer">
                                                            <div className="browse-files-selection-row">
                                                                <span>
                                                                    {selectedCount} of {downloads.length} selected
                                                                    {' · '}
                                                                    <span
                                                                        className={`browse-files-select-all ${busy ? 'inert' : ''}`}
                                                                        onClick={() => !busy && toggleSelectAllFiles(downloads.length)}
                                                                    >
                                                                        {selectedCount >= downloads.length ? 'Deselect all' : 'Select all'}
                                                                    </span>
                                                                </span>
                                                                <span className="mono">{formatBytes(totalBytes)}</span>
                                                            </div>
                                                            <div className="browse-files-actions">
                                                                <button
                                                                    type="button"
                                                                    className="btn-ghost"
                                                                    disabled={busy || selectedCount === 0}
                                                                    onClick={() => setSelectedFileIndexes(new Set())}
                                                                >
                                                                    Cancel
                                                                </button>
                                                                <button
                                                                    type="button"
                                                                    className="btn-primary"
                                                                    disabled={busy || selectedCount === 0}
                                                                    onClick={() => runInstall(downloads.filter((_, i) => selectedFileIndexes.has(i)))}
                                                                >
                                                                    {isInstalled ? 'Update' : 'Download & install'} {selectedCount} file{selectedCount === 1 ? '' : 's'}
                                                                </button>
                                                            </div>
                                                        </div>
                                                    </>
                                                );
                                            })()}
                                        </>
                                    )}

                                    {detailTab === 'comments' && (
                                        <>
                                            {commentsState?.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                                            {commentsState?.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load comments" subtitle={commentsState.message}/>}
                                            {commentsState?.kind === 'ready' && commentsState.hasTopic && (
                                                <div className="browse-comment-write">
                                                    <Avatar name={status?.Username || '?'} size={28}/>
                                                    <div className="browse-comment-write-box">
                                                        <textarea
                                                            className="browse-comment-input"
                                                            placeholder="Write a reply..."
                                                            value={commentDraft}
                                                            disabled={postingComment}
                                                            onInput={(e) => setCommentDraft((e.target as HTMLTextAreaElement).value)}
                                                        />
                                                        <button
                                                            type="button"
                                                            className="btn-primary browse-comment-post-btn"
                                                            disabled={postingComment || commentDraft.trim() === ''}
                                                            onClick={postComment}
                                                        >
                                                            {postingComment ? 'Posting...' : 'Post'}
                                                        </button>
                                                    </div>
                                                </div>
                                            )}
                                            {commentsState?.kind === 'ready' && !commentsState.hasTopic && (
                                                <EmptyState icon="fa-comment-slash" title="No support topic" subtitle="The author hasn't linked a discussion thread."/>
                                            )}
                                            {commentsState?.kind === 'ready' && commentsState.hasTopic && commentsState.posts.length === 0 && (
                                                <EmptyState icon="fa-comments" title="No comments yet" subtitle="The support topic exists. Write the first comment above."/>
                                            )}
                                            {commentsState?.kind === 'ready' && commentsState.posts.map((post) => {
                                                // Two real signals for "this reply's author started the topic": the
                                                // dropped-first-post name match (LoversLabCommentList.TopicAuthor,
                                                // the only signal that covers the topic's own opening post, which
                                                // never carries its own Author badge - confirmed live) and the
                                                // real per-reply badge itself (Post.IsTopicAuthor, confirmed live to
                                                // cover every later reply they post in the same topic too, not
                                                // just their first).
                                                const isTopicAuthor = post.IsTopicAuthor || (topicAuthor !== '' && post.Author === topicAuthor);
                                                const contentBlocks = post.ContentBlocks ?? [];
                                                return (
                                                    <div key={post.ID} className="browse-comment">
                                                        <div className="browse-comment-authorcol">
                                                            <Avatar name={post.Author} url={post.AuthorAvatarURL} size={44}/>
                                                            <span
                                                                className={post.AuthorURL ? 'browse-comment-author clickable' : 'browse-comment-author'}
                                                                onClick={() => post.AuthorURL && BrowserOpenURL(post.AuthorURL)}
                                                            >
                                                                {post.Author}
                                                            </span>
                                                            {post.AuthorTitle && <span className="browse-comment-authortitle">{post.AuthorTitle}</span>}
                                                            {post.AuthorGroup && <span className="browse-comment-authorgroup">{post.AuthorGroup}</span>}
                                                            {post.AuthorPostCount > 0 && <span className="browse-comment-authorposts mono">{post.AuthorPostCount} posts</span>}
                                                        </div>
                                                        <div className="browse-comment-body">
                                                            <div className="browse-comment-header">
                                                                {isTopicAuthor && <span className="browse-comment-badge author">TOPIC AUTHOR</span>}
                                                                {post.IsPopular && <span className="browse-comment-badge popular">POPULAR POST</span>}
                                                                <span className="browse-comment-posted">{post.Posted}{post.Edited ? ' (edited)' : ''}</span>
                                                                <div className="spacer"/>
                                                                {post.URL && (
                                                                    <i
                                                                        className="fa-solid fa-up-right-from-square"
                                                                        title="Open this reply on LoversLab"
                                                                        onClick={() => BrowserOpenURL(post.URL)}
                                                                    />
                                                                )}
                                                            </div>
                                                            {contentBlocks.length > 0 ? (
                                                                <div className="browse-comment-content rich">
                                                                    <DescriptionBlocksView blocks={contentBlocks}/>
                                                                </div>
                                                            ) : post.Content && (
                                                                <div className="browse-comment-content">{post.Content}</div>
                                                            )}
                                                            {post.Attachments.length > 0 && (
                                                                <div className="browse-comment-attachments">
                                                                    {post.Attachments.map((a, i) => (
                                                                        <span key={i} className="browse-comment-attachment" onClick={() => BrowserOpenURL(a.URL)}>
                                                                            <i className={`fa-solid ${a.IsImage ? 'fa-image' : 'fa-paperclip'}`}/> {a.Filename || 'attachment'}
                                                                        </span>
                                                                    ))}
                                                                </div>
                                                            )}
                                                            <div className="browse-comment-footer">
                                                                <span className="browse-comment-likes"><i className="fa-solid fa-heart"/> {post.Reactions}</span>
                                                            </div>
                                                        </div>
                                                    </div>
                                                );
                                            })}
                                            {commentsState?.kind === 'ready' && commentsState.hasTopic && commentsState.page < commentsState.totalPages && (
                                                <div className="browse-comments-load-more">
                                                    {commentsState.loadingMore ? (
                                                        <span><i className="fa-solid fa-spinner fa-spin"/> Loading more...</span>
                                                    ) : (
                                                        <span className="link-btn" onClick={loadMoreComments}>Load more comments</span>
                                                    )}
                                                </div>
                                            )}
                                        </>
                                    )}
                                </div>
                            </div>

                            <div className="browse-detail-right">
                                {extras && <span className="browse-tag browse-detail-tag" style={{background: extras.tagColor}}>{extras.tag}</span>}
                                <div className="browse-detail-title" title={detailFor.Title}>{detailFor.Title}</div>
                                {detailState?.kind === 'ready' && (
                                    <div
                                        className={detailState.detail.Author.URL ? 'browse-detail-authorrow clickable' : 'browse-detail-authorrow'}
                                        onClick={() => detailState.detail.Author.URL && BrowserOpenURL(detailState.detail.Author.URL)}
                                    >
                                        <Avatar name={detailState.detail.Author.Name} url={detailState.detail.Author.ImageURL} size={32}/>
                                        <div className="browse-detail-authorrow-text">
                                            <div className="browse-detail-authorname">{detailState.detail.Author.Name}</div>
                                            <div className="browse-detail-authorlabel">Author{detailState.detail.Author.URL ? ' · opens profile' : ''}</div>
                                        </div>
                                    </div>
                                )}
                                {extras && extras.tags.length > 0 && (
                                    <div className="browse-detail-tags">
                                        {extras.tags.map((t) => <span key={t} className="browse-detail-tagpill">{t}</span>)}
                                    </div>
                                )}
                                <div className="browse-detail-actions">
                                    {isInstalled ? (
                                        <span className="browse-detail-installed-pill"><i className="fa-solid fa-circle-check"/> Installed</span>
                                    ) : (
                                        <button type="button" className="btn-primary browse-detail-install-btn" onClick={() => setDetailTab('files')}>
                                            <i className="fa-solid fa-download"/> Install
                                        </button>
                                    )}
                                </div>
                                {detailState?.kind === 'ready' && (
                                    <div className="browse-detail-statgrid">
                                        <div className="browse-detail-statitem">
                                            <div className="mono value">{detailState.detail.Views.toLocaleString()}</div>
                                            <div className="label">Views</div>
                                        </div>
                                        <div className="browse-detail-statitem">
                                            <div className="mono value">{detailState.detail.Downloads.toLocaleString()}</div>
                                            <div className="label">Downloads</div>
                                        </div>
                                        {extras && (
                                            <>
                                                <div className="browse-detail-statitem">
                                                    <div className="mono value">{extras.followers}</div>
                                                    <div className="label">Followers</div>
                                                </div>
                                                <div className="browse-detail-statitem">
                                                    <div className="mono value">{extras.likes}</div>
                                                    <div className="label">Likes</div>
                                                </div>
                                            </>
                                        )}
                                    </div>
                                )}
                                {extras && (
                                    <div className="browse-detail-datesgrid">
                                        <span className="label">Submitted</span><span className="value">{extras.submitted}</span>
                                        <span className="label">Published</span><span className="value">{extras.published}</span>
                                        {detailState?.kind === 'ready' && detailState.detail.DateModified && (
                                            <>
                                                <span className="label">Updated</span><span className="value">{formatUpdated(detailState.detail.DateModified)}</span>
                                            </>
                                        )}
                                        {detailState?.kind === 'ready' && detailState.detail.FileSize && (
                                            <>
                                                <span className="label">File size</span><span className="value mono">{detailState.detail.FileSize}</span>
                                            </>
                                        )}
                                    </div>
                                )}
                                {extras && extras.requirements.length > 0 && (
                                    <div className="browse-detail-requirements">
                                        <div className="browse-detail-section-label">REQUIREMENTS</div>
                                        {extras.requirements.map((r, i) => (
                                            <div
                                                key={i}
                                                className={`browse-requirement-row clickable ${r.installed ? 'installed' : ''}`}
                                                title="Open in Workspace"
                                                onClick={() => onOpenInWorkspace(r.name)}
                                            >
                                                <i className={`fa-solid ${r.installed ? 'fa-check' : 'fa-circle'}`}/>
                                                <span className="browse-requirement-name">{r.name}</span>
                                                {r.installed && <span className="browse-requirement-status mono">INSTALLED</span>}
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

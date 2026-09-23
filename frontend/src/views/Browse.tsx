import './Browse.css';
import {Fragment, h} from 'preact';
import type {JSX} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {
    CancelLoversLabInstall,
    ClearLoversLabCredentials,
    LoversLabCategories,
    LoversLabChangelog,
    LoversLabComments,
    LoversLabDownloadDialog,
    LoversLabFileDetail,
    LoversLabFiles,
    LoversLabInstall,
    LoversLabInstalledMods,
    LoversLabPostComment,
    LoversLabStatus,
    SaveLoversLabCredentials,
    UninstallLoversLabMod,
} from '../../wailsjs/go/main/App';
import type {app, library, loverslab} from '../../wailsjs/go/models';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import {Avatar} from '../components/Avatar';
import {EmptyState} from '../components/EmptyState';
import {openContextMenu} from '../data/contextMenu';
import {mockCommentExtrasFor, mockExtrasFor} from '../data/browseMockData';
import {checkLoversLabNotifications, loversLabNotificationsURL, useLoversLabUnreadCount} from '../data/loversLabNotifications';
import {checkLoversLabUpdates} from '../data/modUpdates';
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
    | { kind: 'ready'; files: loverslab.FileSummary[]; totalPages: number };

type ChangelogState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; entries: loverslab.ChangelogEntry[] };

type DetailState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; detail: loverslab.FileDetail };

type CommentsState =
    | { kind: 'loading' }
    | { kind: 'error'; message: string }
    | { kind: 'ready'; posts: loverslab.Post[]; totalPages: number; hasTopic: boolean };

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

// The Files tab's own list of what's downloadable for the open file - loaded once,
// lazily, the first time that tab is opened.
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

// Installing (from a Files-tab row's Install/Update button) and uninstalling (from
// the header or the Files tab) share this one piece of state - both are shown the
// same way, inline at the top of the tab body, never a second popup on top of the
// detail modal itself.
type InstallState =
    | { kind: 'idle' }
    | { kind: 'confirm'; download: loverslab.FileDownload }
    | { kind: 'installing'; download: loverslab.FileDownload; progress: LoversLabInstallProgressEvent | null }
    | { kind: 'uninstall-confirm' }
    | { kind: 'uninstalling' }
    | { kind: 'error'; message: string };

function errorText(err: unknown): string {
    return String(err).replace(/^Error:\s*/, '');
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
    stateIcon?: 'installed' | 'missing' | null;
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
            <i
                className={`fa-solid fa-grip ${mode === 'cards' ? 'active' : ''}`}
                title="Card view"
                onClick={() => onChange('cards')}
            />
            <i
                className={`fa-solid fa-list ${mode === 'tree' ? 'active' : ''}`}
                title="List view"
                onClick={() => onChange('tree')}
            />
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
                                <div
                                    className="browse-tree-thumb"
                                    style={it.thumbnailURL ? {backgroundImage: `url(${it.thumbnailURL})`} : undefined}
                                >
                                    {!it.thumbnailURL && <i className="fa-solid fa-image"/>}
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
                            <div
                                className={it.thumbnailURL ? 'browse-card-thumb' : 'browse-card-thumb placeholder'}
                                style={it.thumbnailURL ? {backgroundImage: `url(${it.thumbnailURL})`} : undefined}
                            >
                                {!it.thumbnailURL && <i className="fa-solid fa-image"/>}
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

// StateBadge is the small circular install-state indicator overlaid on a card's
// thumbnail (and shown plainly in list view) - installed (a real, tracked
// LoversLab install) or missing (installed but its files are gone from disk,
// ContentMissing) - both real. null/undefined (not yet installed) renders nothing,
// rather than a badge for an absence.
function StateBadge({state, className}: {state?: 'installed' | 'missing' | null; className: string}) {
    if (!state) return null;
    return (
        <span className={`browse-state-badge ${className} ${state}`} title={state === 'installed' ? 'Installed' : 'Installed, but its files are missing'}>
            <i className={`fa-solid ${state === 'installed' ? 'fa-check' : 'fa-triangle-exclamation'}`}/>
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
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [busy, setBusy] = useState<'save' | 'clear' | null>(null);
    const [error, setError] = useState('');

    const [categoryState, setCategoryState] = useState<CategoryState>({kind: 'idle'});
    const [selectedCategory, setSelectedCategory] = useState<loverslab.Category | null>(null);
    const [page, setPage] = useState(1);
    const [filesState, setFilesState] = useState<FilesState>({kind: 'idle'});
    const [search, setSearch] = useState('');
    // The one shared choice (see BrowseItemsView) both the browsing grid and the
    // Installed section switch between - the same view either way, not a separate
    // toggle each remembers on its own.
    const [viewMode, setViewMode] = useState<ViewMode>('cards');

    // The "Installed" section: a new place in the sidebar, alongside GAMES, to see and
    // uninstall everything installed from LoversLab for the selected game.
    const [viewingInstalled, setViewingInstalled] = useState(false);
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
    const [commentsPage, setCommentsPage] = useState(1);
    const [commentDraft, setCommentDraft] = useState('');
    const [postingComment, setPostingComment] = useState(false);
    const [filesTabState, setFilesTabState] = useState<FilesTabState>({kind: 'idle'});

    const [installState, setInstallState] = useState<InstallState>({kind: 'idle'});
    const installRequestRef = useRef<string | null>(null);

    useEffect(() => {
        LoversLabStatus().then(setStatus).catch(() => undefined);
    }, []);

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
    function openDetail(file: loverslab.FileSummary) {
        setDetailFor(file);
        setDetailTab('overview');
        setScreenshotIndex(0);
        setCommentsPage(1);
        setInstallState({kind: 'idle'});
        setFilesTabState({kind: 'idle'});

        setDetailState({kind: 'loading'});
        LoversLabFileDetail(file.URL)
            .then((detail) => setDetailState({kind: 'ready', detail}))
            .catch((err) => setDetailState({kind: 'error', message: errorText(err)}));

        setChangelogState({kind: 'loading'});
        LoversLabChangelog(file.URL)
            .then((entries) => setChangelogState({kind: 'ready', entries: entries ?? []}))
            .catch((err) => setChangelogState({kind: 'error', message: errorText(err)}));
    }

    useEffect(() => {
        if (!detailFor) {
            setCommentsState(null);
            setTopicAuthor('');
            return;
        }
        let cancelled = false;
        setCommentsState({kind: 'loading'});
        LoversLabComments(detailFor.URL, commentsPage)
            .then((result) => {
                if (cancelled) return;
                setCommentsState({kind: 'ready', posts: result.Posts ?? [], totalPages: result.TotalPages || 1, hasTopic: result.HasTopic});
                setTopicAuthor((prev) => mergeTopicAuthor(prev, result.TopicAuthor));
            })
            .catch((err) => { if (!cancelled) setCommentsState({kind: 'error', message: errorText(err)}); });
        return () => { cancelled = true; };
    }, [detailFor, commentsPage]);

    // The Files tab's own list of what's downloadable, loaded once the first time that
    // tab is opened (not eagerly with the rest of the detail, since most visits to a
    // mod's page never need it).
    useEffect(() => {
        if (!detailFor || detailTab !== 'files' || filesTabState.kind !== 'idle') return;
        setFilesTabState({kind: 'loading'});
        LoversLabDownloadDialog(detailFor.URL)
            .then((downloads) => setFilesTabState({kind: 'ready', downloads: downloads ?? []}))
            .catch((err) => setFilesTabState({kind: 'error', message: errorText(err)}));
    }, [detailFor, detailTab, filesTabState.kind]);

    // Opening a mod straight from the Installed list: only FileID/Title/FileURL are
    // known there (see LoversLabInstalledMod) - the rest (author, screenshots,
    // description) loads the same way it does for a card from the browsing grid.
    function openInstalledDetail(m: app.LoversLabInstalledMod) {
        openDetail({ID: m.FileID, Title: m.Title, URL: m.FileURL, Author: '', AuthorURL: '', Updated: '', ThumbnailURL: ''} as loverslab.FileSummary);
    }

    // Posting a reply: always reloads page 1 afterward rather than trying to splice the
    // new reply into whatever page is currently shown - simpler, and correct regardless
    // of which page the person was looking at when they posted.
    async function postComment() {
        if (!detailFor) return;
        const content = commentDraft.trim();
        if (!content) return;
        setPostingComment(true);
        try {
            await LoversLabPostComment(detailFor.URL, content);
            setCommentDraft('');
            notify('success', 'Comment posted.');
            if (commentsPage === 1) {
                const result = await LoversLabComments(detailFor.URL, 1);
                setCommentsState({kind: 'ready', posts: result.Posts ?? [], totalPages: result.TotalPages || 1, hasTopic: result.HasTopic});
                setTopicAuthor((prev) => mergeTopicAuthor(prev, result.TopicAuthor));
            } else {
                setCommentsPage(1);
            }
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
    }

    async function runInstall(download: loverslab.FileDownload) {
        if (!detailFor) return;
        const requestId = `ll-install-${Date.now()}-${Math.random().toString(36).slice(2)}`;
        installRequestRef.current = requestId;
        setInstallState({kind: 'installing', download, progress: null});
        const off = EventsOn('loverslab-install-progress', (p: LoversLabInstallProgressEvent) => {
            if (p.RequestID === requestId) setInstallState({kind: 'installing', download, progress: p});
        });
        try {
            const dateModified = detailState?.kind === 'ready' ? detailState.detail.DateModified : '';
            await LoversLabInstall(selectedGame, requestId, detailFor, dateModified, download.URL);
            notify('success', `Installed '${detailFor.Title}'.`);
            setInstallState({kind: 'idle'});
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
    const canSave = !busy && username.trim() !== '' && password !== '';
    const isInstalled = detailFor ? installedIds.has(detailFor.ID) : false;
    const missingFilesCount = installedState.kind === 'ready' ? installedState.mods.filter((m) => m.ContentMissing).length : 0;
    // The mock extras (see browseMockData.ts) for whichever mod's detail is open -
    // null while nothing is open, since every one of these fields is only ever
    // rendered inside the detail overlay.
    const extras = detailFor ? mockExtrasFor(detailFor.ID) : null;
    const screenshots = detailState?.kind === 'ready' ? detailState.detail.Screenshots : [];
    const heroShot = screenshots.length > 0 ? screenshots[Math.min(screenshotIndex, screenshots.length - 1)] : null;
    const heroURL = heroShot ? (heroShot.ThumbnailURL || heroShot.URL) : (detailFor?.ThumbnailURL || '');
    const heroFullURL = heroShot ? (heroShot.URL || heroShot.ThumbnailURL) : (detailFor?.ThumbnailURL || '');

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
        return {
            id: f.ID,
            title: f.Title,
            thumbnailURL: f.ThumbnailURL,
            tag: extras.tag,
            tagColor: extras.tagColor,
            stateIcon: installedIds.has(f.ID) ? 'installed' : null,
            authorName: f.Author,
            lineOne: f.Author,
            onLineOneClick: f.AuthorURL ? () => BrowserOpenURL(f.AuthorURL) : undefined,
            lineTwo: f.Updated,
            onClick: () => openDetail(f),
        };
    }), [visibleFiles, installedIds]);

    const installedItems = useMemo<BrowseListItem[]>(() => {
        if (installedState.kind !== 'ready') return [];
        return installedState.mods.map((m) => ({
            id: m.FileID,
            title: m.Title,
            thumbnailURL: '',
            tag: mockExtrasFor(m.FileID).tag,
            tagColor: mockExtrasFor(m.FileID).tagColor,
            stateIcon: m.ContentMissing ? 'missing' : 'installed',
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
    }, [installedState, confirmUninstallId]);

    return (
        <div className="browse">
            <div className="browse-sources">
                <div className="sidebar-label">SOURCES</div>
                <div className="browse-source-row active">
                    <span className="browse-source-swatch loverslab">LL</span>
                    <span className="browse-source-name">LoversLab</span>
                    <span className={`browse-source-dot ${status?.SignedIn ? 'good' : 'neutral'}`}/>
                </div>
                <div className="browse-source-row disabled">
                    <i className="fa-solid fa-plus browse-source-icon"/>
                    <span>More later</span>
                </div>

                {status?.SignedIn && (
                    <>
                        <div className="sidebar-label games-label">GAMES</div>
                        <div className="browse-games">
                            {categoryState.kind === 'loading' && <div className="browse-games-note">Loading...</div>}
                            {categoryState.kind === 'error' && <div className="browse-games-note error">{categoryState.message}</div>}
                            {categoryState.kind === 'ready' && categoryState.categories.map((cat) => (
                                <div
                                    key={cat.URL}
                                    className={`browse-category-row depth-${cat.Depth} ${selectedCategory?.URL === cat.URL ? 'active' : ''}`}
                                    onClick={() => selectCategory(cat)}
                                >
                                    <span className="cat-name" title={cat.Name}>{cat.Name}</span>
                                    <span className="mono cat-count">{cat.Files}</span>
                                </div>
                            ))}
                            {categoryState.kind === 'ready' && categoryState.categories.length === 0 && (
                                <div className="browse-games-note">
                                    LoversLab's Paradox Games section could not be found.
                                </div>
                            )}
                        </div>

                        <div className="sidebar-label">INSTALLED</div>
                        <div className="browse-games">
                            <div
                                className={`browse-category-row depth-0 ${viewingInstalled ? 'active' : ''}`}
                                onClick={selectInstalled}
                            >
                                <span className="cat-name">Installed mods</span>
                                <span className="mono cat-count">{installedIds.size}</span>
                            </div>
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
                        <div className="browse-toolbar">
                            <div className="browse-installed-title">Installed from LoversLab</div>
                            <div className="spacer"/>
                            <ViewModeToggle mode={viewMode} onChange={setViewMode}/>
                        </div>
                        {installedState.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                        {installedState.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load your installed mods" subtitle={installedState.message}/>}
                        {installedState.kind === 'ready' && (
                            <BrowseItemsView
                                items={installedItems}
                                viewMode={viewMode}
                                emptyIcon="fa-box-open"
                                emptyMessage="Nothing installed from LoversLab yet for this game."
                            />
                        )}
                    </>
                ) : !status?.SignedIn ? (
                    <EmptyState icon="fa-lock" title="Sign in to browse LoversLab" subtitle="Enter a username or email and a password on the right to see your Paradox games' real Downloads sections."/>
                ) : !selectedCategory ? (
                    <EmptyState icon="fa-gamepad" title="Pick a game" subtitle="Choose a game from the sidebar to see what's in it."/>
                ) : (
                    <>
                        <div className="browse-toolbar">
                            <div className="search-box">
                                <i className="fa-solid fa-magnifying-glass"/>
                                <input
                                    placeholder={`Search ${selectedCategory.Name}...`}
                                    value={search}
                                    onInput={(e) => setSearch((e.target as HTMLInputElement).value)}
                                />
                            </div>
                            <div className="spacer"/>
                            {filesState.kind === 'ready' && filesState.totalPages > 1 && (
                                <div className="browse-pager">
                                    <i
                                        className={`fa-solid fa-chevron-left ${page <= 1 ? 'inert' : ''}`}
                                        onClick={() => page > 1 && setPage(page - 1)}
                                    />
                                    <span className="mono">Page {page} of {filesState.totalPages}</span>
                                    <i
                                        className={`fa-solid fa-chevron-right ${page >= filesState.totalPages ? 'inert' : ''}`}
                                        onClick={() => page < filesState.totalPages && setPage(page + 1)}
                                    />
                                </div>
                            )}
                            <ViewModeToggle mode={viewMode} onChange={setViewMode}/>
                        </div>

                        {filesState.kind === 'loading' && <EmptyState icon="fa-spinner fa-spin" title="Loading..."/>}
                        {filesState.kind === 'error' && <EmptyState icon="fa-triangle-exclamation" tone="error" title="Couldn't load this category" subtitle={filesState.message}/>}
                        {filesState.kind === 'ready' && (
                            <BrowseItemsView
                                items={browsingItems}
                                viewMode={viewMode}
                                emptyIcon={search ? 'fa-magnifying-glass' : 'fa-box-open'}
                                emptyMessage={search ? 'No files match that search on this page.' : 'No files here.'}
                            />
                        )}
                    </>
                )}
            </div>

            <div className="browse-account">
                <div className="browse-account-head">
                    <div className="sidebar-label">LOVERSLAB SIGN-IN</div>
                    {status?.SignedIn && (
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
                    )}
                </div>
                {!status ? (
                    <p className="status-page">Loading...</p>
                ) : (
                    <>
                        {status.SignedIn ? (
                            <div className="browse-account-identity">
                                <Avatar name={status.Username} size={34}/>
                                <div className="browse-account-identity-text">
                                    <div className="browse-account-username">{status.Username}</div>
                                    <div className="browse-account-protection mono">{status.Protection}</div>
                                </div>
                            </div>
                        ) : (
                            <div className={`browse-account-status ${status.Unreadable ? 'warn' : 'neutral'}`}>
                                <i className={`fa-solid ${status.Unreadable ? 'fa-triangle-exclamation' : 'fa-user'}`}/>
                                <span>
                                    {status.Unreadable
                                        ? 'A sign-in is saved but cannot be read on this computer - sign in again.'
                                        : 'Browsing works signed out. Downloads and comments need an account.'}
                                </span>
                            </div>
                        )}

                        {status.SignedIn && missingFilesCount > 0 && (
                            <div className="browse-account-alert">
                                <div className="browse-account-alert-title mono">FILES MISSING &middot; {missingFilesCount}</div>
                                <div className="browse-account-alert-body">
                                    {missingFilesCount === 1 ? 'One installed mod' : `${missingFilesCount} installed mods`} can't be found on
                                    disk any more. Open Installed in the sidebar to reinstall or forget {missingFilesCount === 1 ? 'it' : 'them'}.
                                </div>
                            </div>
                        )}

                        <label className="browse-account-field">
                            <span className="browse-account-label">Username or email</span>
                            <input
                                className="browse-account-input"
                                value={username}
                                disabled={busy !== null}
                                placeholder={status.SignedIn ? status.Username : 'Your LoversLab username or email'}
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
                                placeholder={status.SignedIn ? 'Enter a new password to replace it' : 'Your LoversLab password'}
                                onInput={(e) => setPassword((e.target as HTMLInputElement).value)}
                                onKeyDown={(e) => e.key === 'Enter' && canSave && save()}
                            />
                        </label>

                        {error && <div className="browse-account-error"><i className="fa-solid fa-circle-exclamation"/> {error}</div>}

                        <div className="browse-account-actions">
                            <button type="button" className="btn-primary" disabled={!canSave} onClick={save}>
                                {busy === 'save' ? 'Saving...' : status.SignedIn ? 'Update sign-in' : 'Sign in'}
                            </button>
                            {status.SignedIn && (
                                <button type="button" className="btn-ghost" disabled={busy !== null} onClick={clearSignIn}>
                                    {busy === 'clear' ? 'Removing...' : 'Sign out'}
                                </button>
                            )}
                        </div>

                        <div className="browse-account-note">
                            <i className="fa-solid fa-lock"/> {status.Protection}
                        </div>
                    </>
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
                                            style={{backgroundImage: `url(${heroURL})`}}
                                            title="Open full size"
                                            onClick={() => BrowserOpenURL(heroFullURL)}
                                        >
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
                                                    style={{backgroundImage: `url(${s.ThumbnailURL || s.URL})`}}
                                                    onClick={() => setScreenshotIndex(i)}
                                                />
                                            ))}
                                        </div>
                                    )}
                                </div>
                                <div className="browse-detail-tabs">
                                    <span className={`browse-detail-tab ${detailTab === 'overview' ? 'active' : ''}`} onClick={() => setDetailTab('overview')}>
                                        Overview
                                    </span>
                                    <span className={`browse-detail-tab ${detailTab === 'files' ? 'active' : ''}`} onClick={() => setDetailTab('files')}>
                                        Files
                                    </span>
                                    <span className={`browse-detail-tab ${detailTab === 'changelog' ? 'active' : ''}`} onClick={() => setDetailTab('changelog')}>
                                        Changelog
                                    </span>
                                    <span className={`browse-detail-tab ${detailTab === 'comments' ? 'active' : ''}`} onClick={() => setDetailTab('comments')}>
                                        Comments{commentsState?.kind === 'ready' && commentsState.hasTopic ? ` (${commentsState.posts.length})` : ''}
                                    </span>
                                </div>
                                <div className="browse-detail-tab-body">
                                    {installState.kind !== 'idle' && (
                                        <div className="browse-install-panel">
                                            {installState.kind === 'error' && (
                                                <div className="browse-install-error">
                                                    <i className="fa-solid fa-circle-exclamation"/> {installState.message}
                                                    <span className="browse-install-dismiss" onClick={() => setInstallState({kind: 'idle'})}>Dismiss</span>
                                                </div>
                                            )}
                                            {installState.kind === 'confirm' && (
                                                <>
                                                    <div className="browse-install-label">
                                                        Download and install <strong>{installState.download.Name}</strong>? If you already have
                                                        this mod installed from LoversLab, this replaces it with this version.
                                                    </div>
                                                    <div className="browse-install-actions">
                                                        <button type="button" className="btn-primary" onClick={() => runInstall(installState.download)}>Install</button>
                                                        <button type="button" className="btn-ghost" onClick={() => setInstallState({kind: 'idle'})}>Cancel</button>
                                                    </div>
                                                </>
                                            )}
                                            {installState.kind === 'installing' && (
                                                <>
                                                    <div className="browse-install-label">
                                                        {installState.progress?.Stage === 'extracting' ? 'Extracting...' : 'Downloading...'}
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
                                            {detailState?.kind === 'ready' && detailState.detail.DescriptionBlocks.length > 0 && (
                                                <div className="browse-detail-description rich">
                                                    {detailState.detail.DescriptionBlocks.map((block, i) => (
                                                        block.ImageURL ? (
                                                            <img key={i} className="browse-description-image" src={block.ImageURL} alt="" loading="lazy"/>
                                                        ) : (
                                                            <p key={i}>{block.Runs.map((run, j) => <DescriptionRunView key={j} run={run}/>)}</p>
                                                        )
                                                    ))}
                                                </div>
                                            )}
                                            {detailState?.kind === 'ready' && detailState.detail.DescriptionBlocks.length === 0 && detailState.detail.Description && (
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
                                                <div key={i} className="browse-changelog-entry">
                                                    <div className="browse-changelog-version">
                                                        <span className="mono">{entry.Version}</span>
                                                        {entry.Released && <span className="browse-changelog-released">{entry.Released}</span>}
                                                    </div>
                                                    <div className="browse-changelog-description">{entry.Description}</div>
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
                                            {filesTabState.kind === 'ready' && filesTabState.downloads.map((d, i) => (
                                                <div key={i} className="browse-file-row">
                                                    <i className="fa-solid fa-file-zipper"/>
                                                    <span className="browse-file-name" title={d.Name}>{d.Name}</span>
                                                    <button
                                                        type="button"
                                                        className="btn-primary"
                                                        disabled={installState.kind !== 'idle'}
                                                        onClick={() => setInstallState({kind: 'confirm', download: d})}
                                                    >
                                                        {isInstalled ? 'Update' : 'Install'}
                                                    </button>
                                                </div>
                                            ))}
                                        </>
                                    )}

                                    {detailTab === 'comments' && (
                                        <>
                                            {commentsState?.kind === 'ready' && commentsState.totalPages > 1 && (
                                                <div className="browse-comments-pager-row">
                                                    <i
                                                        className={`fa-solid fa-chevron-left ${commentsPage <= 1 ? 'inert' : ''}`}
                                                        onClick={() => commentsPage > 1 && setCommentsPage(commentsPage - 1)}
                                                    />
                                                    <span className="mono">{commentsPage} / {commentsState.totalPages}</span>
                                                    <i
                                                        className={`fa-solid fa-chevron-right ${commentsPage >= commentsState.totalPages ? 'inert' : ''}`}
                                                        onClick={() => commentsPage < commentsState.totalPages && setCommentsPage(commentsPage + 1)}
                                                    />
                                                </div>
                                            )}
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
                                                const cx = mockCommentExtrasFor(post.ID);
                                                const isTopicAuthor = topicAuthor !== '' && post.Author === topicAuthor;
                                                return (
                                                    <div key={post.ID} className="browse-comment">
                                                        <div className="browse-comment-authorcol">
                                                            <Avatar name={post.Author} size={44}/>
                                                            <span
                                                                className={post.AuthorURL ? 'browse-comment-author clickable' : 'browse-comment-author'}
                                                                onClick={() => post.AuthorURL && BrowserOpenURL(post.AuthorURL)}
                                                            >
                                                                {post.Author}
                                                            </span>
                                                            {cx.authorTitle && <span className="browse-comment-authortitle">{cx.authorTitle}</span>}
                                                            <span className="browse-comment-authorgroup">{cx.authorGroup}</span>
                                                            <span className="browse-comment-authorposts mono">{cx.authorPostCount} posts</span>
                                                        </div>
                                                        <div className="browse-comment-body">
                                                            <div className="browse-comment-header">
                                                                {isTopicAuthor && <span className="browse-comment-badge author">TOPIC AUTHOR</span>}
                                                                {cx.isPopular && <span className="browse-comment-badge popular">POPULAR POST</span>}
                                                                <span className="browse-comment-posted">{post.Posted}{cx.edited ? ' (edited)' : ''}</span>
                                                                <div className="spacer"/>
                                                                {post.URL && (
                                                                    <i
                                                                        className="fa-solid fa-up-right-from-square"
                                                                        title="Open this reply on LoversLab"
                                                                        onClick={() => BrowserOpenURL(post.URL)}
                                                                    />
                                                                )}
                                                            </div>
                                                            {post.Content && <div className="browse-comment-content">{post.Content}</div>}
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
                                                                <span className="browse-comment-likes"><i className="fa-solid fa-heart"/> {cx.reactions}</span>
                                                            </div>
                                                        </div>
                                                    </div>
                                                );
                                            })}
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

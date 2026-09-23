import './Browse.css';
import {Fragment, h} from 'preact';
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
import type {library, loverslab, main} from '../../wailsjs/go/models';
import {BrowserOpenURL, EventsOn} from '../../wailsjs/runtime/runtime';
import {openContextMenu} from '../data/contextMenu';
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

// The detail modal's three tabs, Nexus/Steam Workshop/Thunderstore-style: a
// description with screenshots and the changelog, the file's own downloadable
// files (pick one to install), and comments (read and, per Phase 1, write).
type DetailTab = 'description' | 'files' | 'comments';

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
    | { kind: 'ready'; mods: main.LoversLabInstalledMod[] };

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

export function Browse({games, selectedGame}: {
    games: library.GameInfo[];
    selectedGame: string;
}) {
    const unreadNotifications = useLoversLabUnreadCount();
    const [status, setStatus] = useState<main.LoversLabStatus | null>(null);
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [busy, setBusy] = useState<'save' | 'clear' | null>(null);
    const [error, setError] = useState('');

    const [categoryState, setCategoryState] = useState<CategoryState>({kind: 'idle'});
    const [selectedCategory, setSelectedCategory] = useState<loverslab.Category | null>(null);
    const [page, setPage] = useState(1);
    const [filesState, setFilesState] = useState<FilesState>({kind: 'idle'});
    const [search, setSearch] = useState('');

    // The "Installed" section: a new place in the sidebar, alongside GAMES, to see and
    // uninstall everything installed from LoversLab for the selected game.
    const [viewingInstalled, setViewingInstalled] = useState(false);
    const [installedState, setInstalledState] = useState<InstalledModsState>({kind: 'idle'});
    const [confirmUninstallId, setConfirmUninstallId] = useState<number | null>(null);
    // Which open file ids are installed - kept up to date alongside installedState,
    // read by the detail modal to decide whether to offer Uninstall or Install/Update.
    const [installedIds, setInstalledIds] = useState<Set<number>>(new Set());

    const [detailFor, setDetailFor] = useState<loverslab.FileSummary | null>(null);
    const [detailTab, setDetailTab] = useState<DetailTab>('description');
    const [detailState, setDetailState] = useState<DetailState | null>(null);
    const [changelogState, setChangelogState] = useState<ChangelogState | null>(null);
    const [commentsState, setCommentsState] = useState<CommentsState | null>(null);
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
        setDetailTab('description');
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
            return;
        }
        let cancelled = false;
        setCommentsState({kind: 'loading'});
        LoversLabComments(detailFor.URL, commentsPage)
            .then((result) => { if (!cancelled) setCommentsState({kind: 'ready', posts: result.Posts ?? [], totalPages: result.TotalPages || 1, hasTopic: result.HasTopic}); })
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
    function openInstalledDetail(m: main.LoversLabInstalledMod) {
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

    const files = filesState.kind === 'ready' ? filesState.files : [];
    const visibleFiles = useMemo(() => {
        const q = search.trim().toLowerCase();
        if (!q) return files;
        return files.filter((f) => f.Title.toLowerCase().includes(q) || f.Author.toLowerCase().includes(q));
    }, [files, search]);

    return (
        <div className="browse">
            <div className="browse-sources">
                <div className="sidebar-label">SOURCES</div>
                <div className="browse-source-row active">
                    <i className="fa-solid fa-heart browse-source-icon loverslab"/>
                    <span>LoversLab</span>
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
                        </div>
                        {installedState.kind === 'loading' && <div className="browse-status-note">Loading...</div>}
                        {installedState.kind === 'error' && <div className="browse-status-note error">{installedState.message}</div>}
                        {installedState.kind === 'ready' && installedState.mods.length === 0 && (
                            <div className="browse-status-note">Nothing installed from LoversLab yet for this game.</div>
                        )}
                        {installedState.kind === 'ready' && installedState.mods.length > 0 && (
                            <div className="browse-installed-list">
                                {installedState.mods.map((m) => (
                                    <div
                                        key={m.FileID}
                                        className="browse-installed-row"
                                        onClick={() => confirmUninstallId !== m.FileID && openInstalledDetail(m)}
                                        onContextMenu={(e) => openContextMenu(e, [
                                            {label: 'Open on LoversLab', onClick: () => BrowserOpenURL(m.FileURL)},
                                            {label: 'Uninstall', danger: true, separatorBefore: true, onClick: () => setConfirmUninstallId(m.FileID)},
                                        ])}
                                    >
                                        {confirmUninstallId === m.FileID ? (
                                            <div className="browse-installed-confirm">
                                                <span>Uninstall this mod? This deletes its files from your mod folder.</span>
                                                <span className="spacer"/>
                                                <span className="btn-ghost danger" onClick={(e) => { e.stopPropagation(); void uninstallFromList(m.FileID, m.Title); }}>Uninstall</span>
                                                <span className="btn-ghost" onClick={(e) => { e.stopPropagation(); setConfirmUninstallId(null); }}>Keep</span>
                                            </div>
                                        ) : (
                                            <>
                                                <div className="browse-installed-main">
                                                    <div className="browse-installed-name">{m.Title}</div>
                                                    {m.ContentMissing && (
                                                        <div className="browse-installed-missing">
                                                            <i className="fa-solid fa-triangle-exclamation"/> Files not found on disk
                                                        </div>
                                                    )}
                                                </div>
                                                <div className="browse-installed-date mono">
                                                    {m.InstalledAt ? new Date(m.InstalledAt * 1000).toLocaleDateString() : ''}
                                                </div>
                                            </>
                                        )}
                                    </div>
                                ))}
                            </div>
                        )}
                    </>
                ) : !status?.SignedIn ? (
                    <div className="browse-gate">
                        <i className="fa-solid fa-lock"/>
                        <div className="browse-gate-title">Sign in to browse LoversLab</div>
                        <div>Enter a username or email and a password on the right to see your Paradox games' real Downloads sections.</div>
                    </div>
                ) : !selectedCategory ? (
                    <div className="browse-gate">
                        <i className="fa-solid fa-gamepad"/>
                        <div className="browse-gate-title">Pick a game</div>
                        <div>Choose a game from the sidebar to see what's in it.</div>
                    </div>
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
                        </div>

                        {filesState.kind === 'loading' && <div className="browse-status-note">Loading...</div>}
                        {filesState.kind === 'error' && <div className="browse-status-note error">{filesState.message}</div>}
                        {filesState.kind === 'ready' && (
                            <div className="browse-grid">
                                {visibleFiles.map((f) => (
                                    <div key={f.ID} className="browse-card" onClick={() => openDetail(f)}>
                                        {f.ThumbnailURL ? (
                                            <div className="browse-card-thumb" style={{backgroundImage: `url(${f.ThumbnailURL})`}}/>
                                        ) : (
                                            <div className="browse-card-thumb placeholder"><i className="fa-solid fa-image"/></div>
                                        )}
                                        <div className="browse-card-body">
                                            <div className="browse-card-title" title={f.Title}>{f.Title}</div>
                                            <div
                                                className="browse-card-meta"
                                                title={f.Author}
                                                onClick={(e) => { e.stopPropagation(); if (f.AuthorURL) BrowserOpenURL(f.AuthorURL); }}
                                            >
                                                by {f.Author}
                                            </div>
                                            <div className="browse-card-stats">
                                                <span><i className="fa-solid fa-clock"/>{f.Updated}</span>
                                            </div>
                                        </div>
                                    </div>
                                ))}
                                {visibleFiles.length === 0 && (
                                    <div className="browse-status-note">
                                        {search ? 'No files match that search on this page.' : 'No files here.'}
                                    </div>
                                )}
                            </div>
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
                        <div className={`browse-account-status ${status.SignedIn ? 'good' : status.Unreadable ? 'warn' : 'neutral'}`}>
                            <i className={`fa-solid ${status.SignedIn ? 'fa-circle-check' : status.Unreadable ? 'fa-triangle-exclamation' : 'fa-user'}`}/>
                            <span>
                                {status.SignedIn
                                    ? `Signed in as ${status.Username}.`
                                    : status.Unreadable
                                        ? 'A sign-in is saved but cannot be read on this computer - sign in again.'
                                        : 'Not signed in yet.'}
                            </span>
                        </div>

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
                            <div className="browse-detail-sidebar">
                                {(detailFor.ThumbnailURL || (detailState?.kind === 'ready' && detailState.detail.Screenshots.length > 0)) && (
                                    <div
                                        className="browse-detail-sidebar-thumb"
                                        style={{backgroundImage: `url(${
                                            detailFor.ThumbnailURL ||
                                            (detailState?.kind === 'ready' ? (detailState.detail.Screenshots[0].ThumbnailURL || detailState.detail.Screenshots[0].URL) : '')
                                        })`}}
                                    />
                                )}
                                <div className={`browse-detail-installed-flag ${isInstalled ? 'yes' : 'no'}`}>
                                    <i className={`fa-solid ${isInstalled ? 'fa-circle-check' : 'fa-circle'}`}/> {isInstalled ? 'Installed' : 'Not installed'}
                                </div>
                                {detailState?.kind === 'ready' && (
                                    <div className="browse-detail-stats">
                                        <div
                                            className={detailState.detail.Author.URL ? 'browse-detail-stat clickable' : 'browse-detail-stat'}
                                            onClick={() => detailState.detail.Author.URL && BrowserOpenURL(detailState.detail.Author.URL)}
                                        >
                                            <i className="fa-solid fa-user"/> {detailState.detail.Author.Name}
                                        </div>
                                        {detailState.detail.Version && (
                                            <div className="browse-detail-stat"><i className="fa-solid fa-code-branch"/> {detailState.detail.Version}</div>
                                        )}
                                        {detailState.detail.FileSize && (
                                            <div className="browse-detail-stat"><i className="fa-solid fa-weight-hanging"/> {detailState.detail.FileSize}</div>
                                        )}
                                        <div className="browse-detail-stat"><i className="fa-solid fa-eye"/> {detailState.detail.Views.toLocaleString()} views</div>
                                        <div className="browse-detail-stat"><i className="fa-solid fa-download"/> {detailState.detail.Downloads.toLocaleString()} downloads</div>
                                    </div>
                                )}
                            </div>
                            <div className="browse-detail-main">
                                <div className="browse-detail-tabs">
                                    <span className={`browse-detail-tab ${detailTab === 'description' ? 'active' : ''}`} onClick={() => setDetailTab('description')}>
                                        Description
                                    </span>
                                    <span className={`browse-detail-tab ${detailTab === 'files' ? 'active' : ''}`} onClick={() => setDetailTab('files')}>
                                        Files
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

                                    {detailTab === 'description' && (
                                        <>
                                            {detailState?.kind === 'loading' && <div className="browse-status-note">Loading...</div>}
                                            {detailState?.kind === 'error' && <div className="browse-status-note error">{detailState.message}</div>}
                                            {detailState?.kind === 'ready' && detailState.detail.Screenshots.length > 0 && (
                                                <div className="browse-detail-screenshots">
                                                    {detailState.detail.Screenshots.map((s, i) => (
                                                        <div
                                                            key={i}
                                                            className="browse-detail-screenshot"
                                                            style={{backgroundImage: `url(${s.ThumbnailURL || s.URL})`}}
                                                            title="Open full size"
                                                            onClick={() => BrowserOpenURL(s.URL || s.ThumbnailURL)}
                                                        />
                                                    ))}
                                                </div>
                                            )}
                                            {detailState?.kind === 'ready' && detailState.detail.Description && (
                                                <div className="browse-detail-description">{detailState.detail.Description}</div>
                                            )}

                                            <div className="browse-detail-section-label">CHANGELOG</div>
                                            {changelogState?.kind === 'loading' && <div className="browse-status-note">Loading...</div>}
                                            {changelogState?.kind === 'error' && <div className="browse-status-note error">{changelogState.message}</div>}
                                            {changelogState?.kind === 'ready' && changelogState.entries.length === 0 && (
                                                <div className="browse-status-note">The author hasn't written release notes for this file.</div>
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
                                            {filesTabState.kind === 'loading' && <div className="browse-status-note">Finding downloadable files...</div>}
                                            {filesTabState.kind === 'error' && <div className="browse-status-note error">{filesTabState.message}</div>}
                                            {filesTabState.kind === 'ready' && filesTabState.downloads.length === 0 && (
                                                <div className="browse-status-note">No downloadable files were found for this mod.</div>
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
                                            {commentsState?.kind === 'loading' && <div className="browse-status-note">Loading...</div>}
                                            {commentsState?.kind === 'error' && <div className="browse-status-note error">{commentsState.message}</div>}
                                            {commentsState?.kind === 'ready' && commentsState.hasTopic && (
                                                <div className="browse-comment-write">
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
                                            )}
                                            {commentsState?.kind === 'ready' && !commentsState.hasTopic && (
                                                <div className="browse-status-note">This file has no support topic to comment on.</div>
                                            )}
                                            {commentsState?.kind === 'ready' && commentsState.hasTopic && commentsState.posts.length === 0 && (
                                                <div className="browse-status-note">No one has replied yet - be the first.</div>
                                            )}
                                            {commentsState?.kind === 'ready' && commentsState.posts.map((post) => (
                                                <div key={post.ID} className="browse-comment">
                                                    <div className="browse-comment-header">
                                                        <span
                                                            className={post.AuthorURL ? 'browse-comment-author clickable' : 'browse-comment-author'}
                                                            onClick={() => post.AuthorURL && BrowserOpenURL(post.AuthorURL)}
                                                        >
                                                            {post.Author}
                                                        </span>
                                                        <span className="browse-comment-posted">{post.Posted}</span>
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
                                                </div>
                                            ))}
                                        </>
                                    )}
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

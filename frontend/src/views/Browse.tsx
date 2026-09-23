import './Browse.css';
import {Fragment, h} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {
    ClearLoversLabCredentials,
    LoversLabCategories,
    LoversLabChangelog,
    LoversLabComments,
    LoversLabFileDetail,
    LoversLabFiles,
    LoversLabStatus,
    SaveLoversLabCredentials,
} from '../../wailsjs/go/main/App';
import type {library, loverslab, main} from '../../wailsjs/go/models';
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime';
import {notify} from '../data/notifications';

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
    | { kind: 'ready'; posts: loverslab.Post[]; totalPages: number };

function errorText(err: unknown): string {
    return String(err).replace(/^Error:\s*/, '');
}

export function Browse({games, selectedGame}: {
    games: library.GameInfo[];
    selectedGame: string;
}) {
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

    const [detailFor, setDetailFor] = useState<loverslab.FileSummary | null>(null);
    const [detailState, setDetailState] = useState<DetailState | null>(null);
    const [changelogState, setChangelogState] = useState<ChangelogState | null>(null);
    const [commentsState, setCommentsState] = useState<CommentsState | null>(null);
    const [commentsPage, setCommentsPage] = useState(1);

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
        setSelectedCategory(cat);
        setPage(1);
        setSearch('');
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
        setCommentsPage(1);

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
            .then((result) => { if (!cancelled) setCommentsState({kind: 'ready', posts: result.Posts ?? [], totalPages: result.TotalPages || 1}); })
            .catch((err) => { if (!cancelled) setCommentsState({kind: 'error', message: errorText(err)}); });
        return () => { cancelled = true; };
    }, [detailFor, commentsPage]);

    function closeDetail() {
        setDetailFor(null);
        setDetailState(null);
        setChangelogState(null);
        setCommentsState(null);
    }

    const gameName = games.find((g) => g.ID === selectedGame)?.DisplayName ?? selectedGame;
    const canSave = !busy && username.trim() !== '' && password !== '';

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
                    </>
                )}

                <div className="sidebar-footer">
                    <div className="mono line">
                        Unofficial places to find {gameName} mods, outside the Steam Workshop.
                    </div>
                </div>
            </div>

            <div className="browse-main">
                {!status?.SignedIn ? (
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
                <div className="sidebar-label">LOVERSLAB SIGN-IN</div>
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
                            <i className="fa-solid fa-up-right-from-square" title="Open on LoversLab" onClick={() => BrowserOpenURL(detailFor.URL)}/>
                            <i className="fa-solid fa-xmark close-btn" onClick={closeDetail}/>
                        </div>
                        <div className="browse-detail-body">
                            {detailState?.kind === 'loading' && <div className="browse-status-note">Loading...</div>}
                            {detailState?.kind === 'error' && <div className="browse-status-note error">{detailState.message}</div>}
                            {detailState?.kind === 'ready' && (
                                <>
                                    {detailState.detail.Screenshots.length > 0 && (
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

                                    <div className="browse-detail-meta">
                                        <span
                                            className={detailState.detail.Author.URL ? 'clickable' : ''}
                                            onClick={() => detailState.detail.Author.URL && BrowserOpenURL(detailState.detail.Author.URL)}
                                        >
                                            <i className="fa-solid fa-user"/> {detailState.detail.Author.Name}
                                        </span>
                                        {detailState.detail.Version && <span><i className="fa-solid fa-code-branch"/> {detailState.detail.Version}</span>}
                                        {detailState.detail.FileSize && <span><i className="fa-solid fa-weight-hanging"/> {detailState.detail.FileSize}</span>}
                                        <span><i className="fa-solid fa-eye"/> {detailState.detail.Views.toLocaleString()}</span>
                                        <span><i className="fa-solid fa-download"/> {detailState.detail.Downloads.toLocaleString()}</span>
                                    </div>

                                    {detailState.detail.Description && (
                                        <div className="browse-detail-description">{detailState.detail.Description}</div>
                                    )}
                                </>
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

                            <div className="browse-detail-section-label">
                                COMMENTS
                                {commentsState?.kind === 'ready' && commentsState.totalPages > 1 && (
                                    <span className="browse-detail-section-pager">
                                        <i
                                            className={`fa-solid fa-chevron-left ${commentsPage <= 1 ? 'inert' : ''}`}
                                            onClick={() => commentsPage > 1 && setCommentsPage(commentsPage - 1)}
                                        />
                                        <span className="mono">{commentsPage} / {commentsState.totalPages}</span>
                                        <i
                                            className={`fa-solid fa-chevron-right ${commentsPage >= commentsState.totalPages ? 'inert' : ''}`}
                                            onClick={() => commentsPage < commentsState.totalPages && setCommentsPage(commentsPage + 1)}
                                        />
                                    </span>
                                )}
                            </div>
                            {commentsState?.kind === 'loading' && <div className="browse-status-note">Loading...</div>}
                            {commentsState?.kind === 'error' && <div className="browse-status-note error">{commentsState.message}</div>}
                            {commentsState?.kind === 'ready' && commentsState.posts.length === 0 && (
                                <div className="browse-status-note">
                                    This file has no support topic, or no one has replied to it yet.
                                </div>
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
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

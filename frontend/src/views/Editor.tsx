import './Editor.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {OpenModFolder, PinnedMods, ScanGame, SetModPinned} from '../../wailsjs/go/main/App';
import type {app, library} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {EmptyState} from '../components/EmptyState';
import {SourceBadge} from '../components/SourceBadge';
import {openContextMenu} from '../data/contextMenu';
import type {Draft} from '../data/editorDraft';
import {EditorEdit} from './EditorEdit';
import {EditorNew} from './EditorNew';
import {EditorPublish} from './EditorPublish';
import {EditorChecks} from './EditorChecks';
import {EditorTranslate} from './EditorTranslate';

type EditorTab = 'new' | 'edit' | 'checks' | 'translate' | 'publish';

// A plain character per tab, not an icon font - "+"/pen/check/"Aa"/up-arrow, each colored to
// match its own tab's active/inactive state (see .editor-tab-glyph in Editor.css).
const TABS: { key: EditorTab; label: string; glyph: string }[] = [
    {key: 'new', label: 'New', glyph: '+'},
    {key: 'edit', label: 'Edit', glyph: '✎'},
    {key: 'checks', label: 'Checks', glyph: '✓'},
    {key: 'translate', label: 'Translate', glyph: 'Aa'},
    {key: 'publish', label: 'Publish', glyph: '↑'},
];

// The Editor: change a mod's own name, versions, tags, dependencies and thumbnail, with a
// preview of every file it would write before anything is saved. Only mods made by the person
// (not Steam Workshop content, not the Paradox Launcher's own, not this app's generated patch)
// can be changed here - see modedit.go's editTarget for exactly what that means. The New tab
// creates a brand-new mod, or duplicates the one currently selected - see EditorNew.
export function Editor({games, selectedGame, gameVersion, onOpenToolsSettings}: {
    games: library.GameInfo[];
    selectedGame: string;
    gameVersion: string;
    // Jumps to Settings > Tools - the Translate tab's own "no DeepL key saved" link.
    onOpenToolsSettings: () => void;
}) {
    const [mods, setMods] = useState<library.ModSummary[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [selectedId, setSelectedId] = useState<string | null>(null);
    const [search, setSearch] = useState('');
    const [tab, setTab] = useState<EditorTab>('edit');
    // Unsaved edits, per mod ID, kept while the Editor stays open - switching mods or tabs never
    // loses what was typed.
    const [drafts, setDrafts] = useState<Map<string, Draft>>(new Map());
    // The Checks tab's own last result, per mod ID, kept the same way as drafts above - the
    // Checks tab itself unmounts whenever another Editor tab is showing (see the conditional
    // render below), so without this a result would otherwise vanish the moment you looked at
    // Edit and came back. Cleared on a "mods-changed" event (a save, or any other on-disk
    // change) since a cached result is a snapshot of files that may have just changed.
    const [checkResults, setCheckResults] = useState<Map<string, app.CheckResult>>(new Map());
    const [pinned, setPinned] = useState<Set<string>>(new Set());
    const latestScan = useRef(0);
    // Set by EditorNew right before a create/duplicate finishes, so the next load() selects the
    // result by name (its ID may not be known yet - see internal/scan's modID()) and switches to
    // Edit, matching how selecting any other mod always lands there.
    const pendingSelectName = useRef<string | null>(null);

    function load() {
        if (!selectedGame) return;
        const seq = ++latestScan.current;
        setLoading(true);
        setError('');
        ScanGame(selectedGame, '')
            .then((summary) => {
                if (seq !== latestScan.current) return;
                setMods(summary.Mods);
                const pendingName = pendingSelectName.current;
                if (pendingName) {
                    const created = summary.Mods.find((m) => m.Name === pendingName);
                    if (created) {
                        pendingSelectName.current = null;
                        setSelectedId(created.ID);
                        return;
                    }
                }
                setSelectedId((prev) => (prev && summary.Mods.some((m) => m.ID === prev) ? prev : summary.Mods[0]?.ID ?? null));
                if (summary.Mods.length === 0) setTab('new');
            })
            .catch((err) => { if (seq === latestScan.current) setError(String(err)); })
            .finally(() => { if (seq === latestScan.current) setLoading(false); });
    }

    function loadPins() {
        if (!selectedGame) return;
        PinnedMods(selectedGame).then((ids) => setPinned(new Set(ids))).catch(() => undefined);
    }

    useEffect(() => {
        setDrafts(new Map());
        setCheckResults(new Map());
        setSelectedId(null);
        setTab('edit');
        pendingSelectName.current = null;
        load();
        loadPins();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    useEffect(() => {
        if (!selectedGame) return;
        const off = EventsOn('mods-changed', (gameId: string) => {
            if (gameId !== selectedGame) return;
            load();
            // A cached check result is a snapshot of what was on disk when it ran; anything
            // that just changed the mods (a save, an external edit) may have made it stale.
            setCheckResults(new Map());
        });
        return () => off();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    const visible = useMemo(() => {
        const q = search.trim().toLowerCase();
        const filtered = q ? mods.filter((m) => m.Name.toLowerCase().includes(q)) : mods;
        // Pinned mods first, stable otherwise.
        return [...filtered].sort((a, b) => Number(pinned.has(b.ID)) - Number(pinned.has(a.ID)));
    }, [mods, search, pinned]);

    const selected = mods.find((m) => m.ID === selectedId) ?? null;

    function setDraftFor(id: string, draft: Draft | null) {
        setDrafts((prev) => {
            const next = new Map(prev);
            if (draft) next.set(id, draft); else next.delete(id);
            return next;
        });
    }

    function setCheckResultFor(id: string, result: app.CheckResult | null) {
        setCheckResults((prev) => {
            const next = new Map(prev);
            if (result) next.set(id, result); else next.delete(id);
            return next;
        });
    }

    function togglePin(modId: string) {
        const next = !pinned.has(modId);
        setPinned((prev) => {
            const copy = new Set(prev);
            if (next) copy.add(modId); else copy.delete(modId);
            return copy;
        });
        SetModPinned(selectedGame, modId, next).catch(() => loadPins());
    }

    function afterCreated(name: string) {
        pendingSelectName.current = name;
        setTab('edit');
        load();
    }

    function selectMod(id: string) {
        setSelectedId(id);
        setTab('edit');
    }

    if (games.length === 0) {
        return <EmptyState icon="fa-pen-ruler" title="No games to manage yet" subtitle="Open Manage games in Settings to pick one."/>;
    }

    return (
        <div className="editor-body">
            <div className="editor-list-pane">
                <div className="editor-list-header">
                    <div className="search-box">
                        <i className="fa-solid fa-magnifying-glass"/>
                        <input placeholder={`Search ${mods.length} mods...`} value={search} onInput={(e) => setSearch((e.target as HTMLInputElement).value)}/>
                    </div>
                </div>
                <div className="editor-list-rows">
                    <div className={`editor-list-new ${tab === 'new' ? 'active' : ''}`} onClick={() => setTab('new')}>
                        <i className="fa-solid fa-plus"/> New mod
                    </div>
                    {loading && mods.length === 0 && (
                        <EmptyState icon="fa-spinner fa-spin" title="Scanning mods..."/>
                    )}
                    {error && <EmptyState icon="fa-triangle-exclamation" title="Couldn't load mods" subtitle={error} tone="error"/>}
                    {!loading && !error && visible.length === 0 && (
                        search.trim() ? (
                            <EmptyState icon="fa-magnifying-glass" title="No mods found"/>
                        ) : (
                            <EmptyState
                                icon="fa-pen-ruler"
                                title={`No mods for ${games.find((g) => g.ID === selectedGame)?.DisplayName ?? 'this game'} yet`}
                                subtitle="You're on the New tab. Create one from a template."
                            />
                        )
                    )}
                    {visible.map((m) => (
                        <div
                            key={m.ID}
                            className={`editor-list-row ${m.ID === selectedId ? 'selected' : ''}`}
                            onClick={() => selectMod(m.ID)}
                            onContextMenu={(e) => openContextMenu(e, [
                                {label: pinned.has(m.ID) ? 'Unpin' : 'Pin', onClick: () => togglePin(m.ID)},
                                {label: 'Open folder', onClick: () => OpenModFolder(selectedGame, m.ID), separatorBefore: true},
                            ])}
                        >
                            <SourceBadge source={m.Source} name={m.Name}/>
                            <span className="editor-list-name">{m.Name}</span>
                            {pinned.has(m.ID) && <i className="fa-solid fa-thumbtack editor-list-pin" title="Pinned"/>}
                            {drafts.has(m.ID) && <span className="editor-list-dot" title="Unsaved changes"/>}
                        </div>
                    ))}
                </div>
            </div>

            <div className="editor-detail-pane">
                {selected ? (
                    <div className="editor-detail-header">
                        <div className="editor-detail-title">
                            <SourceBadge source={selected.Source} name={selected.Name}/>
                            <span>{selected.Name}</span>
                        </div>
                        <span className="link-btn" onClick={() => OpenModFolder(selectedGame, selected.ID)}>
                            <i className="fa-solid fa-folder-open"/> Open folder
                        </span>
                        <span className="editor-actions-spacer"/>
                        {drafts.has(selected.ID) && (
                            <span className="editor-unsaved-pill">
                                <span className="editor-unsaved-dot"/>
                                Unsaved changes
                            </span>
                        )}
                    </div>
                ) : (
                    <div className="editor-detail-header">
                        <div className="editor-detail-title">
                            <i className="fa-solid fa-file-circle-plus"/>
                            <span>New mod</span>
                        </div>
                    </div>
                )}
                <div className="editor-tabs">
                    {TABS.map((t) => {
                        const findingCount = t.key === 'checks' && selected ? checkResults.get(selected.ID)?.Findings.length ?? 0 : 0;
                        return (
                            <span key={t.key} className={`editor-tab ${tab === t.key ? 'active' : ''}`} onClick={() => setTab(t.key)}>
                                <span className="editor-tab-glyph">{t.glyph}</span>
                                {t.label}
                                {findingCount > 0 && <span className="editor-tab-badge">{findingCount}</span>}
                            </span>
                        );
                    })}
                </div>
                <div className="editor-tab-body">
                    {tab === 'new' && (
                        <EditorNew gameId={selectedGame} selected={selected} onCreated={afterCreated}/>
                    )}
                    {tab !== 'new' && !selected && (
                        <EmptyState icon="fa-pen-ruler" title="No mod selected" subtitle="Select a mod from the list, or create one from New."/>
                    )}
                    {tab === 'edit' && selected && (
                        <EditorEdit
                            key={selected.ID}
                            gameId={selectedGame}
                            gameVersion={gameVersion}
                            mod={selected}
                            installedNames={mods.map((m) => m.Name)}
                            initialDraft={drafts.get(selected.ID) ?? null}
                            onDraft={(draft) => setDraftFor(selected.ID, draft)}
                            onSaved={load}
                        />
                    )}
                    {tab === 'checks' && selected && (
                        <EditorChecks
                            key={selected.ID}
                            gameId={selectedGame}
                            gameName={games.find((g) => g.ID === selectedGame)?.DisplayName ?? ''}
                            gameVersion={gameVersion}
                            mod={selected}
                            installedNames={mods.map((m) => m.Name)}
                            initialResult={checkResults.get(selected.ID) ?? null}
                            onResult={(result) => setCheckResultFor(selected.ID, result)}
                        />
                    )}
                    {tab === 'translate' && selected && (
                        <EditorTranslate
                            gameId={selectedGame}
                            gameName={games.find((g) => g.ID === selectedGame)?.DisplayName ?? ''}
                            mod={selected}
                            onOpenToolsSettings={onOpenToolsSettings}
                        />
                    )}
                    {tab === 'publish' && selected && <EditorPublish gameId={selectedGame} mod={selected}/>}
                </div>
            </div>
        </div>
    );
}

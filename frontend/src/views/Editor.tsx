import './Editor.css';
import {h} from 'preact';
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {OpenModFolder, ScanGame} from '../../wailsjs/go/main/App';
import type {library} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';
import {EmptyState} from '../components/EmptyState';
import {SourceBadge} from '../components/SourceBadge';
import type {Draft} from '../data/editorDraft';
import {EditorEdit} from './EditorEdit';
import {EditorPublish} from './EditorPublish';
import {EditorChecks} from './EditorChecks';

type EditorTab = 'edit' | 'publish' | 'checks';

const TABS: { key: EditorTab; label: string; icon: string }[] = [
    {key: 'edit', label: 'Edit', icon: 'fa-pen'},
    {key: 'publish', label: 'Publish', icon: 'fa-cloud-arrow-up'},
    {key: 'checks', label: 'Checks', icon: 'fa-shield-halved'},
];

// The Editor: change a mod's own name, versions, tags, dependencies and thumbnail, with a
// preview of every file it would write before anything is saved. Only mods made by the person
// (not Steam Workshop content, not the Paradox Launcher's own, not this app's generated patch)
// can be changed here - see modedit.go's editTarget for exactly what that means.
export function Editor({games, selectedGame, gameVersion}: {
    games: library.GameInfo[];
    selectedGame: string;
    gameVersion: string;
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
    const latestScan = useRef(0);

    function load() {
        if (!selectedGame) return;
        const seq = ++latestScan.current;
        setLoading(true);
        setError('');
        ScanGame(selectedGame, '')
            .then((summary) => {
                if (seq !== latestScan.current) return;
                setMods(summary.Mods);
                setSelectedId((prev) => (prev && summary.Mods.some((m) => m.ID === prev) ? prev : summary.Mods[0]?.ID ?? null));
            })
            .catch((err) => { if (seq === latestScan.current) setError(String(err)); })
            .finally(() => { if (seq === latestScan.current) setLoading(false); });
    }

    useEffect(() => {
        setDrafts(new Map());
        setSelectedId(null);
        setTab('edit');
        load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    useEffect(() => {
        if (!selectedGame) return;
        const off = EventsOn('mods-changed', (gameId: string) => { if (gameId === selectedGame) load(); });
        return () => off();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedGame]);

    const visible = useMemo(() => {
        const q = search.trim().toLowerCase();
        if (!q) return mods;
        return mods.filter((m) => m.Name.toLowerCase().includes(q));
    }, [mods, search]);

    const selected = mods.find((m) => m.ID === selectedId) ?? null;

    function setDraftFor(id: string, draft: Draft | null) {
        setDrafts((prev) => {
            const next = new Map(prev);
            if (draft) next.set(id, draft); else next.delete(id);
            return next;
        });
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
                    {loading && mods.length === 0 && (
                        <EmptyState icon="fa-spinner fa-spin" title="Scanning mods..."/>
                    )}
                    {error && <EmptyState icon="fa-triangle-exclamation" title="Couldn't load mods" subtitle={error} tone="error"/>}
                    {!loading && !error && visible.length === 0 && (
                        <EmptyState icon="fa-magnifying-glass" title="No mods found"/>
                    )}
                    {visible.map((m) => (
                        <div
                            key={m.ID}
                            className={`editor-list-row ${m.ID === selectedId ? 'selected' : ''}`}
                            onClick={() => setSelectedId(m.ID)}
                        >
                            <SourceBadge source={m.Source} name={m.Name}/>
                            <span className="editor-list-name">{m.Name}</span>
                            {drafts.has(m.ID) && <span className="editor-list-dot" title="Unsaved changes"/>}
                        </div>
                    ))}
                </div>
            </div>

            {!selected ? (
                <div className="editor-detail-pane">
                    <EmptyState icon="fa-pen-ruler" title="No mod selected" subtitle="Select a mod from the list to see and edit it."/>
                </div>
            ) : (
                <div className="editor-detail-pane">
                    <div className="editor-detail-header">
                        <div className="editor-detail-title">
                            <SourceBadge source={selected.Source} name={selected.Name}/>
                            <span>{selected.Name}</span>
                        </div>
                        <span className="link-btn" onClick={() => OpenModFolder(selectedGame, selected.ID)}>
                            <i className="fa-solid fa-folder-open"/> Open folder
                        </span>
                    </div>
                    <div className="editor-tabs">
                        {TABS.map((t) => (
                            <span key={t.key} className={`editor-tab ${tab === t.key ? 'active' : ''}`} onClick={() => setTab(t.key)}>
                                <i className={`fa-solid ${t.icon}`}/> {t.label}
                            </span>
                        ))}
                    </div>
                    <div className="editor-tab-body">
                        {tab === 'edit' && (
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
                        {tab === 'publish' && <EditorPublish mod={selected}/>}
                        {tab === 'checks' && <EditorChecks mod={selected}/>}
                    </div>
                </div>
            )}
        </div>
    );
}

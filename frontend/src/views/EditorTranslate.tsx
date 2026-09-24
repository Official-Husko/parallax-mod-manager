import {Fragment, h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {CancelTranslate, TranslateEligibility, TranslateLanguages, TranslateMod} from '../../wailsjs/go/main/App';
import type {app, library} from '../../wailsjs/go/models';
import {EventsOn} from '../../wailsjs/runtime/runtime';

// The Translate tab: machine-translate a mod's own English localisation text,
// one key at a time, via DeepL's official API (needs a key of your own - see
// Settings > Tools) or either of two free, unofficial DeepL-powered wrappers
// (Translanova, Vust). Always English -> a chosen target language, or every
// real language at once ("All languages", the default). See
// internal/app/translate.go and docs/ for the real mechanism - in short: an
// incremental cache remembers what's already been translated, so re-running
// this only pays for what's new or changed.
//
// Two destinations: "Author mode" writes straight into this mod's own
// localisation/<lang>/ folder (only offered when the mod is editable here at
// all - reuses the exact same Editable/Overridable/"Continue anyway" signal
// the Edit tab already does). "Player mode" generates a separate companion
// mod instead, leaving this mod untouched - always available, including for
// a Steam Workshop or Paradox Launcher mod, since it only ever reads the
// source.

type Service = 'deepl' | 'translanova' | 'vust';

const SERVICES: { service: Service; name: string; badge: 'free' | 'key'; desc: string }[] = [
    {service: 'translanova', name: 'Translanova', badge: 'free', desc: 'General-purpose, no account needed.'},
    {service: 'vust', name: 'Vust', badge: 'free', desc: 'Faster for short strings, weaker on long event text.'},
    {service: 'deepl', name: 'DeepL API', badge: 'key', desc: 'Best quality. Needs your own API key.'},
];

type LogTone = 'info' | 'success' | 'warn' | 'error';

interface LogLine {
    time: string;
    tone: LogTone;
    text: string;
}

function timestamp(): string {
    return new Date().toLocaleTimeString([], {hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false});
}

// describeStage turns one raw progress event into the log line shown for it -
// deliberately coarse (only real milestones, never one line per key): the
// progress bar itself already shows the fine-grained "103/894" count.
// "language" doubles as the unconfirmed-folder notice (Message is only ever
// populated there when the target folder name isn't confirmed for this game).
function describeStage(p: TranslateProgressEvent, languageName: string): { tone: LogTone; text: string } | null {
    switch (p.Stage) {
        case 'opening':
            return {tone: 'info', text: 'Reading this mod\'s own English text...'};
        case 'opened':
            return {tone: 'info', text: `Opened ${p.Count.toLocaleString()} English source file${p.Count === 1 ? '' : 's'}`};
        case 'language':
            return p.Message
                ? {tone: 'warn', text: `${languageName || p.Language}: ${p.Message}`}
                : {tone: 'info', text: `${languageName || p.Language}: translating...`};
        case 'language_done':
            return {tone: 'success', text: `${languageName || p.Language}: ${p.Count.toLocaleString()} key${p.Count === 1 ? '' : 's'} written`};
        case 'writing':
            return {tone: 'info', text: 'Writing the translated file(s)...'};
        case 'done':
            return {tone: 'success', text: 'Done'};
        case 'error':
            return {tone: 'error', text: p.Message || 'Something went wrong.'};
        default:
            return null;
    }
}

// TranslateProgressEvent mirrors app.TranslateProgress's own JSON shape - it
// is only ever an event payload (see TranslateMod's own "translate-progress"
// doc comment), never a bound method's return type, so Wails generates no
// TypeScript type for it.
interface TranslateProgressEvent {
    RequestID: string;
    Stage: string;
    Language: string;
    Key: string;
    Message: string;
    Done: number;
    Total: number;
    LanguageIndex: number;
    LanguageTotal: number;
    Count: number;
    OutputFile: string;
}

export function EditorTranslate({gameId, gameName, mod, onOpenToolsSettings}: {
    gameId: string;
    // The game's own display name - "Stellaris", say - for TARGET LANGUAGE's own confirmed-
    // folder-name note.
    gameName: string;
    mod: library.ModSummary;
    // Jumps to Settings > Tools - the DeepL row's own "no key saved" link.
    onOpenToolsSettings: () => void;
}) {
    const [languages, setLanguages] = useState<app.TranslateLanguage[]>([]);
    const [eligibility, setEligibility] = useState<app.TranslateEligibility | null>(null);
    const [eligibilityError, setEligibilityError] = useState('');
    const [service, setService] = useState<Service>('translanova');
    const [targetCode, setTargetCode] = useState('ALL');
    const [mode, setMode] = useState<'author' | 'player'>('player');
    const [forced, setForced] = useState(false);
    const [running, setRunning] = useState(false);
    const [progress, setProgress] = useState<{
        done: number; total: number; language: string; languageIndex: number; languageTotal: number; outputFile: string;
    } | null>(null);
    const [log, setLog] = useState<LogLine[]>([]);
    const [error, setError] = useState('');
    const requestIdRef = useRef('');
    // Mirrors `languages` for the progress-event handler below, so that
    // handler's own subscribing effect only ever needs to depend on
    // gameId - never resubscribing (and briefly running with no listener
    // at all mid-swap) just because the language list finished loading.
    const languagesRef = useRef<app.TranslateLanguage[]>([]);

    useEffect(() => {
        TranslateLanguages().then((l) => {
            languagesRef.current = l;
            setLanguages(l);
        }).catch(() => undefined);
    }, []);

    useEffect(() => {
        setEligibility(null);
        setEligibilityError('');
        setForced(false);
        setLog([]);
        setError('');
        setProgress(null);
        TranslateEligibility(gameId, mod.ID)
            .then((e) => {
                setEligibility(e);
                setMode(e.AuthorModeOffered ? 'author' : 'player');
            })
            .catch((err) => setEligibilityError(String(err)));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [gameId, mod.ID]);

    useEffect(() => {
        const off = EventsOn('translate-progress', (eventGameId: string, p: TranslateProgressEvent) => {
            if (eventGameId !== gameId || p.RequestID !== requestIdRef.current) return;
            setProgress({
                done: p.Done, total: p.Total, language: p.Language,
                languageIndex: p.LanguageIndex, languageTotal: p.LanguageTotal, outputFile: p.OutputFile,
            });
            const name = languagesRef.current.find((l) => l.Code === p.Language)?.Name ?? '';
            const line = describeStage(p, name);
            if (line) setLog((prev) => [...prev, {time: timestamp(), tone: line.tone, text: line.text}]);
        });
        return () => off();
    }, [gameId]);

    // Excludes the "All languages" pseudo-entry - it's not a real target folder, so it has no
    // confirmed/unconfirmed status of its own to count.
    const realLanguages = languages.filter((l) => l.Code !== 'ALL');
    const confirmedCount = realLanguages.filter((l) => l.Confirmed).length;
    const unconfirmedCount = realLanguages.length - confirmedCount;

    const effectivelyOffered = !!eligibility && (mode === 'player' || eligibility.AuthorModeOffered || (eligibility.AuthorModeOverridable && forced));
    const canRun = !!eligibility && eligibility.HasEnglishContent && effectivelyOffered && (service !== 'deepl' || eligibility.DeepLKeyReady) && !running;

    function run() {
        const requestId = `translate-${Date.now()}-${Math.random().toString(36).slice(2)}`;
        requestIdRef.current = requestId;
        setRunning(true);
        setError('');
        setProgress({done: 0, total: 0, language: '', languageIndex: 0, languageTotal: 0, outputFile: ''});
        setLog([{time: timestamp(), tone: 'info', text: `Starting (${SERVICES.find((s) => s.service === service)?.name}, ${mode === 'author' ? 'writing into this mod' : 'generating a companion mod'})...`}]);

        TranslateMod(gameId, mod.ID, requestId, {
            Service: service,
            TargetCode: targetCode,
            Mode: mode,
            Force: forced,
        } as unknown as app.TranslateRequest)
            .then((result) => {
                const msg = mode === 'player' && result.CompanionModID
                    ? `Translated ${result.Translated} key(s). Generated "${result.CompanionModID}" - open Workspace to add it to your load order.`
                    : `Translated ${result.Translated} key(s).`;
                setLog((prev) => [...prev, {time: timestamp(), tone: 'success', text: msg}]);
            })
            .catch((err) => {
                const text = String(err);
                setError(text);
                setLog((prev) => [...prev, {time: timestamp(), tone: 'error', text}]);
            })
            .finally(() => setRunning(false));
    }

    function cancel() {
        if (requestIdRef.current) CancelTranslate(requestIdRef.current);
    }

    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-card">
                    <div className="editor-card-title">SERVICE</div>
                    <div className="mode-option-list">
                        {SERVICES.map((s) => {
                            const active = service === s.service;
                            const disabled = running;
                            return (
                                <div
                                    key={s.service}
                                    className={`mode-option ${active ? 'active' : ''} ${disabled ? 'disabled' : ''}`}
                                    onClick={() => !disabled && setService(s.service)}
                                >
                                    <i className={`fa-solid ${active ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${active ? 'on' : 'off'}`}/>
                                    <div className="mode-option-main">
                                        <div className="mode-option-name">
                                            {s.name}
                                            <span className={`chip ${s.badge === 'free' ? 'chip-free' : 'chip-key'}`}>{s.badge === 'free' ? 'FREE' : 'API KEY'}</span>
                                        </div>
                                        <div className="mode-option-desc">{s.desc}</div>
                                        {s.service === 'deepl' && eligibility && !eligibility.DeepLKeyReady && (
                                            <div className="mode-option-warn">
                                                No key saved &middot;{' '}
                                                <span className="mode-option-warn-link" onClick={(e) => { e.stopPropagation(); onOpenToolsSettings(); }}>
                                                    Settings &rsaquo; Tools
                                                </span>
                                            </div>
                                        )}
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">TARGET LANGUAGE</div>
                    <div className="editor-field">
                        <span className="editor-label">Translate into</span>
                        <select className="editor-input" disabled={running} value={targetCode} onChange={(e) => setTargetCode((e.target as HTMLSelectElement).value)}>
                            {languages.map((l) => (
                                <option key={l.Code} value={l.Code}>{l.Name}{!l.Confirmed && l.Code !== 'ALL' ? ' (unconfirmed for this game)' : ''}</option>
                            ))}
                        </select>
                        {realLanguages.length > 0 && (
                            <p className="editor-muted">
                                {confirmedCount} {confirmedCount === 1 ? 'is' : 'are'} confirmed for {gameName || 'this game'}.
                                {unconfirmedCount > 0 && ` The other ${unconfirmedCount} ${unconfirmedCount === 1 ? 'is' : 'are'} listed as "(unconfirmed for this game)" and use a best-guess folder name.`}
                            </p>
                        )}
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">DESTINATION</div>
                    <div className={`mode-option ${mode === 'author' ? 'active' : ''} ${!eligibility?.AuthorModeOffered && !(eligibility?.AuthorModeOverridable && forced) ? 'disabled' : ''}`} onClick={() => !running && eligibility && (eligibility.AuthorModeOffered || (eligibility.AuthorModeOverridable && forced)) && setMode('author')}>
                        <i className={`fa-solid ${mode === 'author' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'author' ? 'on' : 'off'}`}/>
                        <div className="mode-option-main">
                            <div className="mode-option-name">Update this mod directly</div>
                            <div className="mode-option-desc">
                                {eligibility?.AuthorModeOffered
                                    ? `Adds the translated files to ${mod.Name}. For the mod's author.`
                                    : eligibility?.AuthorModeOverridable
                                        ? eligibility.AuthorModeReason
                                        : eligibility?.AuthorModeReason || 'Not available for this mod.'}
                            </div>
                            {!eligibility?.AuthorModeOffered && eligibility?.AuthorModeOverridable && !forced && (
                                <button type="button" className="btn-ghost" onClick={(e) => { e.stopPropagation(); setForced(true); }}>Continue anyway</button>
                            )}
                        </div>
                    </div>
                    <div className={`mode-option ${mode === 'player' ? 'active' : ''}`} onClick={() => !running && setMode('player')}>
                        <i className={`fa-solid ${mode === 'player' ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio ${mode === 'player' ? 'on' : 'off'}`}/>
                        <div className="mode-option-main">
                            <div className="mode-option-name">Generate a separate mod for personal use</div>
                            <div className="mode-option-desc">
                                Creates "Parallax Auto-Translations: {mod.Name}". Works for Workshop mods too. You add
                                it to your load order in Workspace yourself.
                            </div>
                        </div>
                    </div>
                </div>

                {eligibilityError && (
                    <div className="editor-alert bad">
                        <span className="editor-alert-icon"/>
                        <div className="editor-alert-body"><div className="editor-alert-text">{eligibilityError}</div></div>
                    </div>
                )}
                {eligibility && !eligibility.HasEnglishContent && (
                    <div className="editor-alert bad">
                        <span className="editor-alert-icon"/>
                        <div className="editor-alert-body">
                            <div className="editor-alert-title">No English text found</div>
                            <div className="editor-alert-text">Searched localisation/english/ and localization/english/. Translate needs English source strings.</div>
                        </div>
                    </div>
                )}

                {!running && (
                    <div className="editor-actions">
                        <button type="button" className="btn-primary" disabled={!canRun} onClick={run}>Translate</button>
                    </div>
                )}
                {error && (
                    <div className="editor-alert bad">
                        <span className="editor-alert-icon"/>
                        <div className="editor-alert-body"><div className="editor-alert-text">{error}</div></div>
                    </div>
                )}
            </div>

            <div className="editor-column">
                {eligibility && eligibility.HasEnglishContent && (
                    <div className="editor-card">
                        <div className="editor-card-title">SOURCE</div>
                        <div className="translate-source-stats">
                            <div>
                                <div className="translate-source-stat-value mono">{eligibility.EnglishKeyCount.toLocaleString()}</div>
                                <div className="editor-hint">English keys</div>
                            </div>
                            <div>
                                <div className="translate-source-stat-value mono">{eligibility.EnglishFileCount.toLocaleString()}</div>
                                <div className="editor-hint">files</div>
                            </div>
                            <div>
                                <div className="translate-source-stat-value mono">{eligibility.TargetCount.toLocaleString()}</div>
                                <div className="editor-hint">targets</div>
                            </div>
                        </div>
                        <span className="editor-hint mono">{eligibility.SourcePath}</span>
                    </div>
                )}

                <div className="editor-card editor-changes">
                    <div className="editor-card-title">
                        PROGRESS
                        {progress && progress.languageTotal > 0 && (
                            <span className="editor-card-count mono">
                                {languages.find((l) => l.Code === progress.language)?.Name || progress.language} &middot; {progress.languageIndex} of {progress.languageTotal}
                            </span>
                        )}
                    </div>
                    {progress && (
                        <>
                            <div className="editor-progress-header-row">
                                <span className="mono">{progress.done.toLocaleString()} / {progress.total.toLocaleString()} keys</span>
                                <span className="mono">{progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0}%</span>
                            </div>
                            <div className="editor-progress-bar">
                                <div className="editor-progress-fill" style={{width: `${progress.total > 0 ? Math.min(100, (progress.done / progress.total) * 100) : 0}%`}}/>
                            </div>
                            <div className="editor-progress-file mono">
                                {progress.outputFile || (running ? 'Starting...' : `${progress.done} translated`)}
                            </div>
                        </>
                    )}
                    {log.length === 0 ? (
                        <div className="editor-muted">Nothing translated yet. Each real step appears here as it happens.</div>
                    ) : (
                        <div className="editor-upload-log">
                            {log.map((line, i) => (
                                <div key={i} className={`editor-log-line ${line.tone}`}>
                                    <span className="editor-log-time mono">{line.time}</span>
                                    <span className="editor-log-text">{line.text}</span>
                                </div>
                            ))}
                        </div>
                    )}
                    {running && (
                        <div className="editor-actions">
                            <button type="button" className="btn-ghost" onClick={cancel}>Cancel</button>
                            <span className="editor-actions-spacer"/>
                            <button type="button" className="btn-primary inert" disabled>Translating...</button>
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

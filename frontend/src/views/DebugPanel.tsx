import './DebugPanel.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DeveloperToolsStatus} from '../../wailsjs/go/main/App';
import {Toggle} from '../components/Toggle';
import {notify} from '../data/notifications';
import {patchPreferences} from '../data/preferencesPatch';
import {inspectorShortcut, openInspector, setShiftRightClickNative} from '../data/developerTools';
import type {DeveloperToolsStatus as Status} from '../data/developerTools';
import {clearMeasurements, getMeasurements, isEnabled, setEnabled} from '../data/profiling';
import type {Measurement} from '../data/profiling';

// How often the Profiling list re-reads data/profiling.ts's running totals while it's on screen -
// Browse (the thing being profiled) lives on a different view, so this panel has no event to
// react to and just polls, the same tradeoff Settings already accepts elsewhere for small stats.
const POLL_MS = 500;

// Settings > Debug: tools for finding problems and testing changes to the interface. The tab is
// offered in development builds only (wails dev, F5 in VS Code) - see devtools.go.
export function DebugPanel() {
    const [status, setStatus] = useState<Status | null>(null);
    const [profilingOn, setProfilingOn] = useState(isEnabled());
    const [measurements, setMeasurements] = useState<Measurement[]>(getMeasurements());

    useEffect(() => {
        DeveloperToolsStatus().then(setStatus).catch(() => undefined);
    }, []);

    useEffect(() => {
        if (!profilingOn) return;
        const iv = window.setInterval(() => setMeasurements(getMeasurements()), POLL_MS);
        return () => window.clearInterval(iv);
    }, [profilingOn]);

    async function toggle() {
        if (!status) return;
        const on = !status.Enabled;
        setStatus({...status, Enabled: on});
        try {
            await patchPreferences({developerTools: on});
            setShiftRightClickNative(on);
            setStatus(await DeveloperToolsStatus());
        } catch (err) {
            setStatus(status);
            notify('error', `Couldn't change the developer tools setting: ${String(err)}`);
        }
    }

    function open() {
        if (!status || !openInspector(status.OS)) {
            notify('info', `Press ${status ? inspectorShortcut(status.OS) : 'the shortcut'} to open the developer tools.`);
        }
    }

    function toggleProfiling() {
        const on = !profilingOn;
        setEnabled(on);
        setProfilingOn(on);
        setMeasurements(getMeasurements());
    }

    function clearProfiling() {
        clearMeasurements();
        setMeasurements([]);
    }

    const shortcut = status ? inspectorShortcut(status.OS) : '';
    // The inspector can only be opened from here where the window has a message for it.
    const canOpenFromHere = status?.OS === 'linux' || status?.OS === 'darwin';
    const maxTotalMs = Math.max(1, ...measurements.map((m) => m.totalMs));

    return (
        <div className="settings-content single">
            <div>
                <div className="settings-title">Debug</div>
                <div className="settings-subtitle">
                    Tools for finding problems and testing changes to the interface. This tab only exists in a
                    development build (<code>wails dev</code>, or F5 in VS Code); a release build does not have it.
                </div>
            </div>

            <div className="sort-rules-list">
                <div className="sort-rule-row">
                    <div className="sort-rule-main">
                        <div className="sort-rule-name">Developer tools</div>
                        <div className="sort-rule-desc">
                            Lets Shift+right-click through to the browser's own menu, which has Inspect Element: look at
                            the page, its network calls and its console.
                        </div>
                    </div>
                    <Toggle on={!!status?.Enabled} onClick={status ? toggle : undefined}/>
                </div>
            </div>

            {status && (
                <div className="debug-how">
                    <div className="debug-how-row">
                        <span>Open the inspector at any time with <code>{shortcut}</code>.</span>
                        {canOpenFromHere && <span className="btn-ghost" onClick={open}>Open developer tools</span>}
                    </div>
                </div>
            )}

            <div>
                <div className="settings-title">Profiling</div>
                <div className="settings-subtitle">
                    Times Browse's own real work (its mount-time fetches and its slowest computations) with the
                    browser's own performance.mark/measure, and ranks it below by total cost. Has a small,
                    constant per-call overhead while on - leave it off unless you're actually investigating.
                </div>
            </div>

            <div className="sort-rules-list">
                <div className="sort-rule-row">
                    <div className="sort-rule-main">
                        <div className="sort-rule-name">Frontend profiling</div>
                        <div className="sort-rule-desc">
                            Turn this on, then open Browse (signed in to LoversLab, or with any real data) to
                            generate measurements.
                        </div>
                    </div>
                    <Toggle on={profilingOn} onClick={toggleProfiling}/>
                </div>
            </div>

            {profilingOn && (
                measurements.length === 0 ? (
                    <div className="debug-how">
                        <div className="debug-how-row">
                            <span>Nothing recorded yet - open Browse to generate some measurements.</span>
                        </div>
                    </div>
                ) : (
                    <div className="profiling-list">
                        {measurements.map((m) => (
                            <div className="profiling-row" key={m.label}>
                                <span className="profiling-row-bar"/>
                                <div className="profiling-row-body">
                                    <div className="profiling-row-name">{m.label}</div>
                                    <div className="profiling-row-detail">
                                        {m.count}x - avg {m.avgMs.toFixed(1)}ms - max {m.maxMs.toFixed(1)}ms - total {m.totalMs.toFixed(1)}ms
                                    </div>
                                    <div className="profiling-row-bar-track">
                                        <div className="profiling-row-bar-fill" style={{width: `${(m.totalMs / maxTotalMs) * 100}%`}}/>
                                    </div>
                                </div>
                            </div>
                        ))}
                        <div className="debug-how-row">
                            <span>Ranked by total time spent, slowest first.</span>
                            <span className="btn-ghost" onClick={clearProfiling}>Clear</span>
                        </div>
                    </div>
                )
            )}
        </div>
    );
}

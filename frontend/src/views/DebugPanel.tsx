import './DebugPanel.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DeveloperToolsStatus} from '../../wailsjs/go/main/App';
import {Toggle} from '../components/Toggle';
import {notify} from '../data/notifications';
import {patchPreferences} from '../data/preferencesPatch';
import {inspectorShortcut, openInspector, setShiftRightClickNative} from '../data/developerTools';
import type {DeveloperToolsStatus as Status} from '../data/developerTools';

// Settings > Debug: tools for finding problems and testing changes to the interface. The tab is
// offered in development builds only (wails dev, F5 in VS Code) - see devtools.go.
export function DebugPanel() {
    const [status, setStatus] = useState<Status | null>(null);

    useEffect(() => {
        DeveloperToolsStatus().then(setStatus).catch(() => undefined);
    }, []);

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

    const shortcut = status ? inspectorShortcut(status.OS) : '';
    // The inspector can only be opened from here where the window has a message for it.
    const canOpenFromHere = status?.OS === 'linux' || status?.OS === 'darwin';

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
        </div>
    );
}

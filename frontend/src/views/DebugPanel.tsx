import './DebugPanel.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {DeveloperToolsStatus} from '../../wailsjs/go/main/App';
import {Toggle} from '../components/Toggle';
import {notify} from '../data/notifications';
import {patchPreferences} from '../data/preferencesPatch';
import {inspectorShortcut, openInspector, setShiftRightClickNative} from '../data/developerTools';
import type {DeveloperToolsStatus as Status} from '../data/developerTools';

// Settings > Debug: tools for finding problems and testing changes to the interface. Nothing
// here is needed to use the app.
export function DebugPanel() {
    const [status, setStatus] = useState<Status | null>(null);

    useEffect(() => {
        DeveloperToolsStatus().then(setStatus).catch(() => undefined);
    }, []);

    async function toggle() {
        if (!status || !status.BuiltIn) return;
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
                    Tools for finding problems and testing changes to the interface. Nothing here is needed to use the app.
                </div>
            </div>

            {status && !status.BuiltIn && (
                <div className="debug-note warn">
                    <i className="fa-solid fa-triangle-exclamation"/>
                    <span>
                        This build was made without developer tools, so this switch does nothing. Build with{' '}
                        <code>./build.sh</code> (or <code>wails build -devtools</code>) to include them; a
                        {' '}<code>wails dev</code> session always has them.
                    </span>
                </div>
            )}

            <div className="sort-rules-list">
                <div className={`sort-rule-row ${status && !status.BuiltIn ? 'disabled' : ''}`}>
                    <div className="sort-rule-main">
                        <div className="sort-rule-name">Developer tools</div>
                        <div className="sort-rule-desc">
                            Adds the browser's inspector to the app: look at the page, its network calls and its console.
                            Useful for a bug report or when testing a change to the interface.
                        </div>
                    </div>
                    <Toggle on={!!status?.Enabled && !!status?.BuiltIn} onClick={status?.BuiltIn ? toggle : undefined}/>
                </div>
            </div>

            {status?.RestartNeeded && (
                <div className="debug-note warn">
                    <i className="fa-solid fa-rotate"/>
                    <span>Restart the app to {status.Enabled ? 'turn the developer tools on' : 'turn the developer tools off'}: the window's right-click menu is set up when it opens.</span>
                </div>
            )}

            {status?.BuiltIn && status.Enabled && (
                <div className="debug-how">
                    <div className="debug-how-row">
                        <span>Open the inspector with <code>{shortcut}</code>.</span>
                        {canOpenFromHere && <span className="btn-ghost" onClick={open}>Open developer tools</span>}
                    </div>
                    <div>
                        Hold <code>Shift</code> and right-click anywhere for the browser's own menu, which has Inspect Element.
                    </div>
                </div>
            )}

            {status?.BuiltIn && (
                <div className="debug-footnote">
                    The keyboard shortcut belongs to the window itself, so it answers in any build that includes developer
                    tools, whatever this switch says. The switch adds the right-click menu and the button above.
                </div>
            )}
        </div>
    );
}

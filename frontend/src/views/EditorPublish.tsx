import {Fragment, h} from 'preact';
import type {library} from '../../wailsjs/go/models';

// The Publish tab: uploading a mod to the Steam Workshop as a new item, or pushing an update to
// one already there, with a line-by-line log of what the app and Steam did. Not built yet - laid
// out here as it will look, with example content clearly marked as an example, so the shape is
// settled before the real upload code exists. Every control is disabled; nothing here uploads,
// signs in, or talks to Steam.

const EXAMPLE_LOG: { time: string; tone: 'info' | 'success' | 'warn'; text: string }[] = [
    {time: '14:02:11', tone: 'info', text: 'Packing the mod folder (18.4 MB, 214 files)...'},
    {time: '14:02:12', tone: 'info', text: 'Uploading to Steam...'},
    {time: '14:02:19', tone: 'info', text: "Steam: item update created (UGC handle 8471...2201)"},
    {time: '14:02:19', tone: 'success', text: 'Steam confirmed the upload.'},
    {time: '14:02:19', tone: 'info', text: 'Setting the change note, tags and visibility...'},
    {time: '14:02:20', tone: 'success', text: "Published. The Workshop page is live at steamcommunity.com/sharedfiles/filedetails/?id=...  "},
];

export function EditorPublish({mod}: { mod: library.ModSummary }) {
    const hasRemote = !!mod.RemoteFileID;
    return (
        <div className="editor-columns">
            <div className="editor-column">
                <div className="editor-soon-banner">
                    <i className="fa-solid fa-cloud-arrow-up"/>
                    <div>
                        <div className="editor-soon-title">Publishing to the Steam Workshop <span className="chip">Coming later</span></div>
                        <div>
                            Upload this mod as a new Workshop item, or push an update to one you already own. This is a
                            preview of the layout with example content below - nothing here is connected to Steam yet.
                        </div>
                    </div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">STEAM ACCOUNT</div>
                    <div className="editor-muted">Not connected. Publishing will ask you to sign in to Steam when it is built.</div>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">DESTINATION</div>
                    <label className="mode-option disabled">
                        <i className={`fa-solid ${!hasRemote ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio off`}/>
                        <div className="mode-option-main">
                            <div className="mode-option-name">Upload as a new Workshop item</div>
                            <div className="mode-option-desc">Creates a brand new Workshop page for this mod.</div>
                        </div>
                    </label>
                    <label className="mode-option disabled">
                        <i className={`fa-solid ${hasRemote ? 'fa-circle-dot' : 'fa-circle'} mode-option-radio off`}/>
                        <div className="mode-option-main">
                            <div className="mode-option-name">Update the existing item</div>
                            <div className="mode-option-desc">
                                {hasRemote ? <>Pushes changes to Workshop item <span className="mono">{mod.RemoteFileID}</span>, this mod's own.</> : 'This mod has no Workshop item of its own yet.'}
                            </div>
                        </div>
                    </label>
                </div>

                <div className="editor-card">
                    <div className="editor-card-title">DETAILS</div>
                    <label className="editor-field">
                        <span className="editor-label">Change note</span>
                        <textarea className="editor-input" rows={3} disabled placeholder="What changed in this update? Shown to subscribers on the Workshop page."/>
                    </label>
                    <div className="editor-field-row">
                        <label className="editor-field">
                            <span className="editor-label">Visibility</span>
                            <select className="editor-input" disabled>
                                <option>Public</option>
                                <option>Friends only</option>
                                <option>Unlisted</option>
                                <option>Private</option>
                            </select>
                        </label>
                        <label className="editor-field">
                            <span className="editor-label">Tags</span>
                            <input className="editor-input" disabled placeholder="Taken from the mod's own tags"/>
                        </label>
                    </div>
                    <button type="button" className="btn-primary" disabled><i className="fa-solid fa-cloud-arrow-up"/> Publish</button>
                </div>
            </div>

            <div className="editor-column">
                <div className="editor-card editor-changes">
                    <div className="editor-card-title">UPLOAD LOG</div>
                    <div className="editor-muted">
                        Nothing uploaded yet. Once this is built, each step and Steam's own answer appear here as they happen,
                        the same way the activity log works. Example of what that will look like:
                    </div>
                    <div className="editor-example-log">
                        {EXAMPLE_LOG.map((line, i) => (
                            <div key={i} className={`editor-log-line ${line.tone}`}>
                                <span className="editor-log-time mono">{line.time}</span>
                                <span className="editor-log-text">{line.text}</span>
                            </div>
                        ))}
                    </div>
                    <div className="editor-example-tag"><i className="fa-solid fa-flask"/> Example only, not a real upload</div>
                </div>
            </div>
        </div>
    );
}

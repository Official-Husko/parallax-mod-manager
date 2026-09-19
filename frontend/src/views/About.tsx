import './About.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {AboutInfo, OpenPath} from '../../wailsjs/go/main/App';
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime';
import type {about} from '../../wailsjs/go/models';

// What the app is good at, in a line each - kept to things that are really
// built (see the README's "Done" list), never aspirations.
const FEATURES: { icon: string; title: string; text: string }[] = [
    {
        icon: 'fa-bolt',
        title: 'Incremental scanning',
        text: "A stat, hash, parse cache means a mod that hasn't changed is never parsed again - big modlists reopen fast.",
    },
    {
        icon: 'fa-code-compare',
        title: 'Real conflict detection',
        text: 'Finds where mods genuinely define the same object - per definition, not per file - with a side-by-side diff to compare them.',
    },
    {
        icon: 'fa-bandage',
        title: 'Patches that stay honest',
        text: 'Writes your resolutions into a patch mod, then tells you when a mod update makes it out of date.',
    },
    {
        icon: 'fa-arrow-down-arrow-up',
        title: 'Autosort',
        text: 'Orders by declared dependencies and Fixes/Patch tags, and keeps the generated patch at the very end.',
    },
    {
        icon: 'fa-layer-group',
        title: 'Playsets and DLC',
        text: 'Named load orders with per-playset DLC, and a way to bring in the playsets you already have in the Paradox Launcher.',
    },
    {
        icon: 'fa-rocket',
        title: 'Your choice of launch',
        text: 'Start the game through Steam, or straight from its own executable without opening the launcher.',
    },
];

// The stack, shown as plain chips.
const BUILT_WITH = ['Go', 'Wails', 'Preact', 'TypeScript', 'Vite'];

// One GitHub-style badge: a muted label on the left, a colored value on the
// right. Drawn here rather than fetched as an image from a badge service, so
// the page makes no network requests and works offline.
function Badge({label, value, tone}: { label: string; value: string; tone: 'rust' | 'blue' | 'green' | 'amber' | 'grey' }) {
    return (
        <span className="about-badge">
            <span className="about-badge-label">{label}</span>
            <span className={`about-badge-value ${tone}`}>{value}</span>
        </span>
    );
}

function DetailRow({label, value, onOpen}: { label: string; value: string; onOpen?: () => void }) {
    return (
        <div className="about-detail-row">
            <span className="about-detail-label">{label}</span>
            <span className="mono about-detail-value" title={value}>{value || '-'}</span>
            {onOpen && value && (
                <span className="about-detail-open" title="Open in your file manager" onClick={onOpen}>
                    <i className="fa-regular fa-folder-open"/>
                </span>
            )}
        </div>
    );
}

export function About() {
    const [info, setInfo] = useState<about.Info | null>(null);
    const [failed, setFailed] = useState(false);

    useEffect(() => {
        AboutInfo().then(setInfo).catch(() => setFailed(true));
    }, []);

    if (failed) {
        return <div className="about"><p className="status-page error">Couldn't read this build's details.</p></div>;
    }
    if (!info) {
        return <div className="about"/>;
    }

    const platform = `${info.OS}/${info.Arch}`;
    const build = info.Commit ? `${info.Commit}${info.Dirty ? '-dirty' : ''}` : '';

    return (
        <div className="about">
            <div className="about-inner">
                <section className="about-hero">
                    <img className="about-logo" src="/favicon.png" alt=""/>
                    <div className="about-hero-main">
                        <h1 className="about-name">{info.Name}</h1>
                        <p className="about-tagline">
                            A fast, from-scratch mod manager for Paradox Interactive's grand strategy games.
                        </p>
                        <div className="about-badges">
                            <Badge label="version" value={info.Version} tone="rust"/>
                            {info.Games > 0 && <Badge label="games" value={`${info.Games} supported`} tone="green"/>}
                            <Badge label="go" value={info.GoVersion} tone="blue"/>
                            {info.WailsVersion && <Badge label="wails" value={info.WailsVersion} tone="blue"/>}
                            <Badge label="platform" value={platform} tone="grey"/>
                            {build && <Badge label="build" value={build} tone={info.Dirty ? 'amber' : 'grey'}/>}
                        </div>
                    </div>
                </section>

                {info.Links.length > 0 && (
                    <section className="about-links">
                        {info.Links.map((link) => (
                            <span key={link.URL} className="about-link" title={link.URL} onClick={() => BrowserOpenURL(link.URL)}>
                                <i className={link.Icon}/>
                                {link.Label}
                            </span>
                        ))}
                    </section>
                )}

                <section>
                    <div className="about-section-label">WHAT IT DOES</div>
                    <div className="about-features">
                        {FEATURES.map((f) => (
                            <div key={f.title} className="about-feature">
                                <i className={`fa-duotone fa-solid ${f.icon} about-feature-icon`}/>
                                <div className="about-feature-title">{f.title}</div>
                                <div className="about-feature-text">{f.text}</div>
                            </div>
                        ))}
                    </div>
                </section>

                <section className="about-columns">
                    <div className="about-card">
                        <div className="about-section-label">THIS BUILD</div>
                        <DetailRow label="Version" value={info.Version}/>
                        <DetailRow label="Commit" value={build}/>
                        <DetailRow label="Go" value={info.GoVersion}/>
                        <DetailRow label="Wails" value={info.WailsVersion}/>
                        <DetailRow label="Platform" value={platform}/>
                        <DetailRow label="Settings folder" value={info.ConfigDir} onOpen={() => OpenPath(info.ConfigDir).catch(() => undefined)}/>
                        <DetailRow label="Cache folder" value={info.CacheDir} onOpen={() => OpenPath(info.CacheDir).catch(() => undefined)}/>
                    </div>
                    <div className="about-card">
                        <div className="about-section-label">BUILT WITH</div>
                        <div className="about-chips">
                            {BUILT_WITH.map((name) => <span key={name} className="about-chip">{name}</span>)}
                        </div>
                        <p className="about-note">
                            Icons by Font Awesome Pro. Workshop details come from Steam's public web API - the
                            only outside service the app talks to.
                        </p>
                    </div>
                </section>

                <footer className="about-footer">
                    {info.Author && <div>Made by <b>{info.Author}</b></div>}
                    <div className="about-disclaimer">
                        An independent project - not affiliated with or endorsed by Paradox Interactive or Valve.
                    </div>
                </footer>
            </div>
        </div>
    );
}

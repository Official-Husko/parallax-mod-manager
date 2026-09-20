import './About.css';
import {h} from 'preact';
import {useEffect, useState} from 'preact/hooks';
import {AboutInfo, OpenPath} from '../../wailsjs/go/main/App';
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime';
import type {about} from '../../wailsjs/go/models';
import {LogView} from '../components/LogView';
import {LicenceModal} from './LicenceModal';

// The stack, shown as plain chips.
const BUILT_WITH = ['Go', 'Wails', 'Preact', 'TypeScript', 'Vite'];

// What the licence means for someone using the app, in a few lines. A summary of
// the sections that matter day to day (3 grant, 4 and 7 no payment or resale, 5
// donations, 8 and 11 source and same licence, 13 and 14 attribution and forks);
// the About card says it is not the licence itself.
const LICENCE_POINTS = [
    {icon: 'fa-circle-check', color: 'var(--green)', text: 'Free to use, study and modify for non-commercial purposes.'},
    {icon: 'fa-hand-holding-heart', color: 'var(--red)', text: 'Voluntary donations are welcome - they never unlock anything.'},
    {icon: 'fa-ban', color: 'var(--amber)', text: 'No selling, paywalls, or paid editions, features or updates.'},
    {icon: 'fa-code-branch', color: 'var(--blue)', text: 'Copies you share include the source and stay under this licence.'},
    {icon: 'fa-signature', color: 'var(--text-muted)', text: 'Forks need their own name and a link to the official project.'},
];

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

// The About panel in Settings: what this build is, where it keeps its files, and
// a live view of everything the app is doing.
export function AboutPanel() {
    const [info, setInfo] = useState<about.Info | null>(null);
    const [failed, setFailed] = useState(false);
    const [showLicence, setShowLicence] = useState(false);

    useEffect(() => {
        AboutInfo().then(setInfo).catch(() => setFailed(true));
    }, []);

    if (failed) {
        return <div className="settings-content wide"><p className="status-page error">Couldn't read this build's details.</p></div>;
    }
    if (!info) {
        return <div className="settings-content wide"/>;
    }

    const platform = `${info.OS}/${info.Arch}`;
    const build = info.Commit ? `${info.Commit}${info.Dirty ? '-dirty' : ''}` : '';

    return (
        <div className="settings-content wide">
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
                    <div className="about-section-label">ACTIVITY LOG</div>
                    <LogView onOpenFolder={info.LogDir ? () => OpenPath(info.LogDir).catch(() => undefined) : undefined}/>
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
                        <DetailRow label="Log folder" value={info.LogDir} onOpen={() => OpenPath(info.LogDir).catch(() => undefined)}/>
                    </div>
                    <div className="about-card">
                        <div className="about-section-label">BUILT WITH</div>
                        <div className="about-chips">
                            {BUILT_WITH.map((name) => <span key={name} className="about-chip">{name}</span>)}
                        </div>
                        <p className="about-note">
                            Icons by Font Awesome Pro. Workshop details come from Steam's public web API and, in
                            online mode, background images from this project's GitHub repository - the only outside
                            services the app talks to. Set backgrounds to Offline and only Steam is used.
                        </p>
                    </div>
                </section>

                {info.LicenceName && (
                    <section className="about-card about-licence">
                        <div className="about-section-label">LICENCE</div>
                        <div className="about-licence-head">
                            <i className="fa-solid fa-scale-balanced"/>
                            <span className="about-licence-name">{info.LicenceName}</span>
                            {info.LicenceID && <Badge label="licence" value={info.LicenceID} tone="amber"/>}
                        </div>
                        <ul className="about-licence-points">
                            {LICENCE_POINTS.map((p) => (
                                <li key={p.text}>
                                    <i className={`fa-solid ${p.icon}`} style={{color: p.color}}/>
                                    <span>{p.text}</span>
                                </li>
                            ))}
                        </ul>
                        <p className="about-note">
                            That is a summary, not the licence - the full text is what applies.
                        </p>
                        <div className="about-licence-actions">
                            <span className="btn-ghost" onClick={() => setShowLicence(true)}>
                                <i className="fa-solid fa-file-contract"/> Read the full licence
                            </span>
                        </div>
                    </section>
                )}

                <footer className="about-footer">
                    {info.Author && (
                        <div>
                            Made by <b>{info.Author}</b> with <i className="fa-solid fa-heart about-heart" aria-label="love"/>
                        </div>
                    )}
                    {info.LicenceName && (
                        <div className="about-licence-line">
                            Licensed under{' '}
                            <span className="about-footer-link" onClick={() => setShowLicence(true)}>{info.LicenceName}</span>
                        </div>
                    )}
                    <div className="about-disclaimer">
                        An independent project - not affiliated with or endorsed by Paradox Interactive or Valve.
                    </div>
                </footer>
            </div>
            {showLicence && <LicenceModal name={info.LicenceName} id={info.LicenceID} onClose={() => setShowLicence(false)}/>}
        </div>
    );
}

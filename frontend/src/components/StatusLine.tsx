import {h} from 'preact';

// A one-line "here's what's going on" banner - an icon (colored by tone) then the text. The
// Tools (DeepL key) and Steam API settings panels each hand-rolled the exact same markup and
// CSS for this under their own class name; this is the one shared version.
export function StatusLine({icon, tone, text}: {
    icon: string; // a Font Awesome solid glyph name
    tone: 'good' | 'warn' | 'bad' | 'neutral';
    text: string;
}) {
    return (
        <div className={`status-line ${tone}`}>
            <i className={`fa-solid ${icon}`}/>
            <span>{text}</span>
        </div>
    );
}

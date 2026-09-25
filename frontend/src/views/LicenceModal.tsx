import './LicenceModal.css';
import {h} from 'preact';
import type {ComponentChild} from 'preact';
import {useEffect, useMemo, useState} from 'preact/hooks';
import {LicenceText} from '../../wailsjs/go/main/App';
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime';
import {ModalHeader} from '../components/ModalHeader';
import {type Block, type Inline, parseMarkdown} from '../data/markdown';

function renderInlines(inlines: Inline[]): ComponentChild[] {
    return inlines.map((part, i) => {
        switch (part.kind) {
            case 'bold': return <b key={i}>{part.text}</b>;
            case 'code': return <code key={i}>{part.text}</code>;
            case 'link': return <span key={i} className="licence-link" onClick={() => BrowserOpenURL(part.text)}>{part.text}</span>;
            default: return <span key={i}>{part.text}</span>;
        }
    });
}

function renderBlock(block: Block, i: number): ComponentChild {
    switch (block.kind) {
        case 'h1': return <h1 key={i}>{renderInlines(block.inlines)}</h1>;
        case 'h2': return <h2 key={i}>{renderInlines(block.inlines)}</h2>;
        case 'hr': return <hr key={i}/>;
        case 'ul': return <ul key={i}>{block.items.map((item, j) => <li key={j}>{renderInlines(item)}</li>)}</ul>;
        case 'ol': return <ol key={i}>{block.items.map((item, j) => <li key={j}>{renderInlines(item)}</li>)}</ol>;
        default: return <p key={i}>{renderInlines(block.inlines)}</p>;
    }
}

// The project's licence in full - the text shipped inside this build - in a window
// of its own, from the About page.
export function LicenceModal({name, id, onClose}: { name: string; id: string; onClose: () => void }) {
    const [text, setText] = useState<string | null>(null);
    const [failed, setFailed] = useState(false);

    useEffect(() => {
        LicenceText().then(setText).catch(() => setFailed(true));
    }, []);

    useEffect(() => {
        const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [onClose]);

    const blocks = useMemo(() => (text ? parseMarkdown(text) : []), [text]);

    return (
        <div className="overlay" onClick={onClose}>
            <div className="licence-modal" onClick={(e) => e.stopPropagation()}>
                <ModalHeader
                    title={name || 'Licence'} icon="fa-scale-balanced" overlay onClose={onClose} closeTitle="Close (Esc)"
                    headerClassName="licence-modal-header"
                >
                    {id && <span className="mono licence-id">{id}</span>}
                </ModalHeader>
                <div className="licence-modal-body">
                    {failed && <p className="status-page error">Couldn't read the licence text.</p>}
                    {!failed && text === null && <p className="status-page">Loading...</p>}
                    {text !== null && <article className="licence-doc">{blocks.map(renderBlock)}</article>}
                </div>
            </div>
        </div>
    );
}

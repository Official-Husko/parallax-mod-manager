import './ConflictResolver.css';
import {h} from 'preact';
import {useState} from 'preact/hooks';
import {
    conflictFiles,
    contenders,
    diffLeft,
    diffRight,
    matrix,
    matrixHighlight,
    matrixRows,
    resolutionOptions,
} from '../data/mockData';

export function ConflictResolver({onClose}: { onClose: () => void }) {
    const [mode, setMode] = useState<'files' | 'matrix'>('files');

    return (
        <div className="overlay" onClick={onClose}>
            <div className="resolver" onClick={(e) => e.stopPropagation()}>
                <div className="resolver-header">
                    <span className="title">Conflicts</span>
                    <span className="badge hard">4 hard</span>
                    <span className="badge soft">11 overwrites</span>
                    <div className="spacer"/>
                    <span className="mode-toggle">
                        <span className={mode === 'files' ? 'active' : ''} onClick={() => setMode('files')}>Files</span>
                        <span className={mode === 'matrix' ? 'active' : ''} onClick={() => setMode('matrix')}>Matrix</span>
                    </span>
                    <span className="btn-ghost">Auto-resolve all</span>
                    <span className="btn-primary">Apply & re-scan</span>
                    <i className="fa-solid fa-xmark close-btn" onClick={onClose}/>
                </div>

                {mode === 'files' ? <FilesView/> : <MatrixView/>}
            </div>
        </div>
    );
}

function FilesView() {
    return (
        <div className="resolver-body">
            <div className="files-col">
                <div className="col-header">CONTESTED FILES · {conflictFiles.length}</div>
                <div className="files-list">
                    {conflictFiles.map((f) => (
                        <div key={f.path} className="file-item" style={{background: f.bg, borderLeftColor: f.edge}}>
                            <div className="mono path" style={{color: f.pathC}}>{f.path}</div>
                            <div className="note">{f.note}</div>
                        </div>
                    ))}
                </div>
            </div>

            <div className="contenders-col">
                <div className="col-header">CONTENDERS · LOAD ORDER</div>
                <div className="contenders-list">
                    {contenders.map((c) => (
                        <div key={c.name} className={`contender-card ${c.wins ? 'wins' : ''}`}>
                            <div className="contender-head">
                                <span className="mono pos">{c.pos}</span>
                                <span className="name">{c.name}</span>
                                {c.wins && <span className="wins-badge">WINS</span>}
                            </div>
                            <div className="mono meta">{c.meta}</div>
                            <div className={`note ${c.wins ? 'wins-note' : ''}`}>{c.note}</div>
                        </div>
                    ))}
                    <div className="resolution-card">
                        <div className="resolution-title">Resolution</div>
                        {resolutionOptions.map((o) => (
                            <span key={o.label}>
                                <span className={o.selected ? 'radio-on' : 'radio-off'}>{o.selected ? '●' : '○'}</span> {o.label}
                            </span>
                        ))}
                    </div>
                </div>
            </div>

            <div className="diff-col">
                <div className="diff-toolbar">
                    <span className="mono">common/units/division_templates/00_base.txt</span>
                    <div className="spacer"/>
                    <span className="sort-label">Side by side <i className="fa-solid fa-chevron-down"/></span>
                </div>
                <div className="diff-body mono">
                    <div className="diff-pane">
                        {diffLeft.map((d) => (
                            <div key={d.n} className="diff-line" style={{background: d.bg}}>
                                <span className="ln">{d.n}</span><span style={{color: d.c}}>{d.t}</span>
                            </div>
                        ))}
                    </div>
                    <div className="diff-pane">
                        {diffRight.map((d) => (
                            <div key={d.n} className="diff-line" style={{background: d.bg}}>
                                <span className="ln">{d.n}</span><span style={{color: d.c}}>{d.t}</span>
                            </div>
                        ))}
                    </div>
                </div>
                <div className="diff-footer">
                    <span className="mono">+38 -12</span>
                    <span>↑↓ next file · Enter accept · M merge</span>
                </div>
            </div>
        </div>
    );
}

function MatrixView() {
    return (
        <div className="matrix-view">
            <div className="matrix-intro">
                <div className="title">Overlap matrix</div>
                <div className="subtitle">Row overwrites column. Cell darkness = number of shared files.</div>
            </div>
            <div className="matrix-grid-wrap">
                <div className="matrix-labels">
                    {matrixRows.map((r) => <div key={r.label} className="matrix-label">{r.label}</div>)}
                </div>
                <div className="matrix-cells">
                    <div className="matrix-shorts">
                        {matrixRows.map((r) => <div key={r.short} className="matrix-short mono">{r.short}</div>)}
                    </div>
                    {matrix.map((row, ix) => (
                        <div key={ix} className="matrix-row">
                            {row.cells.map((c, jx) => (
                                <div key={jx} className="matrix-cell mono" style={{background: c.bg, color: c.fg}}>{c.v}</div>
                            ))}
                        </div>
                    ))}
                </div>
                <div className="matrix-scale">
                    <div className="sidebar-label">SCALE</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#1a212b'}}/>none</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#4a3a23'}}/>1-20</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#8a5f2a'}}/>21-99</div>
                    <div className="scale-row"><span className="swatch" style={{background: '#d4574e'}}/>100+</div>
                </div>
            </div>
            <div className="matrix-highlight">
                <div className="matrix-highlight-title">{matrixHighlight.title}</div>
                <div className="matrix-highlight-body">{matrixHighlight.body}</div>
            </div>
        </div>
    );
}

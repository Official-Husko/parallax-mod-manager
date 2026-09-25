import './FileTree.css';
import {h} from 'preact';
import type {JSX} from 'preact';
import type {library} from '../../wailsjs/go/models';
import {Checkbox} from './Checkbox';
import {formatBytes} from '../data/format';

type TreeNode = {
    name: string;
    relPath: string;
    isDir: boolean;
    size: number;
    children: TreeNode[];
};

// buildTree turns ListModFiles' flat, already-sorted entry list back into a
// real nested structure - needed to draw VS Code-style guide lines
// correctly (a line only continues past a row when that ancestor still has
// a later sibling, which a flat list alone doesn't tell you).
function buildTree(entries: library.FileEntry[]): TreeNode[] {
    const root: TreeNode = {name: '', relPath: '', isDir: true, size: 0, children: []};
    const byPath = new Map<string, TreeNode>([['', root]]);

    for (const e of entries) {
        const parts = e.RelPath.split('/');
        let parentPath = '';
        for (let i = 0; i < parts.length; i++) {
            const path = parts.slice(0, i + 1).join('/');
            const isRealEntry = i === parts.length - 1;
            let node = byPath.get(path);
            if (!node) {
                node = {
                    name: parts[i],
                    relPath: path,
                    isDir: isRealEntry ? e.IsDir : true,
                    size: isRealEntry ? e.Size : 0,
                    children: [],
                };
                byPath.set(path, node);
                byPath.get(parentPath)!.children.push(node);
            } else if (isRealEntry) {
                // A folder implied by an earlier, deeper entry (e.g. its
                // own file listed before it in the flat, alphabetical
                // order) now has its own real entry - fill in the real
                // IsDir/Size the placeholder guessed at.
                node.isDir = e.IsDir;
                node.size = e.Size;
            }
            parentPath = path;
        }
    }

    sortTree(root.children);
    rollUpDirSizes(root.children);
    return root.children;
}

// rollUpDirSizes fills in each directory node's own size as the sum of its real descendant
// files' sizes - ListModFiles' own FileEntry.Size is always 0 for a directory (there is no
// rolled-up size on the backend at all), so this is the only place a folder's total comes from.
// Returns the subtree's own total, so a parent's sum only ever walks its immediate children.
function rollUpDirSizes(nodes: TreeNode[]): number {
    let total = 0;
    for (const n of nodes) {
        if (n.isDir) n.size = rollUpDirSizes(n.children);
        total += n.size;
    }
    return total;
}

// Folders before files, alphabetical within each group - the convention
// every real file explorer uses, which a flat alphabetical-by-full-path
// sort doesn't quite give you on its own (e.g. "a.txt" sorts before "a/"
// since '.' comes before '/' in ASCII).
function sortTree(nodes: TreeNode[]) {
    nodes.sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        return a.name.localeCompare(b.name);
    });
    for (const n of nodes) sortTree(n.children);
}

type FileVisual = { icon: string; color: string };

const FOLDER: FileVisual = {icon: 'fa-folder', color: 'var(--text-muted)'};
const DEFAULT_FILE: FileVisual = {icon: 'fa-file', color: 'var(--text-dim)'};

// EXTENSION_VISUALS maps a file extension to an icon and color, grouped by
// the real content categories a Paradox mod actually ships (confirmed
// against real Stellaris Workshop mods' content folders on this machine:
// common/, events/, gfx/, interface/, localisation/, music/, sound/,
// fonts/, flags/) - not a generic-purpose file-type registry.
const EXTENSION_VISUALS: Record<string, FileVisual> = {
    // Clausewitz script and interface definitions - by far the most common
    // content type in any mod.
    txt: {icon: 'fa-file-lines', color: 'var(--file-script)'},
    gui: {icon: 'fa-file-lines', color: 'var(--file-script)'},
    gfx: {icon: 'fa-file-lines', color: 'var(--file-script)'},
    asset: {icon: 'fa-file-lines', color: 'var(--file-script)'},
    shader: {icon: 'fa-file-code', color: 'var(--file-script)'},
    fxh: {icon: 'fa-file-code', color: 'var(--file-script)'},
    // Localisation.
    yml: {icon: 'fa-language', color: 'var(--file-locale)'},
    // Descriptor/config-shaped metadata.
    mod: {icon: 'fa-gear', color: 'var(--amber)'},
    json: {icon: 'fa-gear', color: 'var(--amber)'},
    // Textures and images.
    dds: {icon: 'fa-file-image', color: 'var(--file-image)'},
    png: {icon: 'fa-file-image', color: 'var(--file-image)'},
    tga: {icon: 'fa-file-image', color: 'var(--file-image)'},
    jpg: {icon: 'fa-file-image', color: 'var(--file-image)'},
    jpeg: {icon: 'fa-file-image', color: 'var(--file-image)'},
    bmp: {icon: 'fa-file-image', color: 'var(--file-image)'},
    // Audio.
    ogg: {icon: 'fa-file-audio', color: 'var(--file-audio)'},
    wav: {icon: 'fa-file-audio', color: 'var(--file-audio)'},
    mp3: {icon: 'fa-file-audio', color: 'var(--file-audio)'},
    // Fonts.
    ttf: {icon: 'fa-font', color: 'var(--file-font)'},
    otf: {icon: 'fa-font', color: 'var(--file-font)'},
    fnt: {icon: 'fa-font', color: 'var(--file-font)'},
    // Plain documentation.
    md: {icon: 'fa-file-lines', color: 'var(--text-dim)'},
};

function visualFor(node: TreeNode): FileVisual {
    if (node.isDir) return FOLDER;
    const dot = node.name.lastIndexOf('.');
    const ext = dot >= 0 ? node.name.slice(dot + 1).toLowerCase() : '';
    return EXTENSION_VISUALS[ext] ?? DEFAULT_FILE;
}

// SelectionProps switches FileTree into checkbox mode - each row gets a
// checkbox (checked = included, unchecked = excluded), and a row under an
// excluded folder shows excluded too (its checkbox disabled, since
// re-including one file inside an excluded folder isn't offered - only
// re-including the whole folder is) without needing every descendant's own
// path to be in excluded itself. Matches
// workshop.PublishRequest.ExcludePaths' own "excluding a folder excludes
// everything under it" semantics exactly, so what this shows is always
// what publishing will actually do.
export interface SelectionProps {
    excluded: Set<string>;
    onToggle: (relPath: string, isDir: boolean) => void;
}

// Exactly .file-row's own height (FileTree.css) - a caller's windowing maths (useVirtualWindow,
// see FileTreeRows below) depends on it, the same requirement Library.tsx's own ROW_HEIGHT and
// LogView.tsx's own LINE_HEIGHT document.
export const FILE_ROW_HEIGHT = 24;

// buildFileTreeRows turns a mod's flat file listing into one flat array of already-built rows
// (buildTree's own sort and directory-size rollup, then renderNodes' recursive walk - both real
// work: a mod's file list can run into the thousands of entries, see docs/performance-strategy.md
// and library.maxModFileEntries). A caller wraps this in its own useMemo(..., [entries,
// selection, onSelectFile, selectedPath]) so it only redoes that work when one of those actually
// changes, not on every render for an unrelated reason (Publish's own upload-progress ticking is
// exactly this: it re-renders EditorPublish many times a second while running) - this used to be
// a memo() wrapped around the whole rendering component instead; moved out here because
// windowing (FileTreeRows, below) needs the caller to already know the total row count before it
// can even call useVirtualWindow, and the caller also owns whichever element actually scrolls
// (Workspace's shared, multi-tab .detail-content; EditorPublish's own self-contained
// .editor-file-picker) - a component FileTree itself doesn't render can't be virtualized from
// inside FileTree.
export function buildFileTreeRows(
    entries: library.FileEntry[],
    selection: SelectionProps | undefined,
    onSelectFile: ((relPath: string) => void) | undefined,
    selectedPath: string | undefined,
): JSX.Element[] {
    return renderNodes(buildTree(entries), [], selection, false, onSelectFile, selectedPath);
}

// FileTreeRows renders a windowed slice of buildFileTreeRows' own output - a thin,
// non-memoized presentational wrapper (the expensive part is already done by the time rows
// reaches here) that reserves the full list's own total height and offsets by first *
// FILE_ROW_HEIGHT, the same padding-top technique LogView.tsx and Library.tsx use for their own
// flat lists.
export function FileTreeRows({rows, first, last}: {rows: JSX.Element[]; first: number; last: number}) {
    return (
        <div className="file-tree-rows" style={{height: rows.length * FILE_ROW_HEIGHT, paddingTop: first * FILE_ROW_HEIGHT, boxSizing: 'border-box'}}>
            {rows.slice(first, last)}
        </div>
    );
}

// renderNodes recurses depth-first, tracking (for each ancestor level)
// whether that ancestor still has a later sibling - a continuing guide
// line is only drawn where the answer is yes, matching VS Code's own tree
// view rather than a flat "one line per depth level regardless" look.
// ancestorExcluded carries whether some ancestor folder is already excluded,
// so every one of its descendants renders as excluded too without needing
// its own path in selection.excluded.
//
// Returns a flat array (each node's row followed immediately by its own
// children's rows) rather than a nested JSX tree, so a directory's rows
// interleave with its siblings' in one flat list - exactly the shape a
// file tree's rows actually need, without reaching for a keyed Fragment
// per node (which preact's JSX typing here doesn't accept).
function renderNodes(
    nodes: TreeNode[],
    ancestorsContinue: boolean[],
    selection: SelectionProps | undefined,
    ancestorExcluded: boolean,
    onSelectFile?: (relPath: string) => void,
    selectedPath?: string,
): JSX.Element[] {
    return nodes.flatMap((node, i) => {
        const isLast = i === nodes.length - 1;
        const visual = visualFor(node);
        const excluded = ancestorExcluded || !!selection?.excluded.has(node.relPath);
        const clickable = !node.isDir && !!onSelectFile;
        const row = (
            <div
                key={node.relPath}
                className={`file-row ${excluded ? 'excluded' : ''} ${clickable ? 'selectable' : ''} ${clickable && selectedPath === node.relPath ? 'selected' : ''}`}
                onClick={clickable ? () => onSelectFile!(node.relPath) : undefined}
            >
                {ancestorsContinue.length > 0 && (
                    <span className="tree-guides">
                        {ancestorsContinue.map((cont, idx) => (
                            <span key={idx} className={`guide ${cont ? 'continue' : ''}`}/>
                        ))}
                        <span className="guide connector">
                            <span className="v-line" style={{height: isLast ? '50%' : '100%'}}/>
                            <span className="h-line"/>
                        </span>
                    </span>
                )}
                {selection && (
                    <Checkbox
                        checked={!excluded}
                        disabled={ancestorExcluded}
                        title={ancestorExcluded ? 'Included or excluded together with its own folder above' : excluded ? 'Excluded from the upload' : 'Included in the upload'}
                        onChange={() => selection.onToggle(node.relPath, node.isDir)}
                    />
                )}
                <i className={`fa-solid ${visual.icon}`} style={{color: excluded ? undefined : visual.color}}/>
                <span className="mono name">{node.name}</span>
                <span className="mono file-size">{formatBytes(node.size)}</span>
            </div>
        );
        const childRows = node.isDir && node.children.length > 0
            ? renderNodes(node.children, [...ancestorsContinue, !isLast], selection, excluded, onSelectFile, selectedPath)
            : [];
        return [row, ...childRows];
    });
}

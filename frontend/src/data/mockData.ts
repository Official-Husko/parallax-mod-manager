// Static demo content ported from mockup/Mod Manager.dc.html for the
// screens that don't have real backend data behind them yet (Library, DLC,
// Settings, the conflict resolver, and the various dialogs). Numbers and
// names are the mockup's own placeholder example content, kept verbatim -
// this is a visual preview of where those screens are headed, not real
// user data. Every "Chancellery" from the source mockup reads "Parallax
// Mod Manager" here, and every em dash is a hyphen, per this project's
// house style.

export const APP_NAME = 'Parallax Mod Manager';

// --- Library (4a) ---
// The games list and mod table are real (see views/Library.tsx) - only
// "collections" has no backing concept yet, so it stays static.

export const libCollections = [
    {label: 'Favourites', n: 41},
    {label: 'Multiplayer safe', n: 88},
    {label: 'Total conversions', n: 19},
    {label: 'QoL / UI', n: 134},
    {label: 'Archived', n: 207},
];

// --- DLC (4b) ---

export const dlcTypes = [
    {label: 'All packs', n: 29, active: true},
    {label: 'Expansions', n: 12, active: false},
    {label: 'Immersion packs', n: 7, active: false},
    {label: 'Cosmetic packs', n: 6, active: false},
    {label: 'Music packs', n: 4, active: false},
];

export const dlcPresets = [
    {label: 'All owned', dot: '#5fae7e', active: true},
    {label: 'Kaiserreich MP', dot: '#c4623a', active: false},
    {label: 'Vanilla historical', dot: '#5b8fc9', active: false},
    {label: 'Base game only', dot: '#8d99a9', active: false},
];

const dlcRowsRaw: [string, string, string, string, string, boolean, boolean, boolean?][] = [
    ['Together for Victory', 'Expansion', '2016', '420 MB', '2 mods', true, true],
    ['Death or Dishonor', 'Expansion', '2017', '310 MB', '1 mod', true, true],
    ['Waking the Tiger', 'Expansion', '2018', '680 MB', '3 mods', true, true],
    ['Man the Guns', 'Expansion', '2019', '740 MB', '4 mods', true, true],
    ['La Résistance', 'Expansion', '2020', '810 MB', '2 mods', true, true],
    ['Battle for the Bosporus', 'Immersion pack', '2020', '180 MB', '-', true, false],
    ['No Step Back', 'Expansion', '2021', '1.10 GB', '4 mods · 2 hard', true, true, true],
    ['By Blood Alone', 'Expansion', '2022', '920 MB', '3 mods', true, true],
    ['Arms Against Tyranny', 'Expansion', '2023', '1.02 GB', '2 mods', true, true],
    ['Trial of Allegiance', 'Expansion', '2024', '880 MB', '1 mod', false, false],
    ['Graveyard of Empires', 'Expansion', '2025', '1.24 GB', '-', false, false],
    ['Götterdämmerung', 'Expansion', '2024', '960 MB', '1 mod', true, true],
    ['Allied Armor', 'Cosmetic pack', '2021', '64 MB', '-', true, false],
    ['Axis Armor', 'Cosmetic pack', '2021', '62 MB', '-', true, false],
    ['Eastern Front Planes', 'Cosmetic pack', '2018', '88 MB', '1 mod', true, true],
    ['Allied Speeches Pack', 'Music pack', '2017', '210 MB', '-', true, false],
    ['Sabaton Soundtrack Vol. 3', 'Music pack', '2020', '240 MB', '-', true, false],
    ['Sounds of Rebellion', 'Music pack', '2025', '190 MB', '-', false, false],
];

export const dlcRows = dlcRowsRaw.map((d) => ({
    name: d[0], type: d[1], year: d[2], size: d[3],
    use: d[5] ? d[4] : 'not owned',
    nameC: d[5] ? '#d6dce4' : '#6a7484',
    useC: d[7] ? '#e0a340' : '#8d99a9',
    togBg: d[6] ? '#c4623a' : '#2d3846',
    knobLeft: !d[6],
    knobC: d[6] ? '#0e1218' : '#6a7484',
    highlighted: !!d[7],
}));

export const dlcSelected = {
    name: 'No Step Back',
    meta: 'Expansion · Nov 2021 · owned',
    description: 'Adds the supply network rework, tank designer, and the Eastern European focus trees. Several active mods build on its tank subsystem.',
    requiredBy: [
        {name: 'Kaiserreich', note: 'hard requirement', c: '#d4574e'},
        {name: 'Road to 56', note: 'hard requirement', c: '#d4574e'},
        {name: 'Aircraft Designer Plus', note: 'partial', c: '#e0a340'},
        {name: 'Improved Division Designer', note: 'partial', c: '#e0a340'},
    ],
    warning: '2 active mods will fail to load and 2 will lose features. Parallax Mod Manager will offer to deactivate them.',
    notOwned: ['Graveyard of Empires', 'Trial of Allegiance', 'Sounds of Rebellion'],
    checksum: '8f2a41c9',
};

// --- Settings: game profiles (3c) + sort rules (4c) ---

export const settingsNav = [
    {label: 'Game profiles', key: 'profiles'},
    {label: 'Paths & folders', key: 'paths'},
    {label: 'Sort rules', key: 'sort'},
    {label: 'Conflict scanning', key: 'conflict'},
    {label: 'Updates', key: 'updates'},
    {label: 'Playset sharing', key: 'sharing'},
    {label: 'Appearance', key: 'appearance'},
    {label: 'Advanced', key: 'advanced'},
];

export const sortRules = [
    {i: '1', name: 'Frameworks first', desc: 'Mods tagged as a framework or library load before anything that depends on them.', moves: '7 moves', on: true},
    {i: '2', name: 'Declared dependencies', desc: 'Honour requires/after metadata in descriptor.mod.', moves: '5 moves', on: true},
    {i: '3', name: 'Total conversions before submods', desc: 'A submod that references a conversion loads after it.', moves: '3 moves', on: true},
    {i: '4', name: 'Content before patches', desc: 'Anything named "compat" or "patch" is pushed below what it patches.', moves: '2 moves', on: true},
    {i: '5', name: 'Interface last', desc: 'UI mods override gameplay mods rather than the other way round.', moves: '0 moves', on: true},
    {i: '6', name: 'Alphabetical tiebreak', desc: 'When no rule applies, keep a stable A-Z order.', moves: '-', on: false},
].map((r, ix) => ({
    ...r,
    border: ix === 0 ? '#4a3826' : '#27313f',
    bg: ix === 0 ? '#191510' : '#131923',
}));

export const overrides = [
    {kind: 'PIN', text: 'Ship Name Fix - always last'},
    {kind: 'BEFORE', text: 'Coloured Buttons before UI Scaling Pack'},
    {kind: 'AFTER', text: 'Rt56 Submods - Navy after Road to 56'},
    {kind: 'IGNORE', text: 'Historical Portraits HD - skip conflict scan'},
];

export const sortLog = [
    {name: 'Gigastructural Engineering', move: '#38 → #14', why: 'Rule 3: submod of Planetary Diversity', edge: '#c4623a'},
    {name: 'Mod Menu Framework', move: '#22 → #01', why: 'Rule 1: tagged framework', edge: '#c4623a'},
    {name: 'Tiny Outliner v2', move: '#09 → #03', why: 'Rule 2: requires UI Overhaul Dynamic', edge: '#5b8fc9'},
    {name: 'Rt56 Submods - Navy', move: '#12 → #06', why: 'Manual override: after Road to 56', edge: '#e0a340'},
    {name: 'Coloured Buttons', move: '#18 → #03', why: 'Rule 5: interface last, then override', edge: '#e0a340'},
    {name: 'Final Patch Compat', move: '#07 → #22', why: 'Rule 4: name matches "compat"', edge: '#c4623a'},
    {name: 'KR Music Mod', move: '#31 → #02', why: 'Rule 2: required by Kaiserreich', edge: '#5b8fc9'},
];

// --- Conflict resolver (2a) + overlap matrix (2b) ---

const conflictFilesRaw: [string, string, string][] = [
    ['common/units/division_templates/00_base.txt', 'Road to 56 ↔ Improved Division Designer', '#d4574e'],
    ['common/ideas/00_generic.txt', 'Kaiserreich ↔ Road to 56', '#d4574e'],
    ['common/technologies/infantry.txt', '3 mods', '#d4574e'],
    ['common/countries/DEU.txt', 'Kaiserreich ↔ Endsieg', '#d4574e'],
    ['interface/countrypoliticsview.gui', 'Coloured Buttons ↔ UI Scaling', '#e0a340'],
    ['gfx/interface/topbar.dds', '2 mods', '#e0a340'],
    ['common/ai_strategy/00_ai.txt', 'Expert AI ↔ Road to 56', '#e0a340'],
    ['localisation/english/focus_l_english.yml', '4 mods', '#e0a340'],
    ['common/on_actions/00_on_actions.txt', '2 mods', '#e0a340'],
    ['map/strategicregions/01_europe.txt', 'Better Political Map', '#e0a340'],
    ['common/scripted_effects/00_effects.txt', '2 mods', '#e0a340'],
    ['common/national_focus/germany.txt', 'Endsieg ↔ TWR', '#e0a340'],
];

export const conflictFiles = conflictFilesRaw.map((f, ix) => ({
    path: f[0], note: f[1], edge: f[2],
    bg: ix === 0 ? '#1c1512' : 'transparent',
    pathC: ix === 0 ? '#eec3bd' : '#c3cad4',
}));

export const contenders = [
    {pos: '#05', name: 'Road to 56', meta: '412 lines · modified 2026-08-30', note: 'Loses - earlier in order', wins: false},
    {pos: '#15', name: 'Improved Division Designer', meta: '438 lines · modified 2026-09-04', note: 'Last in order - full file replace', wins: true},
];

export const resolutionOptions = [
    {label: 'Keep load-order winner', selected: true},
    {label: 'Force Road to 56', selected: false},
    {label: 'Generate merge patch', selected: false},
    {label: 'Exclude file from both', selected: false},
];

type DiffLine = [string, '' | '+' | '-'];

const diffLColor = (t: '' | '+' | '-') => (t === '+' ? '#9ecdb2' : t === '-' ? '#eec3bd' : '#7c889a');
const diffLBg = (t: '' | '+' | '-') => (t === '+' ? '#14241b' : t === '-' ? '#241514' : 'transparent');
const mkDiff = (lines: DiffLine[]) => lines.map((l, ix) => ({n: ix + 1, t: l[0], c: diffLColor(l[1]), bg: diffLBg(l[1])}));

export const diffLeft = mkDiff([
    ['division_template = {', ''], ['  name = "Infanterie"', ''], ['  division_names_group = GER_INF', ''],
    ['  regiments = {', ''], ['    infantry = { x=0 y=0 }', '-'], ['    infantry = { x=0 y=1 }', '-'],
    ['    artillery = { x=1 y=0 }', ''], ['  }', ''], ['  support = {', ''],
    ['    engineer = { x=0 y=0 }', ''], ['  }', ''], ['  priority = 2', '-'], ['}', ''],
    ['', ''], ['division_template = {', ''], ['  name = "Panzer"', ''], ['  regiments = {', ''], ['    light_armor = { x=0 y=0 }', ''],
]);

export const diffRight = mkDiff([
    ['division_template = {', ''], ['  name = "Infanterie"', ''], ['  division_names_group = GER_INF', ''],
    ['  regiments = {', ''], ['    infantry = { x=0 y=0 }', '+'], ['    infantry = { x=0 y=1 }', '+'],
    ['    infantry = { x=0 y=2 }', '+'], ['    artillery = { x=1 y=0 }', ''], ['  }', ''], ['  support = {', ''],
    ['    engineer = { x=0 y=0 }', ''], ['    recon = { x=1 y=0 }', '+'], ['  }', ''], ['  priority = 3', '+'], ['}', ''],
    ['', ''], ['division_template = {', ''], ['  name = "Panzer"', ''],
]);

const mLabels: [string, string][] = [
    ['Kaiserreich', 'KR'], ['Road to 56', 'Rt56'], ['Expert AI', 'ExAI'], ['Coloured Buttons', 'ColB'],
    ['Div Designer', 'IDD'], ['Endsieg', 'Ends'], ['Political Map', 'BPM'], ['UI Scaling', 'UIS'],
];
const matrixRaw = [
    [0, 412, 0, 0, 88, 301, 0, 0], [412, 0, 61, 0, 140, 77, 12, 0], [0, 61, 0, 0, 0, 0, 0, 0], [0, 0, 0, 0, 0, 0, 0, 14],
    [88, 140, 0, 0, 0, 0, 0, 0], [301, 77, 0, 0, 0, 0, 0, 0], [0, 12, 0, 0, 0, 0, 0, 3], [0, 0, 0, 14, 0, 0, 3, 0],
];
const cellBg = (v: number) => (v === 0 ? '#1a212b' : v < 21 ? '#4a3a23' : v < 100 ? '#8a5f2a' : '#d4574e');

export const matrixRows = mLabels.map((l) => ({label: l[0], short: l[1]}));
export const matrix = matrixRaw.map((row) => ({
    cells: row.map((v) => ({v: v || '', bg: cellBg(v), fg: v >= 100 ? '#1c1512' : '#c9b795'})),
}));
export const matrixHighlight = {
    title: 'Road to 56 → Kaiserreich · 412 files',
    body: 'Both are total conversions. Running them together is not supported by either author. Suggested: move one to a separate playset.',
};

export const domains = ['C', 'E', 'G', 'I', 'L', 'M'];

// --- Playsets (3a) ---

const playsetsRaw: [string, string, string, string, string][] = [
    ['Kaiserreich MP', 'ACTIVE', '#c4623a', '62 mods · 4 conflicts · shared with 3 friends', 'Open'],
    ['Vanilla+ Historical', 'READY', '#5fae7e', '14 mods · clean · last played 2 days ago', 'Activate'],
    ['Millennium Dawn', 'NEEDS 2', '#e0a340', '9 mods · 2 missing dependencies', 'Activate'],
    ['Old World Blues', 'READY', '#5fae7e', '7 mods · clean', 'Activate'],
    ['Testing sandbox', 'STALE', '#6a7484', '31 mods · built for 1.15', 'Activate'],
];

export const mockPlaysets = playsetsRaw.map((p, ix) => ({
    name: p[0], state: p[1], stateC: p[2], meta: p[3], action: p[4],
    border: ix === 0 ? '#4a3826' : '#27313f',
    bg: ix === 0 ? '#191510' : '#131923',
    actionC: ix === 0 ? '#e0a340' : '#8d99a9',
}));

// --- Update checker (3d) ---

const updatesRaw: [string, string, string, string, string, boolean][] = [
    ['Kaiserreich: Legacy of the Weltkrieg', 'changelog: naval rework, 41 fixes', '#6a7484', '0.26.1', '0.26.2', true],
    ['Road to 56', 'changelog: 1.16.4 compatibility', '#6a7484', '13.9', '14.0', true],
    ['Expert AI 4.0', 'still targets 1.16.2 - active in your playset', '#e0a340', '4.1', '4.2', true],
    ['Coloured Buttons', 'minor fixes', '#6a7484', '2.8', '2.9', true],
    ['Historical Portraits HD', 'new art for 14 nations', '#6a7484', '1.4', '1.5', true],
    ['Endsieg', 'changelog: focus tree additions', '#6a7484', '3.0', '3.1', true],
    ['Thousand Week Reich', 'major release · 1.2 GB download', '#6a7484', '1.7', '2.0', true],
    ['Better Peace Deals', 'abandoned by author · last update 2024', '#e0a340', '1.2', '1.2', false],
    ['Improved Division Designer', 'changelog: not published', '#6a7484', '2.0', '2.1', false],
];

export const updates = updatesRaw.map((u) => ({
    name: u[0], note: u[1], noteC: u[2], from: u[3], to: u[4],
    boxC: u[5] ? '#c4623a' : '#3c4858',
    boxBg: u[5] ? '#c4623a' : 'transparent',
}));

// --- Pre-flight (3e) ---

export const preflight = [
    {icon: 'fa-check', title: 'All 62 mods present', detail: 'Nothing missing from disk or Workshop.', c: '#5fae7e', action: '', actionC: ''},
    {icon: 'fa-xmark', title: '4 hard conflicts unresolved', detail: 'Road to 56 and Kaiserreich overwrite 412 shared files.', c: '#d4574e', action: 'Resolve', actionC: '#e0a340'},
    {icon: 'fa-triangle-exclamation', title: '2 mods target an older patch', detail: 'Expert AI 4.0 and Better Peace Deals were built for 1.15.', c: '#e0a340', action: 'Review', actionC: '#e0a340'},
    {icon: 'fa-check', title: 'Dependency chain complete', detail: 'Every required mod is active and correctly ordered.', c: '#5fae7e', action: '', actionC: ''},
    {icon: 'fa-check', title: 'Load order matches your friends', detail: 'Same checksum as Kaiserreich MP shared 11:04.', c: '#5fae7e', action: '', actionC: ''},
];

// --- First-run wizard (3f) ---

export const wizardSteps = [
    {n: 1, label: 'Find games'},
    {n: 2, label: 'Mod folders'},
    {n: 3, label: 'Import playsets'},
    {n: 4, label: 'Preferences'},
];

// --- Workspace (1a) static detail-panel content ---
// The selected mod's real Name/Version/Source/Tags come from the actual
// scan; everything below is decorative content the backend doesn't
// compute yet, shown verbatim from the mockup per the approved plan.

export const workspaceStaticDetail = {
    thumbnailLabel: 'MOD THUMBNAIL 584x300',
    updatedLine: 'updated 3 days ago',
    supports: '1.16.* [ok] matches',
    size: '4.21 GB · 38,914 files',
    category: 'Total conversion',
    description: 'An alternate history total conversion set in a world where the Central Powers won the Great War. Replaces the full 1936 bookmark, focus trees for 40+ nations, and the economic subsystem.',
    requires: [
        {name: 'Kaiserreich Music Mod', note: 'active #4', c: '#5fae7e'},
        {name: 'KR Unit Pack', note: 'not installed', c: '#e0a340'},
    ],
    overwrites: [
        {name: 'Road to 56 · 412', pct: 62, c: '#d4574e'},
        {name: 'Expert AI · 61', pct: 24, c: '#e0a340'},
        {name: 'Coloured Bu · 8', pct: 9, c: '#5b8fc9'},
    ],
};

export const workspaceTags = ['Total conversion', 'Alt history', '1936', 'Focus trees', 'Multiplayer'];

const fileTreeRaw: [string, string, number, string, string, string][] = [
    ['fa-chevron-down', 'common/', 12, '#c3cad4', '18,402', '#8d99a9'],
    ['fa-chevron-down', 'countries/', 26, '#c3cad4', '1,204', '#8d99a9'],
    ['', 'DEU.txt', 40, '#eec3bd', 'conflict', '#d4574e'],
    ['', 'FRA.txt', 40, '#8d99a9', '', '#8d99a9'],
    ['', 'RUS.txt', 40, '#8d99a9', '', '#8d99a9'],
    ['fa-chevron-right', 'ideas/', 26, '#c3cad4', '812', '#8d99a9'],
    ['fa-chevron-right', 'national_focus/', 26, '#c3cad4', '2,918', '#8d99a9'],
    ['fa-chevron-right', 'technologies/', 26, '#c3cad4', '404', '#8d99a9'],
    ['fa-chevron-right', 'units/', 26, '#c3cad4', '1,102', '#8d99a9'],
    ['fa-chevron-right', 'events/', 12, '#c3cad4', '3,981', '#8d99a9'],
    ['fa-chevron-down', 'gfx/', 12, '#c3cad4', '11,206', '#8d99a9'],
    ['fa-chevron-right', 'flags/', 26, '#c3cad4', '2,840', '#8d99a9'],
    ['fa-chevron-right', 'interface/', 26, '#c3cad4', '6,112', '#8d99a9'],
    ['fa-chevron-right', 'portraits/', 26, '#c3cad4', '2,254', '#8d99a9'],
    ['fa-chevron-right', 'interface/', 12, '#c3cad4', '918', '#8d99a9'],
    ['fa-chevron-right', 'localisation/', 12, '#c3cad4', '4,102', '#8d99a9'],
    ['fa-chevron-right', 'map/', 12, '#c3cad4', '305', '#8d99a9'],
    ['', 'descriptor.mod', 12, '#8d99a9', '', '#8d99a9'],
];

export const fileTree = fileTreeRaw.map((f) => ({icon: f[0], name: f[1], pad: f[2], c: f[3], badge: f[4], badgeC: f[5]}));

export const losesTo = [
    {name: 'Road to 56', n: '412 files', c: '#d4574e', w: '86%'},
    {name: 'Expert AI 4.0', n: '61 files', c: '#e0a340', w: '13%'},
    {name: 'Coloured Buttons', n: '8 files', c: '#5b8fc9', w: '2%'},
];

export const byDomain = [
    {path: 'common/', n: '318', c: '#d4574e'},
    {path: 'gfx/', n: '92', c: '#e0a340'},
    {path: 'interface/', n: '41', c: '#e0a340'},
    {path: 'localisation/', n: '30', c: '#e0a340'},
    {path: 'events/', n: '0', c: '#8d99a9'},
    {path: 'map/', n: '0', c: '#8d99a9'},
];

export const changelog = [
    {ver: '0.26.2', date: '8 Sep 2026', tag: 'INSTALLED', tagC: '#5fae7e', body: 'Naval rework for the Entente powers, 41 bug fixes, and a new Bharatiya Commune focus branch.', files: '+1,204 -318 files · common/, events/', edge: '#c4623a'},
    {ver: '0.26.1', date: '22 Aug 2026', tag: '', tagC: '#6a7484', body: 'Hotfix for a crash on the 1936 start when playing as Germany with the economic subsystem enabled.', files: '+12 -4 files · common/', edge: '#33404f'},
    {ver: '0.26', date: '3 Aug 2026', tag: '', tagC: '#6a7484', body: 'Major release. Rewritten economy, 6 new focus trees, updated map for the Central American theatre.', files: '+8,911 -2,402 files · all domains', edge: '#33404f'},
    {ver: '0.25.4', date: '14 Jun 2026', tag: '', tagC: '#6a7484', body: 'Compatibility pass for game patch 1.16.', files: '+204 -188 files · common/, interface/', edge: '#33404f'},
];

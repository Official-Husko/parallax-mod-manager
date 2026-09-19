// Static demo content ported from mockup/Mod Manager.dc.html for the
// screens that don't have real backend data behind them yet (Library, DLC,
// Settings, the conflict resolver, and the various dialogs). Numbers and
// names are the mockup's own placeholder example content, kept verbatim -
// this is a visual preview of where those screens are headed, not real
// user data. Every "Chancellery" from the source mockup reads "Parallax
// Mod Manager" here, and every em dash is a hyphen, per this project's
// house style.

export const APP_NAME = 'Parallax Mod Manager';

// --- Settings: manage games (3c) + sort rules (4c) ---

export const settingsNav = [
    {label: 'Manage games', key: 'manage'},
    {label: 'Paths & folders', key: 'paths'},
    {label: 'Launch options', key: 'launch'},
    {label: 'Playsets', key: 'playsets'},
    {label: 'Sort rules', key: 'sort'},
    {label: 'Conflict scanning', key: 'conflict'},
    {label: 'Updates', key: 'updates'},
    {label: 'Appearance', key: 'appearance'},
    {label: 'Advanced', key: 'advanced'},
];

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

// --- First-run wizard (3f) ---

export const wizardSteps = [
    {n: 1, label: 'Find games'},
    {n: 2, label: 'Mod folders'},
    {n: 3, label: 'Import playsets'},
    {n: 4, label: 'Preferences'},
];


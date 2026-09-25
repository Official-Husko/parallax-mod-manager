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
    {label: 'Launch options', key: 'launch'},
    {label: 'Playsets', key: 'playsets'},
    {label: 'Sort rules', key: 'sort'},
    {label: 'Conflict rules', key: 'conflict'},
    {label: 'Updates', key: 'updates'},
    {label: 'Steam API', key: 'steam'},
    {label: 'Tools', key: 'tools'},
    {label: 'Browse', key: 'browse'},
    {label: 'Backup', key: 'backup'},
    {label: 'Appearance', key: 'appearance'},
    {label: 'Advanced', key: 'advanced'},
    {label: 'Debug', key: 'debug'},
    {label: 'About', key: 'about'},
];

export const domains = ['C', 'E', 'G', 'I', 'L', 'M'];

// --- First-run wizard (3f) ---

export const wizardSteps = [
    {n: 1, label: 'Find games'},
    {n: 2, label: 'Mod folders'},
    {n: 3, label: 'Import playsets'},
    {n: 4, label: 'Preferences'},
];


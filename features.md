# Features

What Parallax Mod Manager can do today, one line each. This is the at-a-glance list; the
[README](README.md) has the detailed progress notes and the "not yet built" list, and [docs/](docs/)
explains how the underlying Paradox modding concepts work.

Kept current: every feature that lands gets a line here in the same commit, and anything worth naming
for security, privacy, reliability or performance is called out (see the last two sections).

## Games

- **Six registered games** - Stellaris, Crusader Kings III, Europa Universalis IV, Hearts of Iron IV,
  Imperator: Rome and Victoria 3, from a data-driven list you can override with a file, no rebuild
  (only Stellaris is verified against a real install so far).
- **First-run wizard** - detects which registered games are really installed and how many mods each
  has, lets you pick the ones to manage, and lets you point it at an install it missed.
- **Manage games and paths** - per-game install folder (change, reset to auto-detect, open in the
  file manager), extra mod folders searched recursively, and a per-game "managed by Parallax" switch.
- **Installed game version** - shown next to the game name, and used to flag mods built for another
  version.
- **Game update notice** - notices when a game updated (at startup and when the window regains focus)
  and tells you once.

## Mods and scanning

- **Mod discovery** - reads classic `descriptor.mod` and the Paradox Launcher's JSON descriptors, and
  classifies each mod as Steam Workshop, Paradox Launcher or local.
- **Workshop mods before the game links them** - finds subscribed Workshop items that have no stub yet
  and writes the missing stub when you launch with one enabled.
- **Live folder watching** - the mod list updates as mods are added or removed, without losing the
  load order you are building.
- **Mod detail panel** - real description, declared game version and dependencies, file tree, the mods
  it conflicts with, thumbnail, open-folder and Workshop page buttons.
- **Library** - every managed game's mods in one searchable table with sizes, plus cross-game
  collections and bulk "add to playset" / "move to collection".
- **Mod update tracking** - on each startup, what was updated, changed on disk, removed, or deleted
  from the Workshop since last time, with a card in the sidebar and a review window.

## Playsets and load order

- **Playsets** - named, ordered mod selections you can save, rename, delete and reload, with per-game
  options for what opens at startup (nothing, the last one, or a pinned one).
- **Unsaved changes are hard to miss** - the Save button blinks while the load order differs from the saved playset
  (a mod added, removed or moved), stops when you save, and holds steady instead if your system asks for less motion.
- **Drag and drop** - between Available and Active, and to reorder, with multi-select and a live
  insertion line.
- **Autosort** - fixes and utility mods to the end, and every mod after its declared dependencies,
  with dependency cycles detected and reported.
- **Pre-flight checks** - missing mods, hard conflicts and dependency problems, computed from your
  real load order before you launch.
- **Import from the Paradox Launcher** - reads the launcher's own playsets, read-only, and loads one as
  an unsaved draft.
- **Per-mod problem flags** - version mismatch, hard conflict and dependency issue each have their own
  icon and color, with a rich tooltip listing exactly what is wrong.

## Conflicts and patch mods

- **Conflict detection** - real load-order winners per content type, with dependency-aware suppression
  and same-mod duplicates handled the way the game does.
- **Conflict resolver** - winner and losers side by side with syntax highlighting and a line diff,
  an overlap matrix, and a two-step preview-then-apply flow that stays fast on huge modlists.
- **Manual winner per conflict** - pick a different winner for one contested key and reset it later.
- **Generated patch mod** - writes a real mod with the winning text of every conflict copied byte for
  byte, loaded last, including localisation.
- **Patch staleness detection** - tells you when a source mod changed and the patch is out of date,
  with the reasons and a one-click regenerate.
- **Live conflict counts** - the sidebar counts only the mods in the load order you are editing.

## DLC

- **DLC toggling per playset** - disable specific installed DLC, with type filter, sizes and Steam
  Store details (never prices).
- **Every DLC Steam lists** - including ones you do not have installed or that are unreleased, shown
  honestly as not installed.
- **Stale references shown, not dropped** - a disabled DLC that is no longer found is listed with a
  one-click clear.

## Launching

- **Launch with the playset active** - writes the game's own load files and starts the game through
  Steam, or directly for non-Steam installs.
- **Parallax Direct launch mode** - opt in per game to start the real executable and skip the
  launcher.
- **Play without a playset** - launching with nothing loaded touches no state and just starts the game.
- **Stop playing** - a two-step stop button that works however the game was started.
- **Live game log** - a filterable, colored window on the game's own `error.log` and other logs while
  it runs.

## Steam Workshop data

- **Workshop details** - subscriber, favorite and view counts, last-updated time and the author's
  description for Workshop mods.
- **Author profiles** - real author name, avatar and profile link in the mod list and detail panel.
- **Recent update notes** - the mod's own changelog entries, on demand.
- **Optional Steam API key** - add your own Steam Web API key under Settings > Steam API to also see the
  mods the free API cannot return (unlisted ones, for example): Complete use (recommended), Backup use
  only when the free API cannot answer, or Free API use only (the default, which keeps no key). If the
  key runs out of requests the app falls back to the free API by itself.
- **Unlisted, private and deleted mods flagged** - Workshop mods that are unlisted (teal), private (pink) or
  deleted (orange) get their own icon and color in the Available and Active lists and a notice on the mod, with a
  hover that says how the app knows.
- **Steam status codes understood** - Steam's own result codes are read properly, so a busy Steam or a
  rate limit is never mistaken for a mod being deleted, and unlisted mods are shown as such.

## Mod preservation

- **Backups before Steam removes a mod** - when a Workshop mod is found deleted or private, its files are copied
  1:1 into your backup folder while they are still on disk, and a background check every 30 minutes catches mods
  deleted while the app is open. Steam cannot be asked to wait, so it copies in time rather than holding the deletion.
- **Choose what is backed up** - deleted and private mods (recommended), every Workshop mod (asks first and shows the
  size and free space), or off; a single mod can always be backed up from its right-click menu.
- **Your own backup folder** - by default a "Parallax Mod Backups" folder inside the app's settings folder, one folder
  per game, changeable in Settings > Backup. Existing backups stay where they are.
- **Size limit and free-space guard** - cap the total size of all backups (MB, GB or TB) and keep some free space on
  the backup drive (1 GB by default, switchable). When one is hit, backups pause, deleted and private mods go first, and a
  red notification stays until you answer it with OK or Review (which opens Settings > Backup).
- **Free up space** - delete backups of mods still installed and available on the Workshop, or (not recommended) of mods
  that are deleted or private, each with a preview and a confirmation; single backups can be deleted from the list.
- **Safe copies** - a copy replaces an older one only when complete, mods that lose files while being copied are
  marked incomplete, and a full disk or an overlapping folder is refused with a message.
- **Backup status on the mod** - flagged mods say whether their copy is saved (and when), partial, in progress,
  impossible because Steam already removed the files, or off, and a notification announces each finished copy.

## Appearance and interface

- **Rotating per-game backgrounds** - cross-fading art behind the whole UI, which you can turn off,
  freeze, or slow down.
- **Online or offline backgrounds** - stream them from GitHub, or download the ones you want once and
  work offline, with a progress window that can be stopped and resumed.
- **Notifications for slow work** - a stacking toast system with progress bars and retry actions.
- **Themed controls and rich tooltips** - the app's own dropdowns, right-click menus, and tooltips
  instead of the webview's defaults.
- **Views stay alive** - switching between Workspace, Library and DLC keeps unsaved edits and running
  fetches.

## Diagnostics and licence

- **Activity log** - a live, colored, filterable log of what the app is doing, also kept in a rotating
  file for bug reports.
- **Start-up report in the log** - the top of every log says which build and which computer it is about (operating
  system, session, webview, CPU, memory, graphics card and driver), plus the app's folders, Steam, key settings and the
  games found, so a bug report needs less asking. It stays pinned at the top of the log however long the run gets.
- **About page** - version, commit, platform and where your settings, cache and logs live, with
  buttons to open them.
- **Licence in the app** - the full non-commercial licence, readable inside the About page.

## Performance

- **Incremental parsing cache** - unchanged mods cost almost nothing on the next launch; a corrupt
  cache entry just means that one mod is read again.
- **Fast warm rescans** - about 0.3 s instead of 2.3 s on an 86-mod install, with results identical to
  the slow path.
- **Mod list appears at once** - names, versions and sources show in about a millisecond, before the
  slower conflict pass finishes.
- **Parallel where it is safe** - files are parsed on all cores, and merged in load order so the result
  never depends on timing.
- **Small executable** - the artwork is not bundled, so the app is no longer a roughly 400 MB download
  (the interface itself is 2.5 MB).

## Security and privacy

- **No account, no telemetry** - the only outside services are Steam (Workshop and Store data) and, for
  online backgrounds only, GitHub. The About page itself makes no network requests.
- **Backups stay on your computer** - mod backups are plain folders in a location you choose and are never sent anywhere.
- **Steam API key kept encrypted** - if you enter a Steam Web API key (optional), it is checked with
  Steam before it is saved, encrypted with a key derived from your computer so the settings file is
  useless on any other machine, never shown again, never written to the activity log, and deleted from
  the file when you choose Free API use only.
- **The start-up report is anonymous** - it never includes your computer or user name, serial numbers, addresses, a Steam
  key or account id, or mod names, and it stays in your local log.
- **Nothing is uploaded** - the activity log stays on your computer.
- **Your data is written safely** - settings, playsets and caches are written to a temporary file and
  renamed into place, so a crash never leaves a half-written file; a playset that fails to load is an
  error, never a silent reset to empty.
- **Game files are handled carefully** - the Paradox Launcher's database is opened read-only, the game's
  own registry files are merged rather than overwritten, and launch state is only ever written to an
  explicit folder.
- **Careful process handling** - the game is recognised by its executable's exact name, never by a
  fragment of a command line, so stopping it can never end an unrelated program.
- **Downloads are verified** - background images are written to a temporary file and only kept when the
  size matches, and the app serves offline images only from its own image folders.

# Parallax Mod Manager

A mod manager for Paradox Interactive grand strategy games (Stellaris, EU4, HOI4, CK3,
Victoria 3, and friends) - a from-scratch reimplementation built on a Go backend instead of
.NET/Avalonia, and specifically designed to fix performance problems well known in this space
(see [Why not just use Irony?](#why-not-just-use-irony)). See
[docs/performance-strategy.md](docs/performance-strategy.md) for the research behind that
claim, and [docs/](docs/) generally for the Paradox-modding domain knowledge this project is
built on.

**Status: early development.** The desktop shell runs a real scan end to end: pick a game, list
its installed mods, arrange an enabled/ordered playset, save it, resolve real conflicts (with
real load-order winners and a real overlap matrix), and launch the game with that playset
active. Six Paradox games are registered (data-driven, not hardcoded - see
[Progress](#progress)), though only Stellaris has actually been verified against a real install
so far. A first-run wizard detects installed games for real and lets you point it at one
auto-detection misses. DLC toggling per playset is real too. The rest of the app's screens (a
cross-game library, playset sharing) exist as a faithful visual preview of where this is headed,
but aren't functional yet - see [Progress](#progress) for exactly which parts are real and which
are still a mockup. Nothing here is ready to fully replace your existing mod manager yet.

## Why not just use Irony?

Irony works, and this project owes its domain knowledge to reading its source. But it has
real, documented problems: 5–7 minute load times on large mod lists, memory that the
maintainer has confirmed can hit ~6GB with 200+ mods "by design," and several long-open
GitHub issues where the app hangs indefinitely with no way to tell if it's still working or
stuck. The root cause, found by reading Irony's own code
([docs/performance-strategy.md](docs/performance-strategy.md)): it has **zero cross-run
caching for mod files** - every launch fully re-parses every enabled mod from scratch, with
concurrency hardcoded to 4–6 mods regardless of how many CPU cores are available.

| | Irony Mod Manager | Parallax Mod Manager |
|---|---|---|
| Cross-run mod caching | None - full re-parse every launch | Stat → hash → parse layered cache; unchanged mods cost near-zero on relaunch |
| Parsing parallelism | Hardcoded cap (4–6 mods), regardless of CPU count | Worker pool sized to `runtime.NumCPU()` |
| Content hashing | Fast non-cryptographic (MetroHash) - already reasonable | Same idea (xxHash), plus a single chokepoint package (`internal/xhash`) so it can't accidentally be swapped for something disk-unsafe |
| Conflict detection | O(n) hash-bucket grouping - already reasonable | Same approach, plus dependency-aware suppression and a same-mod cross-file collapse Irony's design doesn't need to distinguish (see `internal/conflict`) |
| Cache integrity | One confirmed real bug: bad state trusted silently, corrupting the cache | Versioned format, fails closed to "re-parse this one mod" on any corruption or version mismatch - never propagates bad state |
| Progress feedback | Phase-level; several GitHub issues report indefinite, indistinguishable-from-hung scans | The mod list itself (names, versions, sources) needs no content parsing, so it renders immediately instead of waiting behind a "Scanning..." wall - confirmed on a real 86-mod install, the list appears in ~1ms versus ~6s for the full conflict-resolved result |
| Runtime | .NET + Avalonia | Go + Wails (native webview, no bundled runtime) |

This list grows as features land - see [Progress](#progress) below, which is kept current.

## Stack

- **Backend**: Go, via [Wails v2](https://wails.io).
- **Frontend**: TypeScript + Preact + Vite, under `frontend/`.
- **Reference material**: a read-only clone of an existing open-source Paradox mod manager
  lives locally at `reference-src/` for research purposes only - it's git-ignored and not part
  of this codebase. See [CLAUDE.md](CLAUDE.md).

## Progress

### Done

- **Wails desktop shell** - `main.go`/`app.go` at the repo root, Preact+TS+Vite frontend,
  builds to a native binary via `wails build`.
- **Clausewitz script parser** (`internal/script`) - tokenizer + recursive-descent parser
  for the `key = value` / nested `{ }` format almost all Paradox game/mod content is written
  in, including `@variable` definitions/references and comparison operators.
- **Localization parser** (`internal/locale`) - the separate `.yml` format.
- **Mod descriptor parsing** (`internal/mod`) - both the classic Clausewitz `descriptor.mod`
  format and the newer Paradox Launcher JSON format (v1 and v2 relationship shapes); source
  classification (Steam Workshop / Paradox Launcher / local) by filename convention.
- **Steam Workshop resolution** (`internal/steam`) - parses Steam's own
  `libraryfolders.vdf` to find the right Workshop content folder across multiple libraries, and
  across multiple Steam installation roots (a native install and a Flatpak one, say) - every
  root is tried, not just the first.
- **Per-game configuration** (`internal/game`) - executable discovery via the Paradox
  Launcher's own `launcher-settings.json`, with a fallback (its exact location varies per game -
  most keep it at the install root, some nest it under a `launcher/` subfolder). The user-data
  directory is OS-aware (Linux uses `$XDG_DATA_HOME`, not a Windows-style Documents folder),
  confirmed against a real Linux Stellaris install - which also confirmed Stellaris ships a
  native Linux binary, no Proton involved. See the data-driven game registry entry below for
  where each game's configuration actually comes from now.
- **Mod scanner** (`internal/scan`) - discovers and classifies every mod in a game's user
  mod folder; a bad descriptor is a non-fatal per-mod error, not an aborted scan. Also
  independently discovers subscribed Steam Workshop items the game hasn't linked into that
  folder yet: confirmed on a real install, a subscribed item's content can be fully downloaded
  with no `mod/ugc_<id>.mod` stub for it at all (Steam/the Paradox Launcher appear to write that
  file lazily, not the moment a subscription finishes) - since every Workshop item's own content
  folder already carries its own self-contained descriptor, the scanner reads that directly
  instead of waiting for a stub to exist. When such a mod is actually enabled in a playset,
  launching writes the missing stub (mirroring the item's own descriptor, never overwriting one
  Steam or the launcher already wrote) so the game can actually find it - see
  [docs/mod-sources.md](docs/mod-sources.md).
- **Incremental cache** (`internal/cache`) - the stat → hash → parse layered, on-disk,
  versioned cache described above. This is the project's core performance thesis, and it's
  the one piece the research found nothing surveyed in this space (not the reference codebase,
  not the Rust `tiger` validators, not CWTools) has actually built. Persisted as `gob`, not
  JSON: a real large Workshop mod's cache file (confirmed on a ~1,800-file total conversion)
  reached 165MB as JSON, and just *loading* it back - before any actual parsing - cost over half
  a second on every call, undermining the whole point of a 100%-cache-hit relaunch. Gob measured
  roughly half the file size and 3-5x faster to encode/decode on that same real data; see
  [docs/performance-strategy.md](docs/performance-strategy.md) for the full before/after.
- **Fast warm rescans** (`internal/pipeline`, `internal/conflict`, `internal/library`) - a rescan
  (every scan, manual pick, patch generation or mod-folder change) with every cache file warm went
  from about 2.3 s to about 0.3 s on this project's real 86-mod install (now 1.27 million definitions
  from 81 mods): unchanged mod caches are no longer rewritten on every run, conflict detection only
  examines keys more than one mod defines instead of resolving all of them, the index is built in
  parallel shards, mods are read several at a time (results still merged in load order), and the cache
  uses a compact binary record that stores each file's repeated strings once (bounds-checked, so a
  corrupt cache is simply rebuilt). Each change has a test proving the result is identical to the slow
  path. See
  [docs/performance-strategy.md](docs/performance-strategy.md) for the profile and what's left.
- **Localisation parsing fixes** (`internal/locale`, `internal/cache`) - the activity log's new cache
  line showed 645 files on the real install being silently skipped as unparseable. 629 had a `#`
  comment after a value (valid in the game's own files), one had a doubled BOM, and a few dozen had one
  malformed line that cost the whole file; all three are fixed (a bad line is now skipped and noted, the
  rest of the file is kept, as the game does). About a thousand localisation conflicts had been hidden:
  contested keys on that install went from 4,788 to 5,841. Every mod cache is stamped with a parser
  version (`cache.ParserVersion`), so a fixed parser gets a fresh look at files it once rejected instead
  of trusting a stale "failed" verdict.
- **Patch status no longer cries wolf at startup** (`internal/library`) - the first scan of every
  launch has no playset yet, so nothing is loaded and nothing conflicts; every key the generated
  patch covers then looked "no longer conflicting", and a persistent "patch is out of date" warning
  was raised until the real scan replaced it. With no mods loaded there is nothing to compare the
  patch to, so it is now reported as present and left unjudged. (The activity log's own repeated
  status line is what exposed it: the same warning appeared on 86 of 771 lines.)
- **Parsing pipeline** (`internal/pipeline`) - wires the cache and parsers together behind a
  bounded worker pool; parses one mod's files in parallel, merges results in deterministic
  order (parse in parallel, merge sequentially - load-order correctness, not just speed), and
  reports per-file progress.
- **Conflict detection** (`internal/conflict`) - indexes every enabled mod's parsed
  definitions by `(Type, ID)`, tells genuine conflicts apart from duplicated content via
  content-hash comparison, and resolves each genuine conflict to a winner via a per-Type
  LIOS/FIOS load-order rule. Two things beyond the base algorithm: dependency-aware
  suppression (a declared mod dependency can explain away what would otherwise be a
  conflict, without ever letting a partially-explained 3+-way conflict go silently
  unreported), and correctly collapsing a *single* mod's own same-key duplicates across its
  own files before any cross-mod comparison happens - the Clausewitz engine's "later file
  wins" doesn't care whether the two files belong to the same mod or different ones, so
  detection has to apply the same rule either way. Pure, no file I/O - see
  [docs/conflict-resolution.md](docs/conflict-resolution.md).
- **Game launching, including real `mods_registry.json`/`game_data.json` UUID tracking**
  (`internal/launch`) - writes `dlc_load.json` (the ordered enabled-mods list + disabled-DLC
  list a classic-descriptor game reads on startup), confirmed byte-for-byte against a real
  Stellaris install, then launches the game via the Steam protocol URL (falling back to a
  direct executable for non-Steam installs). `game_data.json`'s `modsOrder` field holds mod
  UUIDs from `mods_registry.json`, a different identifier space than `dlc_load.json` entirely -
  this project now maintains its own UUID per mod there too, reusing an existing entry's UUID
  (matched by its `gameRegistryId`, the same `"mod/<id>.mod"` string `dlc_load.json` uses) when
  one's already registered rather than minting a fresh one every launch, so the UUID a mod gets
  stays stable across launches. Both `mods_registry.json` and `game_data.json` are read-merge-
  write, not a full overwrite like `dlc_load.json` - entries/fields this project didn't touch
  (another mod's registry entry, `game_data.json`'s own `isEulaAccepted` flag) round-trip
  completely untouched rather than getting silently reset or dropped. The one function in this
  package allowed to actually open a URL or spawn a process sits behind a small `Launcher`
  interface - nothing else in the package, and nothing in its test suite, can touch the real OS.
  Writing state requires an explicit target directory with **no fallback to a real path** -
  stricter than `internal/scan`'s override, because this package writes rather than reads. See
  [docs/game-launching.md](docs/game-launching.md).
- **Stop playing, and the game's own logs live** (`internal/gameproc`, `internal/gamelog`,
  `Workspace.tsx`, `GameLogModal.tsx`) - while the game is running the **Play** button
  becomes a red **Stop playing** button, however the game was started (from here, from Steam, from the
  Paradox Launcher), and it turns back the moment the game closes. A first press only asks ("click
  again to stop", it lapses after a few seconds), since ending a game loses whatever it hadn't
  saved; the second asks the game to close and forces it if it hasn't after five seconds. The game
  is recognised by its own executable's name, never by a substring of a command line (a file
  manager with the game's mod folder open is not the game, and would otherwise have been killed);
  a launcher script such as Hearts of Iron IV's `run_hoi4` is followed to the real program it starts,
  so stopping ends both. Under Proton the same names with `.exe` are matched too. The button under
  Play, formerly a disabled "Export log", is now **View log**: a live window on the game's own log
  files (`error.log` first, since that is where broken mods show up), with the last lines at once and
  new ones as the game writes them, filterable by warnings or errors and by text, with a dot showing
  whether the game is running. The follower copes with what these files actually do: the game
  truncates them on every start (the view resets), can append megabytes in a second (only the newest
  is read, and the skip is noted), and writes a line in pieces (a line appears only once complete).
  Process listing and stopping is written for Linux (`/proc`), Windows and macOS; it has been run
  against real processes on Linux only. See [docs/game-launching.md](docs/game-launching.md).
- **Domain bars with the letter cut out** (`Workspace.tsx`, `Workspace.css`) - each row of the Active
  load order shows six bars, one per content folder (C common, E events, G gfx, I interface,
  L localisation, M map), coloured by what really happens to that mod there: red when everything it
  contests is lost, amber for a mix, quiet grey when it wins or isn't contested. The letter is stenciled
  into the bar itself instead of being printed separately in the column header: each bar is a CSS mask
  with the letter as a hole, so the row's own background (hover, selected) shows through it. The glyphs
  are drawn as SVG shapes rather than font text, so they look the same on every platform.
- **Launcher bypass, opt-in per game** (Settings' Launch Options panel, `launch.LaunchMode`) -
  since mod/playset activation already happens entirely through this project's own state writes
  above, the Paradox Launcher has nothing left to do; "Parallax Direct" mode resolves and starts
  a game's real executable straight from its own `launcher-settings.json`, skipping the launcher
  (and Steam) entirely. Configured one game at a time - each game gets its own choice between the
  normal Steam / Paradox Launcher path (the default) and Parallax Direct, since Steam
  integration (overlay, achievements, DLC ownership checks) isn't guaranteed to carry over to a
  directly-spawned process on every game. Fixed a real path-resolution bug along the way:
  `launcher-settings.json`'s own `exePath` is relative to *its own* directory, not the install
  root - true for Stellaris/EU4/HOI4, where the two are the same, but CK3, Imperator: Rome, and
  Victoria 3 nest it under a `launcher/` subfolder, so the previous `installDir`-relative join
  pointed outside the install for those three. A second bypass strategy - replacing the
  launcher's own binary with a small shim so Steam keeps owning the process, shown in the UI
  today as "Steam Direct (Recommended)" - is designed for (`LaunchMode` is a named string, not a
  bool, specifically so it can be added later) but not yet built; once it is, it's intended to
  become the default, since it keeps Steam integration Parallax Direct can't promise on every
  game. See [docs/game-launching.md](docs/game-launching.md).
- **Play never depends on a playset being loaded** (`app.go`'s `LaunchGame`) - launching with no
  playset name selected skips every one of this project's own state writes (`dlc_load.json`,
  `mods_registry.json`, `game_data.json`) entirely and starts the game against whatever's already
  on disk untouched, rather than refusing to launch or inventing an empty state of its own. Paired
  with a new **Playsets** settings panel (renamed from the former "Playset sharing" placeholder
  entry) that configures, one game at a time, whether Workspace automatically reloads a playset
  when you open that game: off (always start blank), autoload whichever one was last saved or
  switched to (the default), or always autoload one specific pinned playset regardless of what
  else you've used since. Playset sharing itself (export/import as a shareable code) now lives as
  its own row inside that same panel rather than a top-level tab, still unbuilt - see
  [Not yet built](#not-yet-built).
- **Importing existing Paradox Launcher playsets** (`internal/launcherdb`) - reads a game's real
  `launcher-v2.sqlite`, strictly read-only (confirmed schema against real, live databases for two
  different classic-descriptor games on this machine, not just a secondary source - see
  [docs/launcher-database.md](docs/launcher-database.md)), and lists every playset it holds with
  its own mods in real load-order position. The Workspace's own Playsets switcher shows whatever's
  found under a "From the Paradox Launcher" section; importing one loads it as the current,
  unsaved draft - the same "build it, then explicitly Save" flow "New" already uses, not an
  immediate write into this project's own playset storage - resolving each mod back to this
  project's own id and reporting (never silently dropping) anything not currently found by this
  project's own scan. A fixture test proves the read-only connection actually rejects a write,
  not just documents the intent as a comment.
- **Wired UI** (`internal/library`, `app.go`, `frontend/src/app.tsx`) - a real scan view: pick a
  game, and `ScanGame` runs the full scan → parse → conflict-resolve chain and renders the mod
  list and any conflicts. `internal/library` owns the orchestration and the frontend-facing
  summary types; `app.go` stays a thin adapter (per `CLAUDE.md`) that resolves the one real OS
  path (`os.UserCacheDir()`) this layer needs. Two boundary pitfalls worth knowing about if
  extending this: Wails maps Go's `uint64`/`int64` straight to a JS `number`, which silently
  loses precision past 2^53 - so summary types never carry a raw hash field, only what the UI
  actually needs; and Go `iota` enums (`mod.Source`, etc.) are converted to strings
  (`"workshop"`, not `1`) before crossing the boundary, since a bare int means nothing in JS.
  Verified against the installed Wails v2 source, not assumed. `ScanGame` also emits a
  `scan-quick` event partway through its own run: a mod's name/version/source needs no content
  parsing, so the full mod list is known - and pushed to the frontend - well before conflict
  detection's slower per-mod parsing finishes, letting the Workspace view render the list
  immediately instead of sitting behind a blocking "Scanning..." state (`internal/library`'s
  `LoadGame` takes an `OnQuickSummary` callback for this; `app.go` is the only thing that turns
  it into a Wails event). Confirmed against a real 86-mod install: the list appears in about a
  millisecond, roughly 5000× before the fully conflict-resolved result lands a few seconds later.
- **Playsets and game launching** (`internal/playset`, `internal/library`, `app.go`,
  `frontend/src/app.tsx`) - named, ordered mod selections persisted as versioned JSON
  (Paradox's own "playset" term, not a generic "collection"), following the same irreplaceable-
  user-data philosophy as `internal/launch`'s state writes: a corrupt or version-mismatched
  playset file errors on load rather than silently resetting to empty. A mod left out of a
  playset's order is skipped by `internal/library` before parsing even starts, not just
  filtered out afterward - disabling mods is a real scan-time performance win, not just a
  smaller conflict set. Saved playsets can be **renamed and deleted** from the Playsets dialog:
  a rename keeps the mods, order and DLC choices exactly, refuses a name that's empty or already taken
  (judged on the file the name would land in, since names are sanitized into file names, so it can
  never overwrite another playset), writes the new one before removing the old so a failure can't lose
  it, and repoints the settings that remember a playset by name (last active, pinned for auto-load);
  deleting one clears those settings, and never throws away the load order on screen. The app also
  **notices when a game updates**: it remembers each game's last-seen version, checks at startup and
  whenever the window regains focus (Steam may have updated it in the background), and raises a
  persistent notification once per update - and a generated patch made for another game major.minor
  is flagged as out of date. The Workspace UI (available/active load-order lists, a mod detail
  panel, and an actions rail with the playset picker and the launch button) adopts the visual
  design and terminology from `mockup/Mod Manager.dc.html`, a local design reference kept out
  of version control; that mockup also sketches a larger product (a cross-game library, DLC
  management, a file-level conflict resolver, playset sharing, an update checker), most of which
  now exists as a static visual preview (the conflict resolver is a real, working exception -
  see below) - see the entries below for exactly what's real.
  `app.go`'s `LaunchGame` is the one function in the whole
  codebase that opens Steam and starts the real game process; `internal/atomicfile` now holds
  the shared atomic-JSON-write helper this package, `internal/cache`, and `internal/launch` all
  need, extracted once a third consumer showed up.
- **Data-driven, UUID-keyed game registry** (`internal/game`, `internal/jsonc`,
  `internal/gamemedia`, `data/games.jsonc`) - every registered game's configuration (Steam App
  ID, descriptor format, scan folders, launcher-settings path, and so on) lives in a JSONC file
  (JSON with comments and trailing commas - this project's house format for its own on-disk
  data, hand-rolled with no new dependency) rather than Go literals, loaded through the same
  embed-plus-live-override pattern as game art: a compiled-in default that a real file dropped
  in the user's config directory can override with no rebuild, gated on a
  `required_parallax_version` field so an incompatible override falls back safely (with a
  visible in-app notice) instead of risking a schema the running app might not understand. Each
  game's identity is a permanent UUID rather than a short slug, since it's what playset storage,
  the mod-parsing cache, and the frontend all address a game by. Six games are registered
  (Stellaris, Crusader Kings III, Europa Universalis IV, Hearts of Iron IV, Imperator: Rome,
  Victoria 3); real install detection reads Steam's own `appmanifest_<id>.acf` (the authoritative
  install-folder name Steam itself uses - never guessed from a display name), confirmed against
  a real `appmanifest_281990.acf` on a real Stellaris install. Only Stellaris has actually been
  verified end to end against a real installed copy; the other five were seeded from public
  Paradox game data and haven't been tested against a real install of any of them yet.
- **First-run wizard** (`frontend/src/views/FirstRunWizard.tsx`, `App.DetectGames`,
  `App.BrowseForGameInstall`/`BrowseForAnyGameInstall`) - real, not a mockup replica: it detects
  which registered games are actually installed and how many mods each already has, lets you
  pick which ones Parallax Mod Manager should manage, shows real saved playsets per game, opens
  a native folder picker (verified against the game's real signature files, never trusted on
  say-so) when auto-detection misses a game or finds the wrong copy, and its last step sets the
  same real preferences described below (not a static preview of them).
- **Live mod-folder watching and real preferences** (`internal/watch`, `internal/preferences`,
  `Workspace.tsx`, `Settings.tsx`) - the Workspace's mod list watches the current game's mod
  folder in the background (`fsnotify`, debounced so a bulk copy or archive extract collapses
  into one refresh instead of several) and updates the moment a mod is added or removed,
  without discarding the load order you've already built or Available-list selections you
  haven't added yet - a refresh only prunes mods that no longer exist, via Wails' event bridge
  (`EventsEmit`/`EventsOn`) rather than polling. The app's own writes (generating the patch,
  purging mods) are muted, since it already refreshes the list itself when they finish; a watcher
  restarts only when the scanning setting is toggled, not on every settings save; and if the
  operating system's event queue overflows (so changes were lost) that counts as a change. Three real preferences persist across restarts
  (JSONC, `internal/preferences`): scanning for new mods (gates the watcher above), closing the
  manager after a successful launch, and remembering the last game you managed so it's
  preselected next time. "Warn on patch mismatch" is stored alongside them but stays an
  explicit placeholder - this project has no concept yet of a mod's compatible game version to
  warn about.
- **Real mod detail panel** (`internal/library`'s `ListModFiles`/`ModFolderPath`, `app.go`,
  `Workspace.tsx`) - the Workspace's per-mod detail view reads the mod's actual descriptor and
  on-disk content instead of showing mockup placeholder text: its declared supported-game-version
  and dependency list (with a best-effort found/not-found check against the other scanned mods,
  by name - the only thing a plain dependency-name string can be matched against), a real
  Files tab (walks the mod's real content folder, capped at 2,000 entries so a huge total-
  conversion mod doesn't hang the tree view, with total size and last-modified time computed
  from the same walk), a real Conflicts tab (which other mods this one's genuine conflicts
  involve, from the same conflict detection Workspace already runs - no fabricated file counts
  or percentages this project has no way to back), and a working "Open folder" button plus a
  real Steam Workshop page link for Workshop mods (built from the descriptor's own
  `remote_file_id`, opened via `github.com/pkg/browser` - already this project's Steam-launch
  mechanism, not a new network dependency). A classic-format mod's descriptor has no description
  field at all (see docs/paradox-mod-format.md), so that section honestly says so rather than
  showing invented text; the Changes tab does the same for update history, since that's Steam
  Workshop's own metadata and this project doesn't fetch anything from the network. The panel's
  thumbnail is a real image too (`internal/library.ModThumbnail`), resolved the same way real
  Stellaris Workshop mods actually ship one: the classic descriptor's own `picture` field first
  (now parsed - `internal/mod`'s classic descriptor parser previously dropped it), falling back
  to a file literally named `thumbnail.png` in the mod's content root, since Steam Workshop
  writes one there even when a mod's descriptor never declares a `picture` field at all -
  confirmed against several real mods on this machine that only work with the fallback in place.
  A mod with genuinely no usable image (a non-web format like a Paradox-native `.dds` texture,
  or a descriptor pointing at a file that isn't actually there) honestly shows no thumbnail
  rather than a broken image or a guess. Confirmed on this project's own real 86-mod Stellaris
  install: 80 resolve a real thumbnail, 6 genuinely have none.
- **Real conflict resolver** (`internal/library`'s `ConflictCandidate`/`Winner`/`ReadModFile`,
  `ConflictResolver.tsx`) - lists every genuine conflict Workspace's own conflict detection
  already found, real load-order-ranked candidates with the actual computed winner (LIOS/FIOS,
  the same rule `conflict.Resolve` itself applies - no separate guess), and the winning and
  losing files' real content side by side, with real syntax highlighting (`frontend/src/data/
  highlight.ts`) for both editable formats this project's mods actually contain - Clausewitz
  script and Paradox locale `.yml` - tokenized by the exact same rules this project's own real
  parsers use (`internal/script/lexer.go`, `internal/locale/locale.go`), not a separate guessed
  grammar, so a key, string, number, comment, `@variable`, or `yes`/`no` literal is colored
  exactly as this project's own scanner would classify it, with a real line-level diff
  (`frontend/src/data/lineDiff.ts`) tinting the lines only one side has. Switching to another
  contested key is instant however big the modlist is: the clicked key highlights and the
  contender cards update immediately, both side-by-side panes show a spinner with a short
  status line ("Reading the file from disk...", "Comparing the two files...") while the files
  load and the diff is computed off the critical path, and only the rows and lines actually on
  screen are ever in the DOM (`frontend/src/data/useVirtualWindow.ts`). Choosing a
  different winner is a two-step, reviewable action: clicking a contender (or a radio) only
  previews it, showing its file next to the current winner's with the real line diff of what it
  would replace, and nothing is saved until you press Apply, which locks the window behind an
  "Applying... please wait" overlay until the rescan that makes it official comes back. In a synthetic
  benchmark of 12,000 contested keys and 6,000-line files, a click went from a window frozen
  for seconds to a ~3 ms render. On the Go side, `ReadModFile` looks a mod's folder up in a cache filled by the
  last scan (`internal/library.ContentPathCache`) instead of re-scanning the whole game for
  every file it reads. The overlap matrix is computed
  client-side from that same real conflict data (which mod pairs share the most contested keys), capped to the 30
  most-contested mods so the grid stays fast and legible against a large real modlist rather than
  rendering every mod that touches at least one conflict. "Auto-resolve all" is real (it clears
  every manual pick so the load order decides again), and "Generate patch" (below) writes the
  resolutions out. The mockup's two other resolution ideas survive only as clearly disabled
  entries in the Resolution card, each with a design note instead of code - see "Not yet built"
  below.
- **Real patch-mod generation** (`internal/library.GeneratePatch`, `docs/patch-mods.md`) -
  writes a real, generated mod that pins down the winning definition's *exact original source
  bytes* (via `definition.Span`, never a re-serialized parse tree) for every genuine conflict,
  named to sort after everything else so it actually takes effect. Researching this surfaced a
  real, previously-unknown gap in this project's own conflict detection: Stellaris merges files
  across mods by ASCIIbetical filename first, falling back to mod load order only when two
  files share an identical name - confirmed against Paradox's own modding wiki and a second
  independent source - so the manager's predicted winner and the real game's actual winner can
  diverge whenever two conflicting mods don't happen to share a filename. A generated patch mod
  fixes this outright rather than working around it, since its own file names are entirely
  under this project's control. Localization conflicts are patched too, not just classic script
  ones - a localisation `Type` gets a real `.yml` file with its own language header and UTF-8
  BOM, matching what real Paradox locale files carry. Confirmed on this project's own real
  86-mod Stellaris install: all 5,841 conflicts patched with byte-exact content, zero skipped,
  including 1,595 localization entries across 10 real languages (209 of them carrying a trailing
  `# comment`, kept verbatim), and every generated file parses back with nothing lost. Classic-descriptor
  games only; regenerated from a clean slate on every call so a resolved-then-later-removed
  conflict never leaves a stale override behind.
  The patch is a **proper mod**: a `descriptor.mod` inside its folder plus the registering stub in
  the game's mod folder (name, a version that moves every regeneration, a `supported_version`
  derived from the installed game, tags, and a picture), with the built-in placeholder thumbnail
  (`data/patch_thumbnail.png`, overridable by a `patch_thumbnail.png` in the config folder) written
  into every new patch. It also carries a **manifest** (`internal/patchmanifest`) recording, for
  every patched key, a content hash of each candidate's version and which mod's text was copied, so
  a scan can tell when the patch has gone stale - a source mod updated, another mod started
  defining the key, a source mod is gone, a different winner was chosen, or a new conflict
  appeared. Stale keys get an **orange line** in the Conflict Resolver with the reasons spelled out,
  a banner offers **Regenerate patch**, and the Workspace raises a persistent notification. The
  comparison is on content, not version numbers or file times, so an update that never touched a
  patched key doesn't flag anything. Checked against this project's real 86-mod install: 5,841
  conflicts patched and then all reported current. The generated patch is no longer counted as a
  competitor in the conflicts it resolves. Its descriptor also lists every loaded mod as a
  dependency (by name, in load order), so the launcher loads it after all of them, and Autosort
  keeps it at the very end - a setting under **Settings > Advanced** ("Keep the generated patch
  last", on by default) lets you turn that off and have Autosort leave it where you put it.
  A real **per-conflict manual override** (`internal/patchoverride`) lets a user pick a specific
  different winner for one contested key, instead of the automatic load-order rule - a radio
  next to each candidate in the Conflict Resolver plus a "Keep load-order winner" option to reset
  it, both staged and previewed before an explicit Apply. One override file per game (matching patch generation's own one-per-game scope,
  for the same reason), degrading gracefully like `internal/preferences` rather than erroring like
  `internal/playset` - low-stakes, re-derivable data where a missing or corrupt file just means
  every conflict falls back to its automatic winner. A stale override (the chosen mod removed,
  disabled, or never a real candidate for that key) is silently ignored rather than erroring, the
  same safe fallback a missing override already has. Both the Conflict Resolver's own display and
  `GeneratePatch`'s actual byte-copying read the exact same computed winner, so what a user sees
  win is always what gets patched - never two separate calculations that could drift apart.
- **About page and activity log** (`Settings > About`, `frontend/src/views/About.tsx`,
  `internal/about`, `internal/applog`) - the app's version, the commit it was built from, Go and
  Wails versions, platform and where its settings, cache and log live (with buttons to open them),
  drawn as GitHub-style badges and Font Awesome Pro icons for the link buttons. The badges come from
  the real build details rather than a badge service, so the page makes no network requests, and the
  links and credit line are fixed in the code, not a configurable file. Below them is a live,
  colored **activity log** of what the app is doing - one line per notable step in the same shape as
  Go's own logger (`2026/07/06 00:09:37 [Scan] 'Stellaris': 86 mods, 4788 conflicts (2261ms)`),
  tagged by the part of the app that did it, with quoted names and numbers highlighted and each
  step's duration colored from quiet green through amber to red. It follows new lines as they
  happen, stops following the moment you scroll up, and can be filtered by level, component or text,
  copied out as text, or cleared (the file keeps its lines). Scans (with how well the cache worked
  and how long conflict detection took), patch generation, launches (state written, Steam or
  direct), playsets and collections, game detection and install or extra mod folders, the
  mod-folder watcher, every Steam lookup and background DLC refresh (whose failures used to be
  swallowed silently), Autosort, which settings each save changed, and any uncaught error in the UI
  itself all log to it. A mod file the parsers couldn't fully read is named with its mod and the
  reason (once, when it is first seen or changes - not on every scan), and the generated patch's
  status is logged when it changes rather than on every rescan. Every line is also
  appended to a rotating file (`app.log`, at most about 4 MB across the current and previous file)
  in the cache folder, so there's something to attach to a bug report. The logger is a small
  reusable package - see [docs/logging.md](docs/logging.md) - meant to be used more and more as the
  project grows.
- **Real autosort** (`frontend/src/data/autosort.ts`, Workspace's Autosort button, Settings'
  "Sort rules" panel) - two real, derivable rules, adapted from a proven design (a working
  sibling Stellaris mod-sorting tool on this machine, cross-checked against its own real-world
  tag/dependency conventions): a mod tagged Fixes, Utilities, or Patch moves to the end of the
  load order (the same convention this app's own generated patch mod already follows), and a
  mod moves to load right after every dependency it declares, matched by name against the
  other currently enabled mods, via a stable topological sort (Kahn's algorithm, the same
  class of algorithm the sibling tool itself uses) - efficient by construction (bounded by the
  mod count, not a repeated-rescan heuristic's worst case) and able to actually detect and
  report a genuine dependency cycle instead of silently giving up on it. Dependencies are
  applied last so a tag-based move can never leave one violated. Both rules are individually
  toggleable in Settings, persisted via
  `internal/preferences`. Confirmed meaningful on this project's own real 86-mod Stellaris
  install: 14 mods have genuine declared dependencies (mostly UI Overhaul Dynamic and Planetary
  Diversity submods needing their base mod first) and 17 are tagged for late placement - this
  isn't just correct in theory, it does real, useful work on a real modlist. In-memory only
  (reorders the current load order; the user still has to Save the playset), so there's nothing
  destructive to undo it - close without saving.
- **Real pre-flight checks** (`frontend/src/data/preflight.ts`, the actions rail's PRE-FLIGHT
  section, and the "Ready to launch?" dialog) - both used to show the exact same five hardcoded
  mockup rows (fake mod names like "Road to 56", a fake checksum-matches-your-friends line)
  regardless of what was actually active or actually conflicting. Now genuinely computed from
  data already in memory: real scan errors (or "All N mods present"), the real conflict count
  from the same detection the Conflict Resolver uses, and a real declared-dependency check
  (missing or loading in the wrong order among the currently active mods, the same name-matching
  Autosort's own dependency rule already uses). Two of the mockup's five original rows have no
  honest replacement and were dropped rather than faked: detecting the game's own currently-
  installed version (for "N mods target an older patch") and an online checksum-sharing feature
  (for "matches your friends") don't exist anywhere in this project. The rail's UPDATES card
  similarly no longer shows a fabricated count - the real update checker itself remains a
  separate, not-yet-built feature (see below).
- **Real DLC toggling** (`internal/dlc`, `Dlc.tsx`) - lets a user disable specific installed
  DLC for a saved playset. The write path was already real and already launched
  (`internal/launch` has written `dlc_load.json`'s `disabled_dlcs` field since playsets shipped);
  the missing piece was discovering what DLC actually exists to toggle. Confirmed against a real
  Stellaris install: each DLC is its own folder under `<InstallDir>/dlc/`, holding one `.dlc`
  metadata file in the same plain Clausewitz format a classic descriptor uses - 35 real DLC
  entries discovered this way, spanning every real category (`expansion`, `story_pack`,
  `species_pack`, `content_pack`). Classic-descriptor games only, matching every other
  classic-only precedent in this codebase - see [docs/game-launching.md](docs/game-launching.md).
  Editing DLC for a playset is a deliberately separate, simpler flow from Workspace's own live,
  not-yet-saved editing session: pick a saved playset, toggle, save - and Workspace itself no
  longer silently wipes a playset's DLC choices on its own next save, a real bug this closes
  (it previously always wrote an empty `disabled_dlcs` list on save, regardless of what was
  really there). "Profile" was the mockup's own name for this same saved-load-order-plus-DLC
  concept; this project only ever calls it a **playset**, so every "Profile" label in the UI
  (the top bar included) now says that instead.
- **Real Steam Workshop and Store data** (`internal/steamapi`, `internal/dlcstore`) - the first
  and only place in this project that calls out to a third party over the network; everything
  else works entirely from local files. Two confirmed, unauthenticated public endpoints (see
  [docs/steam-web-api.md](docs/steam-web-api.md) for exact request/response shapes and the real
  gotchas found - `file_size` arriving as a JSON string, descriptions being BBCode, the Store's
  `appdetails` silently returning `null` instead of an error for a batched multi-id request):
  - **Workshop item metadata** (`GetPublishedFileDetails`) powers the mod detail panel's Changes
    tab for Workshop mods - real subscriber/favorite/view counts, last-updated time, and the
    author's own real description, closing a gap classic descriptors can't fill on their own
    (they have no description field at all). Every currently-scanned Workshop mod's id is
    batched into a single request, fetched once, and kept in memory for the app's own runtime
    (`library.WorkshopDetailsCache`) - confirmed against this project's real 86-mod Stellaris
    install: 81 real Workshop mods enriched in one request.
  - **Author details** (Steam Community's own profile XML endpoint) shows a Workshop mod's real
    author name, avatar, and a clickable link to their real profile right in the Changes tab, the
    Workspace's Available list, and next to the detail panel's own "updated x days ago" line -
    confirmed against a real author's public profile during development. Distinct creator ids
    are derived from whatever Workshop metadata is already in memory (no second Workshop fetch
    triggered), deduplicated across mods sharing an author, and fetched once per id for the
    app's runtime (`library.AuthorProfileCache`). A failed lookup (private-in-an-unusual-way
    profile, bad id) is simply absent, not an error - confirmed against Steam's own real
    different-shaped response for an invalid id. Both this fetch and the Workshop metadata fetch
    it depends on run automatically in the background the moment a game's mod scan produces a
    list, for every mod the scan finds - not lazily per mod selection. A real bug here (found and
    fixed): both fetch effects used to gate their own re-entry on the very loading-state value
    they set, which made Preact tear down and cancel each fetch via its own cleanup within
    milliseconds of starting it - every single time, regardless of network speed - so this data
    could never actually finish loading. Confirmed with an isolated Preact+hooks reproduction
    before and after the fix; the effects now use a plain ref for re-entry guarding instead,
    which isn't part of the render/dependency system Preact reacts to.
  - **Recent update notes**, in the same Changes tab - the mod's real changelog, since classic
    descriptors have no version-history field of their own to read. There's no public API for
    this one; `internal/steamapi.GetChangelog` reads it straight from the item's own real
    changelog page HTML (the only place in this project that parses HTML rather than a
    documented JSON/XML response), scoped to its 10 most recent entries. A real, confirmed quirk
    in that page's own markup (an invalid `<div>` nested inside a `<p>`, which a spec-compliant
    parser auto-closes empty exactly like a browser would) shapes how the real body text is
    recovered - see [docs/steam-web-api.md](docs/steam-web-api.md) for the full finding. Fetched
    per mod, on demand, the first time that mod's own Changes tab is opened, not batched across
    every scanned mod - there's no batching endpoint for a page fetch.
  - **Store listing details** (`appdetails`) powers the DLC screen's detail panel (header image,
    release year, short description - deliberately never price: this only ever fetches Store
    data for DLC the user already has installed, so a price has no use here). Persisted per game
    with a timestamp and refreshed at most once a day (`internal/dlcstore`, `dlcstore.MaxAge`) -
    a call always returns whatever's cached immediately, even if stale, and kicks off a
    background refresh (never more than one per game at once) when the cache is missing or too
    old, emitting an event once fresher data actually lands rather than blocking on a live Steam
    round-trip. Bridges the two different DLC identifier spaces (a Store app id vs.
    `disabled_dlcs`' local folder name) via each DLC's own real `steam_id` field, confirmed
    present in every real `.dlc` file checked.
  - **Every DLC Steam knows about for the game, not just what's installed** - a real, honest
    answer to "how do we handle DLC the user doesn't own?" Steam ownership can't actually be
    determined from local files for most Paradox DLC: a real Stellaris install's own
    `appmanifest_<appid>.acf` tracks only 2 of 35 real local DLC folders as separately-installed
    depots (both free items) - the other 33, full paid expansions included, ship to every owner
    of the base game regardless of which DLC they specifically bought. A "not owned" badge built
    from local signals would be wrong most of the time. Instead, `internal/dlcstore.Refresh`
    fetches the base game's own official Steam catalog (`appdetails`' `dlc: []` field - 32 real
    entries for Stellaris, including unreleased content flagged `coming_soon`) and shows every
    entry not already installed as a real, honestly-labeled **NOT INSTALLED** row - real name
    and header image, never toggleable, since there's no local folder to actually enable. A
    genuinely unreleased ("Coming soon") DLC gets real detail too, not just a name: Steam's own
    `coming_soon` flag replaces what would otherwise be a confusing blank release date with an
    honest "Coming soon" label, and its real screenshot gallery (Steam's own preview images,
    click to open full-size) shows in the detail panel - the only real content this project has
    for something with no local footprint to read anything else from.
  - **A real type filter and per-pack size**, matching the design mockup's own DLC screen
    layout: each installed DLC's real `category` field (`internal/dlc.Entry.Category`, e.g.
    `species_pack`) groups the list into a filterable sidebar with real counts, shown as
    "Species Pack" rather than the raw slug. Size is real disk usage of the DLC's own folder
    (`internal/dlc`'s own directory walk, not Steam - the Store API has no size field to fetch)
    - confirmed meaningfully varied on a real Stellaris install: from ~76KB for a free bonus
    pack up to ~110MB for a full expansion, ~1.2GB total across all installed DLC. The mockup's
    own "required by mods" and multiplayer-checksum fields aren't shown - Paradox mods don't
    declare DLC dependencies anywhere this project can parse.
  - **A playset's disabled DLC that's no longer found locally is shown, not silently dropped** -
    a real, easy-to-hit case (removing a DLC folder, or a drive disconnecting, reproduces it
    exactly): the id stays in the playset's real `disabledDlc` list either way, so it's shown as
    its own **NOT FOUND** row (dimmed, no working toggle - nothing to verify) with a one-click
    way to clear the stale reference, plus a footer count so it's never just invisible.
- **The rest of the design mockup's screens** (`frontend/src/views/Library.tsx`,
  `PlaysetsModal.tsx`, `UpdatesModal.tsx`) - a faithful, fully navigable visual preview of the
  mockup's cross-game library, built from the mockup's own example content.
  **Mostly static previews, not working features yet** - they render real Font Awesome Pro
  icons and this project's dark theme, but (with five exceptions) don't read or write real
  data. The exceptions: the Workspace screen's actual mod list, load order, conflict detection,
  playset save/load, launch, mod detail panel, conflict resolver, and autosort are the real,
  working features described above (including the mod detail panel's own real thumbnail art -
  see below); the DLC screen described above; Settings' "Manage games" panel shows the same
  real per-game detection as the first-run wizard, with a working "set path" for anything not
  auto-detected and a per-game "managed by Parallax" toggle - the same choice made in the
  wizard, kept in sync with it (toggling a game here takes effect immediately, live, without a
  restart) rather than the wizard being the only place that choice could ever be made. Its
  "Paths & folders" panel is the fuller per-game path reference, styled to match "Manage games"
  (the same swatch/name/path row) and, like it, a list of per-game cards that start collapsed
  (install path and quick actions only) so registering six-plus games doesn't turn the panel
  into a wall of detail nobody asked to see - expand a card for its mod folder path and extra
  mod folders. Each game's install folder has change/reset-to-auto-detect and "open in file
  manager" actions, backed by the same path-override persistence the wizard's own "Browse..."
  already used - a manually-picked install path now survives a restart instead of needing to be
  re-picked every launch. It also manages a per-game list of **extra mod folders** - any
  additional location (a shared network drive, a manually curated collection) searched
  recursively for more mods alongside the game's own managed mod folder, two ways at once
  (`internal/scan.ScanExtraFolder` and the stale-path fallback below): a self-contained mod
  folder (its own `descriptor.mod` inside, the same convention Steam Workshop content uses) is
  discovered as a new mod outright, and separately, a mod whose *existing* descriptor in the
  game's own mod folder points at a path that no longer exists (the library moved to a new
  drive or folder without the stub being updated) gets reconnected if any extra folder turns
  out to have a same-named subfolder - so pointing Parallax Mod Manager at wherever the mods
  really live now can repair a mod that previously showed up broken/empty, not just add
  entirely new ones. The three real preference toggles described above live on the "Manage
  games" panel, and its "Sort rules" panel is the real autosort configuration described
  above; and the
  Library screen is real, including its own "collections" and bulk actions now, not just the
  games sidebar and mod table - it scans every managed game and lists its actual mods (name,
  source, version, real on-disk size - computed in parallel across every mod at once,
  `internal/library.ModSizes`, confirmed under 70ms for 81 real mods totaling 47GB), searchable
  and filterable by game or by collection, with the two columns this project genuinely can't
  compute yet (last played - no launch history is tracked; a mod's "state" - would need a full
  conflict-resolve pass per game, too expensive to run just for a browsing view) honestly shown
  as `-` rather than invented. **Collections** (`internal/collection`) are a real, persisted,
  cross-game concept distinct from playsets - a user-named, unordered grouping of mods from any
  managed game (a "graphics" bucket, a "for my campaign" bucket), stored the same versioned-JSON
  way playsets are, just without the per-game/load-order scoping a playset needs. **Bulk
  actions** are real too: checkbox multi-select across the table, "Add to playset" (only enabled
  when the selection is all one game, since a playset can't span games - loads the target
  playset, merges in the new mod ids, saves) and "Move to collection" (adds the selection to the
  target collection, and genuinely removes them from the currently-viewed source collection when
  one's active and different from the target - a real move, not just an add) via a small
  click-away picker that can create a new playset/collection inline instead of only picking an
  existing one. "Uninstall" stays a deliberately inert placeholder for now - this project's only
  existing mod-deletion path (`PurgeMods`) is narrowly scoped to already-empty local mods on
  purpose, and a general "delete a mod's real content" action is a materially bigger,
  higher-stakes capability than what shipped here, not something to add as a side effect of
  wiring up the rest of the row. Everything else in this list - playset sharing via codes and the
  update checker - stays a static preview.
- **A real, stacking notification system** (`frontend/src/data/notifications.ts`,
  `NotificationStack.tsx`) - replaces the single ad-hoc startup-notice banner with a proper
  toast stack anchored right below the top bar, so it never covers the app's own title or game
  picker. Multiple notifications stack; info/success ones auto-dismiss after 5 seconds, errors
  and in-progress ones stay until resolved or dismissed by hand. A `progress` kind carries a real
  (or indeterminate, when there's genuinely nothing to measure) progress bar and can be updated
  in place as work continues - used for the Workshop/author-profile background fetch described
  above, so a user sees "Fetching Steam Workshop mod details..." turn into a real success or
  error message (with a working Retry action) rather than wondering whether anything is
  happening at all. A plain module-level store, not a new state-management dependency - any
  component calls `notify()`/`updateNotification()`/`dismiss()` directly; one `<NotificationStack/>`
  in `app.tsx` renders whatever's currently active.
  Every action outcome in the app now goes through it (saving, loading and launching a playset,
  scans, DLC saves, patch generation, conflict overrides, purging, the first-run wizard) via a
  small `trackTask()` helper that shows a progress toast and settles it into a success or error in
  place; the old per-view status lines and banners are gone. The stack draws above modal dialogs, so
  a result raised from inside one is never hidden behind its backdrop.
- **A real custom right-click menu, app-wide** (`frontend/src/data/contextMenu.ts`,
  `ContextMenu.tsx`) - the webview's own native context menu (reload, inspect element, and the
  like - not meaningful chrome for a packaged desktop app) is suppressed everywhere via a single
  app-wide listener in `app.tsx`; anywhere that doesn't open a real menu of its own just shows no
  menu at all on right-click, rather than ever falling back to the browser default. Wired up with
  real, working actions (not a placeholder) on Workspace's Available/Active mod rows - reorder,
  remove, add to load order, open the mod's real folder, open its real Workshop page, copy its
  ID - and on the DLC screen's rows - enable/disable, open the real Steam Store page, clear a
  stale disabled-DLC reference - all of it reusing actions that already existed elsewhere in the
  UI, not new backend surface. The menu clamps itself on-screen (measured against its own real
  rendered size, not a guessed width) so it never opens off the edge of the window, and closes on
  an outside click, Escape, scroll, or the window losing focus - the same behavior a native
  context menu itself has. Same plain module-level store pattern as the notification system
  above, for the same reason (no new state-management dependency).
- **Real OS-file-list-style row selection on Workspace's mod rows** (`frontend/src/data/dragMultiSelect.ts`),
  shared identically by the Available and Active lists - click a row to select just it, or
  shift+click to extend from the last-clicked row through this one (inclusive, by list position),
  exactly like Explorer/Finder's own list view (either replaces the prior selection rather than
  toggling it). `preventDefault()` on the row's own mousedown is what actually stops the browser's
  native text-selection drag from kicking in instead during a shift+click - CSS `user-select: none`
  alone wasn't enough on its own. A mousedown-and-drag across rows used to extend the selection the
  same way, but a plain click/shift+click-only model turned out to be the right split: dragging a
  row now always means "move it" (below), so the two could no longer share the same gesture without
  one reading as a bug - dragging to reposition a row was instead silently reselecting every row the
  cursor crossed.
- **Real drag-and-drop between Available and Active, and within either list on its own**
  (`frontend/src/data/listDragMove.ts`), matching how Mod Organizer 2, Vortex, and RimSort/RimPy
  handle their own active/plugin list: drag a row (or the current multi-selection - dragging one
  of several already-selected rows drags the whole selection, not just the one under the cursor)
  from Available into Active to activate it at exactly the dropped position, drag an Active row
  back into Available to deactivate it at exactly the dropped position, or drag a row within either
  list to reorder it there - every case gets the identical live insertion line tracking the nearest
  row boundary, regardless of which list the drag started in or is headed to. A small ghost preview
  follows the cursor throughout, once the mouse has moved a few real pixels (never on a plain
  click): a single dragged mod shows its real name, a multi-selection names each one individually
  (capped at five, with a "+N more" line - not just a bare count) alongside an arrow/reorder icon
  reflecting where it would actually land. Active's own reordering changes the real, persisted load
  order; Available's is a temporary, session-local arrangement only (it has no saved order of its
  own - `availableOrder` in `Workspace.tsx`, reconciled against whatever mods are actually available
  whenever that set changes, dropping ones that left and appending new ones in their natural sort
  order while leaving everything else exactly where the user dragged it).
- **Real, currently-installed game version detection and mod version-compatibility flagging**
  (`internal/game.GameConfig.GameVersion`, `frontend/src/data/versionCompat.ts`) - the TopBar's
  own game pill now shows the real version (e.g. `v4.4.6`) read from the Paradox Launcher's own
  `launcher-settings.json`, as its own distinct segment next to the game name - a compound pill
  (name, version, the switcher chevron, each its own segment divided by a plain border, only the
  name carrying a background) matching the design mockup's own topbar exactly, not just loose
  inline text; nothing shown at all when a version can't be determined, never a placeholder. The
  switcher dropdown shows every managed game's own real version the same way, not just the
  currently-selected one's, and gained a real "Manage games" entry, always reachable (even
  managing only one game, not gated behind already having more than one to switch between) that
  jumps straight to Settings' own "Manage games" panel. The real version also drives a real
  compatibility check against every mod's own declared `supported_version` (confirmed wildcard
  format - see [docs/paradox-mod-format.md](docs/paradox-mod-format.md)): a mod confirmed
  incompatible with the installed version is colored amber (this project's own color language
  reserves red for hard conflicts only) in both the Available/Active lists' version column and
  the detail panel's own "Supports" row, which used to always show a flat, meaningless green
  regardless of whether that was actually true.
- **Distinct per-mod problem flags** (`frontend/src/data/flags.ts`, `--flag-*` tokens in `App.css`) -
  the Active list's FLAGS column, the detail panel, the pre-flight list and the Autosort dialogs
  now draw the three problems the same way everywhere, each with its own Font Awesome Pro icon
  and hue so they read apart at a glance and without relying on color alone: a version mismatch
  is an amber code-compare, a hard conflict a red burst, and a dependency issue a violet broken
  link (it used to share amber with version warnings and a generic triangle). An ignored version
  warning drops to a quiet grey.
  Hovering any of them opens a **rich tooltip** (`components/Tooltip.tsx`, `data/tooltip.ts`) instead
  of a plain native one: every entry is drawn the way it looks in the list (its colored icon, or the
  mask-cut domain bar, on a wash of its own color) with the explanation next to it - a mod's flags
  list the exact contested-key count and the mods it clashes with, or the dependencies that are
  missing or loading too late; the FLAGS and DOMAINS headers show a legend of every flag and color.
- **Every top-level view stays mounted once visited, instead of unmounting on navigation** - a
  real bug found while chasing why the Workshop/author fetch above never seemed to finish:
  `app.tsx` used to fully unmount a view (`{view === 'x' && <X/>}`) the instant the user
  navigated away from it, tearing down its component state and cancelling any in-flight request
  every single time - so switching to the DLC page and back, for instance, could restart
  Workspace's whole background fetch from zero before it ever got the chance to complete. The
  same unmount was also silently discarding real unsaved work (an in-progress load-order edit in
  Workspace, an unsaved DLC toggle) the moment the user navigated elsewhere. Views now switch via
  CSS (`display: contents` when active, `display: none` otherwise) instead of a real conditional
  render, and only mount lazily on first visit - except Library, which stays lazy even then,
  since its own first load kicks off a real scan across every managed game at once and
  shouldn't run on every app launch for a session that never opens it.
- **A rotating, per-game background image behind the whole app** - drawn from whichever
  game is currently selected (`frontend/src/assets/game_media/background/<gameID>/`; only
  Stellaris ships any art so far, a game with none just shows the plain flat color it always
  had), picking a random image and cross-fading to a new random one on a timer, slightly
  darkened so foreground text stays legible. Every major panel/header/sidebar surface now
  reads its background through a semi-transparent CSS variable instead of a flat opaque
  color, so the art actually shows through the whole UI rather than just the gaps between
  panels. Settings' own "Appearance" panel lets you turn it off entirely, freeze it on
  whichever image is currently showing instead of rotating, or change how often it rotates
  (default every 5 minutes).

All of the above has unit test coverage (table-driven, fixture-based, `go test -race`
clean), including tests that prove behavior rather than just assert on it - e.g.
`internal/pipeline`'s `TestLoadModSecondRunReusesCacheWithoutReparsing` (corrupts a file's
on-disk content while forcing an identical stat, and confirms the cache still short-circuits
the reparse), `internal/conflict`'s `TestDetectKeySameModCrossFileDuplicateCollapsedByRule`
(confirms LIOS vs. FIOS visibly pick different files for the same mod's own duplicate key),
and `internal/launch`'s `TestWriteStateReusesSameUUIDAcrossLaunches` (confirms a mod's real
`mods_registry.json` UUID survives two separate `WriteState` calls unchanged, rather than
churning a fresh one every launch) and `TestWriteStateUpdatesGameDataPreservingUnknownFields`
(confirms a pre-existing `game_data.json`'s `isEulaAccepted` flag survives a `WriteState` call
that legitimately does rewrite the file's `modsOrder`).

### Not yet built

- **JSON-launcher-format games** (Victoria 3, EU5) - `content_load.json`/`playsets.json`'s
  schemas are still unconfirmed against a real install of a game that uses them; Victoria 3 is
  registered with a best-guess descriptor variant flagged as such in `data/games.jsonc`, and EU5
  isn't registered at all yet (it's unreleased, and even public reference data for it is
  incomplete).
- **Five of the six registered games are unverified** - Crusader Kings III, Europa Universalis
  IV, Hearts of Iron IV, and Imperator: Rome are registered from public Paradox game data, not
  confirmed against a real install of any of them the way Stellaris has been.
- **Remote games-list updates** - `data/games.jsonc` supports a live on-disk override already
  (see above), but nothing fetches an update from its own `source` URL yet; that's a deliberate
  follow-up, not an oversight (see `internal/game.LoadRegistry`'s doc comment).
- **Merge patch** - the Conflict Resolver's disabled "Generate merge patch" option: a resolution
  that combines content from more than one candidate instead of picking a single winner. No code
  yet; [docs/merge-patch.md](docs/merge-patch.md) records what it would take (byte-range
  splicing, since there's no script writer, plus a confirmed list of safe-to-combine types) and
  what must be verified against a real install first.
- **Exclude file from both** - the Conflict Resolver's disabled option for dropping a file from
  every mod that supplies it, expressed through the generated patch mod since another mod's files
  can't be edited. No code yet, and the intent behind the label was never written down;
  [docs/exclude-file.md](docs/exclude-file.md) works through the mechanisms and the open
  questions to settle before building.
- **Uploading the activity log** - the log view's **Upload** button is shown disabled. When built it
  opens a review dialog (the redacted log, exactly as it would be sent) with an opt-in "include
  computer details" option (CPU, GPU, OS, mod counts, whether a patch has been generated, and so on)
  to help with statistics and investigating bugs. Never uploads without a confirmation, never sends mod
  names or account IDs. No code behind the button yet; see [docs/log-sharing.md](docs/log-sharing.md).
- **Playset sharing via codes** - the Library screen's collections and bulk actions are now real
  (see [Progress](#progress) above); encoding/decoding a playset as a shareable local code
  ("Import code"/"Share"/"Join a friend's playset" in the Playsets modal) is still a static
  preview. No server needed for this one - it's a local-only encode/decode scheme, not
  cloud sync - just not built yet. See `mockup/Mod Manager.dc.html` (local reference file,
  git-ignored) for the full original design.

## Development

```sh
wails dev -tags webkit2_41    # hot-reload dev mode
wails build -tags webkit2_41  # production build, outputs to build/bin/
go test ./... -race           # backend test suite
```

The `webkit2_41` tag is a Linux-specific quirk (this machine ships `webkit2gtk-4.1`, not the
`4.0` Wails links against by default) - see [CLAUDE.md](CLAUDE.md) for details. It's not
needed on Windows/macOS.

## Documentation

- [CLAUDE.md](CLAUDE.md) - project conventions, layout, and contributor guidance.
- [docs/](docs/) - Paradox modding domain knowledge (file formats, conflict resolution,
  game launching, performance strategy), written from research rather than assumption.

# Parallax Mod Manager

A mod manager for Paradox Interactive grand strategy games (Stellaris, EU4, HOI4, CK3,
Victoria 3, and friends) - a from-scratch reimplementation built on a Go backend instead of
.NET/Avalonia, and specifically designed to fix performance problems well known in this space
(see [Why not just use Irony?](#why-not-just-use-irony)). See
[docs/performance-strategy.md](docs/performance-strategy.md) for the research behind that
claim, and [docs/](docs/) generally for the Paradox-modding domain knowledge this project is
built on.

**Status: early development.** The desktop shell runs a real scan end to end: pick a game, list
its installed mods, arrange an enabled/ordered playset, save it, and launch the game with that
playset active. Six Paradox games are registered (data-driven, not hardcoded - see
[Progress](#progress)), though only Stellaris has actually been verified against a real install
so far. A first-run wizard detects installed games for real and lets you point it at one
auto-detection misses. The rest of the app's screens (a cross-game library, DLC management,
settings, a conflict resolver, playset sharing) exist as a faithful visual preview of where this
is headed, but aren't functional yet - see [Progress](#progress) for exactly which parts are
real and which are still a mockup. Nothing here is ready to fully replace your existing mod
manager yet.

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
| Progress feedback | Phase-level; several GitHub issues report indefinite, indistinguishable-from-hung scans | Per-file progress from the first file scanned |
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
  `libraryfolders.vdf` to find the right Workshop content folder across multiple libraries.
- **Per-game configuration** (`internal/game`) - executable discovery via the Paradox
  Launcher's own `launcher-settings.json`, with a fallback (its exact location varies per game -
  most keep it at the install root, some nest it under a `launcher/` subfolder). The user-data
  directory is OS-aware (Linux uses `$XDG_DATA_HOME`, not a Windows-style Documents folder),
  confirmed against a real Linux Stellaris install - which also confirmed Stellaris ships a
  native Linux binary, no Proton involved. See the data-driven game registry entry below for
  where each game's configuration actually comes from now.
- **Mod scanner** (`internal/scan`) - discovers and classifies every mod in a game's user
  mod folder; a bad descriptor is a non-fatal per-mod error, not an aborted scan.
- **Incremental cache** (`internal/cache`) - the stat → hash → parse layered, on-disk,
  versioned cache described above. This is the project's core performance thesis, and it's
  the one piece the research found nothing surveyed in this space (not the reference codebase,
  not the Rust `tiger` validators, not CWTools) has actually built.
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
- **Game launching** (`internal/launch`) - writes `dlc_load.json` (the ordered enabled-mods
  list + disabled-DLC list a classic-descriptor game reads on startup), confirmed
  byte-for-byte against a real Stellaris install, then launches the game via the Steam
  protocol URL (falling back to a direct executable for non-Steam installs). Deliberately does
  *not* touch `game_data.json`: the same real install shows its `modsOrder` field holds mod
  UUIDs from `mods_registry.json`, a different identifier space than `dlc_load.json` entirely,
  and this project doesn't track those UUIDs yet - writing plain mod-ID strings into a
  UUID-keyed field would be wrong, not just incomplete, so the write path stays absent rather
  than shipping something known to write bad data. The one function in this package allowed to
  actually open a URL or spawn a process sits behind a small `Launcher` interface - nothing
  else in the package, and nothing in its test suite, can touch the real OS. Writing state
  requires an explicit target directory with **no fallback to a real path** - stricter than
  `internal/scan`'s override, because this package writes rather than reads. See
  [docs/game-launching.md](docs/game-launching.md).
- **Wired UI** (`internal/library`, `app.go`, `frontend/src/app.tsx`) - a real scan view: pick a
  game, and `ScanGame` runs the full scan → parse → conflict-resolve chain and renders the mod
  list and any conflicts. `internal/library` owns the orchestration and the frontend-facing
  summary types; `app.go` stays a thin adapter (per `CLAUDE.md`) that resolves the one real OS
  path (`os.UserCacheDir()`) this layer needs. Two boundary pitfalls worth knowing about if
  extending this: Wails maps Go's `uint64`/`int64` straight to a JS `number`, which silently
  loses precision past 2^53 - so summary types never carry a raw hash field, only what the UI
  actually needs; and Go `iota` enums (`mod.Source`, etc.) are converted to strings
  (`"workshop"`, not `1`) before crossing the boundary, since a bare int means nothing in JS.
  Verified against the installed Wails v2 source, not assumed.
- **Playsets and game launching** (`internal/playset`, `internal/library`, `app.go`,
  `frontend/src/app.tsx`) - named, ordered mod selections persisted as versioned JSON
  (Paradox's own "playset" term, not a generic "collection"), following the same irreplaceable-
  user-data philosophy as `internal/launch`'s state writes: a corrupt or version-mismatched
  playset file errors on load rather than silently resetting to empty. A mod left out of a
  playset's order is skipped by `internal/library` before parsing even starts, not just
  filtered out afterward - disabling mods is a real scan-time performance win, not just a
  smaller conflict set. The Workspace UI (available/active load-order lists, a mod detail
  panel, and an actions rail with the playset picker and the launch button) adopts the visual
  design and terminology from `mockup/Mod Manager.dc.html`, a local design reference kept out
  of version control; that mockup also sketches a larger product (a cross-game library, DLC
  management, a file-level conflict resolver, playset sharing, an update checker) most of which
  now exists as a static visual preview - see the two entries below for exactly what's real.
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
  (`EventsEmit`/`EventsOn`) rather than polling. Three real preferences persist across restarts
  (JSONC, `internal/preferences`): scanning for new mods (gates the watcher above), closing the
  manager after a successful launch, and remembering the last game you managed so it's
  preselected next time. "Warn on patch mismatch" is stored alongside them but stays an
  explicit placeholder - this project has no concept yet of a mod's compatible game version to
  warn about.
- **The rest of the design mockup's screens** (`frontend/src/views/Library.tsx`, `Dlc.tsx`,
  `Settings.tsx`, `ConflictResolver.tsx`, `PlaysetsModal.tsx`, `UpdatesModal.tsx`) - a faithful,
  fully navigable visual preview of the mockup's cross-game library, DLC management, settings
  with sort rules, file-level conflict resolver and overlap matrix, and playset picker, built
  from the mockup's own example content. **Mostly static previews, not working features yet** -
  they render real Font Awesome Pro icons and this project's dark theme, but (with three
  exceptions) don't read or write real data. The exceptions: the Workspace screen's actual mod
  list, load order, conflict detection, playset save/load, and launch are the real, working
  feature described above (its decorative bits - thumbnail art, description, the
  Files/Conflicts/Changes tabs - are static preview content like the rest); Settings'
  "Game profiles" panel shows the same real per-game detection as the first-run wizard
  (including a working "set path" for anything not auto-detected, and the three real preference
  toggles described above); and the Library screen's
  games sidebar and mod table are real too - it scans every managed game and lists its actual
  mods (name, source, version), searchable and filterable by game, with columns this project
  can't compute yet (file size, last played, a mod's "state") honestly shown as `-` rather than
  invented. Everything else in this list - Settings' "Sort rules" panel, DLC, the conflict
  resolver, playset sharing, the update checker, and Library's own "collections" and bulk
  actions - stays a static preview.

All of the above has unit test coverage (table-driven, fixture-based, `go test -race`
clean), including tests that prove behavior rather than just assert on it - e.g.
`internal/pipeline`'s `TestLoadModSecondRunReusesCacheWithoutReparsing` (corrupts a file's
on-disk content while forcing an identical stat, and confirms the cache still short-circuits
the reparse), `internal/conflict`'s `TestDetectKeySameModCrossFileDuplicateCollapsedByRule`
(confirms LIOS vs. FIOS visibly pick different files for the same mod's own duplicate key),
and `internal/launch`'s `TestWriteStateNeverWritesGameData` (confirms a pre-existing
`game_data.json` is left byte-for-byte untouched by a `WriteState` call).

### Not yet built

- **Patch-mod generation** - writing a user's resolved conflict choices out as a physical
  mod on disk, appended to the end of the load order. See
  [docs/conflict-resolution.md](docs/conflict-resolution.md)'s "Patch mods" section.
- **`mods_registry.json`-backed UUID tracking** - its schema is now confirmed (see
  [docs/game-launching.md](docs/game-launching.md)), but this project doesn't generate or
  persist mod UUIDs yet, which is what's actually blocking `game_data.json` support (above).
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
- **Real functionality behind the rest of the design mockup** - the cross-game library, DLC
  management, a settings screen with an attributable autosort rule engine, a file-level conflict
  resolver with an overlap matrix, and playset sharing via codes all exist as static visual
  previews now (see [Progress](#progress) above) but don't read or write real data yet. See
  `mockup/Mod Manager.dc.html` (local reference file, git-ignored) for the full design these are
  built from.

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

<p align="center">
  <img src="icon.png" width="120" alt="Parallax Mod Manager icon">
</p>

<h1 align="center">Parallax Mod Manager</h1>
<p align="center"><strong>A fast, from-scratch mod manager for Paradox grand strategy games.</strong></p>

<p align="center">
  <a href="https://github.com/Official-Husko/parallax-mod-manager/actions/workflows/build.yml"><img alt="Build" src="https://github.com/Official-Husko/parallax-mod-manager/actions/workflows/build.yml/badge.svg"></a>
  <a href="https://github.com/Official-Husko/parallax-mod-manager/actions/workflows/vulncheck.yml"><img alt="Vulnerabilities" src="https://github.com/Official-Husko/parallax-mod-manager/actions/workflows/vulncheck.yml/badge.svg"></a>
  <img alt="Version" src="https://img.shields.io/badge/version-1.0.0-6c5ce7">
  <img alt="Status" src="https://img.shields.io/badge/status-early%20development-orange">
  <a href="LICENCE.md"><img alt="Licence" src="https://img.shields.io/badge/licence-PMM--NCSL--1.0-blue"></a>
  <img alt="Go version" src="https://img.shields.io/badge/go-1.27-00ADD8?logo=go&amp;logoColor=white">
  <img alt="Platform" src="https://img.shields.io/badge/platform-linux%20%7C%20windows%20%7C%20macos-lightgrey">
  <img alt="Last commit" src="https://img.shields.io/github/last-commit/Official-Husko/parallax-mod-manager">
</p>

A mod manager for Paradox Interactive grand strategy games - Stellaris, Crusader Kings III,
Europa Universalis IV, Hearts of Iron IV, Imperator: Rome and Victoria 3. Built from scratch with
a Go backend instead of the .NET stack the rest of this space runs on, specifically to fix the
slow scans and heavy memory use existing mod managers are known for - see the
[comparison below](#why-not-just-use-irony) for the receipts.

**Status: early development.** The core workflow already runs end to end: pick a game, build and
save a load order, resolve real conflicts, and launch with a click - along with a full Steam
Workshop and LoversLab browsing/install experience, DLC toggling, backups, an in-app mod editor
and publisher, and auto-translation. Stellaris is the only game confirmed against a real install
so far; the other five are registered and expected to work, just not yet double-checked one by
one. See [Roadmap](#roadmap) for exactly what is and isn't done yet.

The screenshots below are real UI, shown with sample data rather than a live install - Stellaris
is the only game verified against a real one so far.

<table>
<tr>
<td width="50%">
<img src=".github/readme/screenshots/workspace.png" alt="Workspace: an active load order, per-mod domains and flags, and pre-flight checks before launch">
<br><sub>Workspace - build a load order, see conflicts and dependency issues before you launch</sub>
</td>
<td width="50%">
<img src=".github/readme/screenshots/conflicts.png" alt="Conflict Resolver: contested keys, a side-by-side diff, and patch generation">
<br><sub>Conflict Resolver - side-by-side diffs and one-click patch generation</sub>
</td>
</tr>
<tr>
<td width="50%">
<img src=".github/readme/screenshots/dlc.png" alt="DLC screen: per-playset DLC toggling with Steam Store details">
<br><sub>DLC - toggle installed DLC per playset, with Steam Store details</sub>
</td>
<td width="50%">
<img src=".github/readme/screenshots/editor.png" alt="Editor: descriptor and thumbnail editing with a live diff preview before saving">
<br><sub>Editor - edit a mod's descriptor and thumbnail, previewed before you save</sub>
</td>
</tr>
</table>

## Features

A curated tour of the highlights - see [features.md](features.md) for the complete, one-line-each
list of everything the app can do today.

**Mods, load order and conflicts**
- Finds your Steam Workshop, Paradox Launcher and local mods automatically, with live folder
  watching so the list never goes stale, and personal notes on any mod.
- Drag-and-drop load order with one-click autosort (dependencies first, fixes and patches last)
  and pre-flight checks before you hit Play.
- Real conflict detection, not guesswork - exact load-order winners, a side-by-side diff, and a
  generated patch mod carrying the winning content of every conflict, byte for byte.
- A cross-game library with search, sizes and collections for managing mods across every game you
  play, all in one place.

**Steam Workshop**
- Full Workshop details right in the app - subscriber counts, author profiles, changelogs and
  update status - without ever opening a browser.
- Publish or update your own mods straight to the Workshop, with a live upload log and progress
  bar, and open the finished page in Steam or your browser automatically.
- Backs up a mod's files the moment it is found deleted or made private on Steam, before they are
  gone for good.

**LoversLab**
- Browse, sign in and install mods from LoversLab without leaving the app - rich descriptions with
  real colours and formatting, screenshots, changelogs and comments, all in one tabbed view.
- Reply to a file's support thread, or quote another reply, right from the Comments tab.
- Automatic update checks and your real LoversLab notifications, both right in Browse's sidebar.

**DLC and multiplayer**
- Toggle installed DLC per playset, including DLC you do not own, shown honestly rather than
  hidden.
- The four-character multiplayer checksum your friends need to compare load orders, calculated
  offline the moment you save.

**Launching**
- Launches with your playset active through Steam or directly, with a live, filterable game log
  while it runs.
- Parallax Direct launches straight past the Paradox Launcher's extra window, for the games where
  that's supported.

**The Editor**
- Create a mod from scratch (or from a real starter template), edit an existing one's descriptor
  and thumbnail with a live preview, and undo your last save if you change your mind.
- Catch problems before you share a mod - files it overwrites from the base game, script errors
  with the exact line, and dependencies that don't match anything installed.
- Auto-translate a mod's own text with DeepL - one language or every language at once, official key
  or one of two free services.

**Look and feel**
- A background behind the whole interface pulled from each game's own art, rotating or static,
  your choice - or switched off entirely.
- An accent colour that matches whichever game you're managing, or pick your own.

**Trust and privacy**
- No account, no telemetry - Steam and GitHub (for background art) are the only outside services
  this app ever talks to.
- Your Steam API key, DeepL key and LoversLab sign-in are all encrypted to your own computer,
  never shown again once saved.
- Every setting, playset and cache is written atomically, so a crash never leaves a half-written
  file behind.

## Why not just use Irony?

Irony works, and this project owes its domain knowledge to reading its source. But it has
real, documented problems: 5-7 minute load times on large mod lists, memory that the
maintainer has confirmed can hit ~6GB with 200+ mods "by design," and several long-open
GitHub issues where the app hangs indefinitely with no way to tell if it's still working or
stuck. The root cause: it has **zero cross-run caching for mod files** - every launch fully
re-parses every enabled mod from scratch, with concurrency hardcoded to 4-6 mods regardless of
how many CPU cores are available.

| | Irony Mod Manager | Parallax Mod Manager |
|---|---|---|
| Cross-run mod caching | None - full re-parse every launch | Stat -> hash -> parse layered cache; unchanged mods cost near-zero on relaunch |
| Parsing parallelism | Hardcoded cap (4-6 mods), regardless of CPU count | Worker pool sized to your CPU's own core count |
| Cache integrity | One confirmed real bug: bad state trusted silently, corrupting the cache | Versioned format, fails closed to "just re-parse this one mod" on any corruption or version mismatch - never propagates bad state |
| Progress feedback | Phase-level; several GitHub issues report indefinite, indistinguishable-from-hung scans | The mod list itself needs no content parsing, so it appears in about a millisecond instead of waiting behind a "Scanning..." wall |
| Runtime | .NET + Avalonia | Go + Wails (native webview, no bundled runtime) |

This list grows as features land - see [Roadmap](#roadmap) below.

## Building from source

No pre-built downloads yet (see [Roadmap](#roadmap)) - building it yourself takes a few minutes.
The backend is Go via [Wails v2](https://wails.io); the frontend is TypeScript + Preact + Vite.

**You'll need:**
- [Go](https://go.dev/dl/) 1.27 or newer
- [Node.js](https://nodejs.org/) 18 or newer
- The Wails v2 CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Wails' own platform prerequisites (WebView2 on Windows, Xcode command line tools on macOS,
  `webkit2gtk` and friends on Linux) - see its
  [installation guide](https://wails.io/docs/gettingstarted/installation), or run `wails doctor`
  to check what's missing.

**Then:**

```sh
git clone https://github.com/Official-Husko/parallax-mod-manager.git
cd parallax-mod-manager
wails build          # production build, outputs to build/bin/
```

`wails build` installs and builds the frontend for you automatically. On Linux, if your distro
ships `webkit2gtk-4.1` instead of `4.0` (Arch and a few others do), add `-tags webkit2_41` to the
command above - `wails doctor` will tell you if this applies to you.

Want to hack on it instead of just running it? `wails dev` (same `-tags webkit2_41` caveat)
starts a hot-reload dev build.

## Roadmap

Six Paradox games are registered, but only Stellaris has been verified against a real install so
far - Crusader Kings III, Europa Universalis IV, Hearts of Iron IV, Imperator: Rome and Victoria 3
are next. Beyond that:

- Compressed, space-saving backups, and one-click restore from the interface.
- A merge-patch option for conflicts that combines more than one mod's content instead of only
  picking a winner (the engine already exists and runs for one confirmed content type).
- Sharing a playset with friends as a local code - no server involved.
- One-click log uploads for bug reports, and archiving mods Steam has deleted so they aren't lost
  for good.

See [PROGRESS.md](PROGRESS.md) for the full, detailed build log behind this list - every feature
done and not yet built, with the reasoning behind each.

## Documentation

- [features.md](features.md) - every feature the app has today, one line each, including what it
  does for your security and privacy.
- [PROGRESS.md](PROGRESS.md) - the detailed build log behind the Roadmap above.
- [LICENCE.md](LICENCE.md) - the full licence text.

## Licence

Parallax Mod Manager is released under the **Parallax Mod Manager Non-Commercial Source License 1.0**
(PMM-NCSL-1.0) - see [LICENCE.md](LICENCE.md). In short: free to use, study and modify for
non-commercial purposes; it may not be sold, paywalled or offered with paid features, though voluntary
donations are welcome; shared copies must include the source and stay under the same licence; forks need
their own distinguishing name. That is a summary - the licence text is what applies.

Official project: <https://github.com/Official-Husko/parallax-mod-manager>

## Star History

<a href="https://star-history.com/#Official-Husko/parallax-mod-manager&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Official-Husko/parallax-mod-manager&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Official-Husko/parallax-mod-manager&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Official-Husko/parallax-mod-manager&type=Date" />
  </picture>
</a>

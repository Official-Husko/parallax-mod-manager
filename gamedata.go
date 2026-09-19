package main

import "embed"

// embeddedGamesList is this build's built-in games list - see
// internal/game.LoadRegistry for how a user-supplied override on disk can
// take precedence over it, and what happens when it can't.
//
//go:embed data/games.jsonc
var embeddedGamesList []byte

// embeddedGameMedia is this build's built-in per-game art (logos,
// backgrounds) - see internal/gamemedia.Store for how an on-disk override
// directory can take precedence over it.
//
//go:embed data/game_media
var embeddedGameMedia embed.FS

// embeddedPatchThumbnail is the image every newly generated patch mod gets as
// its thumbnail - see internal/library.GeneratePatch. A patch_thumbnail.png
// dropped in the app's config folder takes precedence (see App.patchThumbnail),
// the same override convention the games list and per-game art follow.
//
//go:embed data/patch_thumbnail.png
var embeddedPatchThumbnail []byte

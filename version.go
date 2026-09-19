package main

// AppName is the product's display name, shown on the About page.
const AppName = "Parallax Mod Manager"

// AppVersion gates whether a custom games.jsonc override (see
// internal/game.LoadRegistry) is trusted - a file requiring a newer
// version than this falls back to the built-in list rather than risking a
// schema it might not understand.
const AppVersion = "1.0.0"

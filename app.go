package main

import "github.com/Official-Husko/parallax-mod-manager/internal/app"

// App is the type actually bound to Wails - kept here, in package main, purely so
// Wails' generated frontend bindings keep their existing "main" namespace
// (frontend/wailsjs/go/main/App.*, the main.XXX types in models.ts). Wails names a
// binding's namespace after the Go package the bound struct is declared in, not
// after where its logic actually lives - moving the real App into internal/app
// (see that package's own doc comment for why) would otherwise silently rename
// that namespace to "app" and break every one of the ~30 frontend files that
// import from it, for a difference the frontend never needed to know about.
//
// Embedding *app.App here promotes every one of its methods onto App, so Wails'
// reflection sees them as this type's own - every field and every real method
// still lives in internal/app.App; this is nothing but the thin seam Wails'
// naming convention requires.
type App struct {
	*app.App
}

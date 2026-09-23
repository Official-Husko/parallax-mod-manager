package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/Official-Husko/parallax-mod-manager/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure. main.go is the only file left at
	// the repo root with //go:embed directives reaching into data/ (Wails' rule:
	// a directive can't reach outside its own file's directory tree) - gamedata.go
	// stays here alongside it for the same reason, and this is the one place their
	// embedded bytes get handed to internal/app, which holds everything else. See
	// this package's own app.go for why the value itself is wrapped in a
	// package-main App rather than being *app.App directly.
	a := &App{App: app.New(embeddedGamesList, embeddedLicence, embeddedBackgroundSource, embeddedGameMedia, embeddedPatchThumbnail)}

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Parallax Mod Manager",
		Width:     app.DefaultWindowWidth,
		Height:    app.DefaultWindowHeight,
		MinWidth:  app.MinWindowWidth,
		MinHeight: app.MinWindowHeight,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Answers /backgrounds/... itself (the offline background images, which
			// live in the config folder, not in the build) before anything else.
			Middleware: app.BackgroundMiddleware(a.App),
		},
		// Matches --bg-app in frontend/src/App.css, and must stay fully
		// opaque (A: 255): this is the native window's own background,
		// painted underneath the webview - if any layout bug ever lets
		// content overflow the viewport (or paints before the page's own
		// CSS is ready), a translucent value here would show whatever is
		// behind the window (the desktop, another app) through the gap
		// instead of this app's own dark background.
		BackgroundColour: &options.RGBA{R: 10, G: 13, B: 18, A: 255},
		OnStartup:        func(ctx context.Context) { app.Startup(a.App, ctx) },
		OnShutdown:       func(ctx context.Context) { app.Shutdown(a.App, ctx) },
		Bind: []interface{}{
			a,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

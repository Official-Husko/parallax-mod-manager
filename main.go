package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Parallax Mod Manager",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Answers /backgrounds/... itself (the offline background images, which
			// live in the config folder, not in the build) before anything else.
			Middleware: app.backgroundMiddleware,
		},
		// Matches --bg-app in frontend/src/App.css, and must stay fully
		// opaque (A: 255): this is the native window's own background,
		// painted underneath the webview - if any layout bug ever lets
		// content overflow the viewport (or paints before the page's own
		// CSS is ready), a translucent value here would show whatever is
		// behind the window (the desktop, another app) through the gap
		// instead of this app's own dark background.
		BackgroundColour: &options.RGBA{R: 10, G: 13, B: 18, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

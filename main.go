package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// The window's size. The smallest it can be made is what the interface was checked at: below it
// the Workspace's mod lists lose their names and the launch controls fall off the bottom. The
// side panels shrink toward it (see Workspace.css), and the rail keeps Play in view when it is
// short. 1200x640 still fits a 1080p screen at 150% display scaling. The starting size is a
// little larger, and never below the minimum.
const (
	minWindowWidth      = 1200
	minWindowHeight     = 640
	defaultWindowWidth  = 1280
	defaultWindowHeight = 700
)

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Parallax Mod Manager",
		Width:     defaultWindowWidth,
		Height:    defaultWindowHeight,
		MinWidth:  minWindowWidth,
		MinHeight: minWindowHeight,
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

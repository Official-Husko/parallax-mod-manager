package app

// The window's size. The smallest it can be made is what the interface was checked at: below it
// the Workspace's mod lists lose their names and the launch controls fall off the bottom. The
// side panels shrink toward it (see Workspace.css), and the rail keeps Play in view when it is
// short. 1200x640 still fits a 1080p screen at 150% display scaling. The starting size is a
// little larger, and never below the minimum. Exported: main.go (the only file left at the repo
// root that isn't just //go:embed directives) reads these for its own wails.Run call.
const (
	MinWindowWidth      = 1200
	MinWindowHeight     = 640
	DefaultWindowWidth  = 1280
	DefaultWindowHeight = 700
)

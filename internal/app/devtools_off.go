//go:build !dev

package app

// devBuild says whether this is a development build (wails dev, which F5 in VS Code runs): the
// only kind that has the Debug tab and the developer tools.
const devBuild = false

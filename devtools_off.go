//go:build !devtools && !dev

package main

// devtoolsBuiltIn says whether this build includes the web inspector at all: the
// devtools build tag (wails build -devtools) or a dev build (wails dev).
const devtoolsBuiltIn = false

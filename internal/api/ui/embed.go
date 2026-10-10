// Package ui embeds the built web app. `make web` writes Vite's output into
// dist/; a checkout without that build still compiles and serves a short
// page saying how to build it.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS is the built app rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Built reports whether an index.html is present.
func Built() bool {
	_, err := fs.Stat(FS(), "index.html")
	return err == nil
}

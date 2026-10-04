// Package web holds the playground's frontend build output.
//
// The React application lives in this directory as a Vite project (see
// package.json). The built bundle is embedded here with go:embed so
// decide-playground ships as a single binary with no runtime assets to install.
//
// Only dist/index.html is committed, as a placeholder, so `go build ./...`
// works for contributors without Node. Release builds run the frontend build
// first, so a released binary always embeds the real interface.
package web

import (
	"embed"
	"errors"
	"io/fs"
	"strings"
)

// assets holds the built single-page app.
//
//go:embed all:dist
var assets embed.FS

// Assets returns the built frontend rooted at dist/.
func Assets() (fs.FS, error) {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, errors.New("web: cannot open the embedded frontend: " + err.Error())
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, errors.New("web: the embedded frontend has no index.html; run `make web` and rebuild")
	}
	return sub, nil
}

// IsPlaceholder reports whether the embedded frontend is the committed
// placeholder rather than a real build. decide-playground warns when it is.
func IsPlaceholder() bool {
	data, err := fs.ReadFile(assets, "dist/index.html")
	if err != nil {
		return true
	}
	// A real Vite build always references its hashed bundle from index.html.
	return !strings.Contains(string(data), "/assets/")
}

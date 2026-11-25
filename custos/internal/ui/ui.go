// Package ui embeds the web interface: static HTML, CSS and plain
// JavaScript in separate files (no inline scripts, so the CSP can forbid
// them), no framework, no CDN, no build step.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed www
var files embed.FS

// Handler serves the UI.
func Handler() http.Handler {
	sub, _ := fs.Sub(files, "www")
	return http.FileServer(http.FS(sub))
}

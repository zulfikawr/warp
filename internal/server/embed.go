package server

import (
	"embed"
	"io/fs"
)

//go:embed static/upload.html
var uploadPageHTML string

//go:embed static/styles.css
var stylesCSS string

//go:embed static/app.js
var appJS string

//go:embed static
var staticFS embed.FS

// GetStaticFS returns the embedded static file system for serving static assets
func GetStaticFS() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

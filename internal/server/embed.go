package server

import (
	_ "embed"
)

//go:embed static/upload.html
var uploadPageHTML string

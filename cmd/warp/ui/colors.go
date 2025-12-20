package ui

import "os"

// Color holds ANSI color codes that can be toggled via NO_COLOR
type Color struct {
	Reset   string
	Bold    string
	Dim     string
	Green   string
	Yellow  string
	Magenta string
	Red     string
}

// C is the global color scheme instance
var C Color

func init() {
	SetColorsEnabled(os.Getenv("NO_COLOR") == "")
}

// SetColorsEnabled enables or disables color output
func SetColorsEnabled(enabled bool) {
	if !enabled {
		C = Color{} // All empty strings
		return
	}
	C = Color{
		Reset:   "\033[0m",
		Bold:    "\033[1m",
		Dim:     "\033[2m",
		Green:   "\033[32m",
		Yellow:  "\033[33m",
		Magenta: "\033[35m",
		Red:     "\033[31m",
	}
}

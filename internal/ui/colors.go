package ui

import "os"

// ColorScheme holds ANSI color codes for terminal output
type ColorScheme struct {
	Reset   string
	Bold    string
	Dim     string
	Green   string
	Yellow  string
	Red     string
	Cyan    string
	Magenta string
	Black   string
}

// Colors is the global color scheme instance
var Colors = initColors()

// initColors initializes color codes based on NO_COLOR environment variable
func initColors() ColorScheme {
	if os.Getenv("NO_COLOR") != "" {
		return ColorScheme{} // All empty strings when NO_COLOR is set
	}
	return ColorScheme{
		Reset:   "\033[0m",
		Bold:    "\033[1m",
		Dim:     "\033[2m",
		Green:   "\033[38;2;130;200;130m",
		Yellow:  "\033[33m",
		Red:     "\033[31m",
		Cyan:    "\033[36m",
		Magenta: "\033[35m",
		Black:   "\033[30m",
	}
}

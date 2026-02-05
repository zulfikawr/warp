package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// ===== Dimension Utilities =====

// GetAdaptiveBoxWidth returns an appropriate box width for the given available width
func GetAdaptiveBoxWidth(availWidth, preferredWidth, minPadding int) int {
	if availWidth < preferredWidth+minPadding {
		return max(availWidth-minPadding, 20) // Minimum 20 width
	}
	return preferredWidth
}

// GetStandardBoxWidth returns the standard box width (60) adapted to available width
func GetStandardBoxWidth(availWidth int) int {
	return GetAdaptiveBoxWidth(availWidth, 60, 4)
}

// GetWideBoxWidth returns a wide box width (80) adapted to available width
func GetWideBoxWidth(availWidth int) int {
	return GetAdaptiveBoxWidth(availWidth, 80, 4)
}

// ===== String Utilities =====

// StripAnsi removes ANSI escape codes from a string to get visible length
func StripAnsi(str string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range str {
		if r == 0x1b {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Truncate cuts a string to the specified width
func Truncate(s string, w int) string {
	if len(s) > w {
		return s[:w]
	}
	return s
}

// TruncateWithEllipsis adds "..." if truncated
func TruncateWithEllipsis(s string, w int) string {
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	if w < 3 {
		return Truncate(s, w)
	}
	return Truncate(s, w-3) + "..."
}

// CenterText centers the given text within the width
func CenterText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	visibleText := StripAnsi(text)
	textLen := utf8.RuneCountInString(visibleText)
	if textLen >= width {
		return text
	}
	leftPad := (width - textLen) / 2
	rightPad := width - textLen - leftPad
	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}

// ===== Progress Bar Rendering =====

// renderProgressBar creates a text-based progress bar with the given percentage and width
func renderProgressBar(percent float64, width int, style lipgloss.Style) string {
	if width < 5 {
		width = 5
	}

	// Clamp percent to 0-1 range
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}

	// Calculate filled and empty portions
	barWidth := width - 7 // Account for brackets and percentage display
	if barWidth < 3 {
		barWidth = 3
	}

	filled := int(float64(barWidth) * percent)
	empty := barWidth - filled

	// Build the progress bar
	bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", empty) + "]"
	percentStr := fmt.Sprintf(" %3.0f%%", percent*100)

	return style.Render(bar + percentStr)
}

package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette - exported for use in subpackages
const (
	PrimaryColor  = lipgloss.Color("#FFB86C") // Amber/Orange
	AccentColor   = lipgloss.Color("#FF5555") // Red/Pink for emphasis
	SuccessColor  = lipgloss.Color("#50FA7B") // Bright Green
	WarningColor  = lipgloss.Color("#FFB86C") // Same as primary
	ErrorColor    = lipgloss.Color("#FF5555") // Red
	TextColor     = lipgloss.Color("#FFB86C") // Amber
	DimTextColor  = lipgloss.Color("#808080") // Muted white for contrast
	BgColor       = lipgloss.Color("#000000") // Pure Black
	BorderColor   = lipgloss.Color("#FFB86C") // Amber
	SelectedColor = lipgloss.Color("#6CB3FF") // Amber complementary
	HighlightBg   = lipgloss.Color("#44475A") // Dark Gray
)

var (
	// Custom ASCII Border
	AsciiBorder = lipgloss.Border{
		Top:         "-",
		Bottom:      "-",
		Left:        "|",
		Right:       "|",
		TopLeft:     "+",
		TopRight:    "+",
		BottomLeft:  "+",
		BottomRight: "+",
	}

	// Header and Footer
	HeaderStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
		// Background removed
		Border(AsciiBorder).
		BorderForeground(PrimaryColor). // Amber border
		BorderTop(true).                // Enable Top Border
		BorderLeft(true).
		BorderRight(true).
		BorderBottom(true). // Enable Bottom Border
		Bold(true).
		Padding(0, 1).
		Align(lipgloss.Center).
		Width(80)

	FooterStyle = lipgloss.NewStyle().
			Foreground(DimTextColor).
		// Background removed
		Border(AsciiBorder).
		BorderForeground(PrimaryColor). // Amber border
		BorderTop(true).                // Enable Top Border
		BorderLeft(true).
		BorderRight(true).
		BorderBottom(true). // Enable Bottom Border
		Padding(0, 1).
		Align(lipgloss.Left).
		Width(80)

	// Content area
	ContentStyle = lipgloss.NewStyle().
			Padding(1, 2)

	// Menu and interactive elements
	MenuItemStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor)

	MenuItemSelectedStyle = lipgloss.NewStyle().
				Foreground(SelectedColor).
				Bold(true)

	// Boxes and containers
	BoxStyle = lipgloss.NewStyle().
			Border(AsciiBorder).
			BorderForeground(PrimaryColor).
			Padding(0, 1).
			MarginTop(0).
			MarginBottom(1)

	BoxTitleStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true)

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(DimTextColor).
			Italic(true)

	HighlightStyle = lipgloss.NewStyle().
			Foreground(AccentColor).
			Bold(true)

	DimStyle = lipgloss.NewStyle().
			Foreground(DimTextColor)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(SuccessColor)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ErrorColor)

	WarningStyle = lipgloss.NewStyle().
			Foreground(WarningColor)

	// Status messages
	StatusBoxStyle = lipgloss.NewStyle().
			Border(AsciiBorder).
			BorderForeground(SuccessColor).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	ErrorBoxStyle = lipgloss.NewStyle().
			Border(AsciiBorder).
			BorderForeground(ErrorColor).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	// Code/monospace
	CodeStyle = lipgloss.NewStyle().
			Background(HighlightBg).
			Foreground(PrimaryColor).
			Padding(0, 1).
			Inline(true)

	// Help text
	HelpStyle = lipgloss.NewStyle().
			Foreground(DimTextColor).
			MarginTop(1)

	// Input field
	InputStyle = lipgloss.NewStyle().
			Border(AsciiBorder).
			BorderForeground(PrimaryColor).
			Padding(0, 1).
			Inline(true)
)

// Helpers to update styles based on terminal width
func UpdateWidthStyles(width int) {
	HeaderStyle = HeaderStyle.Width(width)
	FooterStyle = FooterStyle.Width(width)
}

// ===== Common Style Getters =====

// GetLabelStyle returns a bold primary-colored label style
func GetLabelStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
}

// GetSuccessStyle returns a success-colored style
func GetSuccessStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(SuccessColor)
}

// GetDimStyle returns a dimmed text style
func GetDimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(DimTextColor)
}

// GetEnabledStyle returns a style for enabled/active items
func GetEnabledStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(SuccessColor)
}

// GetDisabledStyle returns a style for disabled/inactive items
func GetDisabledStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(WarningColor)
}

// GetErrorStyle returns an error-colored style
func GetErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ErrorColor)
}

// GetTitleStyle returns a title style
func GetTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
}

// GetSelectedStyle returns a style for selected items
func GetSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(SelectedColor).Bold(true)
}

// Render a section with a border and title
func RenderSection(title string, content string) string {
	return BoxStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			BoxTitleStyle.Render(title),
			content,
		),
	)
}

// Render a status message
func RenderStatusBox(message string, success bool) string {
	style := StatusBoxStyle
	if !success {
		style = ErrorBoxStyle
	}
	return style.Render(message)
}

// RenderLabel returns a styled label
func RenderLabel(label string) string {
	return lipgloss.NewStyle().
		Foreground(PrimaryColor).
		Bold(true).
		Render(label)
}

// RenderKeyValue renders a key-value pair
func RenderKeyValue(key, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		RenderLabel(key+":"),
		" ",
		value,
	)
}

// Create a help text with proper styling
func RenderHelp(text string) string {
	return HelpStyle.Render(text)
}

// ===== Text Rendering Helpers =====

// RenderCentered centers text within the given width
func RenderCentered(text string, width int) string {
	return CenterText(text, width)
}

// RenderCenteredStyled centers and styles text within the given width
func RenderCenteredStyled(text string, width int, style lipgloss.Style) string {
	return CenterText(style.Render(text), width)
}

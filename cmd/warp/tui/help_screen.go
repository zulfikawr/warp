package tui

import tea "github.com/charmbracelet/bubbletea"

// HelpScreen is a reusable component for scrollable help screens.
// It handles all the common help screen logic: scrolling, navigation, and rendering.
type HelpScreen struct {
	Title        string
	Lines        []string
	ScrollOffset int
	Width        int
	Height       int
}

// NewHelpScreen creates a new help screen component.
func NewHelpScreen(title string, lines []string) HelpScreen {
	return HelpScreen{
		Title:        title,
		Lines:        lines,
		ScrollOffset: 0,
	}
}

// Update handles all help screen input (scrolling, exit).
// Returns: updated model and whether to exit help screen.
func (h HelpScreen) Update(msg tea.Msg) (HelpScreen, bool) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "esc", "?", "q":
			h.ScrollOffset = 0
			return h, true

		case "up", "k":
			if h.ScrollOffset > 0 {
				h.ScrollOffset--
			}

		case "down", "j":
			availHeight := h.Height - 6
			maxOffset := max(0, len(h.Lines)-availHeight)
			if h.ScrollOffset < maxOffset {
				h.ScrollOffset++
			}

		case "pgup":
			h.ScrollOffset -= h.Height / 2
			if h.ScrollOffset < 0 {
				h.ScrollOffset = 0
			}

		case "pgdown", " ":
			h.ScrollOffset += h.Height / 2
			availHeight := h.Height - 6
			maxOffset := max(0, len(h.Lines)-availHeight)
			if h.ScrollOffset > maxOffset {
				h.ScrollOffset = maxOffset
			}

		case "home":
			h.ScrollOffset = 0

		case "end":
			availHeight := h.Height - 6
			h.ScrollOffset = max(0, len(h.Lines)-availHeight)
		}
	}

	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		h.Width = msg.Width
		h.Height = msg.Height
	}

	return h, false
}

// View renders the help screen.
func (h HelpScreen) View() string {
	return RenderHelpScreen(h.Title, h.Lines, h.Width, h.Height, h.ScrollOffset)
}

// SetSize updates the dimensions of the help screen.
func (h *HelpScreen) SetSize(width, height int) {
	h.Width = width
	h.Height = height
}

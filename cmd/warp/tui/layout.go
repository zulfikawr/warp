package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/internal/network"
)

// ===== Layout Constants =====

const (
	MinSplitLayoutWidth = 84 // Minimum width for side-by-side QR+Info layout
	DefaultBoxWidth     = 60 // Standard box width
	WideBoxWidth        = 80 // Wide box width
	CompactBoxWidth     = 50 // Compact box width
	MinBoxWidth         = 20 // Absolute minimum box width
)

// RenderScreen renders a full TUI screen with standardized layout.
// It handles terminal size checking, header/footer placement, and responsive content.
//
// title: Screen title for the header
// width, height: Current terminal dimensions
// footerItems: List of footer shortcuts
// contentProvider: A callback that returns the main content string (receives available width and height)
func RenderScreen(title string, width, height int, footerItems []string, contentProvider func(availWidth, availHeight int) string) string {
	// Check minimum terminal size
	if width < 10 || height < 10 {
		return "Terminal too small"
	}

	// Update styles with current width
	UpdateWidthStyles(width)

	// Render header (3 lines with border)
	header := RenderHeader(title, width)

	// Render footer (3 lines with border)
	footer := RenderFooter(footerItems, width)

	// Calculate available height for content (header + content + footer = height)
	// Header: 3, Footer: 3, leaving height-6 for content
	// Calculate body height
	bodyHeight := height - 6
	if bodyHeight < 0 {
		bodyHeight = 0
	}

	// Calculate available width for content (width - 2 for borders)
	availWidth := width - 2
	if availWidth < 0 {
		availWidth = 0
	}

	// Get content from provider
	content := contentProvider(availWidth, bodyHeight)

	// Create a style for the body that adds side borders and fills the height
	bodyStyle := lipgloss.NewStyle().
		Width(width - 2). // Subtract 2 for borders
		Height(bodyHeight).
		Border(AsciiBorder).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(true).
		BorderRight(true).
		BorderForeground(PrimaryColor)

	// Render the body
	body := bodyStyle.Render(content)

	// Build the screen
	var screenParts []string
	screenParts = append(screenParts, header)
	screenParts = append(screenParts, body)
	screenParts = append(screenParts, footer)

	return strings.Join(screenParts, "\n")
}

var (
	cachedIP string
)

func getIP() string {
	if cachedIP != "" {
		return cachedIP
	}
	ip, err := network.DiscoverLANIP("")
	if err != nil {
		cachedIP = "Unknown IP"
	} else {
		cachedIP = ip.String()
	}
	return cachedIP
}

// RenderHeader returns a uniform header string with the given title.
func RenderHeader(title string, width int) string {
	ipAddr := getIP()

	styleWidth := width - 2
	contentTargetWidth := styleWidth - 2

	if contentTargetWidth < 0 {
		return ""
	}

	cleanTitle := StripAnsi(title)
	titleLen := utf8.RuneCountInString(cleanTitle)
	ipLen := utf8.RuneCountInString(ipAddr)

	if titleLen+ipLen > contentTargetWidth {
		title = Truncate(cleanTitle, contentTargetWidth-ipLen)
		titleLen = utf8.RuneCountInString(title)
	}

	availSpace := contentTargetWidth - titleLen - ipLen
	if availSpace < 0 {
		availSpace = 0
	}

	headerText := title + strings.Repeat(" ", availSpace) + ipAddr

	return HeaderStyle.Width(styleWidth).Render(headerText)
}

// RenderFooter returns a uniform footer string with the given items.
func RenderFooter(items []string, width int) string {
	var footerText string
	isHome := false
	for _, item := range items {
		if strings.Contains(item, "[Q] Quit") {
			isHome = true
			break
		}
	}

	if isHome {
		footerText = "Use arrow keys or shortcuts to navigate.  [?] Help  [Q] Quit"
	} else {
		footerText = strings.Join(items, " | ")
	}

	contentWidth := width - 2
	if contentWidth < 0 {
		return ""
	}

	if utf8.RuneCountInString(footerText) > contentWidth {
		footerText = Truncate(footerText, contentWidth)
	}

	padding := contentWidth - utf8.RuneCountInString(footerText)
	if padding > 0 {
		footerText = footerText + strings.Repeat(" ", padding)
	}

	return FooterStyle.Width(contentWidth).Render(footerText)
}

// RenderQRInfoLayout renders a side-by-side view with a QR code on the left and info on the right.
// It handles adaptive sizing and separation.
func RenderQRInfoLayout(width, height int, qrCode string, contentProvider func(int) string) string {
	qrRaw := strings.TrimSpace(qrCode)
	qrWidth := lipgloss.Width(qrRaw)
	qrHeight := lipgloss.Height(qrRaw)

	gap := 2
	separatorWidth := 1
	borderPadding := 4

	// Calculate max width for the info block
	maxTextWidth := width - qrWidth - (gap * 2) - separatorWidth - borderPadding

	// Ensure reasonable minimums
	if maxTextWidth < 20 {
		maxTextWidth = 20
	}

	// Try rendering info with natural width first (width=0)
	infoContent := contentProvider(0)
	naturalWidth := lipgloss.Width(infoContent)

	// If natural width exceeds available space, fallback to fixed width wrapping
	if naturalWidth > maxTextWidth {
		infoContent = contentProvider(maxTextWidth)
	}

	infoHeight := lipgloss.Height(infoContent)

	// Vertical Separator
	maxContentHeight := qrHeight
	if infoHeight > maxContentHeight {
		maxContentHeight = infoHeight
	}

	sepStyle := lipgloss.NewStyle().Foreground(PrimaryColor)
	sepLines := make([]string, maxContentHeight)
	for i := range sepLines {
		sepLines[i] = "|"
	}
	separator := sepStyle.Render(strings.Join(sepLines, "\n"))

	combinedContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		qrRaw,
		strings.Repeat(" ", gap),
		separator,
		strings.Repeat(" ", gap),
		infoContent,
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Render(combinedContent))
}

// RenderHelpScreen renders a standardized scrolling help screen
func RenderHelpScreen(title string, lines []string, width, height, offset int) string {
	footerItems := []string{
		"[↑↓] Navigate",
		"[Esc/Q/?] Close",
	}

	return RenderScreen(title, width, height, footerItems, func(w, availHeight int) string {
		totalLines := len(lines)

		// Clamp offset
		if offset > totalLines-availHeight {
			offset = totalLines - availHeight
		}
		if offset < 0 {
			offset = 0
		}

		end := offset + availHeight
		if end > totalLines {
			end = totalLines
		}

		visibleLines := lines[offset:end]
		var rows []string
		for _, l := range visibleLines {
			rows = append(rows, "  "+l)
		}

		// Fill remaining
		remaining := availHeight - len(visibleLines)
		for i := 0; i < remaining; i++ {
			rows = append(rows, "")
		}

		return strings.Join(rows, "\n")
	})
}

// RenderErrorScreen renders a standardized error screen
func RenderErrorScreen(title, errorMsg string, width, height int) string {
	return RenderScreen(title, width, height, []string{"[Esc] Back"}, func(w, availHeight int) string {
		boxWidth := 60
		if w < 60 {
			boxWidth = w - 4
		}

		errorStyle := lipgloss.NewStyle().Foreground(ErrorColor).Bold(true)

		var b strings.Builder
		b.WriteString(errorStyle.Render(CenterText(fmt.Sprintf("✗ Error: %s", errorMsg), boxWidth-4)) + "\n\n")
		b.WriteString(CenterText("Press ESC to return", boxWidth-4) + "\n")

		boxContent := BoxStyle.Width(boxWidth).Render(b.String())
		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, boxContent)
	})
}

// ===== Footer Presets =====

// GetNavigationFooter returns standard navigation footer items.
func GetNavigationFooter() []string {
	return []string{"[↑↓] Navigate", "[⏎] Select", "[Esc] Back"}
}

// GetHelpFooter returns standard help screen footer items.
func GetHelpFooter() []string {
	return []string{"[↑↓] Navigate", "[Esc/Q/?] Close"}
}

// GetEditFooter returns standard edit screen footer items.
func GetEditFooter() []string {
	return []string{"[↑↓] Navigate", "[Space/Type] Edit", "[Ctrl+S] Save", "[Esc] Back"}
}

// GetServerFooter returns footer items for server screens with optional QR toggle.
func GetServerFooter(showQRToggle bool) []string {
	items := []string{"[Esc] Stop", "[?] Help"}
	if showQRToggle {
		items = append([]string{"[Q] Toggle QR"}, items...)
	}
	return items
}

// MergeFooterItems merges custom items with standard ones.
func MergeFooterItems(custom, standard []string) []string {
	return append(custom, standard...)
}

// ===== Centered Box Helpers =====

// RenderCenteredBox renders content in a centered box with the specified width.
func RenderCenteredBox(content string, boxWidth, availWidth, availHeight int) string {
	boxContent := BoxStyle.Width(boxWidth).Render(content)
	return lipgloss.Place(availWidth, availHeight,
		lipgloss.Center, lipgloss.Center, boxContent)
}

// RenderCenteredBoxAdaptive renders content in a centered box with adaptive width.
func RenderCenteredBoxAdaptive(content string, preferredWidth, availWidth, availHeight int) string {
	boxWidth := GetAdaptiveBoxWidth(availWidth, preferredWidth, 4)
	return RenderCenteredBox(content, boxWidth, availWidth, availHeight)
}

// RenderCenteredBoxStandard renders content in a centered box with standard width (60).
func RenderCenteredBoxStandard(content string, availWidth, availHeight int) string {
	boxWidth := GetStandardBoxWidth(availWidth)
	return RenderCenteredBox(content, boxWidth, availWidth, availHeight)
}

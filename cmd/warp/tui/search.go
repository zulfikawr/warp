// Package tui provides the terminal user interface for warp.
// This file contains the search screen model for discovering warp services.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/internal/core"
	"github.com/zulfikawr/warp/internal/discovery"
)

// ConnectMsg is sent when a user selects a server to connect to.
type ConnectMsg struct {
	Service discovery.Service
}

// searchLoadedMsg carries the result of a discovery operation.
type searchLoadedMsg struct {
	services []core.ServiceInfo
	err      error
}

// SearchModel checks for available warp servers on the network.
type SearchModel struct {
	services []core.ServiceInfo
	cursor   int
	offset   int
	width    int
	height   int
	loading  bool
	err      error
	spinner  spinner.Model
}

// NewSearchModel creates a new search model.
func NewSearchModel() SearchModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(PrimaryColor)

	return SearchModel{
		width:   80,
		height:  24,
		cursor:  0,
		offset:  0,
		loading: true,
		spinner: s,
	}
}

// SetHelp is a no-op for compatibility (help removed in simplified version)
func (m *SearchModel) SetHelp() {
	// No-op - help screen removed in simplified version
}

// Init implements tea.Model.
func (m SearchModel) Init() tea.Cmd {
	return tea.Batch(m.performSearch, m.spinner.Tick)
}

// performSearch uses the core.SearchExecutor to discover services.
func (m SearchModel) performSearch() tea.Msg {
	opts := core.SearchOptions{
		Timeout: 3 * time.Second,
	}

	executor := core.NewSearchExecutor(opts, nil)
	services, err := executor.Execute(context.Background())
	return searchLoadedMsg{services: services, err: err}
}

// Update implements tea.Model.
func (m SearchModel) Update(msg tea.Msg) (SearchModel, tea.Cmd, bool) {
	quit := false
	var cmd tea.Cmd

	if m.loading {
		var scmd tea.Cmd
		m.spinner, scmd = m.spinner.Update(msg)
		cmd = tea.Batch(cmd, scmd)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "ctrl+c", "q", "esc":
			quit = true
			return m, nil, quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.services)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.services) > 0 && m.cursor < len(m.services) {
				selected := m.services[m.cursor]
				return m, func() tea.Msg {
					return ConnectMsg{Service: discovery.Service{
						Name:  selected.Name,
						Mode:  selected.Mode,
						Port:  selected.Port,
						URL:   selected.URL,
						Token: selected.Token,
					}}
				}, false
			}
		case "r":
			m.loading = true
			m.services = nil
			m.cursor = 0
			m.offset = 0
			m.err = nil
			return m, tea.Batch(m.performSearch, m.spinner.Tick), false
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case searchLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.services = msg.services
			m.cursor = 0
			m.offset = 0
		}
	}

	// Update scroll offset
	reservedLines := 8
	availHeight := max(m.height-reservedLines, 1)

	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+availHeight {
		m.offset = m.cursor - availHeight + 1
	}

	return m, cmd, quit
}

// View implements tea.Model.
func (m SearchModel) View() string {
	footerItems := []string{
		"[↑↓] Navigate",
		"[⏎] Connect",
		"[R] Refresh",
		"[Esc] Back",
	}

	return RenderScreen("SEARCH NETWORK", m.width, m.height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth

		// Loading state
		if m.loading {
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				m.spinner.View()+" "+DimStyle.Render("Scanning for Warp servers..."))
		}

		// Error state
		if m.err != nil {
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				GetErrorStyle().Render(fmt.Sprintf("Search error: %v\n\nPress R to retry", m.err)))
		}

		// Empty state
		if len(m.services) == 0 {
			emptyMsg := "No servers found.\n\nMake sure other devices are on the same network\nand running 'warp send' or 'warp host'\n\nPress R to refresh"
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				DimStyle.Render(emptyMsg))
		}

		// Calculate box width
		boxWidth := w - 4
		if boxWidth < 40 {
			boxWidth = 40
		}

		labelStyle := lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)

		// Header row
		titleText := "DISCOVERED SERVERS"
		countText := fmt.Sprintf("%d found", len(m.services))
		headerRow := lipgloss.JoinHorizontal(lipgloss.Top,
			labelStyle.Render(titleText),
			lipgloss.PlaceHorizontal(boxWidth-len(titleText)-len(countText), lipgloss.Right, DimStyle.Render(countText)),
		)

		// Build list rows
		var listRows []string
		listRows = append(listRows, headerRow)
		listRows = append(listRows, lipgloss.NewStyle().Foreground(PrimaryColor).Render(strings.Repeat("-", boxWidth)))

		// Calculate visible area
		maxListHeight := availHeight - 4
		if maxListHeight < 1 {
			maxListHeight = 1
		}

		for i := 0; i < maxListHeight; i++ {
			idx := m.offset + i
			if idx < len(m.services) {
				svc := m.services[idx]
				row := m.renderRow(svc, idx == m.cursor, boxWidth)
				listRows = append(listRows, row)
			} else {
				listRows = append(listRows, "")
			}
		}

		listContent := strings.Join(listRows, "\n")
		menuBox := lipgloss.NewStyle().Width(boxWidth).Render(listContent)

		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, menuBox)
	})
}

// renderRow renders a single service row
func (m SearchModel) renderRow(svc core.ServiceInfo, selected bool, maxWidth int) string {
	// Mode icon
	modeIcon := "📤"
	if strings.ToLower(svc.Mode) == "host" {
		modeIcon = "📥"
	}

	// Format mode text
	modeStr := strings.ToUpper(svc.Mode)
	if modeStr == "" {
		modeStr = "UNKNOWN"
	}

	// Build the row: [icon] name ... mode ip:port
	prefix := fmt.Sprintf("%s ", modeIcon)
	suffix := fmt.Sprintf("  %s  %s:%d", modeStr, svc.IP, svc.Port)

	// Calculate available width for name
	nameWidth := maxWidth - len(prefix) - len(suffix)
	if nameWidth < 10 {
		nameWidth = 10
	}

	name := svc.Name
	if len(name) > nameWidth {
		name = TruncateWithEllipsis(name, nameWidth)
	}

	// Build full line with padding
	paddingCount := maxWidth - len(prefix) - len(name) - len(suffix)
	if paddingCount < 1 {
		paddingCount = 1
	}

	fullLine := prefix + name + strings.Repeat(" ", paddingCount) + suffix

	if selected {
		return MenuItemSelectedStyle.Render(fullLine)
	}
	return MenuItemStyle.Render(fullLine)
}

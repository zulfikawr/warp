package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/cmd/warp/help"
)

type state int

const (
	stateNormal state = iota
	stateHelp
)

var MenuItems = []string{
	"Send",
	"Receive",
	"Host",
	"Search", // Key will be handled as 'E'
	"Resume", // Key will be 'M'
	"History",
	"Config",
}

// MenuDescription maps index to description
var MenuDescriptions = []string{
	"Transfer files to another device.",
	"Wait for incoming files.",
	"Start a temporary file server.",
	"Discover devices on local network.",
	"Resume interrupted transfers.",
	"View transfer history.",
	"Edit settings and preferences.",
}

type HomeModel struct {
	Cursor     int
	Width      int
	Height     int
	state      state
	helpScreen HelpScreen
}

func NewHomeModel() HomeModel {
	return HomeModel{
		Cursor:     0,
		Width:      80,
		Height:     24,
		state:      stateNormal,
		helpScreen: NewHelpScreen("WARP HELP", help.RootHelpLines),
	}
}

type TickMsg time.Time

func (m HomeModel) Init() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m HomeModel) Update(msg tea.Msg) (HomeModel, tea.Cmd, bool) {
	quit := false
	enter := false

	// Help state handling
	if m.state == stateHelp {
		h, shouldExit := m.helpScreen.Update(msg)
		m.helpScreen = h
		if shouldExit {
			m.state = stateNormal
		}
		return m, nil, false
	}

	switch msg := msg.(type) {
	case TickMsg:
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return TickMsg(t)
		}), false
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "ctrl+c", "q":
			return m, tea.Quit, false
		case "?":
			m.state = stateHelp
			return m, nil, false
		case "up", "k":
			if m.Cursor > 0 {
				m.Cursor--
			}
		case "down", "j":
			if m.Cursor < len(MenuItems)-1 {
				m.Cursor++
			}
		case "enter":
			enter = true
		case "s": // Send
			m.Cursor = 0
			enter = true
		case "r": // Receive
			m.Cursor = 1
			enter = true
		case "h": // Host
			m.Cursor = 2
			enter = true
		case "e": // Search (mapped to E)
			m.Cursor = 3
			enter = true
		case "m": // Resume (mapped to M)
			m.Cursor = 4
			enter = true
		case "p": // History (was Paused Transfers)
			m.Cursor = 5
			enter = true
		case "c": // Config
			m.Cursor = 6
			enter = true
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.helpScreen.SetSize(msg.Width, msg.Height)
	}
	// If enter is pressed, signal quit so main TUI can handle the action
	if enter {
		return m, nil, true
	}
	return m, nil, quit
}

func (m HomeModel) View() string {
	if m.state == stateHelp {
		return m.helpScreen.View()
	}

	// Footer items aligned with reference
	footerItems := []string{"[?] Help", "[Q] Quit"}

	return RenderScreen("WARP v2.0 - Local Network File Transfer", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		var menuRows []string
		maxRowLength := 0

		// Construct menu rows
		for i, item := range MenuItems {
			// Customize key display
			key := string(item[0])
			switch item {
			case "Search":
				key = "E"
			case "Resume":
				key = "M"
			case "History":
				key = "P"
			}

			itemLabel := fmt.Sprintf("[%s] %s", key, item)
			desc := MenuDescriptions[i]

			rowText := fmt.Sprintf("%-20s | %s", itemLabel, desc)

			if len(rowText) > maxRowLength {
				maxRowLength = len(rowText)
			}

			if m.Cursor == i {
				menuRows = append(menuRows, MenuItemSelectedStyle.Render(rowText))
			} else {
				menuRows = append(menuRows, MenuItemStyle.Render(rowText))
			}
		}

		menuContent := strings.Join(menuRows, "\n\n")

		// Construct Title to match dimensions
		// Col 1: "MAIN MENU           " (Fixed width 20)
		// Col 2: "Select an option:"

		titleText := fmt.Sprintf("%-20s | %s", "MAIN MENU", "Select an option:")

		if len(titleText) > maxRowLength {
			maxRowLength = len(titleText)
		}

		// Separator
		// Use max width found
		separator := lipgloss.NewStyle().Foreground(PrimaryColor).Render(strings.Repeat("-", maxRowLength))

		// Render the box
		menuBox := BoxStyle.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				BoxTitleStyle.Render(titleText),
				separator,
				menuContent,
			),
		)

		return lipgloss.Place(
			m.Width-2,
			availHeight,
			lipgloss.Center,
			lipgloss.Center,
			menuBox,
		)
	})
}

// Package tui provides the terminal user interface for warp.
// This file contains the main application model that orchestrates all screens.
package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Screen represents the current active screen in the TUI.
type Screen int

const (
	ScreenHome Screen = iota
	ScreenSend
	ScreenReceive
	ScreenHost
	ScreenSearch
	ScreenConfig
	ScreenHistory
	ScreenResume
)

// App is the main TUI application model that manages screen navigation
// and delegates updates to the appropriate screen models.
type App struct {
	screen  Screen
	home    HomeModel
	send    SendModel
	receive ReceiveModel
	host    *HostModel
	search  SearchModel
	config  ConfigModel
	history HistoryModel
	resume  ResumeModel
}

// NewApp creates a new TUI application with all screens initialized.
func NewApp() *App {
	cwd, _ := os.Getwd()
	return &App{
		screen:  ScreenHome,
		home:    NewHomeModel(),
		receive: NewReceiveModel(),
		send:    NewSendModel(cwd),
		host:    NewHostModel(),
		search:  NewSearchModel(),
		config:  NewConfigModel(),
		history: NewHistoryModel(),
		resume:  NewResumeModel(),
	}
}

// Init implements tea.Model. It initializes the current screen.
func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd

	switch a.screen {
	case ScreenSend:
		cmds = append(cmds, a.send.Init())
	case ScreenReceive:
		cmds = append(cmds, a.receive.Init())
	case ScreenHost:
		cmds = append(cmds, a.host.Init())
	case ScreenSearch:
		cmds = append(cmds, a.search.Init())
	case ScreenConfig:
		cmds = append(cmds, a.config.Init())
	case ScreenHistory:
		cmds = append(cmds, a.history.Init())
	case ScreenResume:
		cmds = append(cmds, a.resume.Init())
	}

	return tea.Batch(cmds...)
}

// Update implements tea.Model. It handles global keys and delegates
// to the appropriate screen model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle global quit
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	// Propagate window size to all screens
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		a.home, _, _ = a.home.Update(msg)
		a.send, _, _ = a.send.Update(msg)
		a.receive, _, _ = a.receive.Update(msg)
		a.host, _, _ = a.host.Update(msg)
		a.search, _, _ = a.search.Update(msg)
		a.config, _, _ = a.config.Update(msg)
		a.history, _, _ = a.history.Update(msg)
		a.resume, _, _ = a.resume.Update(msg)
	}

	// Handle ConnectMsg from search screen (before screen switch)
	if connectMsg, ok := msg.(ConnectMsg); ok {
		// Route based on service mode
		switch strings.ToLower(connectMsg.Service.Mode) {
		case "send":
			// Server is sending (hosting file for download) - we receive
			a.screen = ScreenReceive
			a.receive = NewReceiveModel()

			token := connectMsg.Service.Token
			if token == "" {
				token = "unknown-token"
			}

			a.receive.Options.Code = token
			return a, a.receive.Init()

		case "host":
			// Server is hosting (receiving uploads) - we send/upload to them
			a.screen = ScreenSend
			cwd, _ := os.Getwd()
			a.send = NewSendModel(cwd)
			// Store the host URL for the send screen to use
			a.send.SetTargetHost(connectMsg.Service.URL, connectMsg.Service.Token)
			return a, func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} }

		default:
			// Unknown mode - default to receive
			a.screen = ScreenReceive
			a.receive = NewReceiveModel()

			token := connectMsg.Service.Token
			if token == "" {
				token = "unknown-token"
			}

			a.receive.Options.Code = token
			return a, a.receive.Init()
		}
	}

	// Handle ResumeSelectedMsg from resume screen (before screen switch)
	if resumeMsg, ok := msg.(ResumeSelectedMsg); ok {
		if resumeMsg.Checkpoint != nil {
			// Route based on transfer direction
			if resumeMsg.Checkpoint.Direction == "download" {
				a.screen = ScreenReceive
				a.receive = NewReceiveModel()
				// Set the checkpoint for resume
				a.receive.SetResumeCheckpoint(resumeMsg.Checkpoint)
				// Send window size to ensure full screen rendering
				return a, tea.Batch(
					a.receive.Init(),
					func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} },
				)
			}
			// For uploads, we'd need to handle differently
			// For now, just go back to home
		}
	}

	switch a.screen {
	case ScreenHome:
		return a.updateHome(msg)
	case ScreenSend:
		return a.updateSend(msg)
	case ScreenReceive:
		return a.updateReceive(msg)
	case ScreenHost:
		return a.updateHost(msg)
	case ScreenSearch:
		return a.updateSearch(msg)
	case ScreenConfig:
		return a.updateConfig(msg)
	case ScreenHistory:
		return a.updateHistory(msg)
	case ScreenResume:
		return a.updateResume(msg)
	}

	return a, nil
}

// View implements tea.Model. It renders the current screen.
func (a *App) View() string {
	switch a.screen {
	case ScreenHome:
		return a.home.View()
	case ScreenSend:
		return a.send.View()
	case ScreenReceive:
		return a.receive.View()
	case ScreenHost:
		return a.host.View()
	case ScreenSearch:
		return a.search.View()
	case ScreenConfig:
		return a.config.View()
	case ScreenHistory:
		return a.history.View()
	case ScreenResume:
		return a.resume.View()
	}
	return ""
}

// updateHome handles updates for the home screen.
func (a *App) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	h, cmd, quit := a.home.Update(msg)
	a.home = h
	if cmd != nil {
		return a, cmd
	}
	if quit {
		switch a.home.Cursor {
		case 0: // SEND
			a.screen = ScreenSend
			cwd, _ := os.Getwd()
			a.send = NewSendModel(cwd)
			return a, func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} }
		case 1: // RECEIVE
			a.screen = ScreenReceive
			a.receive = NewReceiveModel()
			return a, func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} }
		case 2: // HOST
			a.screen = ScreenHost
			a.host = NewHostModel()
			return a, tea.Batch(
				a.host.Init(),
				func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} },
			)
		case 3: // SEARCH
			a.screen = ScreenSearch
			a.search = NewSearchModel()
			return a, tea.Batch(
				a.search.Init(),
				func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} },
			)
		case 4: // RESUME
			a.screen = ScreenResume
			a.resume = NewResumeModel()
			return a, tea.Batch(
				a.resume.Init(),
				func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} },
			)
		case 5: // HISTORY
			a.screen = ScreenHistory
			a.history = NewHistoryModel()
			return a, tea.Batch(
				a.history.Init(),
				func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} },
			)
		case 6: // CONFIG
			a.screen = ScreenConfig
			a.config = NewConfigModel()
			return a, func() tea.Msg { return tea.WindowSizeMsg{Width: a.home.Width, Height: a.home.Height} }
		}
	}
	return a, cmd
}

// updateSend handles updates for the send screen.
func (a *App) updateSend(msg tea.Msg) (tea.Model, tea.Cmd) {
	s, cmd, quit := a.send.Update(msg)
	a.send = s
	if cmd != nil {
		return a, cmd
	}
	if quit {
		a.screen = ScreenHome
		return a, nil
	}
	return a, cmd
}

// updateReceive handles updates for the receive screen.
func (a *App) updateReceive(msg tea.Msg) (tea.Model, tea.Cmd) {
	r, cmd, quit := a.receive.Update(msg)
	a.receive = r
	if cmd != nil {
		return a, cmd
	}
	if quit {
		a.screen = ScreenHome
		return a, nil
	}
	return a, cmd
}

// updateHost handles updates for the host screen.
func (a *App) updateHost(msg tea.Msg) (tea.Model, tea.Cmd) {
	h, cmd, quit := a.host.Update(msg)
	a.host = h
	if cmd != nil {
		return a, cmd
	}
	if quit {
		a.screen = ScreenHome
		return a, nil
	}
	return a, cmd
}

// updateSearch handles updates for the search screen.
func (a *App) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	s, cmd, quit := a.search.Update(msg)
	a.search = s
	if cmd != nil {
		return a, cmd
	}
	if quit {
		a.screen = ScreenHome
		return a, nil
	}
	return a, cmd
}

// updateConfig handles updates for the config screen.
func (a *App) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	c, cmd, quit := a.config.Update(msg)
	a.config = c
	if cmd != nil {
		return a, cmd
	}
	if quit {
		a.screen = ScreenHome
		return a, nil
	}
	return a, cmd
}

// SetScreen sets the current screen and returns the app.
// Used by handlers to configure the initial screen.
func (a *App) SetScreen(s Screen) {
	a.screen = s
}

// Send returns a pointer to the send model for configuration.
func (a *App) Send() *SendModel {
	return &a.send
}

// Receive returns a pointer to the receive model for configuration.
func (a *App) Receive() *ReceiveModel {
	return &a.receive
}

// Host returns a pointer to the host model for configuration.
func (a *App) Host() *HostModel {
	return a.host
}

// Search returns a pointer to the search model for configuration.
func (a *App) Search() *SearchModel {
	return &a.search
}

// Config returns a pointer to the config model for configuration.
func (a *App) Config() *ConfigModel {
	return &a.config
}

// Home returns a pointer to the home model for configuration.
func (a *App) Home() *HomeModel {
	return &a.home
}

// History returns a pointer to the history model for configuration.
func (a *App) History() *HistoryModel {
	return &a.history
}

// updateHistory handles updates for the history screen.
func (a *App) updateHistory(msg tea.Msg) (tea.Model, tea.Cmd) {
	h, cmd, quit := a.history.Update(msg)
	a.history = h
	if cmd != nil {
		return a, cmd
	}
	if quit {
		a.screen = ScreenHome
		return a, nil
	}
	return a, cmd
}

// updateResume handles updates for the resume screen.
func (a *App) updateResume(msg tea.Msg) (tea.Model, tea.Cmd) {
	r, cmd, quit := a.resume.Update(msg)
	a.resume = r
	if cmd != nil {
		return a, cmd
	}
	if quit {
		a.screen = ScreenHome
		return a, nil
	}
	return a, cmd
}

// Resume returns a pointer to the resume model for configuration.
func (a *App) Resume() *ResumeModel {
	return &a.resume
}

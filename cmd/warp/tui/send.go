// Package tui provides the terminal user interface for warp.
// This file contains the send screen model for sharing files.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/internal/core"
	"github.com/zulfikawr/warp/internal/ui"
)

// SendState represents the current state of the send screen.
type SendState int

const (
	StateSendBrowser SendState = iota
	StateSendTextEntry
	StateSendHelp
	StateSendServerStarting
	StateSendWaitingForReceiver
	StateSendComplete
	StateSendError
)

// serverStartedMsg is sent when the server starts successfully.
type serverStartedMsg struct {
	info     *core.ServerInfo
	executor *core.SendExecutor
}

// serverErrorMsg is sent when the server fails to start.
type serverErrorMsg struct {
	err error
}

// SendOptions contains CLI override options for the send command.
type SendOptions struct {
	Port          int
	InterfaceName string
	RateLimitMbps float64
	CacheSizeMB   int64
	NoQR          bool
	NoEncrypt     bool
	TextContent   string
	StdinContent  string
}

// SendModel manages the send screen state and rendering.
type SendModel struct {
	Width      int
	Height     int
	State      SendState
	FilePicker FilePicker
	TextArea   textarea.Model

	// Server State
	uploadTarget string
	serverInfo   *core.ServerInfo
	errorMsg     string
	executor     *core.SendExecutor
	showQR       bool

	ctx           context.Context
	cancel        context.CancelFunc
	AutoStartPath string

	// Target host for uploading to a remote host server
	targetHostURL   string
	targetHostToken string

	// CLI Override Options
	Options SendOptions

	// Help Screen
	helpScreen HelpScreen

	// Spinner for loading states
	spinner spinner.Model
}

// NewSendModel creates a new send model with the given starting directory.
func NewSendModel(startDir string) SendModel {
	ctx, cancel := context.WithCancel(context.Background())
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(PrimaryColor)

	ta := textarea.New()
	ta.Placeholder = "Type or paste text here to send..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(20)
	ta.ShowLineNumbers = false

	fp := NewFilePicker(startDir)
	fp.CustomFooter = []string{"[Tab] Send Text"}

	return SendModel{
		State:      StateSendBrowser,
		FilePicker: fp,
		TextArea:   ta,
		ctx:        ctx,
		cancel:     cancel,
		Width:      80,
		Height:     24,
		helpScreen: NewHelpScreen("URL & PAKE HELP", help.SendHelpLines),
		spinner:    s,
	}
}

// SetTargetHost sets the target host URL and token for uploading to a remote host.
func (m *SendModel) SetTargetHost(url, token string) {
	m.targetHostURL = url
	m.targetHostToken = token
}

// StartUpload initiates an upload for the given path.
func (m *SendModel) StartUpload(path string) {
	m.AutoStartPath = path
	m.uploadTarget = path
	m.State = StateSendServerStarting
}

// Init implements tea.Model.
func (m SendModel) Init() tea.Cmd {
	if m.AutoStartPath != "" {
		return m.startServer(m.AutoStartPath)
	}

	// If text content is provided via CLI, switch to text entry mode
	if m.Options.TextContent != "" {
		m.TextArea.SetValue(m.Options.TextContent)
		m.State = StateSendTextEntry
		return textarea.Blink
	}

	return nil
}

// Update implements tea.Model.
func (m SendModel) Update(msg tea.Msg) (SendModel, tea.Cmd, bool) {
	quit := false
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit, false
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.FilePicker.Width = msg.Width
		m.FilePicker.Height = msg.Height
		m.TextArea.SetWidth(msg.Width - 4)
		// Reserve space for header (3) + footer (3) + border/padding
		m.TextArea.SetHeight(max(1, msg.Height-8))
		m.helpScreen.SetSize(msg.Width, msg.Height)
	}

	switch m.State {
	case StateSendBrowser:
		if msg, ok := msg.(tea.KeyMsg); ok {
			if msg.String() == "?" {
				m.State = StateSendHelp
				return m, nil, false
			}
			if msg.String() == "esc" {
				return m, nil, true
			}
			if msg.String() == "tab" {
				m.State = StateSendTextEntry
				return m, textarea.Blink, false
			}
		}

		var selected bool
		cmd, selected = m.FilePicker.Update(msg)
		if selected {
			var selectedPaths []string
			for _, entry := range m.FilePicker.Entries {
				if entry.Selected {
					selectedPaths = append(selectedPaths, filepath.Join(m.FilePicker.Dir, entry.Name))
				}
			}

			if len(selectedPaths) == 0 {
				if m.FilePicker.Cursor < len(m.FilePicker.Entries) {
					entry := m.FilePicker.Entries[m.FilePicker.Cursor]
					if !entry.IsDir && entry.Name != ".." {
						selectedPaths = append(selectedPaths, filepath.Join(m.FilePicker.Dir, entry.Name))
					}
				}
			}

			if len(selectedPaths) > 0 {
				if len(selectedPaths) == 1 {
					return m, m.startServer(selectedPaths[0]), false
				}
				bundlePath, err := createBundle(selectedPaths)
				if err != nil {
					m.State = StateSendError
					m.errorMsg = err.Error()
					return m, nil, false
				}
				return m, m.startServer(bundlePath), false
			}
		}
		return m, cmd, false

	case StateSendTextEntry:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "esc":
				m.State = StateSendBrowser
				return m, nil, false
			case "tab":
				m.State = StateSendBrowser
				return m, nil, false
			case "ctrl+s":
				if m.TextArea.Value() != "" {
					return m, m.startServer(""), false
				}
			}
		}
		m.TextArea, cmd = m.TextArea.Update(msg)
		return m, cmd, false

	case StateSendHelp:
		h, shouldExit := m.helpScreen.Update(msg)
		m.helpScreen = h
		if shouldExit {
			m.State = StateSendBrowser
		}
		return m, nil, false

	case StateSendServerStarting:
		var scmd tea.Cmd
		m.spinner, scmd = m.spinner.Update(msg)

		if msg, ok := msg.(serverStartedMsg); ok {
			m.State = StateSendWaitingForReceiver
			m.serverInfo = msg.info
			m.executor = msg.executor
			return m, nil, false
		}
		if msg, ok := msg.(serverErrorMsg); ok {
			m.State = StateSendError
			m.errorMsg = msg.err.Error()
			return m, nil, false
		}
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
			m.State = StateSendBrowser
			return m, nil, false
		}
		return m, scmd, false

	case StateSendWaitingForReceiver:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "q":
				m.showQR = !m.showQR
			case "esc":
				if m.executor != nil {
					_ = m.executor.Stop()
					m.executor = nil
				}
				m.State = StateSendBrowser
			}
		}

	case StateSendError, StateSendComplete:
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
			m.State = StateSendBrowser
		}
	}

	return m, nil, quit
}

// View implements tea.Model.
func (m SendModel) View() string {
	switch m.State {
	case StateSendBrowser:
		return m.renderBrowserView()
	case StateSendTextEntry:
		return m.renderTextEntryView()
	case StateSendHelp:
		return m.helpScreen.View()
	case StateSendServerStarting:
		return m.renderServerStartingView()
	case StateSendWaitingForReceiver:
		return m.renderWaitingView()
	case StateSendComplete:
		return m.renderCompleteView()
	case StateSendError:
		return m.renderErrorView()
	}
	return ""
}

// startServer creates and starts the send executor using the core package.
func (m *SendModel) startServer(filePath string) tea.Cmd {
	m.State = StateSendServerStarting

	// Prepare options synchronously to avoid race in Cmd
	opts := core.SendOptions{
		Port:          m.Options.Port,
		InterfaceName: m.Options.InterfaceName,
		RateLimitMbps: m.Options.RateLimitMbps,
		CacheSizeMB:   m.Options.CacheSizeMB,
		NoQR:          m.Options.NoQR,
		NoEncrypt:     m.Options.NoEncrypt,
		StdinContent:  m.Options.StdinContent,
	}

	// Determine if we are sending text or file
	if filePath == "" && (m.State == StateSendTextEntry || m.Options.TextContent != "") {
		if m.State == StateSendTextEntry {
			opts.TextContent = m.TextArea.Value()
		} else {
			opts.TextContent = m.Options.TextContent
		}
		m.uploadTarget = "Text Snippet"
	} else {
		opts.FilePath = filePath
		m.uploadTarget = filePath
	}

	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			// Create executor with no callbacks (TUI handles display)
			executor := core.NewSendExecutor(opts, nil, nil)

			// Start the server
			info, err := executor.Start(m.ctx)
			if err != nil {
				return serverErrorMsg{err: err}
			}

			return serverStartedMsg{
				info:     info,
				executor: executor,
			}
		},
	)
}

// createBundle creates a temporary directory with symlinks to the selected files.
func createBundle(paths []string) (string, error) {
	tmpBase, err := os.MkdirTemp("", "warp-send-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	bundleDir := filepath.Join(tmpBase, "warp-temp")
	if err := os.Mkdir(bundleDir, 0755); err != nil {
		_ = os.RemoveAll(tmpBase)
		return "", fmt.Errorf("failed to create bundle dir: %w", err)
	}
	for _, srcPath := range paths {
		destPath := filepath.Join(bundleDir, filepath.Base(srcPath))
		if err := os.Symlink(srcPath, destPath); err != nil {
			_ = os.RemoveAll(tmpBase)
			return "", fmt.Errorf("failed to link file: %w", err)
		}
	}
	return bundleDir, nil
}

// RENDERERS

func (m SendModel) renderBrowserView() string {
	return m.FilePicker.View()
}

func (m SendModel) renderTextEntryView() string {
	footerItems := []string{
		"[Ctrl+S] Send",
		"[Tab] File Browser",
		"[Esc] Back",
	}

	return RenderScreen("SEND TEXT", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth

		// Ensure textarea matches available width
		m.TextArea.SetWidth(w - 4)

		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, m.TextArea.View())
	})
}

func (m SendModel) renderServerStartingView() string {
	return RenderScreen("STARTING SERVER", m.Width, m.Height, []string{}, func(availWidth, availHeight int) string {
		var b strings.Builder
		w := availWidth
		boxWidth := GetStandardBoxWidth(w)

		filename := filepath.Base(m.uploadTarget)
		titleStyle := GetLabelStyle()

		b.WriteString(titleStyle.Render(fmt.Sprintf("Preparing to share: %s", filename)) + "\n\n")
		b.WriteString(m.spinner.View() + " Starting warp server..." + "\n")

		boxContent := BoxStyle.Width(boxWidth).Render(b.String())
		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, boxContent)
	})
}

func (m SendModel) renderWaitingView() string {
	isWide := m.Width >= MinSplitLayoutWidth

	qrToggle := "Q: Show QR"
	if isWide {
		qrToggle = "" // No toggle needed in wide mode
	} else if m.showQR {
		qrToggle = "Q: Hide QR"
	}

	footerItems := []string{
		"[Esc] Stop",
		"[Ctrl+C] Quit",
	}
	if qrToggle != "" {
		footerItems = append([]string{qrToggle}, footerItems...)
	}

	return RenderScreen("WARP SEND", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		if isWide && m.serverInfo != nil {
			return RenderQRInfoLayout(availWidth, availHeight, m.serverInfo.QRCode, m.renderInfoContent)
		}

		var content string
		var boxWidth int

		if m.showQR && m.serverInfo != nil {
			// Center the QR code inside the box
			qrRaw := strings.TrimSpace(m.serverInfo.QRCode)
			boxWidth = m.Width - 4

			// Use lipgloss to center the text block horizontally
			content = lipgloss.NewStyle().
				Width(boxWidth - 2).
				Align(lipgloss.Center).
				Render(qrRaw)
		} else {
			// Center box, left-aligned text
			boxWidth = 50
			if m.Width < 54 {
				boxWidth = m.Width - 4
			}
			content = m.renderInfoContent(boxWidth - 2)
		}

		boxContent := BoxStyle.Width(boxWidth).Render(content)
		return lipgloss.Place(m.Width, availHeight, lipgloss.Center, lipgloss.Center, boxContent)
	})
}

func (m SendModel) renderInfoContent(width int) string {
	var b strings.Builder
	if width < 0 {
		width = 0
	}

	// Helper for left-aligned text within the fixed width block
	style := lipgloss.NewStyle().Align(lipgloss.Left)
	if width > 0 {
		style = style.Width(width)
	}
	left := style.Render

	labelStyle := GetLabelStyle()
	successStyle := GetSuccessStyle()

	var fileList []string
	if m.executor != nil && m.executor.Server() != nil {
		srv := m.executor.Server()
		if srv.SrcPath != "" {
			fi, err := os.Stat(srv.SrcPath)
			if err == nil && fi.IsDir() {
				entries, _ := os.ReadDir(srv.SrcPath)
				for _, e := range entries {
					name := e.Name()
					if e.IsDir() {
						name = "📁 " + name
					} else {
						name = "📄 " + name
					}
					fileList = append(fileList, name)
				}
			} else {
				fileList = append(fileList, "📄 "+filepath.Base(srv.SrcPath))
			}
		} else if srv.TextContent != "" {
			size := ui.FormatBytes(int64(len(srv.TextContent)))
			fileList = append(fileList, "📝 Text Snippet ("+size+")")
		}
	} else if m.uploadTarget != "" {
		fileList = append(fileList, "📄 "+filepath.Base(m.uploadTarget))
	}

	b.WriteString(labelStyle.Render("Sharing") + "\n")
	maxFiles := 5
	for i, fname := range fileList {
		if i >= maxFiles {
			b.WriteString(left(fmt.Sprintf("... and %d more", len(fileList)-maxFiles)) + "\n")
			break
		}
		b.WriteString(left(fname) + "\n")
	}
	b.WriteString("\n")

	if m.serverInfo != nil {
		b.WriteString(labelStyle.Render("PAKE Code") + "\n")
		b.WriteString(left(m.serverInfo.Code) + "\n\n")

		b.WriteString(labelStyle.Render("URL") + "\n")
		b.WriteString(left(m.serverInfo.URL) + "\n\n")
	}

	encryptStatus := "✓ ENABLED"
	statusColor := successStyle
	if m.Options.NoEncrypt {
		encryptStatus := "✗ DISABLED"
		statusColor = lipgloss.NewStyle().Foreground(WarningColor)
		_ = encryptStatus
	}
	b.WriteString(statusColor.Render(fmt.Sprintf("Encryption: %s", encryptStatus)) + "\n\n")

	b.WriteString(labelStyle.Render("Instructions") + "\n")
	b.WriteString(left("1. Open URL on receiver") + "\n")
	b.WriteString(left("2. Enter PAKE code") + "\n")
	if !m.Options.NoEncrypt {
		b.WriteString(left("3. Transfer begins securely") + "\n")
	}

	return b.String()
}

func (m SendModel) renderCompleteView() string {
	return RenderScreen("UPLOAD COMPLETE", m.Width, m.Height, []string{"Esc: Back"}, func(availWidth, availHeight int) string {
		w := availWidth
		boxWidth := GetStandardBoxWidth(w)

		successStyle := GetSuccessStyle().Bold(true)

		var b strings.Builder
		filename := filepath.Base(m.uploadTarget)
		b.WriteString(successStyle.Render(CenterText(fmt.Sprintf("✓ Successfully uploaded: %s", filename), boxWidth-4)) + "\n\n")
		b.WriteString(CenterText("Returning to file browser...", boxWidth-4) + "\n")

		boxContent := BoxStyle.Width(boxWidth).Render(b.String())
		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, boxContent)
	})
}

func (m SendModel) renderErrorView() string {
	return RenderErrorScreen("UPLOAD ERROR", m.errorMsg, m.Width, m.Height)
}

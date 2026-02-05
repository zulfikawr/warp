// Package tui provides the terminal user interface for warp.
// This file contains the receive screen model for downloading files.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/cmd/warp/help"
	xferprogress "github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/resume"
	"github.com/zulfikawr/warp/internal/ui"
)

// receiveState represents the current state of the receive screen.
type receiveState int

const (
	stateInputCode receiveState = iota
	stateSearching
	stateDownloading
	stateRecvComplete
	stateRecvError
	stateRecvHelp
)

// ReceiveTransferState represents the state of a receive transfer
type ReceiveTransferState int

const (
	ReceiveStateActive ReceiveTransferState = iota
	ReceiveStatePaused
	ReceiveStateComplete
	ReceiveStateFailed
)

// ReceiveOptions contains CLI override options for the receive command.
type ReceiveOptions struct {
	Code       string
	OutputPath string
	Force      bool
	Workers    int
	ChunkSize  int
	NoChecksum bool
	Decrypt    bool
}

// ReceiveModel manages the receive screen state and rendering.
type ReceiveModel struct {
	state         receiveState
	transferState ReceiveTransferState
	codeInput     textinput.Model
	destInput     textinput.Model
	filePicker    FilePicker
	pickingFile   bool
	focusIndex    int // 0: code, 1: dest
	progress      progress.Model
	width         int
	height        int
	err           error
	statusMsg     string
	downloadInfo  xferprogress.Progress
	progChan      chan tea.Msg

	// Options from CLI
	Options ReceiveOptions

	// Resume checkpoint (if resuming)
	resumeCheckpoint *resume.CheckpointSummary

	// Help Screen
	helpScreen HelpScreen

	// Downloader reference for pause/resume
	downloader *TUIDownloader

	// Spinner for loading states
	spinner spinner.Model
}

// NewReceiveModel creates a new receive model.
func NewReceiveModel() ReceiveModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. 7-apple-velocity"
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 30
	ti.TextStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
	ti.PromptStyle = lipgloss.NewStyle().Foreground(PrimaryColor)

	destTi := textinput.New()
	wd, _ := os.Getwd()
	destTi.Placeholder = "Destination Folder"
	destTi.SetValue(wd)
	destTi.CharLimit = 200
	destTi.Width = 30
	destTi.TextStyle = lipgloss.NewStyle().Foreground(DimTextColor) // Initially dim
	destTi.PromptStyle = lipgloss.NewStyle().Foreground(DimTextColor)

	prog := progress.New(progress.WithDefaultGradient())

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(PrimaryColor)

	fp := NewFilePicker(wd)
	fp.DirOnly = true // Only pick directories for destination
	fp.Title = "SELECT A DESTINATION"

	return ReceiveModel{
		state:         stateInputCode,
		transferState: ReceiveStateActive,
		codeInput:     ti,
		destInput:     destTi,
		filePicker:    fp,
		focusIndex:    0,
		progress:      prog,
		width:         80,
		height:        24,
		helpScreen:    NewHelpScreen("RECEIVE HELP", help.ReceiveHelpLines),
		spinner:       s,
	}
}

// Init implements tea.Model.
func (m ReceiveModel) Init() tea.Cmd {
	// Initialize dest input if option provided
	if m.Options.OutputPath != "" {
		m.destInput.SetValue(m.Options.OutputPath)
	}

	// If resuming from checkpoint, show code input with resume context
	// The user needs to re-enter the PAKE code since it's not stored
	if m.resumeCheckpoint != nil {
		// Stay in stateInputCode but with resume context
		// The code input placeholder will indicate this is a resume
		m.codeInput.Placeholder = "Enter PAKE code to resume"
		m.destInput.SetValue(m.resumeCheckpoint.DestinationPath)
		return textinput.Blink
	}

	// If code is provided via options, start searching immediately
	if m.Options.Code != "" {
		m.codeInput.SetValue(m.Options.Code)
		return tea.Batch(textinput.Blink, func() tea.Msg {
			return tea.KeyMsg{Type: tea.KeyEnter}
		})
	}
	return textinput.Blink
}

// SetHelp sets the model to help state.
func (m *ReceiveModel) SetHelp() {
	m.state = stateRecvHelp
}

// SetResumeCheckpoint sets a checkpoint to resume from.
func (m *ReceiveModel) SetResumeCheckpoint(checkpoint *resume.CheckpointSummary) {
	m.resumeCheckpoint = checkpoint
}

// waitForProgress is a command that waits for messages on the channel.
func waitForProgress(sub chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

// Update implements tea.Model.
func (m ReceiveModel) Update(msg tea.Msg) (ReceiveModel, tea.Cmd, bool) {
	var cmd tea.Cmd
	quit := false

	// Handle File Picker
	if m.pickingFile {
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
			m.pickingFile = false
			return m, nil, false
		}

		subCmd, done := (&m.filePicker).Update(msg)
		if done {
			m.pickingFile = false
			// Get selected directory
			selectedPath := m.filePicker.Dir
			for _, e := range m.filePicker.Entries {
				if e.Selected {
					selectedPath = filepath.Join(m.filePicker.Dir, e.Name)
					break
				}
			}
			m.destInput.SetValue(selectedPath)
			m.focusIndex = 1
			m.updateFocus()
		}
		return m, subCmd, false
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit, false
		}

		if m.state != stateInputCode {
			if msg.String() == "?" {
				m.state = stateRecvHelp
				return m, nil, false
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.filePicker.Width = msg.Width
		m.filePicker.Height = msg.Height
		// Ensure progress bar fits in one line
		maxProgressWidth := msg.Width - 20
		if maxProgressWidth < 20 {
			maxProgressWidth = 20
		}
		m.progress.Width = min(maxProgressWidth, 60)
		m.helpScreen.SetSize(msg.Width, msg.Height)

	// Messages from Downloader
	case DownloadStatusMsg:
		m.statusMsg = string(msg)
		return m, waitForProgress(m.progChan), false

	case xferprogress.Progress:
		if msg.Error != nil {
			m.state = stateRecvError
			m.transferState = ReceiveStateFailed
			m.err = msg.Error
			return m, nil, false
		}
		if msg.IsComplete {
			m.state = stateRecvComplete
			m.transferState = ReceiveStateComplete
			m.downloadInfo = msg
			m.progress.SetPercent(1.0)
			return m, nil, false
		}

		// Update progress
		m.state = stateDownloading
		m.downloadInfo = msg

		// Sync transfer state with pause status
		if msg.IsPaused {
			m.transferState = ReceiveStatePaused
		} else {
			m.transferState = ReceiveStateActive
		}

		if msg.TotalBytes > 0 {
			pct := float64(msg.TransferredBytes) / float64(msg.TotalBytes)
			cmd = m.progress.SetPercent(pct)
			return m, tea.Batch(cmd, waitForProgress(m.progChan)), false
		}
		return m, waitForProgress(m.progChan), false

	// Progress bar ticker
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd, false
	}

	// State Machine Handlers
	switch m.state {
	case stateRecvHelp:
		h, shouldExit := m.helpScreen.Update(msg)
		m.helpScreen = h
		if shouldExit {
			// Return to previous state logic
			if m.downloadInfo.IsComplete {
				m.state = stateRecvComplete
			} else if m.statusMsg != "" {
				m.state = stateSearching
				if m.downloadInfo.TotalBytes > 0 || m.downloadInfo.TransferredBytes > 0 {
					m.state = stateDownloading
				}
			} else {
				m.state = stateInputCode
			}
			if m.err != nil {
				m.state = stateRecvError
			}
		}
		return m, nil, false

	case stateInputCode:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "esc":
				quit = true
			case "tab", "shift+tab":
				m.focusIndex = (m.focusIndex + 1) % 2
				m.updateFocus()
				return m, nil, false
			case "ctrl+f":
				m.pickingFile = true
				// Initialize picker with current input value if valid dir, else cwd
				currentVal := m.destInput.Value()
				if info, err := os.Stat(currentVal); err == nil && info.IsDir() {
					m.filePicker.Dir = currentVal
				} else {
					wd, _ := os.Getwd()
					m.filePicker.Dir = wd
				}
				m.filePicker.LoadEntries()
				m.filePicker.DirOnly = true
				return m, nil, false
			case "enter":
				// If focused on code and it's valid, move to dest
				// If focused on dest (or code + dest valid), start
				code := m.codeInput.Value()

				if m.focusIndex == 0 && code != "" {
					m.focusIndex = 1
					m.updateFocus()
					return m, nil, false
				}

				if code != "" {
					m.state = stateSearching
					m.progChan = make(chan tea.Msg)
					m.downloader = NewTUIDownloader()
					// Set output path in downloader options
					m.Options.OutputPath = m.destInput.Value()

					// Check if this is a resume operation
					if m.resumeCheckpoint != nil {
						m.statusMsg = "Resuming transfer..."
						go m.downloader.ResumeFromCheckpoint(m.resumeCheckpoint.SessionID, code, m.progChan)
					} else {
						m.statusMsg = "Initializing..."
						go m.downloader.Receive(code, m.Options.OutputPath, m.progChan)
					}

					return m, tea.Batch(waitForProgress(m.progChan), m.spinner.Tick), false
				}
			case "?":
				m.state = stateRecvHelp
				return m, nil, false
			}
		}

		var cmd tea.Cmd
		if m.focusIndex == 0 {
			m.codeInput, cmd = m.codeInput.Update(msg)
		} else {
			m.destInput, cmd = m.destInput.Update(msg)
		}
		return m, cmd, quit

	case stateRecvError, stateRecvComplete:
		if msg, ok := msg.(tea.KeyMsg); ok {
			if msg.String() == "esc" {
				m.state = stateInputCode
				m.codeInput.Reset()
				m.destInput.Blur()
				m.focusIndex = 0
				m.updateFocus()
				m.err = nil
				m.statusMsg = ""
				m.downloadInfo = xferprogress.Progress{}
				m.transferState = ReceiveStateActive
				return m, nil, false
			}
			if msg.String() == "enter" {
				m.state = stateInputCode
				m.codeInput.Reset()
				m.destInput.Blur()
				m.focusIndex = 0
				m.updateFocus()
				m.err = nil
				m.statusMsg = ""
				m.downloadInfo = xferprogress.Progress{}
				m.transferState = ReceiveStateActive
				return m, nil, false
			}
		}

	case stateSearching, stateDownloading:
		var scmd tea.Cmd
		m.spinner, scmd = m.spinner.Update(msg)

		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "esc":
				// Go back to input code screen
				m.state = stateInputCode
				m.codeInput.Reset()
				m.focusIndex = 0
				m.updateFocus()
				m.statusMsg = ""
				m.downloadInfo = xferprogress.Progress{}
				m.transferState = ReceiveStateActive
				return m, nil, false
			case " ": // Space key for pause/resume
				if m.state == stateDownloading && m.downloader != nil {
					if m.transferState == ReceiveStatePaused {
						_ = m.downloader.Resume()
						m.transferState = ReceiveStateActive
						m.downloadInfo.IsPaused = false
					} else {
						_ = m.downloader.Pause()
						m.transferState = ReceiveStatePaused
						m.downloadInfo.IsPaused = true
					}
					return m, waitForProgress(m.progChan), false
				}
			}
		}
		return m, scmd, false
	}

	return m, nil, quit
}

func (m *ReceiveModel) updateFocus() {
	if m.focusIndex == 0 {
		m.codeInput.Focus()
		m.codeInput.TextStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
		m.codeInput.PromptStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
		m.destInput.Blur()
		m.destInput.TextStyle = lipgloss.NewStyle().Foreground(DimTextColor)
		m.destInput.PromptStyle = lipgloss.NewStyle().Foreground(DimTextColor)
	} else {
		m.codeInput.Blur()
		m.codeInput.TextStyle = lipgloss.NewStyle().Foreground(DimTextColor)
		m.codeInput.PromptStyle = lipgloss.NewStyle().Foreground(DimTextColor)
		m.destInput.Focus()
		m.destInput.TextStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
		m.destInput.PromptStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
	}
}

// View implements tea.Model.
func (m ReceiveModel) View() string {
	if m.state == stateRecvHelp {
		return m.helpScreen.View()
	}

	if m.pickingFile {
		return m.filePicker.View()
	}

	footerItems := []string{
		"[Esc] Back",
		"[?] Help",
	}
	if m.state == stateInputCode {
		footerItems = append([]string{"[⏎] Confirm", "[Tab] Switch", "[Ctrl+F] Browse"}, footerItems...)
	}
	if m.state == stateDownloading {
		if m.transferState == ReceiveStatePaused {
			footerItems = append([]string{"[Space] Resume"}, footerItems...)
		} else {
			footerItems = append([]string{"[Space] Pause"}, footerItems...)
		}
	}

	return RenderScreen("WARP RECEIVE", m.width, m.height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth
		var content string

		labelStyle := GetLabelStyle()
		successStyle := GetSuccessStyle().Bold(true)

		titleStyle := BoxTitleStyle

		switch m.state {
		case stateInputCode:
			titleText := "Enter PAKE code and destination"
			helperText := ""

			// Show resume context if resuming
			if m.resumeCheckpoint != nil {
				titleText = "Resume Download"
				filename := filepath.Base(m.resumeCheckpoint.DestinationPath)
				if filename == "" || filename == "." {
					filename = filepath.Base(m.resumeCheckpoint.SourcePath)
				}
				helperText = fmt.Sprintf("Resuming: %s (%.0f%% complete)\nEnter the PAKE code from the sender", filename, m.resumeCheckpoint.Progress*100)
			}

			// Render inputs
			codeView := m.codeInput.View()
			destView := m.destInput.View()

			inputForm := lipgloss.JoinVertical(lipgloss.Left,
				labelStyle.Render("PAKE Code"),
				codeView,
				" ",
				labelStyle.Render("Destination"),
				destView,
			)

			// Calculate widths for separator
			maxRowLength := len(titleText)
			if m.width > 40 {
				maxRowLength = 40
			}

			// Adjust maxRowLength to fit box
			if maxRowLength < 30 {
				maxRowLength = 30
			}
			if maxRowLength > w-4 {
				maxRowLength = w - 4
			}

			separator := lipgloss.NewStyle().Foreground(PrimaryColor).Render(strings.Repeat("-", maxRowLength))

			innerContent := lipgloss.JoinVertical(
				lipgloss.Left,
				titleStyle.Render(titleText),
				separator,
				"\n"+inputForm+"\n",
			)

			if helperText != "" {
				innerContent = lipgloss.JoinVertical(
					lipgloss.Left,
					innerContent,
					DimStyle.Render(helperText),
				)
			}

			menuBox := BoxStyle.Render(innerContent)

			content = lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, menuBox)

		case stateSearching:
			content = lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				m.spinner.View()+" "+labelStyle.Render(m.statusMsg))

		case stateDownloading:
			var b strings.Builder
			boxWidth := GetStandardBoxWidth(w)
			contentWidth := boxWidth - 6

			// Truncate filename if needed
			filename := m.downloadInfo.FileName
			maxFilenameLen := contentWidth - 15 // Account for "📥 Downloading: " prefix
			if len(filename) > maxFilenameLen && maxFilenameLen > 3 {
				filename = TruncateWithEllipsis(filename, maxFilenameLen)
			}

			statusText := "📥 Downloading"
			if m.downloadInfo.IsResumable {
				statusText += " (Resumable)"
			}
			if m.downloadInfo.IsPaused {
				statusText = "⏸ Paused"
			}

			b.WriteString(labelStyle.Render(CenterText(fmt.Sprintf("%s: %s", statusText, filename), contentWidth)) + "\n\n")

			// Show resume info if applicable
			if m.downloadInfo.ResumedFrom > 0 {
				resumeInfo := fmt.Sprintf("Resumed from: %.1f%%", m.downloadInfo.ResumedFrom)
				b.WriteString(DimStyle.Render(CenterText(resumeInfo, contentWidth)) + "\n")
			}

			// Create progress bar with yellow color for active, gray for paused
			progressStyle := lipgloss.NewStyle().Foreground(WarningColor)
			if m.downloadInfo.IsPaused {
				progressStyle = DimStyle
			}

			// Calculate progress percentage
			percent := 0.0
			if m.downloadInfo.TotalBytes > 0 {
				percent = float64(m.downloadInfo.TransferredBytes) / float64(m.downloadInfo.TotalBytes)
			}

			// Render progress bar ensuring it fits in one line
			progressWidth := contentWidth - 10
			if progressWidth < 10 {
				progressWidth = 10
			}
			progBar := renderProgressBar(percent, progressWidth, progressStyle)
			b.WriteString(CenterText(progBar, contentWidth) + "\n\n")

			// Build stats line with size, speed, and ETA
			var statsLine string
			if m.downloadInfo.TotalBytes > 0 {
				received := ui.FormatBytes(m.downloadInfo.TransferredBytes)
				total := ui.FormatBytes(m.downloadInfo.TotalBytes)

				if m.downloadInfo.SpeedBytesPerSec > 0 && !m.downloadInfo.IsPaused {
					speedStr := ui.FormatSpeed(m.downloadInfo.SpeedBytesPerSec)
					if m.downloadInfo.ETA > 0 && !m.downloadInfo.IsComplete {
						etaStr := ui.FormatDuration(m.downloadInfo.ETA)
						statsLine = fmt.Sprintf("%s / %s | %s | ETA: %s", received, total, speedStr, etaStr)
					} else {
						statsLine = fmt.Sprintf("%s / %s | %s", received, total, speedStr)
					}
				} else {
					statsLine = fmt.Sprintf("%s / %s", received, total)
				}
			} else {
				statsLine = ui.FormatBytes(m.downloadInfo.TransferredBytes)
			}

			// Show chunk progress if available
			if m.downloadInfo.TotalChunks > 0 {
				chunkInfo := fmt.Sprintf(" (%d/%d chunks)", m.downloadInfo.CompletedChunks, m.downloadInfo.TotalChunks)
				if len(statsLine)+len(chunkInfo) <= contentWidth {
					statsLine += chunkInfo
				}
			}

			// Truncate stats line if too long
			if len(statsLine) > contentWidth {
				statsLine = TruncateWithEllipsis(statsLine, contentWidth)
			}
			b.WriteString(CenterText(statsLine, contentWidth))

			// Show pause/resume hint if resumable
			if m.downloadInfo.IsResumable {
				hint := "\n\n" + DimStyle.Render(CenterText("Interrupt with Ctrl+C to pause - resume later", contentWidth))
				b.WriteString(hint)
			}

			boxContent := BoxStyle.Width(boxWidth).Render(b.String())
			content = lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, boxContent)

		case stateRecvComplete:
			var b strings.Builder
			boxWidth := GetStandardBoxWidth(w)
			contentWidth := boxWidth - 6

			b.WriteString(successStyle.Render(CenterText("✓ Download Complete", contentWidth)) + "\n\n")

			centerLine := func(s string) string {
				return CenterText(s, contentWidth) + "\n"
			}

			// Truncate filename if needed
			filename := m.downloadInfo.FileName
			maxFilenameLen := contentWidth - 12
			if len(filename) > maxFilenameLen && maxFilenameLen > 3 {
				filename = TruncateWithEllipsis(filename, maxFilenameLen)
			}

			b.WriteString(centerLine(fmt.Sprintf("File:      %s", filename)))
			b.WriteString(centerLine(fmt.Sprintf("Size:      %s", ui.FormatBytes(m.downloadInfo.TotalBytes))))

			if m.downloadInfo.SpeedBytesPerSec > 0 {
				elapsedSec := float64(m.downloadInfo.TotalBytes) / m.downloadInfo.SpeedBytesPerSec
				b.WriteString(centerLine(fmt.Sprintf("Time:      %.1fs", elapsedSec)))
			}

			// Truncate saved path if needed
			savedPath := m.downloadInfo.SavedPath
			maxPathLen := contentWidth - 12
			if len(savedPath) > maxPathLen && maxPathLen > 3 {
				savedPath = TruncateWithEllipsis(savedPath, maxPathLen)
			}
			b.WriteString(centerLine(fmt.Sprintf("Saved to:  %s", savedPath)))
			if m.downloadInfo.Verified {
				b.WriteString(centerLine("✓ Checksum:  Verified"))
			}

			b.WriteString("\n" + CenterText("Press ENTER to receive another or ESC to return", contentWidth))

			boxContent := BoxStyle.Width(boxWidth).Render(b.String())
			content = lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, boxContent)

		case stateRecvError:
			content = m.renderError()
		}

		return content
	})
}

func (m ReceiveModel) renderError() string {
	return RenderErrorScreen("RECEIVE ERROR", m.err.Error(), m.width, m.height)
}

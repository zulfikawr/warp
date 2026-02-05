// Package tui provides the terminal user interface for warp.
// This file contains the host screen model for receiving files.
package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/internal/core"
	xferprogress "github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/ui"
)

// hostState represents the current state of the host screen.
type hostState int

const (
	stateHostStarting hostState = iota
	stateHostRunning
	stateHostTransferring
	stateHostSummary
	stateHostError
	stateHostHelp
)

// hostStartedMsg is sent when the host server starts successfully.
type hostStartedMsg struct {
	info     *core.ServerInfo
	executor *core.HostExecutor
}

// hostErrorMsg is sent when the host server fails to start.
type hostErrorMsg struct {
	err error
}

// HostOptions contains CLI override options for the host command.
type HostOptions struct {
	InterfaceName string
	DestDir       string
	RateLimitMbps float64
	NoQR          bool
	NoEncrypt     bool
}

// TransferState represents the state of a transfer
type TransferState int

const (
	TransferStateActive TransferState = iota
	TransferStatePaused
	TransferStateComplete
	TransferStateFailed
)

// FileTransferInfo extends TransferProgress with additional state
type FileTransferInfo struct {
	xferprogress.Progress
	State TransferState

	// Resume support (extends base Progress type)
	ChunksComplete int
	ChunksTotal    int
}

// HostModel manages the host screen state and rendering.
type HostModel struct {
	Width       int
	Height      int
	state       hostState
	serverInfo  *core.ServerInfo
	errorMsg    string
	executor    *core.HostExecutor
	showQR      bool
	showSummary bool

	// Progress tracking
	progress       progress.Model
	filesStatus    map[string]*FileTransferInfo
	filesOrder     []string // Track order of files for consistent display
	filesMu        *sync.Mutex
	lastFile       string // Currently transferring file
	currentFileIdx int    // Current file index for pagination (0 = combined view, 1+ = individual files)

	// Help Screen
	helpScreen HelpScreen

	// CLI Override Options
	Options HostOptions

	// Spinner for loading states
	spinner spinner.Model
}

// NewHostModel creates a new host model.
func NewHostModel() *HostModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(PrimaryColor)

	return &HostModel{
		state:          stateHostStarting,
		showQR:         false,
		showSummary:    false,
		progress:       progress.New(progress.WithDefaultGradient()),
		filesStatus:    make(map[string]*FileTransferInfo),
		filesOrder:     make([]string, 0),
		filesMu:        &sync.Mutex{},
		currentFileIdx: 0, // 0 = combined view
		spinner:        s,
	}
}

// Init implements tea.Model.
func (m *HostModel) Init() tea.Cmd {
	return m.startHost()
}

// SetHelp sets the model to help state.
func (m *HostModel) SetHelp() {
	m.state = stateHostHelp
}

// startHost creates and starts the host executor using the core package.
func (m *HostModel) startHost() tea.Cmd {
	m.state = stateHostStarting
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			// Convert TUI options to core options
			opts := core.HostOptions{
				InterfaceName: m.Options.InterfaceName,
				DestDir:       m.Options.DestDir,
				RateLimitMbps: m.Options.RateLimitMbps,
				NoQR:          m.Options.NoQR,
				NoEncrypt:     m.Options.NoEncrypt,
			}

			// Create executor with no callbacks (TUI handles display)
			executor := core.NewHostExecutor(opts, nil, nil)

			// Start the server
			info, err := executor.Start(context.Background())
			if err != nil {
				return hostErrorMsg{err: err}
			}

			return hostStartedMsg{
				info:     info,
				executor: executor,
			}
		},
	)
}

// Update implements tea.Model.
func (m *HostModel) Update(msg tea.Msg) (*HostModel, tea.Cmd, bool) {
	quit := false
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.executor != nil {
				_ = m.executor.Stop()
			}
			return m, tea.Quit, false
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		// Ensure progress bar fits in one line with padding
		maxProgressWidth := msg.Width - 30
		if maxProgressWidth < 20 {
			maxProgressWidth = 20
		}
		m.progress.Width = maxProgressWidth
	}

	if m.state == stateHostStarting {
		var scmd tea.Cmd
		m.spinner, scmd = m.spinner.Update(msg)
		cmd = tea.Batch(cmd, scmd)
	}

	switch msg := msg.(type) {
	case hostStartedMsg:
		m.state = stateHostRunning
		m.serverInfo = msg.info
		m.executor = msg.executor
		return m, m.waitForProgress(), false
	case hostErrorMsg:
		m.state = stateHostError
		m.errorMsg = msg.err.Error()
		return m, nil, false
	case xferprogress.Progress:
		// Handle progress update
		if msg.FileName != "" {
			m.filesMu.Lock()
			// Check if this is a new file
			if _, exists := m.filesStatus[msg.FileName]; !exists {
				m.filesOrder = append(m.filesOrder, msg.FileName)
			}

			// Determine transfer state
			state := TransferStateActive
			if msg.IsComplete {
				state = TransferStateComplete
			} else if msg.IsPaused {
				state = TransferStatePaused
			}

			// Calculate chunks if available (5MB chunks)
			chunksComplete := 0
			chunksTotal := 0
			if msg.TotalBytes > 0 {
				chunkSize := int64(5 * 1024 * 1024)
				chunksTotal = int((msg.TotalBytes + chunkSize - 1) / chunkSize)
				if msg.TransferredBytes > 0 {
					chunksComplete = int(msg.TransferredBytes / chunkSize)
				}
			}

			m.filesStatus[msg.FileName] = &FileTransferInfo{
				Progress: xferprogress.Progress{
					FileName:         msg.FileName,
					TransferredBytes: msg.TransferredBytes,
					TotalBytes:       msg.TotalBytes,
					IsComplete:       msg.IsComplete,
					SpeedBytesPerSec: msg.SpeedBytesPerSec,
					ETA:              msg.ETA,
					StartTime:        msg.StartTime,
					IsPaused:         msg.IsPaused,
				},
				State:          state,
				ChunksComplete: chunksComplete,
				ChunksTotal:    chunksTotal,
			}
			m.lastFile = msg.FileName
			m.filesMu.Unlock()

			if msg.TotalBytes > 0 {
				ratio := float64(msg.TransferredBytes) / float64(msg.TotalBytes)
				cmd = m.progress.SetPercent(ratio)
			}
		}

		if m.state != stateHostSummary && m.state != stateHostHelp {
			m.state = stateHostTransferring
		}

		// Wait for next progress message
		return m, tea.Batch(cmd, m.waitForProgress()), false
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd, false
	}

	switch m.state {
	case stateHostRunning, stateHostTransferring, stateHostSummary:
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "esc":
				// In summary or transferring, go back to QR/info screen
				if m.state == stateHostSummary || m.state == stateHostTransferring {
					m.state = stateHostRunning
					return m, nil, false
				}
				// In running state, stop the server
				if m.executor != nil {
					_ = m.executor.Stop()
					m.executor = nil
				}
				return m, nil, true
			case "?":
				m.state = stateHostHelp
				return m, nil, false
			case "q":
				m.showQR = !m.showQR
			case "s":
				// Toggle summary with 's'
				if m.state == stateHostSummary {
					m.state = stateHostRunning
					if len(m.filesStatus) > 0 {
						m.state = stateHostTransferring
					}
				} else {
					m.state = stateHostSummary
				}
			case "left":
				// Previous view (combined or individual file)
				m.filesMu.Lock()
				if m.currentFileIdx > 0 {
					m.currentFileIdx--
				}
				m.filesMu.Unlock()
			case "right":
				// Next view (individual files)
				m.filesMu.Lock()
				// Max index is len(filesOrder) since 0 = combined view
				if m.currentFileIdx < len(m.filesOrder) {
					m.currentFileIdx++
				}
				m.filesMu.Unlock()
			}
		}

	case stateHostHelp:
		h, shouldExit := m.helpScreen.Update(msg)
		m.helpScreen = h
		if shouldExit {
			if len(m.filesStatus) > 0 {
				m.state = stateHostTransferring
			} else if m.executor != nil {
				m.state = stateHostRunning
			} else {
				m.state = stateHostStarting
			}
		}
		return m, nil, false

	case stateHostError:
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
			return m, nil, true
		}
	}

	return m, cmd, quit
}

// waitForProgress waits for progress updates from the server.
func (m *HostModel) waitForProgress() tea.Cmd {
	if m.executor == nil || m.executor.Server() == nil || m.executor.Server().ProgressChan == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-m.executor.Server().ProgressChan
		if !ok {
			return nil
		}
		return msg
	}
}

// View implements tea.Model.
func (m *HostModel) View() string {
	switch m.state {
	case stateHostStarting:
		return m.renderStarting()
	case stateHostRunning:
		return m.renderRunning()
	case stateHostTransferring:
		return m.renderTransferring()
	case stateHostSummary:
		return m.renderSummary()
	case stateHostError:
		return m.renderError()
	case stateHostHelp:
		return m.helpScreen.View()
	}
	return ""
}

func (m *HostModel) renderRunning() string {
	isWide := m.Width >= MinSplitLayoutWidth

	qrToggle := "[Q] Show QR"
	if isWide {
		qrToggle = ""
	} else if m.showQR {
		qrToggle = "[Q] Hide QR"
	}

	footerItems := []string{
		"[S] Summary",
		"[?] Help",
		"[Esc] Back",
	}
	if qrToggle != "" {
		footerItems = append([]string{qrToggle}, footerItems...)
	}

	return RenderScreen("WARP HOST", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		if isWide && m.serverInfo != nil {
			return RenderQRInfoLayout(availWidth, availHeight, m.serverInfo.QRCode, m.renderInfoContent)
		}

		var content string
		var boxWidth int

		if m.showQR && m.serverInfo != nil {
			qrRaw := strings.TrimSpace(m.serverInfo.QRCode)
			boxWidth = m.Width - 4
			content = lipgloss.NewStyle().
				Width(boxWidth - 2).
				Align(lipgloss.Center).
				Render(qrRaw)
		} else {
			boxWidth = 50
			if m.Width < 54 {
				boxWidth = m.Width - 4
			}
			content = m.renderInfoContent(boxWidth - 2)
		}

		boxContent := BoxStyle.Width(boxWidth).Render(content)
		return lipgloss.Place(m.Width-2, availHeight, lipgloss.Center, lipgloss.Center, boxContent)
	})
}

func (m *HostModel) renderInfoContent(width int) string {
	var b strings.Builder
	if width < 0 {
		width = 0
	}

	style := lipgloss.NewStyle().Align(lipgloss.Left)
	if width > 0 {
		style = style.Width(width)
	}
	left := style.Render

	labelStyle := GetLabelStyle()
	successStyle := GetSuccessStyle()

	b.WriteString(labelStyle.Render("Status") + "\n")
	statusStr := "Listening (HTTP/1.1 + QUIC)"
	if m.serverInfo != nil && len(m.serverInfo.Protocols) > 0 {
		statusStr = fmt.Sprintf("Listening (%s)", strings.Join(m.serverInfo.Protocols, " + "))
	}
	b.WriteString(left(statusStr) + "\n\n")

	if m.serverInfo != nil {
		b.WriteString(labelStyle.Render("PAKE Code") + "\n")
		b.WriteString(left(m.serverInfo.Code) + "\n\n")

		b.WriteString(labelStyle.Render("URL") + "\n")
		b.WriteString(left(m.serverInfo.URL) + "\n\n")
	}

	encryptStatus := "✓ ENABLED"
	statusColor := successStyle
	if m.Options.NoEncrypt {
		encryptStatus = "✗ DISABLED"
		statusColor = lipgloss.NewStyle().Foreground(WarningColor)
	}
	b.WriteString(statusColor.Render(fmt.Sprintf("Encryption: %s", encryptStatus)) + "\n\n")

	b.WriteString(labelStyle.Render("Instructions") + "\n")
	b.WriteString(left("1. Share code/URL with sender") + "\n")
	b.WriteString(left("2. Wait for incoming connection") + "\n")

	return b.String()
}

func (m *HostModel) renderStarting() string {
	footerItems := []string{
		"[S] Summary",
		"[Esc] Back",
	}

	return RenderScreen("WARP HOST", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		var b strings.Builder
		w := availWidth
		boxWidth := GetStandardBoxWidth(w)

		b.WriteString("\n" + CenterText(m.spinner.View()+" Starting server...", boxWidth-4) + "\n")

		boxContent := BoxStyle.Width(boxWidth).Render(b.String())
		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, boxContent)
	})
}

func (m *HostModel) renderTransferring() string {
	footerItems := []string{
		"[←→] Navigate",
		"[S] Summary",
		"[Esc] Back",
	}

	return RenderScreen("WARP HOST", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth
		boxWidth := GetStandardBoxWidth(w)
		contentWidth := boxWidth - 6 // Account for box borders and padding

		m.filesMu.Lock()
		totalFiles := len(m.filesOrder)
		currentIdx := m.currentFileIdx

		// Calculate combined stats
		var totalBytesAll, receivedBytesAll int64
		var combinedSpeed float64
		var completeCount int
		for _, info := range m.filesStatus {
			totalBytesAll += info.TotalBytes
			receivedBytesAll += info.TransferredBytes
			combinedSpeed += info.SpeedBytesPerSec
			// Check if file is complete
			if info.IsComplete || (info.TotalBytes > 0 && info.TransferredBytes >= info.TotalBytes) {
				completeCount++
			}
		}
		m.filesMu.Unlock()

		// Determine title text based on view
		var titleCol1, titleCol2 string
		if currentIdx == 0 {
			// Combined view
			if completeCount == totalFiles && totalFiles > 0 {
				titleCol1 = "COMPLETE"
				titleCol2 = fmt.Sprintf("%d file(s) transferred", totalFiles)
			} else {
				titleCol1 = "RECEIVING"
				titleCol2 = fmt.Sprintf("%d file(s) in progress", totalFiles)
			}
		} else {
			// Individual file view
			fileIdx := currentIdx - 1
			m.filesMu.Lock()
			var filename string
			if fileIdx >= 0 && fileIdx < len(m.filesOrder) {
				filename = filepath.Base(m.filesOrder[fileIdx])
			}
			m.filesMu.Unlock()
			maxNameLen := contentWidth - 20
			if maxNameLen < 10 {
				maxNameLen = 10
			}
			if len(filename) > maxNameLen {
				filename = TruncateWithEllipsis(filename, maxNameLen)
			}
			titleCol1 = filename
			titleCol2 = fmt.Sprintf("File %d of %d", currentIdx, totalFiles)
		}

		// Build title with proper width
		titleText := fmt.Sprintf("%s | %s", titleCol1, titleCol2)

		// Separator matches content width
		separator := lipgloss.NewStyle().Foreground(PrimaryColor).Render(strings.Repeat("-", contentWidth))

		// Build content based on view
		var contentStr string
		if currentIdx == 0 {
			// Combined view - show all files progress combined
			contentStr = m.renderCombinedProgress(contentWidth, totalBytesAll, receivedBytesAll, combinedSpeed, totalFiles, completeCount)
		} else {
			// Individual file view
			fileIdx := currentIdx - 1
			contentStr = m.renderIndividualProgress(contentWidth, fileIdx)
		}

		// Render the box with title
		menuBox := BoxStyle.Width(boxWidth).Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				BoxTitleStyle.Render(titleText),
				separator,
				contentStr,
			),
		)

		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, menuBox)
	})
}

// renderCombinedProgress renders the combined progress for all files
func (m *HostModel) renderCombinedProgress(contentWidth int, totalBytes, receivedBytes int64, speed float64, fileCount, completeCount int) string {
	var b strings.Builder

	// Calculate combined progress
	percent := 0.0
	if totalBytes > 0 {
		percent = float64(receivedBytes) / float64(totalBytes)
	}

	// Ensure 100% when all complete
	allComplete := completeCount == fileCount && fileCount > 0
	if allComplete {
		percent = 1.0
	}

	// Progress bar color
	progressStyle := lipgloss.NewStyle().Foreground(WarningColor)
	if allComplete || percent >= 1.0 {
		progressStyle = lipgloss.NewStyle().Foreground(SuccessColor)
	}

	progressWidth := contentWidth - 10
	if progressWidth < 10 {
		progressWidth = 10
	}
	progBar := renderProgressBar(percent, progressWidth, progressStyle)
	b.WriteString("\n" + CenterText(progBar, contentWidth) + "\n\n")

	// Stats line
	received := ui.FormatBytes(receivedBytes)
	total := ui.FormatBytes(totalBytes)

	var statsLine string
	if allComplete {
		statsLine = fmt.Sprintf("%s / %s | Complete", received, total)
	} else if speed > 0 {
		speedStr := ui.FormatSpeed(speed)
		var eta time.Duration
		if speed > 0 && totalBytes > receivedBytes {
			remaining := totalBytes - receivedBytes
			eta = time.Duration(float64(remaining) / speed * float64(time.Second))
		}
		if eta > 0 {
			etaStr := ui.FormatDuration(eta)
			statsLine = fmt.Sprintf("%s / %s | %s | ETA: %s", received, total, speedStr, etaStr)
		} else {
			statsLine = fmt.Sprintf("%s / %s | %s", received, total, speedStr)
		}
	} else {
		statsLine = fmt.Sprintf("%s / %s", received, total)
	}

	if len(statsLine) > contentWidth {
		statsLine = TruncateWithEllipsis(statsLine, contentWidth)
	}
	b.WriteString(CenterText(statsLine, contentWidth) + "\n")

	// Hint for navigation
	if fileCount > 1 {
		hint := DimStyle.Render("Press → to view individual files")
		b.WriteString("\n" + CenterText(hint, contentWidth))
	}

	return b.String()
}

// renderIndividualProgress renders progress for a single file
func (m *HostModel) renderIndividualProgress(contentWidth int, fileIdx int) string {
	var b strings.Builder

	m.filesMu.Lock()
	var status *FileTransferInfo
	if fileIdx >= 0 && fileIdx < len(m.filesOrder) {
		status = m.filesStatus[m.filesOrder[fileIdx]]
	}
	m.filesMu.Unlock()

	if status == nil {
		b.WriteString(CenterText("No file data", contentWidth))
		return b.String()
	}

	// Check if complete
	isComplete := status.IsComplete || (status.TotalBytes > 0 && status.TransferredBytes >= status.TotalBytes)
	isPaused := status.State == TransferStatePaused

	// Progress bar with appropriate color
	var progressStyle lipgloss.Style
	if isComplete {
		progressStyle = lipgloss.NewStyle().Foreground(SuccessColor)
	} else if status.State == TransferStateFailed {
		progressStyle = lipgloss.NewStyle().Foreground(ErrorColor)
	} else if isPaused {
		progressStyle = DimStyle
	} else {
		progressStyle = lipgloss.NewStyle().Foreground(WarningColor)
	}

	percent := 0.0
	if status.TotalBytes > 0 {
		percent = float64(status.TransferredBytes) / float64(status.TotalBytes)
	}
	if isComplete {
		percent = 1.0
	}

	progressWidth := contentWidth - 10
	if progressWidth < 10 {
		progressWidth = 10
	}
	progBar := renderProgressBar(percent, progressWidth, progressStyle)
	b.WriteString("\n" + CenterText(progBar, contentWidth) + "\n\n")

	// Stats line
	received := ui.FormatBytes(status.TransferredBytes)
	total := ui.FormatBytes(status.TotalBytes)

	var statsLine string
	if isComplete {
		statsLine = fmt.Sprintf("%s / %s | Complete", received, total)
	} else if isPaused {
		statsLine = fmt.Sprintf("%s / %s | Paused", received, total)
	} else if status.SpeedBytesPerSec > 0 {
		speedStr := ui.FormatSpeed(status.SpeedBytesPerSec)
		if status.ETA > 0 {
			etaStr := ui.FormatDuration(status.ETA)
			statsLine = fmt.Sprintf("%s / %s | %s | ETA: %s", received, total, speedStr, etaStr)
		} else {
			statsLine = fmt.Sprintf("%s / %s | %s", received, total, speedStr)
		}
	} else {
		statsLine = fmt.Sprintf("%s / %s", received, total)
	}

	// Show chunk progress if available and resumable
	if status.IsResumable && status.ChunksTotal > 0 {
		chunkInfo := fmt.Sprintf(" (%d/%d chunks)", status.ChunksComplete, status.ChunksTotal)
		if len(statsLine)+len(chunkInfo) <= contentWidth {
			statsLine += chunkInfo
		}
	}

	if len(statsLine) > contentWidth {
		statsLine = TruncateWithEllipsis(statsLine, contentWidth)
	}
	b.WriteString(CenterText(statsLine, contentWidth) + "\n")

	// Show resumable indicator
	if status.IsResumable && !isComplete {
		hint := DimStyle.Render("Resumable transfer (checkpoints saved)")
		b.WriteString("\n" + CenterText(hint, contentWidth))
	}

	// Hint for navigation
	hint := DimStyle.Render("Press ← for combined view")
	b.WriteString("\n" + CenterText(hint, contentWidth))

	return b.String()
}

func (m *HostModel) renderSummary() string {
	footerItems := []string{
		"[S] Back",
		"[Esc] Back to QR",
	}

	return RenderScreen("TRANSFER SUMMARY", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth
		boxWidth := GetStandardBoxWidth(w)
		contentWidth := boxWidth - 6

		m.filesMu.Lock()
		fileCount := len(m.filesStatus)
		m.filesMu.Unlock()

		// Empty state - no border
		if fileCount == 0 {
			emptyMsg := DimStyle.Render("No transfers yet.")
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, emptyMsg)
		}

		var b strings.Builder
		count := 0
		maxLines := availHeight - 4

		m.filesMu.Lock()
		// Use filesOrder to maintain consistent display order
		for _, fileName := range m.filesOrder {
			if count >= maxLines {
				break
			}

			status, exists := m.filesStatus[fileName]
			if !exists {
				continue
			}

			// Determine icon and style based on state
			var icon string
			var lineStyle lipgloss.Style

			// Check if transfer is actually complete (bytes received >= total bytes)
			isActuallyComplete := status.TotalBytes > 0 && status.TransferredBytes >= status.TotalBytes

			switch {
			case status.IsComplete || isActuallyComplete:
				icon = "✓"
				lineStyle = GetSuccessStyle()
			case status.State == TransferStateFailed:
				icon = "✗"
				lineStyle = GetErrorStyle()
			default:
				icon = "⏳"
				lineStyle = lipgloss.NewStyle().Foreground(WarningColor)
			}

			filename := filepath.Base(status.FileName)
			// Truncate filename if needed
			maxFilenameLen := contentWidth - 20 // Account for icon and size
			if len(filename) > maxFilenameLen && maxFilenameLen > 3 {
				filename = TruncateWithEllipsis(filename, maxFilenameLen)
			}

			line := fmt.Sprintf("%s %s (%s)", icon, filename, ui.FormatBytes(status.TotalBytes))
			b.WriteString(lineStyle.Render(Truncate(line, contentWidth)) + "\n")
			count++
		}
		m.filesMu.Unlock()

		boxContent := BoxStyle.Width(boxWidth).Render(b.String())
		return lipgloss.Place(m.Width, availHeight, lipgloss.Center, lipgloss.Center, boxContent)
	})
}

func (m *HostModel) renderError() string {
	return RenderErrorScreen("HOST ERROR", m.errorMsg, m.Width, m.Height)
}

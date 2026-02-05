package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/internal/resume"
	"github.com/zulfikawr/warp/internal/ui"
)

// TransferHistoryStatus represents the status of a historical transfer
type TransferHistoryStatus string

const (
	HistoryStatusComplete   TransferHistoryStatus = "complete"
	HistoryStatusPaused     TransferHistoryStatus = "paused"
	HistoryStatusFailed     TransferHistoryStatus = "failed"
	HistoryStatusInProgress TransferHistoryStatus = "in_progress"
)

// TransferHistoryEntry represents a single transfer history entry
type TransferHistoryEntry struct {
	SessionID   string
	FileName    string
	FilePath    string
	Direction   string // "upload" or "download"
	TotalSize   int64
	Transferred int64
	Progress    float64
	Status      TransferHistoryStatus
	StartTime   time.Time
	EndTime     time.Time
	Speed       float64 // average speed in bytes/sec
	Encrypted   bool
}

// HistoryModel represents the transfer history screen
type HistoryModel struct {
	Width        int
	Height       int
	entries      []*TransferHistoryEntry
	cursor       int
	offset       int
	stateManager *resume.TransferStateManager
	loading      bool
	err          error
	spinner      spinner.Model
}

// NewHistoryModel creates a new history model
func NewHistoryModel() HistoryModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(PrimaryColor)

	return HistoryModel{
		Width:   80,
		Height:  24,
		cursor:  0,
		offset:  0,
		loading: true,
		spinner: s,
	}
}

// Init initializes the history screen
func (m HistoryModel) Init() tea.Cmd {
	return tea.Batch(m.loadHistory, m.spinner.Tick)
}

// Update handles messages for the history screen
func (m HistoryModel) Update(msg tea.Msg) (HistoryModel, tea.Cmd, bool) {
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
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "d":
			// Delete selected entry
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				return m, m.deleteEntry(m.entries[m.cursor]), false
			}
		case "c":
			// Cleanup old entries
			return m, m.cleanupOld, false
		}

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height

	case historyLoadedMsg:
		m.loading = false
		m.entries = msg.entries
		m.stateManager = msg.stateManager
		if len(m.entries) > 0 && m.cursor >= len(m.entries) {
			m.cursor = len(m.entries) - 1
		}

	case historyErrorMsg:
		m.loading = false
		m.err = msg.err

	case entryDeletedMsg:
		// Reload history after deletion
		return m, m.loadHistory, false

	case cleanupCompleteMsg:
		// Reload history after cleanup
		return m, m.loadHistory, false
	}

	// Update scroll offset like filepicker
	reservedLines := 8
	availHeight := max(m.Height-reservedLines, 1)

	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+availHeight {
		m.offset = m.cursor - availHeight + 1
	}

	return m, cmd, quit
}

// View renders the history screen
func (m HistoryModel) View() string {
	footerItems := []string{
		"[↑↓] Navigate",
		"[D] Delete",
		"[C] Cleanup",
		"[Esc] Back",
	}

	return RenderScreen("TRANSFER HISTORY", m.Width, m.Height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth

		// Loading state
		if m.loading {
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				m.spinner.View()+" "+DimStyle.Render("Loading transfer history..."))
		}

		// Error state
		if m.err != nil {
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				GetErrorStyle().Render(fmt.Sprintf("Error: %v", m.err)))
		}

		// Empty state - no border
		if len(m.entries) == 0 {
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				DimStyle.Render("No transfer history found"))
		}

		// Calculate box width like filepicker
		boxWidth := w - 4
		if boxWidth < 40 {
			boxWidth = 40
		}

		labelStyle := lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)

		// Header row with title and count
		titleText := "HISTORY"
		countText := fmt.Sprintf("%d entries", len(m.entries))
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
			if idx < len(m.entries) {
				entry := m.entries[idx]
				row := m.renderRow(entry, idx == m.cursor, boxWidth)
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

// renderRow renders a single history row like filepicker
func (m HistoryModel) renderRow(e *TransferHistoryEntry, selected bool, maxWidth int) string {
	// Determine status icon and prefix
	var statusIcon string
	var statusStyle lipgloss.Style
	switch e.Status {
	case HistoryStatusComplete:
		statusIcon = "✓"
		statusStyle = GetSuccessStyle()
	case HistoryStatusPaused:
		statusIcon = "⏸"
		statusStyle = lipgloss.NewStyle().Foreground(WarningColor)
	case HistoryStatusFailed:
		statusIcon = "✗"
		statusStyle = GetErrorStyle()
	case HistoryStatusInProgress:
		statusIcon = "⏳"
		statusStyle = lipgloss.NewStyle().Foreground(WarningColor)
	default:
		statusIcon = "?"
		statusStyle = DimStyle
	}

	// Direction icon
	dirIcon := "↑"
	if e.Direction == "download" {
		dirIcon = "↓"
	}

	// Get filename from path
	filename := filepath.Base(e.FileName)
	if filename == "" || filename == "." {
		filename = e.FileName
	}

	// Format size
	sizeStr := ui.FormatBytes(e.TotalSize)

	// Format time
	var timeStr string
	if !e.EndTime.IsZero() {
		timeStr = e.EndTime.Format("01/02 15:04")
	} else if !e.StartTime.IsZero() {
		timeStr = e.StartTime.Format("01/02 15:04")
	}

	// Build the row like filepicker: [status] direction filename ... size time
	prefix := fmt.Sprintf("%s %s ", statusIcon, dirIcon)
	suffix := fmt.Sprintf("  %s  %s", sizeStr, timeStr)

	// Calculate available width for filename
	nameWidth := maxWidth - len(prefix) - len(suffix)
	if nameWidth < 10 {
		nameWidth = 10
	}

	// Truncate filename if needed
	if len(filename) > nameWidth {
		filename = TruncateWithEllipsis(filename, nameWidth)
	}

	// Build full line with padding
	paddingCount := maxWidth - len(prefix) - len(filename) - len(suffix)
	if paddingCount < 1 {
		paddingCount = 1
	}

	fullLine := prefix + filename + strings.Repeat(" ", paddingCount) + suffix

	if selected {
		return MenuItemSelectedStyle.Render(fullLine)
	}
	return statusStyle.Render(fullLine)
}

// Messages

type historyLoadedMsg struct {
	entries      []*TransferHistoryEntry
	stateManager *resume.TransferStateManager
}

type historyErrorMsg struct {
	err error
}

type entryDeletedMsg struct{}

type cleanupCompleteMsg struct{}

// Commands

func (m HistoryModel) loadHistory() tea.Msg {
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return historyErrorMsg{err: fmt.Errorf("failed to get home directory: %w", err)}
	}

	// Initialize state manager
	stateManager, err := resume.NewTransferStateManager(homeDir + "/.warp/transfers")
	if err != nil {
		return historyErrorMsg{err: fmt.Errorf("failed to initialize state manager: %w", err)}
	}

	// List all transfers (including completed ones from checkpoints)
	summaries, err := stateManager.ListResumable()
	if err != nil {
		return historyErrorMsg{err: fmt.Errorf("failed to list transfers: %w", err)}
	}

	// Convert to history entries
	entries := make([]*TransferHistoryEntry, 0, len(summaries))
	for _, s := range summaries {
		status := HistoryStatusInProgress
		if s.Progress >= 1.0 {
			status = HistoryStatusComplete
		} else if s.Progress > 0 && s.Progress < 1.0 {
			status = HistoryStatusPaused
		}

		entry := &TransferHistoryEntry{
			SessionID:   s.SessionID,
			FileName:    s.SourcePath,
			FilePath:    s.DestinationPath,
			Direction:   s.Direction,
			TotalSize:   s.TotalSize,
			Transferred: int64(float64(s.TotalSize) * s.Progress),
			Progress:    s.Progress,
			Status:      status,
			StartTime:   s.CreatedAt,
			EndTime:     s.UpdatedAt,
			Encrypted:   s.Encrypted,
		}
		entries = append(entries, entry)
	}

	return historyLoadedMsg{
		entries:      entries,
		stateManager: stateManager,
	}
}

func (m HistoryModel) deleteEntry(e *TransferHistoryEntry) tea.Cmd {
	return func() tea.Msg {
		if m.stateManager != nil {
			if err := m.stateManager.DeleteCheckpoint(e.SessionID); err != nil {
				return historyErrorMsg{err: fmt.Errorf("failed to delete entry: %w", err)}
			}
		}
		return entryDeletedMsg{}
	}
}

func (m HistoryModel) cleanupOld() tea.Msg {
	if m.stateManager == nil {
		return historyErrorMsg{err: fmt.Errorf("state manager not initialized")}
	}
	// Cleanup transfers older than 7 days
	if err := m.stateManager.CleanupStale(7 * 24 * time.Hour); err != nil {
		return historyErrorMsg{err: fmt.Errorf("failed to cleanup: %w", err)}
	}
	return cleanupCompleteMsg{}
}

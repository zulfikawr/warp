// Package tui provides the terminal user interface for warp.
// This file contains the resume screen model for resuming interrupted transfers.
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

// ResumeSelectedMsg is sent when a user selects a checkpoint to resume
type ResumeSelectedMsg struct {
	Checkpoint *resume.CheckpointSummary
}

// resumeLoadedMsg carries the list of resumable transfers
type resumeLoadedMsg struct {
	items []*resume.CheckpointSummary
	err   error
}

// resumeDeletedMsg is sent when a checkpoint is deleted
type resumeDeletedMsg struct {
	err error
}

// ResumeModel manages the resume screen state and rendering.
type ResumeModel struct {
	items         []*resume.CheckpointSummary
	cursor        int
	offset        int
	width         int
	height        int
	loading       bool
	err           error
	stateManager  *resume.TransferStateManager
	confirmDelete bool
	spinner       spinner.Model
}

// NewResumeModel creates a new resume model.
func NewResumeModel() ResumeModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(PrimaryColor)

	return ResumeModel{
		width:   80,
		height:  24,
		cursor:  0,
		offset:  0,
		loading: true,
		spinner: s,
	}
}

// Init implements tea.Model.
func (m ResumeModel) Init() tea.Cmd {
	return tea.Batch(m.loadResumable, m.spinner.Tick)
}

// loadResumable loads the list of resumable transfers.
func (m ResumeModel) loadResumable() tea.Msg {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return resumeLoadedMsg{err: fmt.Errorf("failed to get home directory: %w", err)}
	}

	stateManager, err := resume.NewTransferStateManager(filepath.Join(homeDir, ".warp", "transfers"))
	if err != nil {
		return resumeLoadedMsg{err: fmt.Errorf("failed to initialize state manager: %w", err)}
	}

	summaries, err := stateManager.ListResumable()
	if err != nil {
		return resumeLoadedMsg{err: fmt.Errorf("failed to list transfers: %w", err)}
	}

	// Filter out completed transfers
	var items []*resume.CheckpointSummary
	for _, s := range summaries {
		if s.Progress < 1.0 {
			items = append(items, s)
		}
	}

	return resumeLoadedMsg{items: items}
}

// deleteCheckpoint deletes the selected checkpoint
func (m ResumeModel) deleteCheckpoint() tea.Msg {
	if m.stateManager == nil || m.cursor >= len(m.items) {
		return resumeDeletedMsg{err: fmt.Errorf("invalid state")}
	}

	selected := m.items[m.cursor]
	err := m.stateManager.DeleteCheckpoint(selected.SessionID)
	return resumeDeletedMsg{err: err}
}

// Update implements tea.Model.
func (m ResumeModel) Update(msg tea.Msg) (ResumeModel, tea.Cmd, bool) {
	quit := false
	var cmd tea.Cmd

	if m.loading {
		var scmd tea.Cmd
		m.spinner, scmd = m.spinner.Update(msg)
		cmd = tea.Batch(cmd, scmd)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle delete confirmation first
		if m.confirmDelete {
			switch strings.ToLower(msg.String()) {
			case "y":
				m.confirmDelete = false
				return m, m.deleteCheckpoint, false
			case "n", "esc":
				m.confirmDelete = false
				return m, nil, false
			}
			return m, nil, false
		}

		switch strings.ToLower(msg.String()) {
		case "ctrl+c", "q", "esc":
			quit = true
			return m, nil, quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.items) > 0 && m.cursor < len(m.items) {
				selected := m.items[m.cursor]
				return m, func() tea.Msg {
					return ResumeSelectedMsg{Checkpoint: selected}
				}, false
			}
		case "d", "delete", "backspace":
			if len(m.items) > 0 && m.cursor < len(m.items) {
				m.confirmDelete = true
			}
		case "r":
			m.loading = true
			m.items = nil
			m.cursor = 0
			m.offset = 0
			m.err = nil
			return m, tea.Batch(m.loadResumable, m.spinner.Tick), false
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case resumeLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.items = msg.items
			// Initialize state manager for delete operations
			homeDir, _ := os.UserHomeDir()
			m.stateManager, _ = resume.NewTransferStateManager(filepath.Join(homeDir, ".warp", "transfers"))
			if m.cursor >= len(m.items) && len(m.items) > 0 {
				m.cursor = len(m.items) - 1
			}
		}

	case resumeDeletedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			// Reload after delete
			return m, m.loadResumable, false
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
func (m ResumeModel) View() string {
	footerItems := []string{
		"[↑↓] Navigate",
		"[⏎] Resume",
		"[D] Delete",
		"[R] Refresh",
		"[Esc] Back",
	}

	return RenderScreen("RESUME TRANSFERS", m.width, m.height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth

		// Loading state
		if m.loading {
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				m.spinner.View()+" "+DimStyle.Render("Loading resumable transfers..."))
		}

		// Error state
		if m.err != nil {
			return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center,
				GetErrorStyle().Render(fmt.Sprintf("Error: %v\n\nPress R to retry", m.err)))
		}

		// Empty state
		if len(m.items) == 0 {
			emptyMsg := "No resumable transfers found.\n\nInterrupted transfers will appear here\nand can be resumed later.\n\nPress Esc to go back"
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
		titleText := "RESUMABLE TRANSFERS"
		countText := fmt.Sprintf("%d found", len(m.items))
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
			if idx < len(m.items) {
				item := m.items[idx]
				row := m.renderRow(item, idx == m.cursor, boxWidth)
				listRows = append(listRows, row)
			} else {
				listRows = append(listRows, "")
			}
		}

		// Add delete confirmation if active
		if m.confirmDelete {
			listRows = append(listRows, "")
			listRows = append(listRows, lipgloss.NewStyle().Foreground(ErrorColor).Bold(true).Render("Delete this checkpoint? [Y/N]"))
		}

		listContent := strings.Join(listRows, "\n")
		menuBox := lipgloss.NewStyle().Width(boxWidth).Render(listContent)

		return lipgloss.Place(w, availHeight, lipgloss.Center, lipgloss.Center, menuBox)
	})
}

// renderRow renders a single resumable transfer row
func (m ResumeModel) renderRow(item *resume.CheckpointSummary, selected bool, maxWidth int) string {
	// Direction icon
	dirIcon := "↓"
	if item.Direction == "upload" {
		dirIcon = "↑"
	}

	// Get filename
	filename := filepath.Base(item.DestinationPath)
	if filename == "" || filename == "." {
		filename = filepath.Base(item.SourcePath)
	}

	// Format progress
	progressPct := item.Progress * 100

	// Format size
	sizeStr := ui.FormatBytes(item.TotalSize)

	// Format age
	age := time.Since(item.UpdatedAt)
	ageStr := formatAge(age)

	// Build the row: [dir] filename ... progress% size age
	prefix := fmt.Sprintf("%s ", dirIcon)
	suffix := fmt.Sprintf("  %.0f%%  %s  %s", progressPct, sizeStr, ageStr)

	// Calculate available width for filename
	nameWidth := maxWidth - len(prefix) - len(suffix)
	if nameWidth < 10 {
		nameWidth = 10
	}

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
	return MenuItemStyle.Render(fullLine)
}

// formatAge formats a duration in a human-readable way
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

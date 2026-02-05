package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/zulfikawr/warp/internal/resume"
	"github.com/zulfikawr/warp/internal/ui"
)

// Resume handles the resume command.
// If no arguments, it lists resumable sessions.
// If a session ID is provided, it attempts to resume it.
func Resume(args []string) error {
	// Initialize state manager
	stateManager, err := resume.NewTransferStateManager("")
	if err != nil {
		return fmt.Errorf("failed to initialize state manager: %w", err)
	}

	if len(args) == 0 {
		return listResumableSessions(stateManager)
	}

	sessionID := args[0]
	// Check if session exists and get direction
	checkpoint, err := stateManager.LoadCheckpoint(sessionID)
	if err != nil {
		return fmt.Errorf("failed to load session %s: %w", sessionID, err)
	}

	if checkpoint.IsExpired() {
		return fmt.Errorf("session %s has expired", sessionID)
	}

	// Dispatch based on direction
	if checkpoint.Direction == "upload" {
		// Call Send with resume flags
		// We pass any extra args too, though usually none needed
		sendArgs := []string{"--resume", "--session", sessionID}
		sendArgs = append(sendArgs, args[1:]...)
		return Send(sendArgs)
	} else if checkpoint.Direction == "download" {
		// Call Receive with resume flags
		receiveArgs := []string{"--resume", "--session", sessionID}
		receiveArgs = append(receiveArgs, args[1:]...)
		return Receive(receiveArgs)
	}

	return fmt.Errorf("unknown direction: %s", checkpoint.Direction)
}

func listResumableSessions(stateManager *resume.TransferStateManager) error {
	summaries, err := stateManager.ListResumable()
	if err != nil {
		return fmt.Errorf("failed to list resumable sessions: %w", err)
	}

	// Filter for incomplete sessions
	var resumable []*resume.CheckpointSummary
	for _, s := range summaries {
		if s.Progress < 1.0 {
			resumable = append(resumable, s)
		}
	}

	if len(resumable) == 0 {
		fmt.Println("No resumable sessions found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SESSION ID\tDIRECTION\tFILE\tPROGRESS\tUPDATED\n")

	for _, s := range resumable {
		dirIcon := "↑"
		if s.Direction == "download" {
			dirIcon = "↓"
		}

		fmt.Fprintf(w, "%s\t%s %s\t%s\t%.1f%%\t%s ago\n",
			s.SessionID,
			dirIcon, s.Direction,
			filepath.Base(s.SourcePath),
			s.Progress*100,
			ui.FormatDuration(time.Since(s.UpdatedAt)),
		)
	}

	w.Flush()
	fmt.Println()
	fmt.Println("To resume a session, run:")
	fmt.Println("  warp resume <session_id>")

	return nil
}

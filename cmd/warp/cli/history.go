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

// History lists past transfer sessions from the local state manager.
func History(args []string) error {
	// Initialize state manager
	// We use the default directory by passing empty string, which NewTransferStateManager handles
	stateManager, err := resume.NewTransferStateManager("")
	if err != nil {
		return fmt.Errorf("failed to initialize state manager: %w", err)
	}

	summaries, err := stateManager.ListResumable()
	if err != nil {
		return fmt.Errorf("failed to list transfer history: %w", err)
	}

	if len(summaries) == 0 {
		fmt.Println("No transfer history found.")
		return nil
	}

	// Initialize tabwriter
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	fmt.Fprintf(w, "SESSION ID\tDIRECTION\tFILE\tSIZE\tPROGRESS\tSTATUS\tAGE\n")

	for _, s := range summaries {
		// Determine status
		status := "Paused"
		if s.Progress >= 1.0 {
			status = "Complete"
		}

		// Determine color based on status/direction
		statusColor := ui.Colors.Yellow
		if status == "Complete" {
			statusColor = ui.Colors.Green
		}

		dirIcon := "↑"
		if s.Direction == "download" {
			dirIcon = "↓"
		}

		// Format output
		fmt.Fprintf(w, "%s\t%s %s\t%s\t%s\t%.1f%%\t%s%s%s\t%s ago\n",
			s.SessionID,
			dirIcon, s.Direction,
			filepath.Base(s.SourcePath),
			ui.FormatBytes(s.TotalSize),
			s.Progress*100,
			statusColor, status, ui.Colors.Reset,
			ui.FormatDuration(time.Since(s.CreatedAt)),
		)
	}

	w.Flush()
	return nil
}

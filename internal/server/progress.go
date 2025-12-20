package server

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zulfikawr/warp/internal/ui"
)

// MultiFileProgress tracks progress for multiple concurrent file downloads
type MultiFileProgress struct {
	mu             sync.Mutex
	files          map[string]*FileProgress // sessionID -> progress
	fileOrder      []string                 // maintain display order
	totalSize      int64
	totalReceived  int64
	startTime      time.Time
	lastUpdate     time.Time
	displayActive  bool
	summaryPrinted bool // prevents duplicate summary display
}

// FileProgress tracks individual file download progress
type FileProgress struct {
	filename  string
	size      int64
	received  int64
	complete  bool
	startTime time.Time
	endTime   time.Time
}

// printMultiFileProgress displays a dynamic multi-file progress UI
func (s *Server) printMultiFileProgress() {
	if s.multiFileDisplay == nil {
		return
	}

	display := s.multiFileDisplay
	display.mu.Lock()
	defer display.mu.Unlock()

	if len(display.files) == 0 {
		return
	}

	// Skip if summary was already printed (prevents duplicate display)
	if display.summaryPrinted {
		return
	}

	if !display.displayActive {
		fmt.Printf("\n")
		display.displayActive = true
	}

	fmt.Printf("Receiving %d file(s) (%s total):\033[K\n", len(display.files), ui.FormatBytes(display.totalSize))

	for _, sessionID := range display.fileOrder {
		fileProgress := display.files[sessionID]
		percent := float64(0)
		if fileProgress.size > 0 {
			percent = float64(fileProgress.received) / float64(fileProgress.size) * 100
			if percent > 100 {
				percent = 100
			}
		}

		barWidth := 20
		var filled int
		if fileProgress.complete {
			filled = barWidth
			percent = 100
		} else {
			filled = int(percent / 5)
			if filled < 0 {
				filled = 0
			}
			if filled > barWidth {
				filled = barWidth
			}
		}
		bar := strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled)

		displayName := fileProgress.filename
		if len(displayName) > 35 {
			displayName = displayName[:32] + "..."
		}

		fmt.Printf("%-39s [%s%s%s] %s%3.0f%%%s\033[K\n",
			displayName,
			ui.Colors.Green, bar, ui.Colors.Reset,
			ui.Colors.Green, percent, ui.Colors.Reset)
	}

	fmt.Printf("---------------------------------------------------------------------------------\033[K\n")

	allComplete := true
	for _, fileProgress := range display.files {
		if !fileProgress.complete {
			allComplete = false
			break
		}
	}

	overallPercent := float64(0)
	if allComplete {
		overallPercent = 100
		display.totalReceived = display.totalSize
	} else if display.totalSize > 0 {
		overallPercent = float64(display.totalReceived) / float64(display.totalSize) * 100
		if overallPercent > 100 {
			overallPercent = 100
		}
	}

	overallBarWidth := 20
	var overallFilled int
	if allComplete {
		overallFilled = overallBarWidth
	} else {
		overallFilled = int(overallPercent / 5)
		if overallFilled < 0 {
			overallFilled = 0
		}
		if overallFilled > overallBarWidth {
			overallFilled = overallBarWidth
		}
	}
	overallBar := strings.Repeat("=", overallFilled) + strings.Repeat(" ", overallBarWidth-overallFilled)

	elapsed := time.Since(display.startTime)
	speed := float64(0)
	if elapsed.Seconds() > 0 {
		speed = float64(display.totalReceived) / elapsed.Seconds()
	}

	fmt.Printf("%sOverall:%s [%s%s%s] %s%3.0f%%%s | %s/%s | %s\033[K\n",
		ui.Colors.Dim, ui.Colors.Reset,
		ui.Colors.Green, overallBar, ui.Colors.Reset,
		ui.Colors.Green, overallPercent, ui.Colors.Reset,
		ui.FormatBytes(display.totalReceived), ui.FormatBytes(display.totalSize),
		ui.FormatSpeed(speed))

	if allComplete {
		// Only print summary once (check if we haven't already printed it)
		if display.displayActive {
			fmt.Printf("\n%s✓ All Downloads Complete%s\n\n", ui.Colors.Green, ui.Colors.Reset)

			wallTime := time.Since(display.startTime)
			avgSpeed := float64(0)
			if wallTime.Seconds() > 0 {
				avgSpeed = float64(display.totalSize) / wallTime.Seconds()
			}

			fmt.Printf("%sSummary:%s\n", ui.Colors.Dim, ui.Colors.Reset)
			fmt.Printf("  Files:        %d\n", len(display.files))
			fmt.Printf("  Total Size:   %s\n", ui.FormatBytes(display.totalSize))
			fmt.Printf("  Time:         %s\n", ui.FormatDuration(wallTime))
			fmt.Printf("  Avg Speed:    %s\n", ui.FormatSpeed(avgSpeed))
			fmt.Println()
			// Mark summary as printed to prevent duplicates
			display.summaryPrinted = true
			display.displayActive = false
		}
	} else {
		// Move cursor back up to redraw progress (header + files + separator + overall)
		numLines := 1 + len(display.fileOrder) + 1 + 1
		fmt.Printf("\033[%dA", numLines)
	}
}

// removeFileFromDisplay removes a file from the multi-file progress display
func (s *Server) removeFileFromDisplay(sessionID string) {
	if s.multiFileDisplay == nil {
		return
	}

	display := s.multiFileDisplay
	display.mu.Lock()
	defer display.mu.Unlock()

	if fileProgress, exists := display.files[sessionID]; exists {
		// Subtract from totals
		display.totalSize -= fileProgress.size
		display.totalReceived -= fileProgress.received

		// Remove from map
		delete(display.files, sessionID)

		// Remove from order slice
		for i, id := range display.fileOrder {
			if id == sessionID {
				display.fileOrder = append(display.fileOrder[:i], display.fileOrder[i+1:]...)
				break
			}
		}

		// Clear and redraw if display is active
		if display.displayActive {
			// Clear the current display
			numLines := 1 + len(display.fileOrder) + 1 + 2 // header + old files + separator + overall + removed file
			for i := 0; i < numLines; i++ {
				fmt.Print("\033[2K") // Clear line
				if i < numLines-1 {
					fmt.Print("\n")
				}
			}
			fmt.Printf("\033[%dA", numLines-1) // Move back up

			// If no files left, reset display
			if len(display.files) == 0 {
				display.displayActive = false
			}
		}
	}
}

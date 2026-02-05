package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/internal/core"
	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/resume"
	"github.com/zulfikawr/warp/internal/ui"
)

// Receive executes the receive command in CLI mode.
// It parses flags, creates a ReceiveExecutor, and displays progress to stdout.
func Receive(args []string) error {
	opts, err := parseReceiveFlags(args)
	if err != nil {
		return err
	}

	// Set log level based on verbosity
	if opts.Verbose > 0 {
		logging.SetLevel(opts.Verbose)
	}

	// Handle resume mode
	if opts.Resume {
		return handleReceiveResume(opts)
	}

	// Prompt for code if not provided
	if opts.Code == "" {
		opts.Code = promptForCode()
		if opts.Code == "" {
			return fmt.Errorf("receive requires a PAKE code")
		}
	}

	// Create executor with CLI callbacks
	exec := core.NewReceiveExecutor(opts, printStatus, printReceiveProgress)

	// Execute the download
	savedPath, err := exec.Execute(context.Background())
	if err != nil {
		return err
	}

	// Print final newline after progress bar
	fmt.Println()

	if savedPath != "(stdout)" {
		fmt.Printf("File saved to: %s\n", savedPath)
	}

	return nil
}

// parseReceiveFlags parses command-line flags into ReceiveOptions.
func parseReceiveFlags(args []string) (core.ReceiveOptions, error) {
	// Count -v flags and filter them out
	verbosity, filteredArgs := countVerbosity(args)

	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	fs.Usage = help.PrintReceive

	// Define flags
	code := fs.String("code", "", "PAKE code for secure transfer")
	fs.StringVar(code, "c", "", "PAKE code (shorthand)")

	out := fs.String("output", "", "output path for downloaded file")
	fs.StringVar(out, "o", "", "output path (shorthand)")

	force := fs.Bool("force", false, "overwrite existing files")
	fs.BoolVar(force, "f", false, "overwrite (shorthand)")

	workers := fs.Int("workers", 0, "number of parallel download workers")

	chunkSize := fs.Int("chunk-size", 0, "chunk size in MB for parallel downloads")

	noChecksum := fs.Bool("no-checksum", false, "skip checksum verification")

	decrypt := fs.Bool("decrypt", false, "decrypt received file")

	resume := fs.Bool("resume", false, "resume a paused download")

	sessionID := fs.String("session", "", "specific session ID to resume")

	if err := fs.Parse(filteredArgs); err != nil {
		return core.ReceiveOptions{}, fmt.Errorf("failed to parse flags: %w", err)
	}

	opts := core.ReceiveOptions{
		Code:        *code,
		OutputPath:  *out,
		Force:       *force,
		Workers:     *workers,
		ChunkSizeMB: *chunkSize,
		NoChecksum:  *noChecksum,
		Decrypt:     *decrypt,
		Resume:      *resume,
		SessionID:   *sessionID,
		Verbose:     verbosity,
	}

	// Check for positional argument (code or URL)
	if fs.NArg() > 0 {
		arg := fs.Arg(0)
		if strings.HasPrefix(arg, "http") {
			// Direct URL - not supported in new architecture, use code instead
			return core.ReceiveOptions{}, fmt.Errorf("direct URL not supported, please use PAKE code")
		}
		opts.Code = arg
	}

	return opts, nil
}

// promptForCode prompts the user to enter a PAKE code.
func promptForCode() string {
	fmt.Print("Enter PAKE code: ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

// printReceiveProgress displays download progress to stdout.
func printReceiveProgress(p progress.Progress) {
	if p.IsComplete {
		fmt.Printf("\nDownload complete: %s\n", p.SavedPath)
		return
	}

	// Print progress bar
	fmt.Printf("\rDownloading... %3.0f%% (%s / %s) %.1f Mbps",
		p.Percent(),
		ui.FormatBytes(p.TransferredBytes),
		ui.FormatBytes(p.TotalBytes),
		p.SpeedMbps())
}

// handleReceiveResume handles resuming a paused download
func handleReceiveResume(opts core.ReceiveOptions) error {
	// Get home directory for checkpoint storage
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Initialize state manager
	stateManager, err := resume.NewTransferStateManager(homeDir + "/.warp/transfers")
	if err != nil {
		return fmt.Errorf("failed to initialize state manager: %w", err)
	}

	// If specific session ID provided, resume that session
	if opts.SessionID != "" {
		return resumeSpecificDownload(opts.SessionID, stateManager, opts.Code)
	}

	// Otherwise, list resumable downloads and prompt for selection
	return listAndSelectDownload(stateManager, opts.Code)
}

// resumeSpecificDownload resumes a specific download session
func resumeSpecificDownload(sessionID string, stateManager *resume.TransferStateManager, pakeCode string) error {
	// Load the checkpoint
	checkpoint, err := stateManager.LoadCheckpoint(sessionID)
	if err != nil {
		return fmt.Errorf("failed to load session %s: %w", sessionID, err)
	}

	// Verify it's a download
	if checkpoint.Direction != "download" {
		return fmt.Errorf("session %s is not a download (direction: %s)", sessionID, checkpoint.Direction)
	}

	// Check if expired
	if checkpoint.IsExpired() {
		return fmt.Errorf("session %s has expired", sessionID)
	}

	// If encrypted, prompt for PAKE code if not provided
	if checkpoint.Encrypted && pakeCode == "" {
		pakeCode = promptForCode()
		if pakeCode == "" {
			return fmt.Errorf("PAKE code required for encrypted transfer")
		}
	}

	// Display session info
	fmt.Fprintf(os.Stderr, "Resuming download: %s\n", checkpoint.DestinationPath)
	fmt.Fprintf(os.Stderr, "Progress: %.1f%% (%d/%d chunks)\n",
		checkpoint.GetProgress()*100,
		len(checkpoint.CompletedChunks),
		checkpoint.TotalChunks)

	// Create transfer session
	session := resume.NewTransferSession(checkpoint, stateManager)

	// If encrypted, restore encryption state
	if checkpoint.Encrypted && checkpoint.EncryptionState != nil {
		encryptManager := resume.NewEncryptionStateManager()
		if err := encryptManager.RestoreState(checkpoint.EncryptionState, pakeCode); err != nil {
			return fmt.Errorf("failed to restore encryption state: %w", err)
		}
		session.EncryptManager = encryptManager
	}

	// Start the transfer
	ctx := context.Background()
	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("failed to start transfer: %w", err)
	}

	// Wait for completion
	if err := session.Wait(ctx); err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nDownload completed successfully!\n")
	fmt.Fprintf(os.Stderr, "File saved to: %s\n", checkpoint.DestinationPath)
	return nil
}

// listAndSelectDownload lists resumable downloads and prompts for selection
func listAndSelectDownload(stateManager *resume.TransferStateManager, pakeCode string) error {
	// List all resumable transfers
	summaries, err := stateManager.ListResumable()
	if err != nil {
		return fmt.Errorf("failed to list resumable transfers: %w", err)
	}

	// Filter by direction and exclude completed transfers
	var filtered []*resume.CheckpointSummary
	for _, s := range summaries {
		if s.Direction == "download" && s.Progress < 1.0 {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return fmt.Errorf("no resumable downloads found")
	}

	// Display list
	fmt.Fprintln(os.Stderr, "Resumable downloads:")
	for i, s := range filtered {
		age := time.Since(s.UpdatedAt)
		fmt.Fprintf(os.Stderr, "%d. %s\n", i+1, s.DestinationPath)
		fmt.Fprintf(os.Stderr, "   Progress: %.1f%% | Size: %s | Updated: %s ago\n",
			s.Progress*100,
			ui.FormatBytes(s.TotalSize),
			ui.FormatDuration(age))
		if s.Encrypted {
			fmt.Fprintf(os.Stderr, "   [Encrypted]\n")
		}
		fmt.Fprintln(os.Stderr)
	}

	// Prompt for selection
	fmt.Fprint(os.Stderr, "Select transfer to resume (1-"+fmt.Sprint(len(filtered))+"): ")
	scanner := bufio.NewScanner(os.Stdin)
	var selection int
	if scanner.Scan() {
		_, err := fmt.Sscanf(scanner.Text(), "%d", &selection)
		if err != nil {
			return fmt.Errorf("invalid selection: %w", err)
		}
	} else {
		return fmt.Errorf("failed to read selection")
	}

	if selection < 1 || selection > len(filtered) {
		return fmt.Errorf("invalid selection: must be between 1 and %d", len(filtered))
	}

	// Resume selected session
	selected := filtered[selection-1]
	return resumeSpecificDownload(selected.SessionID, stateManager, pakeCode)
}

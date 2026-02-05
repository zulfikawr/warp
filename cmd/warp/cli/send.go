package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/internal/core"
	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/resume"
	"github.com/zulfikawr/warp/internal/ui"
)

// Send executes the send command in CLI mode.
// It parses flags, creates a SendExecutor, and displays progress to stdout.
func Send(args []string) error {
	opts, err := parseSendFlags(args)
	if err != nil {
		return err
	}

	// Set log level based on verbosity
	if opts.Verbose > 0 {
		logging.SetLevel(opts.Verbose)
	}

	// Handle resume mode
	if opts.Resume {
		return handleSendResume(opts)
	}

	// Create executor with CLI callbacks
	exec := core.NewSendExecutor(opts, printStatus, printSendProgress)

	// Start the server
	ctx := context.Background()
	info, err := exec.Start(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = exec.Stop() }()

	// Display server info
	printServerInfo(info, opts)

	// Wait for interrupt signal
	return exec.Wait(ctx)
}

// parseSendFlags parses command-line flags into SendOptions.
func parseSendFlags(args []string) (core.SendOptions, error) {
	// Count -v flags and filter them out
	verbosity, filteredArgs := countVerbosity(args)

	fs := flag.NewFlagSet("send", flag.ExitOnError)
	fs.Usage = help.PrintSend

	// Define flags
	port := fs.Int("port", 0, "specific port to listen on")
	fs.IntVar(port, "p", 0, "specific port (shorthand)")

	noQR := fs.Bool("no-qr", false, "disable QR code display")

	iface := fs.String("interface", "", "network interface to bind to")
	fs.StringVar(iface, "i", "", "network interface (shorthand)")

	text := fs.String("text", "", "send text instead of file")

	stdin := fs.Bool("stdin", false, "read content from stdin")

	rateLimit := fs.Float64("rate-limit", 0, "bandwidth limit in Mbps")

	cacheSize := fs.Int64("cache-size", 0, "file cache size in MB")

	noEncrypt := fs.Bool("no-encrypt", false, "disable PAKE encryption")

	resume := fs.Bool("resume", false, "resume a paused upload")

	sessionID := fs.String("session", "", "specific session ID to resume")

	if err := fs.Parse(filteredArgs); err != nil {
		return core.SendOptions{}, fmt.Errorf("failed to parse flags: %w", err)
	}

	opts := core.SendOptions{
		Port:          *port,
		InterfaceName: *iface,
		RateLimitMbps: *rateLimit,
		CacheSizeMB:   *cacheSize,
		NoQR:          *noQR,
		NoEncrypt:     *noEncrypt,
		TextContent:   *text,
		Resume:        *resume,
		SessionID:     *sessionID,
		Verbose:       verbosity,
	}

	// Read from stdin if requested
	if *stdin {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return core.SendOptions{}, fmt.Errorf("failed to read from stdin: %w", err)
			}
			opts.StdinContent = string(data)
		}
	}

	// Get file path from positional argument
	if fs.NArg() > 0 {
		opts.FilePath = fs.Arg(0)
	}

	// Validate: need either file, text, stdin, or resume mode
	if !opts.Resume && opts.FilePath == "" && opts.TextContent == "" && opts.StdinContent == "" {
		return core.SendOptions{}, fmt.Errorf("send requires a path, --text, --stdin, or --resume")
	}

	return opts, nil
}

// printServerInfo displays server information to stderr.
func printServerInfo(info *core.ServerInfo, opts core.SendOptions) {
	fmt.Fprintf(os.Stderr, "Server started on :%d\n", info.Port)
	fmt.Fprintf(os.Stderr, "Code: %s%s%s\n", ui.Colors.Bold, info.Code, ui.Colors.Reset)
	fmt.Fprintf(os.Stderr, "Service: warp-%s._warp._tcp.local.\n", info.Code)
	fmt.Fprintf(os.Stderr, "Local URL: %s\n", info.URL)
	fmt.Fprintf(os.Stderr, "Metrics: http://%s:%d/metrics\n", info.IP, info.Port)

	if !opts.NoQR && info.QRCode != "" {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, ui.Colors.Bold+"Scan QR code on another device:"+ui.Colors.Reset)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, info.QRCode)
		fmt.Fprintln(os.Stderr, ui.Colors.Dim+"Tip: Open the URL in any browser to download"+ui.Colors.Reset)
	}

	fmt.Fprint(os.Stderr, "\n"+ui.Colors.Yellow+"Press Ctrl+C to stop server"+ui.Colors.Reset+"\n")
}

// printSendProgress displays transfer progress to stdout.
func printSendProgress(p progress.Progress) {
	if p.IsComplete {
		fmt.Printf("\rSent %s: %s (100%%)\n", p.FileName, ui.FormatBytes(p.TotalBytes))
	} else {
		percent := p.Percent()
		fmt.Printf("\rSending %s: %.1f%% (%s / %s)",
			p.FileName, percent,
			ui.FormatBytes(p.TransferredBytes),
			ui.FormatBytes(p.TotalBytes))
	}
}

// printStatus displays status messages to stderr.
func printStatus(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

// handleSendResume handles resuming a paused upload
func handleSendResume(opts core.SendOptions) error {
	// Get home directory for checkpoint storage
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Import resume package
	stateManager, err := resume.NewTransferStateManager(homeDir + "/.warp/transfers")
	if err != nil {
		return fmt.Errorf("failed to initialize state manager: %w", err)
	}

	// If specific session ID provided, resume that session
	if opts.SessionID != "" {
		return resumeSpecificSession(opts.SessionID, stateManager)
	}

	// Otherwise, list resumable uploads and prompt for selection
	return listAndSelectSession(stateManager, "upload")
}

// resumeSpecificSession resumes a specific transfer session
func resumeSpecificSession(sessionID string, stateManager *resume.TransferStateManager) error {
	// Load the checkpoint
	checkpoint, err := stateManager.LoadCheckpoint(sessionID)
	if err != nil {
		return fmt.Errorf("failed to load session %s: %w", sessionID, err)
	}

	// Verify it's an upload
	if checkpoint.Direction != "upload" {
		return fmt.Errorf("session %s is not an upload (direction: %s)", sessionID, checkpoint.Direction)
	}

	// Check if expired
	if checkpoint.IsExpired() {
		return fmt.Errorf("session %s has expired", sessionID)
	}

	// Display session info
	fmt.Fprintf(os.Stderr, "Resuming upload: %s\n", checkpoint.SourcePath)
	fmt.Fprintf(os.Stderr, "Progress: %.1f%% (%d/%d chunks)\n",
		checkpoint.GetProgress()*100,
		len(checkpoint.CompletedChunks),
		checkpoint.TotalChunks)

	// Create transfer session
	session := resume.NewTransferSession(checkpoint, stateManager)

	// Start the transfer
	ctx := context.Background()
	if err := session.Start(ctx); err != nil {
		return fmt.Errorf("failed to start transfer: %w", err)
	}

	// Wait for completion
	if err := session.Wait(ctx); err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nUpload completed successfully!\n")
	return nil
}

// listAndSelectSession lists resumable sessions and prompts for selection
func listAndSelectSession(stateManager *resume.TransferStateManager, direction string) error {
	// List all resumable transfers
	summaries, err := stateManager.ListResumable()
	if err != nil {
		return fmt.Errorf("failed to list resumable transfers: %w", err)
	}

	// Filter by direction
	var filtered []*resume.CheckpointSummary
	for _, s := range summaries {
		if s.Direction == direction {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return fmt.Errorf("no resumable %ss found", direction)
	}

	// Display list
	fmt.Fprintf(os.Stderr, "Resumable %ss:\n\n", direction)
	for i, s := range filtered {
		age := time.Since(s.UpdatedAt)
		fmt.Fprintf(os.Stderr, "%d. %s\n", i+1, s.SourcePath)
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
	var selection int
	if _, err := fmt.Scanf("%d", &selection); err != nil {
		return fmt.Errorf("invalid selection: %w", err)
	}

	if selection < 1 || selection > len(filtered) {
		return fmt.Errorf("invalid selection: must be between 1 and %d", len(filtered))
	}

	// Resume selected session
	selected := filtered[selection-1]
	return resumeSpecificSession(selected.SessionID, stateManager)
}

package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/internal/core"
	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/resume"
	"github.com/zulfikawr/warp/internal/ui"
)

// Host executes the host command in CLI mode.
// It parses flags, creates a HostExecutor, and displays progress to stdout.
func Host(args []string) error {
	opts, err := parseHostFlags(args)
	if err != nil {
		return err
	}

	// Set log level based on verbosity
	if opts.Verbose > 0 {
		logging.SetLevel(opts.Verbose)
	}

	// Handle resume mode - restore pending upload sessions
	if opts.Resume {
		if err := restorePendingSessions(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to restore pending sessions: %v\n", err)
		}
	}

	// Create executor with CLI callbacks
	exec := core.NewHostExecutor(opts, printStatus, printHostProgress)

	// Start the server
	ctx := context.Background()
	info, err := exec.Start(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = exec.Stop() }()

	// Display server info
	printHostInfo(info, opts, exec.DestDir())

	// Wait for interrupt signal
	return exec.Wait(ctx)
}

// parseHostFlags parses command-line flags into HostOptions.
func parseHostFlags(args []string) (core.HostOptions, error) {
	// Count -v flags and filter them out
	verbosity, filteredArgs := countVerbosity(args)

	fs := flag.NewFlagSet("host", flag.ExitOnError)
	fs.Usage = help.PrintHost

	// Define flags
	iface := fs.String("interface", "", "network interface to bind to")
	fs.StringVar(iface, "i", "", "network interface (shorthand)")

	dest := fs.String("dest", "", "destination directory for uploads")
	fs.StringVar(dest, "d", "", "destination directory (shorthand)")

	noQR := fs.Bool("no-qr", false, "disable QR code display")

	rateLimit := fs.Float64("rate-limit", 0, "bandwidth limit in Mbps")

	noEncrypt := fs.Bool("no-encrypt", false, "disable PAKE encryption")

	resume := fs.Bool("resume", false, "restore pending upload sessions")

	if err := fs.Parse(filteredArgs); err != nil {
		return core.HostOptions{}, fmt.Errorf("failed to parse flags: %w", err)
	}

	return core.HostOptions{
		InterfaceName: *iface,
		DestDir:       *dest,
		RateLimitMbps: *rateLimit,
		NoQR:          *noQR,
		NoEncrypt:     *noEncrypt,
		Resume:        *resume,
		Verbose:       verbosity,
	}, nil
}

// printHostInfo displays host server information to stderr.
func printHostInfo(info *core.ServerInfo, opts core.HostOptions, destDir string) {
	fmt.Fprintf(os.Stderr, "Hosting uploads to '%s'\n", destDir)
	fmt.Fprintf(os.Stderr, "Code: %s\n", info.Code)

	if opts.RateLimitMbps > 0 {
		fmt.Fprintf(os.Stderr, "Rate limit: %.1f Mbps\n", opts.RateLimitMbps)
	}

	if !opts.NoQR && info.QRCode != "" {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, ui.Colors.Bold+"Scan QR code to upload from mobile:"+ui.Colors.Reset)
		fmt.Fprintln(os.Stderr, info.QRCode)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, ui.Colors.Dim+"Tip: Drag and drop files in the browser"+ui.Colors.Reset)
	}

	fmt.Fprintf(os.Stderr, "\n"+ui.Colors.Green+"Open this on another device to upload:"+ui.Colors.Reset+"\n%s\n", info.URL)
}

// printHostProgress displays upload progress to stdout.
func printHostProgress(p progress.Progress) {
	if p.IsComplete {
		fmt.Printf("\rReceived %s: %s (100%%)\n", p.FileName, ui.FormatBytes(p.TotalBytes))
	} else {
		percent := p.Percent()
		fmt.Printf("\rReceiving %s: %.1f%% (%s / %s)",
			p.FileName, percent,
			ui.FormatBytes(p.TransferredBytes),
			ui.FormatBytes(p.TotalBytes))
	}
}

// restorePendingSessions restores pending upload sessions from checkpoints
func restorePendingSessions(_ core.HostOptions) error {
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

	// List all resumable transfers
	summaries, err := stateManager.ListResumable()
	if err != nil {
		return fmt.Errorf("failed to list resumable transfers: %w", err)
	}

	// Filter for uploads (host receives uploads from clients)
	var pending []*resume.CheckpointSummary
	for _, s := range summaries {
		// Check if not expired by comparing with current time
		if s.Direction == "upload" && time.Now().Before(s.UpdatedAt.Add(24*time.Hour)) {
			pending = append(pending, s)
		}
	}

	if len(pending) == 0 {
		fmt.Fprintln(os.Stderr, "No pending upload sessions to restore")
		return nil
	}

	// Display restored sessions
	fmt.Fprintf(os.Stderr, "Restored %d pending upload session(s):\n", len(pending))
	for _, s := range pending {
		age := time.Since(s.UpdatedAt)
		fmt.Fprintf(os.Stderr, "  - %s (%.1f%%, %s ago)\n",
			s.SourcePath,
			s.Progress*100,
			ui.FormatDuration(age))
	}
	fmt.Fprintln(os.Stderr)

	return nil
}

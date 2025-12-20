package commands

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/zulfikawr/warp/cmd/warp/ui"
	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/errors"
	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/server"
	uipkg "github.com/zulfikawr/warp/internal/ui"
)

// Host executes the host command
func Host(args []string) error {
	// Load configuration (config file → env vars)
	cfg, err := config.LoadConfig()
	if err != nil {
		return errors.ConfigError("Failed to load configuration", err)
	}

	// Count -v flags and filter them out
	verbosity, filteredArgs := countVerbosity(args)

	fs := flag.NewFlagSet("host", flag.ExitOnError)
	fs.Usage = hostHelp
	// Use config defaults for flags (config → env → flags precedence)
	iface := fs.String("interface", cfg.DefaultInterface, "network interface")
	fs.StringVar(iface, "i", cfg.DefaultInterface, "")
	dest := fs.String("dest", cfg.UploadDir, "destination directory for uploads")
	fs.StringVar(dest, "d", cfg.UploadDir, "")
	noQR := fs.Bool("no-qr", cfg.NoQR, "disable QR")
	rateLimit := fs.Float64("rate-limit", cfg.RateLimitMbps, "bandwidth limit in Mbps")
	if err := fs.Parse(filteredArgs); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	// Set log level based on verbosity
	if verbosity > 0 {
		logging.SetLevel(verbosity)
	}

	// Ensure destination exists
	if err := os.MkdirAll(*dest, 0o755); err != nil {
		return errors.PermissionError("create directory", *dest, err)
	}

	tok, err := crypto.GenerateToken(nil)
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}
	srv := &server.Server{
		InterfaceName: *iface,
		Token:         tok,
		HostMode:      true,
		UploadDir:     *dest,
	}

	// Apply optional configurations
	srv.RateLimitMbps = *rateLimit

	url, err := srv.Start()
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	defer func() { _ = srv.Shutdown() }()

	fmt.Fprintf(os.Stderr, "Hosting uploads to '%s'\n", *dest)
	fmt.Fprintf(os.Stderr, "Token: %s\n", tok)
	if *rateLimit > 0 {
		fmt.Fprintf(os.Stderr, "Rate limit: %.1f Mbps\n", *rateLimit)
	}
	fmt.Fprintf(os.Stderr, "Features: Parallel chunks, SHA256 verification, WebSocket progress\n")

	if !*noQR {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, ui.C.Bold+"Scan QR code to upload from mobile:"+ui.C.Reset)
		_ = uipkg.PrintQR(url)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, ui.C.Dim+"Tip: Drag and drop files in the browser"+ui.C.Reset)
	}

	fmt.Fprintf(os.Stderr, "\n"+ui.C.Green+"Open this on another device to upload:"+ui.C.Reset+"\n%s\n", url)

	// Wait for interrupt signal for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down gracefully...")

	return nil
}

func hostHelp() {
	fmt.Println(ui.C.Bold + ui.C.Green + "warp host" + ui.C.Reset + " - Receive uploads into a directory you control")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Usage:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp host" + ui.C.Reset + " [flags]")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Description:" + ui.C.Reset)
	fmt.Println("  Start an upload server and receive files from other devices.")
	fmt.Println("  Uploaded files are saved to the specified directory.")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Flags:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Yellow + "-i, --interface" + ui.C.Reset + "   bind to a specific network interface")
	fmt.Println("  " + ui.C.Yellow + "-d, --dest" + ui.C.Reset + "        destination directory for uploads (default: .)")
	fmt.Println("  " + ui.C.Yellow + "--rate-limit" + ui.C.Reset + "      limit upload bandwidth in Mbps (0 = unlimited)")
	fmt.Println("  " + ui.C.Yellow + "--no-qr" + ui.C.Reset + "           skip printing the QR code")
	fmt.Println("  " + ui.C.Yellow + "--encrypt" + ui.C.Reset + "         require encrypted uploads with password")
	fmt.Println("  " + ui.C.Yellow + "-v, --verbose" + ui.C.Reset + "     verbose logging (use -vv or -vvv for more detail)")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Examples:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp host" + ui.C.Reset + "                                " + ui.C.Dim + "# Accept uploads to current directory" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp host" + ui.C.Reset + " -d ./uploads                   " + ui.C.Dim + "# Save uploads to ./uploads" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp host" + ui.C.Reset + " -d ./downloads -i eth0         " + ui.C.Dim + "# Bind to specific interface" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp host" + ui.C.Reset + " --rate-limit 50 -d ./uploads   " + ui.C.Dim + "# Limit to 50 Mbps" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp host" + ui.C.Reset + " --encrypt -d ./secure          " + ui.C.Dim + "# Require encrypted uploads" + ui.C.Reset)
}

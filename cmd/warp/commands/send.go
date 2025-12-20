package commands

import (
	"flag"
	"fmt"
	"io"
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

// Send executes the send command
func Send(args []string) error {
	// Load configuration (config file → env vars)
	cfg, err := config.LoadConfig()
	if err != nil {
		return errors.ConfigError("Failed to load configuration", err)
	}

	// Count -v flags and filter them out
	verbosity, filteredArgs := countVerbosity(args)

	fs := flag.NewFlagSet("send", flag.ExitOnError)
	fs.Usage = sendHelp
	// Use config defaults for flags (config → env → flags precedence)
	port := fs.Int("port", cfg.DefaultPort, "specific port")
	fs.IntVar(port, "p", cfg.DefaultPort, "")
	noQR := fs.Bool("no-qr", cfg.NoQR, "disable QR")
	iface := fs.String("interface", cfg.DefaultInterface, "network interface")
	fs.StringVar(iface, "i", cfg.DefaultInterface, "")
	text := fs.String("text", "", "send text instead of file")
	stdin := fs.Bool("stdin", false, "read from stdin")
	rateLimit := fs.Float64("rate-limit", cfg.RateLimitMbps, "bandwidth limit in Mbps")
	cacheSize := fs.Int64("cache-size", cfg.CacheSizeMB, "file cache size in MB")
	if err := fs.Parse(filteredArgs); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	// Set log level based on verbosity
	if verbosity > 0 {
		logging.SetLevel(verbosity)
	}

	tok, err := crypto.GenerateToken(nil)
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}

	var srv *server.Server

	// Handle text sharing
	if *text != "" {
		srv = &server.Server{InterfaceName: *iface, Token: tok, TextContent: *text}
	} else if *stdin {
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		srv = &server.Server{InterfaceName: *iface, Token: tok, TextContent: string(data)}
	} else {
		// Handle file/directory
		if fs.NArg() < 1 {
			return fmt.Errorf("send requires a path, --text, or --stdin")
		}
		path := fs.Arg(0)

		// Check if path exists
		if _, err := os.Stat(path); err != nil {
			return errors.FileNotFoundError(path, err)
		}

		srv = &server.Server{
			InterfaceName: *iface,
			Token:         tok,
			SrcPath:       path,
		}
	}

	// Apply optional configurations
	srv.RateLimitMbps = *rateLimit
	srv.MaxCacheSize = *cacheSize * 1024 * 1024 // Convert MB to bytes

	url, err := srv.Start()
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	defer func() { _ = srv.Shutdown() }()

	// Display server info
	fmt.Fprintf(os.Stderr, "Server started on :%d\n", srv.Port)
	serviceName := fmt.Sprintf("warp-%s._warp._tcp.local.", tok[:6])
	fmt.Fprintf(os.Stderr, "Service: %s\n", serviceName)
	fmt.Fprintf(os.Stderr, "Local URL: %s\n", url)
	fmt.Fprintf(os.Stderr, "Metrics: http://%s:%d/metrics\n", srv.IP.String(), srv.Port)

	if !*noQR {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, ui.C.Bold+"Scan QR code on another device:"+ui.C.Reset)
		_ = uipkg.PrintQR(url)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, ui.C.Dim+"Tip: Open the URL in any browser to download"+ui.C.Reset)
	}

	fmt.Fprint(os.Stderr, "\n"+ui.C.Yellow+"Press Ctrl+C to stop server"+ui.C.Reset+"\n")

	// Wait for interrupt signal for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down gracefully...")

	return nil
}

func sendHelp() {
	fmt.Println(ui.C.Bold + ui.C.Green + "warp send" + ui.C.Reset + " - Share a file, directory, or text snippet")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Usage:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " [flags] <path>")
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " --text <text>")
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " --stdin < file")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Description:" + ui.C.Reset)
	fmt.Println("  Start a server and share a file, directory, or text with another device.")
	fmt.Println("  The recipient can download using the generated URL or token.")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Flags:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Yellow + "-p, --port" + ui.C.Reset + "        choose specific port (default: random)")
	fmt.Println("  " + ui.C.Yellow + "-i, --interface" + ui.C.Reset + "   bind to a specific network interface")
	fmt.Println("  " + ui.C.Yellow + "--text string" + ui.C.Reset + "     send a text snippet instead of a file")
	fmt.Println("  " + ui.C.Yellow + "--stdin" + ui.C.Reset + "           read text content from stdin")
	fmt.Println("  " + ui.C.Yellow + "--rate-limit" + ui.C.Reset + "      limit download bandwidth in Mbps (0 = unlimited)")
	fmt.Println("  " + ui.C.Yellow + "--cache-size" + ui.C.Reset + "      file cache size in MB (default: 100)")
	fmt.Println("  " + ui.C.Yellow + "--no-qr" + ui.C.Reset + "           skip printing the QR code")
	fmt.Println("  " + ui.C.Yellow + "--encrypt" + ui.C.Reset + "         encrypt transfer with password (prompts if not provided)")
	fmt.Println("  " + ui.C.Yellow + "-v, --verbose" + ui.C.Reset + "     verbose logging (use -vv or -vvv for more detail)")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Examples:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " ./photo.jpg                    " + ui.C.Dim + "# Share a file" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " ./documents/                   " + ui.C.Dim + "# Share a directory" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " --text \"hello world\"           " + ui.C.Dim + "# Share text" + ui.C.Reset)
	fmt.Println("  echo \"hello\" | " + ui.C.Green + "warp send" + ui.C.Reset + " --stdin         " + ui.C.Dim + "# Read from stdin" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " -p 8080 ./file.zip             " + ui.C.Dim + "# Use specific port" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " --rate-limit 10 ./video.mp4    " + ui.C.Dim + "# Limit to 10 Mbps" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp send" + ui.C.Reset + " --encrypt ./secret.pdf         " + ui.C.Dim + "# Encrypted transfer" + ui.C.Reset)
}

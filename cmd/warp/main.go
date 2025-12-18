package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zulfikawr/warp/internal/client"
	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/discovery"
	"github.com/zulfikawr/warp/internal/server"
	"github.com/zulfikawr/warp/internal/ui"
)

// ANSI colors for readable help output (toggled via --no-color / NO_COLOR)
var (
	cReset   string
	cBold    string
	cDim     string
	cGreen   string
	cYellow  string
	cMagenta string
)

func setColorsEnabled(enabled bool) {
	if !enabled {
		cReset, cBold, cDim, cGreen, cYellow, cMagenta = "", "", "", "", "", ""
		return
	}
	cReset = "\033[0m"
	cBold = "\033[1m"
	cDim = "\033[2m"
	cGreen = "\033[32m"
	cYellow = "\033[33m"
	cMagenta = "\033[35m"
}

// filter out global flags that subcommands don't recognize
func filterGlobalFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--no-color" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func main() {
	log.SetFlags(0)
	// Determine color usage from env and global flag
	enableColors := os.Getenv("NO_COLOR") == ""
	for _, a := range os.Args[1:] {
		if a == "--no-color" {
			enableColors = false
			break
		}
	}
	setColorsEnabled(enableColors)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	sub := os.Args[1]
	switch sub {
	case "send":
		sendCmd(filterGlobalFlags(os.Args[2:]))
	case "host":
		hostCmd(filterGlobalFlags(os.Args[2:]))
	case "receive":
		receiveCmd(filterGlobalFlags(os.Args[2:]))
	case "search":
		searchCmd(filterGlobalFlags(os.Args[2:]))
	case "config":
		configCmd(filterGlobalFlags(os.Args[2:]))
	case "completion":
		completionCmd(filterGlobalFlags(os.Args[2:]))
	case "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("     ")
	fmt.Println("██   ██  ▀▀█▄ ████▄ ████▄")
	fmt.Println("██ █ ██ ▄█▀██ ██ ▀▀ ██ ██")
	fmt.Println(" ██▀██  ▀█▄██ ██    ████▀")
	fmt.Println("                    ██    ")
	fmt.Println("                    ▀▀    ")
	fmt.Println(cDim + "a quick file and text transfer with SHA256 verification" + cReset)
	fmt.Println()

	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " [flags] <path>")
	fmt.Println("  " + cGreen + "warp send" + cReset + " --text <text>")
	fmt.Println("  " + cGreen + "warp send" + cReset + " --stdin < file")
	fmt.Println("  " + cGreen + "warp host" + cReset + " [flags]")
	fmt.Println("  " + cGreen + "warp receive" + cReset + " [flags] <url>")
	fmt.Println("  " + cGreen + "warp search" + cReset + " [flags]")
	fmt.Println("  " + cGreen + "warp config" + cReset + " [show|edit|path]")
	fmt.Println("  " + cGreen + "warp completion" + cReset + " [bash|zsh|fish|powershell]")
	fmt.Println()

	fmt.Println(cBold + "Commands:" + cReset)
	fmt.Println("  " + cMagenta + "send" + cReset + "  Share a file, directory, or text snippet")
	fmt.Println("\t" + cYellow + "-p, --port" + cReset + "        choose specific port (default random)")
	fmt.Println("\t" + cYellow + "-i, --interface" + cReset + "   bind to a specific network interface")
	fmt.Println("\t" + cYellow + "--text string" + cReset + "     send a text snippet instead of a file")
	fmt.Println("\t" + cYellow + "--stdin" + cReset + "           read text from stdin")
	fmt.Println("\t" + cYellow + "--rate-limit" + cReset + "      limit bandwidth in Mbps (e.g., 10)")
	fmt.Println("\t" + cYellow + "--cache-size" + cReset + "      file cache size in MB (default 100)")
	fmt.Println("\t" + cYellow + "--no-qr" + cReset + "           skip printing the QR code")
	fmt.Println()
	fmt.Println("  " + cMagenta + "host" + cReset + "  Receive uploads into a directory you control")
	fmt.Println("\t" + cYellow + "-i, --interface" + cReset + "   bind to a specific network interface")
	fmt.Println("\t" + cYellow + "-d, --dest" + cReset + "        destination directory for uploads (default .)")
	fmt.Println("\t" + cYellow + "--rate-limit" + cReset + "      limit bandwidth in Mbps (e.g., 10)")
	fmt.Println("\t" + cYellow + "--no-qr" + cReset + "           skip printing the QR code")
	fmt.Println()
	fmt.Println("  " + cMagenta + "receive" + cReset + "  Download from a warp URL (with parallel chunks & checksum)")
	fmt.Println("\t" + cYellow + "-o, --output" + cReset + "      write to a specific file or directory")
	fmt.Println("\t" + cYellow + "-f, --force" + cReset + "       overwrite existing files")
	fmt.Println("\t" + cYellow + "--workers" + cReset + "         parallel upload workers (default 3)")
	fmt.Println("\t" + cYellow + "--chunk-size" + cReset + "      chunk size in MB (default 2)")
	fmt.Println("\t" + cYellow + "--no-checksum" + cReset + "     skip SHA256 verification")
	fmt.Println("\t" + cYellow + "--from-clipboard" + cReset + "  scan QR code from clipboard")
	fmt.Println()
	fmt.Println("  " + cMagenta + "search" + cReset + "   Discover nearby warp hosts via mDNS")
	fmt.Println("\t" + cYellow + "--timeout" + cReset + "          duration to wait for discovery (default 3s)")
	fmt.Println()
	fmt.Println("  " + cMagenta + "config" + cReset + "   Manage configuration file")
	fmt.Println("\t" + cYellow + "show" + cReset + "              display current configuration")
	fmt.Println("\t" + cYellow + "edit" + cReset + "              open config file in $EDITOR")
	fmt.Println("\t" + cYellow + "path" + cReset + "              show config file path")
	fmt.Println()
	fmt.Println("  " + cMagenta + "completion" + cReset + "   Generate shell completion scripts")
	fmt.Println("\t" + cYellow + "bash" + cReset + "              generate bash completion")
	fmt.Println("\t" + cYellow + "zsh" + cReset + "               generate zsh completion")
	fmt.Println("\t" + cYellow + "fish" + cReset + "              generate fish completion")
	fmt.Println("\t" + cYellow + "powershell" + cReset + "        generate powershell completion")
	fmt.Println()

	fmt.Println(cBold + "Examples:" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " ./photo.jpg " + cDim + "		    # Share a file" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " --text \"hello\" " + cDim + "	            # Share text" + cReset)
	fmt.Println("  " + cGreen + "warp host" + cReset + " -d uploads " + cDim + "		            # Save uploads to dir" + cReset)
	fmt.Println("  " + cGreen + "warp search" + cReset + " " + cDim + "				    # Discover hosts" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " http://hostname:port/<token> " + cDim + "# Download" + cReset)
	fmt.Println()
	fmt.Println(cDim + "Use \"warp <command> -h\" for command-specific help." + cReset)
}

func sendHelp() {
	fmt.Println(cBold + cGreen + "warp send" + cReset + " - Share a file, directory, or text snippet")
	fmt.Println()
	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " [flags] <path>")
	fmt.Println("  " + cGreen + "warp send" + cReset + " --text <text>")
	fmt.Println("  " + cGreen + "warp send" + cReset + " --stdin < file")
	fmt.Println()
	fmt.Println(cBold + "Description:" + cReset)
	fmt.Println("  Start a server and share a file, directory, or text with another device.")
	fmt.Println("  The recipient can download using the generated URL or token.")
	fmt.Println()
	fmt.Println(cBold + "Flags:" + cReset)
	fmt.Println("  " + cYellow + "-p, --port" + cReset + "        choose specific port (default: random)")
	fmt.Println("  " + cYellow + "-i, --interface" + cReset + "   bind to a specific network interface")
	fmt.Println("  " + cYellow + "--text string" + cReset + "     send a text snippet instead of a file")
	fmt.Println("  " + cYellow + "--stdin" + cReset + "           read text content from stdin")
	fmt.Println("  " + cYellow + "--rate-limit" + cReset + "      limit download bandwidth in Mbps (0 = unlimited)")
	fmt.Println("  " + cYellow + "--cache-size" + cReset + "      file cache size in MB (default: 100)")
	fmt.Println("  " + cYellow + "--no-qr" + cReset + "           skip printing the QR code")
	fmt.Println("  " + cYellow + "--encrypt" + cReset + "         encrypt transfer with password (prompts if not provided)")
	fmt.Println("  " + cYellow + "-v, --verbose" + cReset + "     verbose logging")
	fmt.Println()
	fmt.Println(cBold + "Examples:" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " ./photo.jpg                    " + cDim + "# Share a file" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " ./documents/                   " + cDim + "# Share a directory" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " --text \"hello world\"           " + cDim + "# Share text" + cReset)
	fmt.Println("  echo \"hello\" | " + cGreen + "warp send" + cReset + " --stdin         " + cDim + "# Read from stdin" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " -p 8080 ./file.zip             " + cDim + "# Use specific port" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " --rate-limit 10 ./video.mp4    " + cDim + "# Limit to 10 Mbps" + cReset)
	fmt.Println("  " + cGreen + "warp send" + cReset + " --encrypt ./secret.pdf         " + cDim + "# Encrypted transfer" + cReset)
}

func hostHelp() {
	fmt.Println(cBold + cGreen + "warp host" + cReset + " - Receive uploads into a directory you control")
	fmt.Println()
	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  " + cGreen + "warp host" + cReset + " [flags]")
	fmt.Println()
	fmt.Println(cBold + "Description:" + cReset)
	fmt.Println("  Start an upload server and receive files from other devices.")
	fmt.Println("  Uploaded files are saved to the specified directory.")
	fmt.Println()
	fmt.Println(cBold + "Flags:" + cReset)
	fmt.Println("  " + cYellow + "-i, --interface" + cReset + "   bind to a specific network interface")
	fmt.Println("  " + cYellow + "-d, --dest" + cReset + "        destination directory for uploads (default: .)")
	fmt.Println("  " + cYellow + "--rate-limit" + cReset + "      limit upload bandwidth in Mbps (0 = unlimited)")
	fmt.Println("  " + cYellow + "--no-qr" + cReset + "           skip printing the QR code")
	fmt.Println("  " + cYellow + "--encrypt" + cReset + "         require encrypted uploads with password")
	fmt.Println("  " + cYellow + "-v, --verbose" + cReset + "     verbose logging")
	fmt.Println()
	fmt.Println(cBold + "Examples:" + cReset)
	fmt.Println("  " + cGreen + "warp host" + cReset + "                                " + cDim + "# Accept uploads to current directory" + cReset)
	fmt.Println("  " + cGreen + "warp host" + cReset + " -d ./uploads                   " + cDim + "# Save uploads to ./uploads" + cReset)
	fmt.Println("  " + cGreen + "warp host" + cReset + " -d ./downloads -i eth0         " + cDim + "# Bind to specific interface" + cReset)
	fmt.Println("  " + cGreen + "warp host" + cReset + " --rate-limit 50 -d ./uploads   " + cDim + "# Limit to 50 Mbps" + cReset)
	fmt.Println("  " + cGreen + "warp host" + cReset + " --encrypt -d ./secure          " + cDim + "# Require encrypted uploads" + cReset)
}

func receiveHelp() {
	fmt.Println(cBold + cGreen + "warp receive" + cReset + " - Download from a warp URL")
	fmt.Println()
	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " [flags] <url>")
	fmt.Println()
	fmt.Println(cBold + "Description:" + cReset)
	fmt.Println("  Connect to a warp server and download the shared file or text.")
	fmt.Println("  Files are verified with SHA256 checksums automatically.")
	fmt.Println("  Supports parallel chunk uploads for large files (configurable workers).")
	fmt.Println("  Text content is printed to stdout by default.")
	fmt.Println()
	fmt.Println(cBold + "Flags:" + cReset)
	fmt.Println("  " + cYellow + "-o, --output" + cReset + "      write to a specific file or directory")
	fmt.Println("  " + cYellow + "-f, --force" + cReset + "       overwrite existing files without prompting")
	fmt.Println("  " + cYellow + "--workers" + cReset + "         number of parallel upload workers (default: 3)")
	fmt.Println("  " + cYellow + "--chunk-size" + cReset + "      chunk size in MB for parallel uploads (default: 2)")
	fmt.Println("  " + cYellow + "--no-checksum" + cReset + "     skip SHA256 checksum verification (faster)")
	fmt.Println("  " + cYellow + "--decrypt" + cReset + "         decrypt transfer with password (prompts if not provided)")
	fmt.Println("  " + cYellow + "-v, --verbose" + cReset + "     verbose logging")
	fmt.Println()
	fmt.Println(cBold + "Examples:" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " http://host:port/d/token                      " + cDim + "# Download file" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " http://host:port/d/token -o myfile.zip        " + cDim + "# Save with custom name" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " http://host:port/d/token -d downloads         " + cDim + "# Save to directory" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " http://host:port/d/token --workers 5          " + cDim + "# Use 5 parallel workers" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " http://host:port/d/token --no-checksum        " + cDim + "# Skip verification" + cReset)
	fmt.Println("  " + cGreen + "warp receive" + cReset + " http://host:port/d/token --decrypt            " + cDim + "# Decrypt transfer" + cReset)
}

func searchHelp() {
	fmt.Println(cBold + cGreen + "warp search" + cReset + " - Discover nearby warp hosts via mDNS")
	fmt.Println()
	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  " + cGreen + "warp search" + cReset + " [flags]")
	fmt.Println()
	fmt.Println(cBold + "Description:" + cReset)
	fmt.Println("  Search for warp servers on your local network using mDNS (Bonjour).")
	fmt.Println("  Displays discovered hosts with their names, modes, and URLs.")
	fmt.Println()
	fmt.Println(cBold + "Flags:" + cReset)
	fmt.Println("  " + cYellow + "--timeout" + cReset + "          duration to wait for discovery (default: 3s)")
	fmt.Println()
	fmt.Println(cBold + "Examples:" + cReset)
	fmt.Println("  " + cGreen + "warp search" + cReset + "                        " + cDim + "# Search with default 3s timeout" + cReset)
	fmt.Println("  " + cGreen + "warp search" + cReset + " --timeout 5s           " + cDim + "# Search for 5 seconds" + cReset)
	fmt.Println("  " + cGreen + "warp search" + cReset + " --timeout 100ms        " + cDim + "# Quick search" + cReset)
}

func sendCmd(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	fs.Usage = sendHelp
	port := fs.Int("port", 0, "specific port")
	fs.IntVar(port, "p", 0, "")
	noQR := fs.Bool("no-qr", false, "disable QR")
	iface := fs.String("interface", "", "network interface")
	fs.StringVar(iface, "i", "", "")
	text := fs.String("text", "", "send text instead of file")
	stdin := fs.Bool("stdin", false, "read from stdin")
	rateLimit := fs.Float64("rate-limit", 0, "bandwidth limit in Mbps")
	cacheSize := fs.Int64("cache-size", 100, "file cache size in MB")
	verbose := fs.Bool("verbose", false, "verbose logging")
	fs.BoolVar(verbose, "v", false, "")
	fs.Parse(args)

	tok, err := crypto.GenerateToken(nil)
	if err != nil { log.Fatal(err) }

	var srv *server.Server

	// Handle text sharing
	if *text != "" {
		srv = &server.Server{InterfaceName: *iface, Token: tok, TextContent: *text}
	} else if *stdin {
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil { log.Fatal(err) }
		srv = &server.Server{InterfaceName: *iface, Token: tok, TextContent: string(data)}
	} else {
		// Handle file/directory
		if fs.NArg() < 1 {
			log.Fatal("send requires a path, --text, or --stdin")
		}
		path := fs.Arg(0)
		srv = &server.Server{
			InterfaceName: *iface,
			Token: tok,
			SrcPath: path,
		}
	}
	
	// Apply optional configurations
	if srv != nil {
		srv.RateLimitMbps = *rateLimit
		srv.MaxCacheSize = *cacheSize * 1024 * 1024 // Convert MB to bytes
	}

	url, err := srv.Start()
	if err != nil { log.Fatal(err) }
	defer srv.Shutdown()

	// Display server info
	fmt.Fprintf(os.Stderr, "Server started on :%d\n", srv.Port)
	serviceName := fmt.Sprintf("warp-%s._warp._tcp.local.", tok[:6])
	fmt.Fprintf(os.Stderr, "Service: %s\n", serviceName)
	fmt.Fprintf(os.Stderr, "Local URL: %s\n", url)
	fmt.Fprintf(os.Stderr, "Metrics: http://%s:%d/metrics\n", srv.IP.String(), srv.Port)

	if !*noQR {
		fmt.Fprintln(os.Stderr)
		_ = ui.PrintQR(url)
	}

	fmt.Fprintf(os.Stderr, "\nPress Ctrl+C to stop server\n")

	// Wait for interrupt signal for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down gracefully...")
}

func receiveCmd(args []string) {
	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	fs.Usage = receiveHelp
	out := fs.String("output", "", "output path")
	fs.StringVar(out, "o", "", "")
	force := fs.Bool("force", false, "overwrite existing")
	fs.BoolVar(force, "f", false, "")
	workers := fs.Int("workers", 3, "parallel upload workers")
	chunkSizeMB := fs.Int("chunk-size", 2, "chunk size in MB")
	noChecksum := fs.Bool("no-checksum", false, "skip checksum verification")
	verbose := fs.Bool("verbose", false, "verbose logging")
	fs.BoolVar(verbose, "v", false, "")
	fs.Parse(args)
	
	if fs.NArg() < 1 {
		log.Fatal("receive requires a URL")
	}
	url := fs.Arg(0)
	
	if *verbose {
		fmt.Printf("Configuration: workers=%d, chunk-size=%dMB, checksum=%v\n",
			*workers, *chunkSizeMB, !*noChecksum)
	}
	
	// Note: Workers and chunk-size are for future client-side parallel downloads
	// Currently used by server-side parallel uploads via HTML client
	file, err := client.Receive(url, *out, *force, os.Stdout)
	if err != nil { log.Fatal(err) }
	if file == "(stdout)" {
		// Text was output to stdout, just print newline
		fmt.Println()
	}
	// Removed redundant "Saved to" print since receiver.go now prints it
}

func hostCmd(args []string) {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	fs.Usage = hostHelp
	iface := fs.String("interface", "", "network interface")
	fs.StringVar(iface, "i", "", "")
	dest := fs.String("dest", ".", "destination directory for uploads")
	fs.StringVar(dest, "d", ".", "")
	noQR := fs.Bool("no-qr", false, "disable QR")
	rateLimit := fs.Float64("rate-limit", 0, "bandwidth limit in Mbps")
	verbose := fs.Bool("verbose", false, "verbose logging")
	fs.BoolVar(verbose, "v", false, "")
	fs.Parse(args)

	// Ensure destination exists
	if err := os.MkdirAll(*dest, 0o755); err != nil {
		log.Fatal(err)
	}

	tok, err := crypto.GenerateToken(nil)
	if err != nil { log.Fatal(err) }
	srv := &server.Server{
		InterfaceName: *iface,
		Token: tok,
		HostMode: true,
		UploadDir: *dest,
	}
	
	// Apply optional configurations
	srv.RateLimitMbps = *rateLimit
	
	url, err := srv.Start()
	if err != nil { log.Fatal(err) }
	defer srv.Shutdown()

	fmt.Fprintf(os.Stderr, "Hosting uploads to '%s'\n", *dest)
	fmt.Fprintf(os.Stderr, "Token: %s\n", tok)
	if *rateLimit > 0 {
		fmt.Fprintf(os.Stderr, "Rate limit: %.1f Mbps\n", *rateLimit)
	}
	fmt.Fprintf(os.Stderr, "Features: Parallel chunks, SHA256 verification, WebSocket progress\n")

	if !*noQR {
		fmt.Fprintln(os.Stderr)
		_ = ui.PrintQR(url)
	}

	fmt.Fprintf(os.Stderr, "\nOpen this on another device to upload:\n%s\n", url)

	// Wait for interrupt signal for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down gracefully...")
}

func searchCmd(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	fs.Usage = searchHelp
	timeout := fs.Duration("timeout", 3*time.Second, "discovery timeout")
	fs.Parse(args)

	fmt.Println("Searching for warp services on local network...")
	fmt.Println()

	services, err := discovery.Browse(context.Background(), *timeout)
	if err != nil {
		log.Fatal(err)
	}

	if len(services) == 0 {
		fmt.Println("No warp hosts found")
		return
	}

	fmt.Printf("Found %d service", len(services))
	if len(services) != 1 {
		fmt.Print("s")
	}
	fmt.Println(":")
	fmt.Println()

	for i, svc := range services {
		fmt.Printf("%d. %s\n", i+1, svc.Name)
		fmt.Printf("   Mode: %s\n", svc.Mode)
		fmt.Printf("   Address: %s:%d\n", svc.IP, svc.Port)
		fmt.Printf("   URL: %s\n", svc.URL)
		if i < len(services)-1 {
			fmt.Println()
		}
	}
}

func configCmd(args []string) {
	if len(args) == 0 {
		configHelp()
		os.Exit(0)
	}

	subcmd := args[0]
	switch subcmd {
	case "show":
		cfg, err := config.LoadConfig()
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		configPath := config.GetConfigPath()
		fmt.Println(cBold + "Current Configuration:" + cReset)
		fmt.Printf("  Config file: %s\n", configPath)
		fmt.Println()
		fmt.Printf("  %-20s %v\n", "Default Interface:", cfg.DefaultInterface)
		fmt.Printf("  %-20s %d\n", "Default Port:", cfg.DefaultPort)
		fmt.Printf("  %-20s %d bytes\n", "Buffer Size:", cfg.BufferSize)
		fmt.Printf("  %-20s %d GB\n", "Max Upload Size:", cfg.MaxUploadSize/(1024*1024*1024))
		fmt.Printf("  %-20s %.1f Mbps\n", "Rate Limit:", cfg.RateLimitMbps)
		fmt.Printf("  %-20s %d MB\n", "Cache Size:", cfg.CacheSizeMB)
		fmt.Printf("  %-20s %d MB\n", "Chunk Size:", cfg.ChunkSizeMB)
		fmt.Printf("  %-20s %d\n", "Parallel Workers:", cfg.ParallelWorkers)
		fmt.Printf("  %-20s %v\n", "No QR Code:", cfg.NoQR)
		fmt.Printf("  %-20s %v\n", "No Checksum:", cfg.NoChecksum)
		fmt.Printf("  %-20s %s\n", "Upload Directory:", cfg.UploadDir)

	case "edit":
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		
		configPath := config.GetConfigPath()
		
		// Create config file if it doesn't exist
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			cfg := config.DefaultConfig()
			if err := config.SaveConfig(cfg); err != nil {
				log.Fatalf("Failed to create config file: %v", err)
			}
			fmt.Printf("Created new config file at: %s\n", configPath)
		}
		
		// Open editor
		cmd := fmt.Sprintf("%s %s", editor, configPath)
		fmt.Printf("Opening %s...\n", configPath)
		if err := syscall.Exec("/bin/sh", []string{"/bin/sh", "-c", cmd}, os.Environ()); err != nil {
			log.Fatalf("Failed to open editor: %v", err)
		}

	case "path":
		fmt.Println(config.GetConfigPath())

	case "-h", "--help", "help":
		configHelp()

	default:
		fmt.Printf("Unknown config subcommand: %s\n", subcmd)
		configHelp()
		os.Exit(1)
	}
}

func configHelp() {
	fmt.Println(cBold + cGreen + "warp config" + cReset + " - Manage configuration file")
	fmt.Println()
	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  " + cGreen + "warp config show" + cReset + "  Display current configuration")
	fmt.Println("  " + cGreen + "warp config edit" + cReset + "  Open config file in $EDITOR")
	fmt.Println("  " + cGreen + "warp config path" + cReset + "  Show config file path")
	fmt.Println()
	fmt.Println(cBold + "Configuration File:" + cReset)
	fmt.Println("  Location: ~/.config/warp/warp.yaml")
	fmt.Println("  Format:   YAML")
	fmt.Println()
	fmt.Println(cBold + "Available Settings:" + cReset)
	fmt.Println("  " + cYellow + "default_interface" + cReset + "  Network interface to bind to")
	fmt.Println("  " + cYellow + "default_port" + cReset + "       Port to use (0 = random)")
	fmt.Println("  " + cYellow + "buffer_size" + cReset + "        I/O buffer size in bytes")
	fmt.Println("  " + cYellow + "max_upload_size" + cReset + "    Maximum upload size in bytes")
	fmt.Println("  " + cYellow + "rate_limit_mbps" + cReset + "    Bandwidth limit in Mbps")
	fmt.Println("  " + cYellow + "cache_size_mb" + cReset + "      File cache size in MB")
	fmt.Println("  " + cYellow + "chunk_size_mb" + cReset + "      Chunk size for parallel uploads")
	fmt.Println("  " + cYellow + "parallel_workers" + cReset + "   Number of parallel upload workers")
	fmt.Println("  " + cYellow + "no_qr" + cReset + "              Skip QR code display")
	fmt.Println("  " + cYellow + "no_checksum" + cReset + "        Skip SHA256 verification")
	fmt.Println("  " + cYellow + "upload_dir" + cReset + "         Default upload directory")
	fmt.Println()
	fmt.Println(cBold + "Examples:" + cReset)
	fmt.Println("  " + cGreen + "warp config show" + cReset + "              " + cDim + "# View current settings" + cReset)
	fmt.Println("  " + cGreen + "warp config edit" + cReset + "              " + cDim + "# Edit configuration" + cReset)
	fmt.Println("  " + cGreen + "warp config path" + cReset + "              " + cDim + "# Show config location" + cReset)
	fmt.Println()
	fmt.Println(cDim + "Configuration values can also be set via environment variables:" + cReset)
	fmt.Println(cDim + "  WARP_RATE_LIMIT_MBPS=10 warp send file.zip" + cReset)
}

func completionCmd(args []string) {
	if len(args) == 0 {
		completionHelp()
		os.Exit(0)
	}

	shell := args[0]
	switch shell {
	case "bash":
		generateBashCompletion()
	case "zsh":
		generateZshCompletion()
	case "fish":
		generateFishCompletion()
	case "powershell":
		generatePowershellCompletion()
	case "-h", "--help", "help":
		completionHelp()
	default:
		fmt.Printf("Unknown shell: %s\n", shell)
		completionHelp()
		os.Exit(1)
	}
}

func completionHelp() {
	fmt.Println(cBold + cGreen + "warp completion" + cReset + " - Generate shell completion scripts")
	fmt.Println()
	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  " + cGreen + "warp completion" + cReset + " [bash|zsh|fish|powershell]")
	fmt.Println()
	fmt.Println(cBold + "Available Shells:" + cReset)
	fmt.Println("  " + cYellow + "bash" + cReset + "              Bash completion script")
	fmt.Println("  " + cYellow + "zsh" + cReset + "               Zsh completion script")
	fmt.Println("  " + cYellow + "fish" + cReset + "              Fish completion script")
	fmt.Println("  " + cYellow + "powershell" + cReset + "        PowerShell completion script")
	fmt.Println()
	fmt.Println(cBold + "Installation:" + cReset)
	fmt.Println()
	fmt.Println(cBold + "  Bash:" + cReset)
	fmt.Println("    $ warp completion bash > /etc/bash_completion.d/warp")
	fmt.Println("    $ source /etc/bash_completion.d/warp")
	fmt.Println()
	fmt.Println(cBold + "  Zsh:" + cReset)
	fmt.Println("    $ warp completion zsh > /usr/local/share/zsh/site-functions/_warp")
	fmt.Println("    $ autoload -U compinit && compinit")
	fmt.Println()
	fmt.Println(cBold + "  Fish:" + cReset)
	fmt.Println("    $ warp completion fish > ~/.config/fish/completions/warp.fish")
	fmt.Println()
	fmt.Println(cBold + "  PowerShell:" + cReset)
	fmt.Println("    $ warp completion powershell | Out-String | Invoke-Expression")
	fmt.Println()
}

func generateBashCompletion() {
	script := `# bash completion for warp
_warp_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    # Main commands
    if [ $COMP_CWORD -eq 1 ]; then
        opts="send host receive search config completion"
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi
    
    # Subcommand completion
    case "${COMP_WORDS[1]}" in
        send)
            opts="-p --port -i --interface --text --stdin --rate-limit --cache-size --no-qr -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            # File completion for paths
            if [[ ! ${cur} == -* ]]; then
                COMPREPLY=( $(compgen -f -- ${cur}) )
            fi
            ;;
        host)
            opts="-i --interface -d --dest --rate-limit --no-qr -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        receive)
            opts="-o --output -f --force --workers --chunk-size --no-checksum -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        search)
            opts="--timeout -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        config)
            if [ $COMP_CWORD -eq 2 ]; then
                opts="show edit path"
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            fi
            ;;
        completion)
            if [ $COMP_CWORD -eq 2 ]; then
                opts="bash zsh fish powershell"
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            fi
            ;;
    esac
}

complete -F _warp_completion warp
`
	fmt.Print(script)
}

func generateZshCompletion() {
	script := `#compdef warp

_warp() {
    local curcontext="$curcontext" state line
    typeset -A opt_args

    _arguments -C \
        '1: :->command' \
        '*:: :->args'

    case $state in
        command)
            local commands=(
                'send:Share a file, directory, or text snippet'
                'host:Receive uploads into a directory'
                'receive:Download from a warp URL'
                'search:Discover nearby warp hosts'
                'config:Manage configuration file'
                'completion:Generate shell completion scripts'
            )
            _describe 'command' commands
            ;;
        args)
            case $line[1] in
                send)
                    _arguments \
                        {-p,--port}'[Port number]' \
                        {-i,--interface}'[Network interface]' \
                        '--text[Send text snippet]' \
                        '--stdin[Read from stdin]' \
                        '--rate-limit[Bandwidth limit in Mbps]' \
                        '--cache-size[Cache size in MB]' \
                        '--no-qr[Skip QR code]' \
                        {-h,--help}'[Show help]' \
                        '*:file:_files'
                    ;;
                host)
                    _arguments \
                        {-i,--interface}'[Network interface]' \
                        {-d,--dest}'[Destination directory]' \
                        '--rate-limit[Bandwidth limit in Mbps]' \
                        '--no-qr[Skip QR code]' \
                        {-h,--help}'[Show help]'
                    ;;
                receive)
                    _arguments \
                        {-o,--output}'[Output file]' \
                        {-f,--force}'[Force overwrite]' \
                        '--workers[Parallel workers]' \
                        '--chunk-size[Chunk size in MB]' \
                        '--no-checksum[Skip checksum]' \
                        {-h,--help}'[Show help]'
                    ;;
                search)
                    _arguments \
                        '--timeout[Discovery timeout]' \
                        {-h,--help}'[Show help]'
                    ;;
                config)
                    local config_commands=(
                        'show:Display current configuration'
                        'edit:Open config file in editor'
                        'path:Show config file path'
                    )
                    _describe 'config command' config_commands
                    ;;
                completion)
                    local shells=(
                        'bash:Bash completion'
                        'zsh:Zsh completion'
                        'fish:Fish completion'
                        'powershell:PowerShell completion'
                    )
                    _describe 'shell' shells
                    ;;
            esac
            ;;
    esac
}

_warp "$@"
`
	fmt.Print(script)
}

func generateFishCompletion() {
	script := `# fish completion for warp

# Main commands
complete -c warp -f -n '__fish_use_subcommand' -a send -d 'Share a file, directory, or text snippet'
complete -c warp -f -n '__fish_use_subcommand' -a host -d 'Receive uploads into a directory'
complete -c warp -f -n '__fish_use_subcommand' -a receive -d 'Download from a warp URL'
complete -c warp -f -n '__fish_use_subcommand' -a search -d 'Discover nearby warp hosts'
complete -c warp -f -n '__fish_use_subcommand' -a config -d 'Manage configuration file'
complete -c warp -f -n '__fish_use_subcommand' -a completion -d 'Generate shell completion scripts'

# send command
complete -c warp -f -n '__fish_seen_subcommand_from send' -s p -l port -d 'Port number'
complete -c warp -f -n '__fish_seen_subcommand_from send' -s i -l interface -d 'Network interface'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l text -d 'Send text snippet'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l stdin -d 'Read from stdin'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l rate-limit -d 'Bandwidth limit in Mbps'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l cache-size -d 'Cache size in MB'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l no-qr -d 'Skip QR code'
complete -c warp -f -n '__fish_seen_subcommand_from send' -s h -l help -d 'Show help'

# host command
complete -c warp -f -n '__fish_seen_subcommand_from host' -s i -l interface -d 'Network interface'
complete -c warp -f -n '__fish_seen_subcommand_from host' -s d -l dest -d 'Destination directory'
complete -c warp -f -n '__fish_seen_subcommand_from host' -l rate-limit -d 'Bandwidth limit in Mbps'
complete -c warp -f -n '__fish_seen_subcommand_from host' -l no-qr -d 'Skip QR code'
complete -c warp -f -n '__fish_seen_subcommand_from host' -s h -l help -d 'Show help'

# receive command
complete -c warp -f -n '__fish_seen_subcommand_from receive' -s o -l output -d 'Output file'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -s f -l force -d 'Force overwrite'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -l workers -d 'Parallel workers'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -l chunk-size -d 'Chunk size in MB'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -l no-checksum -d 'Skip checksum'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -s h -l help -d 'Show help'

# search command
complete -c warp -f -n '__fish_seen_subcommand_from search' -l timeout -d 'Discovery timeout'
complete -c warp -f -n '__fish_seen_subcommand_from search' -s h -l help -d 'Show help'

# config command
complete -c warp -f -n '__fish_seen_subcommand_from config' -a 'show' -d 'Display current configuration'
complete -c warp -f -n '__fish_seen_subcommand_from config' -a 'edit' -d 'Open config file in editor'
complete -c warp -f -n '__fish_seen_subcommand_from config' -a 'path' -d 'Show config file path'

# completion command
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'bash' -d 'Bash completion'
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'zsh' -d 'Zsh completion'
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'fish' -d 'Fish completion'
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'powershell' -d 'PowerShell completion'
`
	fmt.Print(script)
}

func generatePowershellCompletion() {
	script := `# PowerShell completion for warp

Register-ArgumentCompleter -Native -CommandName warp -ScriptBlock {
    param($commandName, $wordToComplete, $commandAst, $fakeBoundParameters)

    $commands = @(
        [System.Management.Automation.CompletionResult]::new('send', 'send', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Share a file')
        [System.Management.Automation.CompletionResult]::new('host', 'host', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Receive uploads')
        [System.Management.Automation.CompletionResult]::new('receive', 'receive', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Download from URL')
        [System.Management.Automation.CompletionResult]::new('search', 'search', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Discover hosts')
        [System.Management.Automation.CompletionResult]::new('config', 'config', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Manage config')
        [System.Management.Automation.CompletionResult]::new('completion', 'completion', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Generate completion')
    )

    $commands | Where-Object { $_.CompletionText -like "$wordToComplete*" }
}
`
	fmt.Print(script)
}

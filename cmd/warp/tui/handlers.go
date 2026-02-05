// Package tui provides the terminal user interface for warp.
// This file contains command handlers that parse flags and launch TUI screens.
package tui

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/ui"
)

// RunApp starts the TUI application with the given app model.
func RunApp(a *App) {
	p := tea.NewProgram(a, tea.WithAltScreen())
	if err := p.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", ui.Colors.Red, err, ui.Colors.Reset)
		os.Exit(1)
	}
}

// HandleSend parses flags and launches the Send TUI.
func HandleSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	port := fs.Int("port", 0, "specific port")
	fs.IntVar(port, "p", 0, "")
	noQR := fs.Bool("no-qr", false, "disable QR")
	iface := fs.String("interface", "", "network interface")
	fs.StringVar(iface, "i", "", "")
	text := fs.String("text", "", "send text instead of file")
	stdin := fs.Bool("stdin", false, "read from stdin")
	rateLimit := fs.Float64("rate-limit", 0, "bandwidth limit in Mbps")
	cacheSize := fs.Int64("cache-size", 0, "file cache size in MB")
	noEncrypt := fs.Bool("no-encrypt", false, "disable encryption")
	verbose := fs.Bool("verbose", false, "verbose logging")
	fs.BoolVar(verbose, "v", false, "")

	// Custom usage to trigger TUI help
	fs.Usage = func() {
		launchSendHelp()
	}

	// Handle help flag manually to redirect to TUI
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			launchSendHelp()
			return
		}
	}

	_ = fs.Parse(args)

	a := NewApp()
	a.SetScreen(ScreenSend)

	opts := SendOptions{
		Port:          *port,
		InterfaceName: *iface,
		RateLimitMbps: *rateLimit,
		CacheSizeMB:   *cacheSize,
		NoQR:          *noQR,
		NoEncrypt:     *noEncrypt,
		TextContent:   *text,
	}

	if *stdin {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			bytes, _ := io.ReadAll(os.Stdin)
			opts.StdinContent = string(bytes)
		}
	}

	a.Send().Options = opts

	if fs.NArg() > 0 {
		path := fs.Arg(0)
		fi, err := os.Stat(path)
		if err == nil {
			absPath, _ := filepath.Abs(path)
			if !fi.IsDir() {
				a.Send().AutoStartPath = absPath
				a.Send().FilePicker.Dir = filepath.Dir(absPath)
			} else {
				a.Send().FilePicker.Dir = absPath
			}
		}
	} else if opts.TextContent != "" || opts.StdinContent != "" {
		a.Send().StartUpload("text-snippet")
	}

	RunApp(a)
}

func launchSendHelp() {
	a := NewApp()
	a.SetScreen(ScreenSend)
	a.Send().State = StateSendHelp
	RunApp(a)
	os.Exit(0)
}

// HandleHost parses flags and launches the Host TUI.
func HandleHost(args []string) {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	iface := fs.String("interface", "", "network interface")
	fs.StringVar(iface, "i", "", "")
	dest := fs.String("dest", "", "destination directory")
	fs.StringVar(dest, "d", "", "")
	noQR := fs.Bool("no-qr", false, "disable QR")
	rateLimit := fs.Float64("rate-limit", 0, "bandwidth limit in Mbps")
	noEncrypt := fs.Bool("no-encrypt", false, "disable encryption")
	verbose := fs.Bool("verbose", false, "verbose logging")
	fs.BoolVar(verbose, "v", false, "")

	// Handle help flag manually
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			launchHostHelp()
			return
		}
	}

	_ = fs.Parse(args)

	a := NewApp()
	a.SetScreen(ScreenHost)

	a.Host().Options = HostOptions{
		InterfaceName: *iface,
		DestDir:       *dest,
		RateLimitMbps: *rateLimit,
		NoQR:          *noQR,
		NoEncrypt:     *noEncrypt,
	}

	RunApp(a)
}

func launchHostHelp() {
	a := NewApp()
	a.SetScreen(ScreenHost)
	a.Host().SetHelp()
	RunApp(a)
	os.Exit(0)
}

// HandleReceive parses flags and launches the Receive TUI.
func HandleReceive(args []string) {
	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	code := fs.String("code", "", "PAKE code")
	fs.StringVar(code, "c", "", "")
	out := fs.String("output", "", "output path")
	fs.StringVar(out, "o", "", "")
	force := fs.Bool("force", false, "overwrite existing")
	fs.BoolVar(force, "f", false, "")
	workers := fs.Int("workers", 0, "parallel workers")
	chunkSize := fs.Int("chunk-size", 0, "chunk size MB")
	noChecksum := fs.Bool("no-checksum", false, "skip checksum")
	decrypt := fs.Bool("decrypt", false, "decrypt")
	verbose := fs.Bool("verbose", false, "verbose logging")
	fs.BoolVar(verbose, "v", false, "")

	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			launchReceiveHelp()
			return
		}
	}

	_ = fs.Parse(args)

	a := NewApp()
	a.SetScreen(ScreenReceive)

	opts := ReceiveOptions{
		Code:       *code,
		OutputPath: *out,
		Force:      *force,
		Workers:    *workers,
		ChunkSize:  *chunkSize,
		NoChecksum: *noChecksum,
		Decrypt:    *decrypt,
	}

	if fs.NArg() > 0 {
		opts.Code = fs.Arg(0)
	}

	a.Receive().Options = opts
	RunApp(a)
}

func launchReceiveHelp() {
	a := NewApp()
	a.SetScreen(ScreenReceive)
	a.Receive().SetHelp()
	RunApp(a)
	os.Exit(0)
}

// HandleSearch parses flags and launches the Search TUI.
func HandleSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)

	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			launchSearchHelp()
			return
		}
	}

	_ = fs.Parse(args)

	a := NewApp()
	a.SetScreen(ScreenSearch)
	RunApp(a)
}

func launchSearchHelp() {
	a := NewApp()
	a.SetScreen(ScreenSearch)
	a.Search().SetHelp()
	RunApp(a)
	os.Exit(0)
}

// HandleConfig handles 'warp config' subcommands.
func HandleConfig(args []string) {
	if len(args) == 0 {
		launchConfigTUI()
		return
	}

	switch args[0] {
	case "init":
		launchConfigTUI()
	case "path":
		fmt.Println(config.GetConfigPath())
	case "show":
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			return
		}
		fmt.Printf("Config File: %s\n", config.GetConfigPath())
		fmt.Printf("%+v\n", cfg)
	case "edit":
		launchConfigTUI()
	case "--help", "-h":
		launchConfigHelp()
	default:
		launchConfigHelp()
	}
}

func launchConfigTUI() {
	a := NewApp()
	a.SetScreen(ScreenConfig)
	RunApp(a)
}

func launchConfigHelp() {
	a := NewApp()
	a.SetScreen(ScreenConfig)
	a.Config().SetHelp()
	RunApp(a)
	os.Exit(0)
}

// HandleHistory handles 'warp history' subcommand.
func HandleHistory(args []string) {
	// Parse flags for consistency and help support
	fs := flag.NewFlagSet("history", flag.ExitOnError)

	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			// launchHistoryHelp() // History screen doesn't have a help mode yet, just show TUI
			break
		}
	}

	_ = fs.Parse(args)

	a := NewApp()
	a.SetScreen(ScreenHistory)
	RunApp(a)
}

// HandleResume handles 'warp resume' subcommand.
func HandleResume(args []string) {
	fs := flag.NewFlagSet("resume", flag.ExitOnError)

	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			break
		}
	}

	_ = fs.Parse(args)

	a := NewApp()
	a.SetScreen(ScreenResume)
	RunApp(a)
}

// HandleDefault attempts to treat the argument as a file path for sending.
func HandleDefault(arg string) error {
	fi, statErr := os.Stat(arg)
	if statErr == nil && !fi.IsDir() {
		a := NewApp()
		a.SetScreen(ScreenSend)
		absPath, _ := filepath.Abs(arg)
		a.Send().AutoStartPath = absPath
		a.Send().FilePicker.Dir = filepath.Dir(absPath)

		RunApp(a)
		return nil
	}
	return fmt.Errorf("unknown command: %s", arg)
}

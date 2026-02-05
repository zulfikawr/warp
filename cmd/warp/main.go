// Package main is the entry point for the warp application.
// It handles command-line argument parsing and routes to either CLI or TUI mode.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zulfikawr/warp/cmd/warp/cli"
	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/cmd/warp/tui"
	"github.com/zulfikawr/warp/internal/ui"
)

func main() {
	log.SetFlags(0)

	// Check for CLI mode flag first
	isCLI := false
	args := os.Args[1:]
	filteredArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--cli" {
			isCLI = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	// Launch TUI home screen if no arguments provided and not CLI mode
	if len(filteredArgs) == 0 && !isCLI {
		tui.RunApp(tui.NewApp())
		return
	}

	if len(filteredArgs) == 0 {
		fmt.Println("Usage: warp [--cli] <command> [args...]")
		return
	}

	var err error
	sub := filteredArgs[0]
	subArgs := filteredArgs[1:]

	if isCLI {
		if err := runCLICommand(sub, subArgs); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch sub {
	case "send":
		tui.HandleSend(subArgs)
	case "host":
		tui.HandleHost(subArgs)
	case "receive":
		tui.HandleReceive(subArgs)
	case "search":
		tui.HandleSearch(subArgs)
	case "help":
		help.PrintRoot()
	case "config":
		tui.HandleConfig(subArgs)
	case "history":
		tui.HandleHistory(subArgs)
	case "resume":
		tui.HandleResume(subArgs)
	case "version":
		cli.Version()
		return
	default:
		// Attempt to interpret as a file path shortcut for send
		err = tui.HandleDefault(sub)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", ui.Colors.Red, err, ui.Colors.Reset)
		os.Exit(1)
	}
}

// runCLICommand executes a command in CLI mode (text-only output).
func runCLICommand(cmd string, args []string) error {
	switch cmd {
	case "send":
		return cli.Send(args)
	case "receive":
		return cli.Receive(args)
	case "host":
		return cli.Host(args)
	case "search":
		return cli.Search(args)
	case "history":
		return cli.History(args)
	case "resume":
		return cli.Resume(args)
	case "version":
		cli.Version()
		return nil
	case "config":
		return cli.Config(args)
	case "help":
		help.PrintRoot()
		return nil
	default:
		return fmt.Errorf("command '%s' not implemented in CLI mode yet", cmd)
	}
}

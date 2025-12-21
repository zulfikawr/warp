package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zulfikawr/warp/cmd/warp/commands"
	"github.com/zulfikawr/warp/cmd/warp/completion"
	"github.com/zulfikawr/warp/cmd/warp/ui"
	"github.com/zulfikawr/warp/internal/errors"
)

// filterGlobalFlags removes global flags that subcommands don't recognize
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
	ui.SetColorsEnabled(enableColors)

	if len(os.Args) < 2 {
		ui.PrintUsage()
		os.Exit(2)
	}

	var err error
	sub := os.Args[1]
	switch sub {
	case "send":
		err = commands.Send(filterGlobalFlags(os.Args[2:]))
	case "host":
		err = commands.Host(filterGlobalFlags(os.Args[2:]))
	case "receive":
		err = commands.Receive(filterGlobalFlags(os.Args[2:]))
	case "search":
		err = commands.Search(filterGlobalFlags(os.Args[2:]))
	case "config":
		err = commands.Config(filterGlobalFlags(os.Args[2:]))
	case "speedtest":
		err = commands.Speedtest(filterGlobalFlags(os.Args[2:]))
	case "completion":
		err = completion.Generate(filterGlobalFlags(os.Args[2:]))
	case "-h", "--help":
		ui.PrintUsage()
		return
	default:
		ui.PrintUsage()
		os.Exit(2)
	}

	// Handle errors in one place
	if err != nil {
		// Format user-friendly errors nicely
		if errors.IsUserError(err) {
			fmt.Fprintf(os.Stderr, "%s%s%s\n", ui.C.Red, err.Error(), ui.C.Reset)
		} else {
			// For non-user errors, just show the error
			fmt.Fprintf(os.Stderr, "%sError: %v%s\n", ui.C.Red, err, ui.C.Reset)
		}
		os.Exit(1)
	}
}

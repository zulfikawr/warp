package help

import (
	"fmt"

	"github.com/zulfikawr/warp/internal/ui"
)

var SearchHelpLines = []string{
	ui.Colors.Bold + ui.Colors.Green + "warp search" + ui.Colors.Reset + " - Discover nearby warp hosts via mDNS",
	"",
	ui.Colors.Bold + "Usage:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp search" + ui.Colors.Reset + " [flags]",
	"",
	ui.Colors.Bold + "Description:" + ui.Colors.Reset,
	"  Search for warp servers on your local network using mDNS (Bonjour).",
	"  Displays discovered hosts with their names, modes, and URLs.",
	"",
	ui.Colors.Bold + "Flags:" + ui.Colors.Reset,
	"  " + ui.Colors.Yellow + "--timeout" + ui.Colors.Reset + "          duration to wait for discovery (default: 3s)",
	"",
	ui.Colors.Bold + "Examples:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp search" + ui.Colors.Reset + "                        " + ui.Colors.Dim + "# Search with default 3s timeout" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp search" + ui.Colors.Reset + " --timeout 5s           " + ui.Colors.Dim + "# Search for 5 seconds" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp search" + ui.Colors.Reset + " --timeout 100ms        " + ui.Colors.Dim + "# Quick search" + ui.Colors.Reset,
}

func PrintSearch() {
	for _, line := range SearchHelpLines {
		fmt.Println(line)
	}
}

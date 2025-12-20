package commands

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/zulfikawr/warp/cmd/warp/ui"
	"github.com/zulfikawr/warp/internal/discovery"
)

// Search executes the search command
func Search(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	fs.Usage = searchHelp
	timeout := fs.Duration("timeout", 3*time.Second, "discovery timeout")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	fmt.Println("Searching for warp services on local network...")
	fmt.Println()

	services, err := discovery.Browse(context.Background(), *timeout)
	if err != nil {
		return fmt.Errorf("service discovery failed: %w", err)
	}

	if len(services) == 0 {
		fmt.Println("No warp hosts found")
		return nil
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

	return nil
}

func searchHelp() {
	fmt.Println(ui.C.Bold + ui.C.Green + "warp search" + ui.C.Reset + " - Discover nearby warp hosts via mDNS")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Usage:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp search" + ui.C.Reset + " [flags]")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Description:" + ui.C.Reset)
	fmt.Println("  Search for warp servers on your local network using mDNS (Bonjour).")
	fmt.Println("  Displays discovered hosts with their names, modes, and URLs.")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Flags:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Yellow + "--timeout" + ui.C.Reset + "          duration to wait for discovery (default: 3s)")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Examples:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp search" + ui.C.Reset + "                        " + ui.C.Dim + "# Search with default 3s timeout" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp search" + ui.C.Reset + " --timeout 5s           " + ui.C.Dim + "# Search for 5 seconds" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp search" + ui.C.Reset + " --timeout 100ms        " + ui.C.Dim + "# Quick search" + ui.C.Reset)
}

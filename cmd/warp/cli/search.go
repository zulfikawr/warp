package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/internal/core"
)

// Search executes the search command in CLI mode.
// It discovers warp services on the local network and displays them.
func Search(args []string) error {
	opts, err := parseSearchFlags(args)
	if err != nil {
		return err
	}

	// Create executor with CLI callback
	exec := core.NewSearchExecutor(opts, printStatus)

	// Execute the search
	services, err := exec.Execute(context.Background())
	if err != nil {
		return err
	}

	// Display results
	printSearchResults(services)

	return nil
}

// parseSearchFlags parses command-line flags into SearchOptions.
func parseSearchFlags(args []string) (core.SearchOptions, error) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	fs.Usage = help.PrintSearch

	timeout := fs.Duration("timeout", 3*time.Second, "discovery timeout duration")

	if err := fs.Parse(args); err != nil {
		return core.SearchOptions{}, fmt.Errorf("failed to parse flags: %w", err)
	}

	return core.SearchOptions{
		Timeout: *timeout,
	}, nil
}

// printSearchResults displays discovered services to stdout.
func printSearchResults(services []core.ServiceInfo) {
	fmt.Println()

	if len(services) == 0 {
		fmt.Println("No warp hosts found")
		return
	}

	// Pluralize correctly
	serviceWord := "service"
	if len(services) != 1 {
		serviceWord = "services"
	}
	fmt.Printf("Found %d %s:\n\n", len(services), serviceWord)

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

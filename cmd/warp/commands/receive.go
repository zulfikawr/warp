package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/zulfikawr/warp/cmd/warp/ui"
	"github.com/zulfikawr/warp/internal/client"
	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/errors"
	"github.com/zulfikawr/warp/internal/logging"
)

// Receive executes the receive command
func Receive(args []string) error {
	// Load configuration (config file → env vars)
	cfg, err := config.LoadConfig()
	if err != nil {
		return errors.ConfigError("Failed to load configuration", err)
	}

	// Count -v flags and filter them out
	verbosity, filteredArgs := countVerbosity(args)

	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	fs.Usage = receiveHelp
	out := fs.String("output", "", "output path")
	fs.StringVar(out, "o", "", "")
	force := fs.Bool("force", false, "overwrite existing")
	fs.BoolVar(force, "f", false, "")
	// Use config defaults for flags (config → env → flags precedence)
	workers := fs.Int("workers", cfg.ParallelWorkers, "parallel upload workers")
	chunkSizeMB := fs.Int("chunk-size", cfg.ChunkSizeMB, "chunk size in MB")
	noChecksum := fs.Bool("no-checksum", cfg.NoChecksum, "skip checksum verification")
	if err := fs.Parse(filteredArgs); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	// Set log level based on verbosity
	if verbosity > 0 {
		logging.SetLevel(verbosity)
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("receive requires a URL")
	}
	url := fs.Arg(0)

	if verbosity > 0 {
		fmt.Printf("Configuration: workers=%d, chunk-size=%dMB, checksum=%v\n",
			*workers, *chunkSizeMB, !*noChecksum)
	}

	// Note: Workers and chunk-size are for future client-side parallel downloads
	// Currently used by server-side parallel uploads via HTML client
	file, err := client.Receive(url, *out, *force, os.Stdout)
	if err != nil {
		return err // client.Receive already wraps errors appropriately
	}
	if file == "(stdout)" {
		// Text was output to stdout, just print newline
		fmt.Println()
	}
	// Removed redundant "Saved to" print since receiver.go now prints it

	return nil
}

func receiveHelp() {
	fmt.Println(ui.C.Bold + ui.C.Green + "warp receive" + ui.C.Reset + " - Download from a warp URL")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Usage:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp receive" + ui.C.Reset + " [flags] <url>")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Description:" + ui.C.Reset)
	fmt.Println("  Connect to a warp server and download the shared file or text.")
	fmt.Println("  Files are verified with SHA256 checksums automatically.")
	fmt.Println("  Supports parallel chunk uploads for large files (configurable workers).")
	fmt.Println("  Text content is printed to stdout by default.")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Flags:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Yellow + "-o, --output" + ui.C.Reset + "      write to a specific file or directory")
	fmt.Println("  " + ui.C.Yellow + "-f, --force" + ui.C.Reset + "       overwrite existing files without prompting")
	fmt.Println("  " + ui.C.Yellow + "--workers" + ui.C.Reset + "         number of parallel upload workers (default: 3)")
	fmt.Println("  " + ui.C.Yellow + "--chunk-size" + ui.C.Reset + "      chunk size in MB for parallel uploads (default: 2)")
	fmt.Println("  " + ui.C.Yellow + "--no-checksum" + ui.C.Reset + "     skip SHA256 checksum verification (faster)")
	fmt.Println("  " + ui.C.Yellow + "--decrypt" + ui.C.Reset + "         decrypt transfer with password (prompts if not provided)")
	fmt.Println("  " + ui.C.Yellow + "-v, --verbose" + ui.C.Reset + "     verbose logging (use -vv or -vvv for more detail)")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Examples:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp receive" + ui.C.Reset + " http://host:port/d/token                      " + ui.C.Dim + "# Download file" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp receive" + ui.C.Reset + " http://host:port/d/token -o myfile.zip        " + ui.C.Dim + "# Save with custom name" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp receive" + ui.C.Reset + " http://host:port/d/token -d downloads         " + ui.C.Dim + "# Save to directory" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp receive" + ui.C.Reset + " http://host:port/d/token --workers 5          " + ui.C.Dim + "# Use 5 parallel workers" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp receive" + ui.C.Reset + " http://host:port/d/token --no-checksum        " + ui.C.Dim + "# Skip verification" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp receive" + ui.C.Reset + " http://host:port/d/token --decrypt            " + ui.C.Dim + "# Decrypt transfer" + ui.C.Reset)
}

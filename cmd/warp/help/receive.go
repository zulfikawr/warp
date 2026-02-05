package help

import (
	"fmt"

	"github.com/zulfikawr/warp/internal/ui"
)

var ReceiveHelpLines = []string{
	ui.Colors.Bold + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " - Download from a warp URL or PAKE code",
	"",
	ui.Colors.Bold + "Usage:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " [flags] <code|url>",
	"",
	ui.Colors.Bold + "Description:" + ui.Colors.Reset,
	"  Connect to a warp server and download the shared file or text.",
	"  You can provide either a 12-bit PAKE code (e.g., 76-tourist-teach) or a direct URL.",
	"  If a code is provided, it will search for servers in the local network.",
	"  Files are verified with SHA256 checksums automatically.",
	"  Supports parallel chunk uploads for large files (configurable workers).",
	"  Text content is printed to stdout by default.",
	"",
	ui.Colors.Bold + "Flags:" + ui.Colors.Reset,
	"  " + ui.Colors.Yellow + "-o, --output" + ui.Colors.Reset + "      write to a specific file or directory",
	"  " + ui.Colors.Yellow + "-f, --force" + ui.Colors.Reset + "       overwrite existing files without prompting",
	"  " + ui.Colors.Yellow + "--workers" + ui.Colors.Reset + "         number of parallel upload workers (default: 3)",
	"  " + ui.Colors.Yellow + "--chunk-size" + ui.Colors.Reset + "      chunk size in MB for parallel uploads (default: 2)",
	"  " + ui.Colors.Yellow + "--no-checksum" + ui.Colors.Reset + "     skip SHA256 checksum verification (faster)",
	"  " + ui.Colors.Yellow + "--decrypt" + ui.Colors.Reset + "         decrypt transfer with password (prompts if not provided)",
	"  " + ui.Colors.Yellow + "-v, --verbose" + ui.Colors.Reset + "     verbose logging (use -vv or -vvv for more detail)",
	"",
	ui.Colors.Bold + "Examples:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " 7-apple-velocity                " + ui.Colors.Dim + "# Secure transfer via code" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " http://host:port/d/code           " + ui.Colors.Dim + "# Download via URL" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " http://host:port/d/code -o file   " + ui.Colors.Dim + "# Save with custom name" + ui.Colors.Reset,
}

func PrintReceive() {
	for _, line := range ReceiveHelpLines {
		fmt.Println(line)
	}
}

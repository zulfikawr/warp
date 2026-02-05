package help

import (
	"fmt"

	"github.com/zulfikawr/warp/internal/ui"
)

var RootHelpLines = []string{
	"     ",
	"██   ██  ▀▀█▄ ████▄ ████▄",
	"██ █ ██ ▄█▀██ ██ ▀▀ ██ ██",
	" ██▀██  ▀█▄██ ██    ████▀",
	"                    ██    ",
	"                    ▀▀    ",
	ui.Colors.Dim + "a quick file and text transfer with SHA256 verification" + ui.Colors.Reset,
	"",
	ui.Colors.Bold + "Usage:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " [flags] <path>",
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --text <text>",
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --stdin < file",
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + " [flags]",
	"  " + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " [flags] <code|url>",
	"  " + ui.Colors.Green + "warp search" + ui.Colors.Reset + " [flags]",
	"  " + ui.Colors.Green + "warp history" + ui.Colors.Reset + " [flags]",
	"  " + ui.Colors.Green + "warp resume" + ui.Colors.Reset + " [flags] <session-id>",
	"  " + ui.Colors.Green + "warp config" + ui.Colors.Reset + " [show|edit|path]",
	"",
	ui.Colors.Bold + "Commands:" + ui.Colors.Reset,
	"  " + ui.Colors.Magenta + "send" + ui.Colors.Reset + "  Share a file, directory, or text snippet",
	"    " + ui.Colors.Yellow + "-p, --port" + ui.Colors.Reset + "        choose specific port (default random)",
	"    " + ui.Colors.Yellow + "-i, --interface" + ui.Colors.Reset + "   bind to a specific network interface",
	"    " + ui.Colors.Yellow + "--text string" + ui.Colors.Reset + "     send a text snippet instead of a file",
	"    " + ui.Colors.Yellow + "--stdin" + ui.Colors.Reset + "           read text from stdin",
	"    " + ui.Colors.Yellow + "--rate-limit" + ui.Colors.Reset + "      limit bandwidth in Mbps (e.g., 10)",
	"    " + ui.Colors.Yellow + "--cache-size" + ui.Colors.Reset + "      file cache size in MB (default 100)",
	"    " + ui.Colors.Yellow + "--no-qr" + ui.Colors.Reset + "           skip printing the QR code",
	"",
	"  " + ui.Colors.Magenta + "host" + ui.Colors.Reset + "  Receive uploads into a directory you control",
	"    " + ui.Colors.Yellow + "-i, --interface" + ui.Colors.Reset + "   bind to a specific network interface",
	"    " + ui.Colors.Yellow + "-d, --dest" + ui.Colors.Reset + "        destination directory for uploads (default .)",
	"    " + ui.Colors.Yellow + "--rate-limit" + ui.Colors.Reset + "      limit bandwidth in Mbps (e.g., 10)",
	"    " + ui.Colors.Yellow + "--no-qr" + ui.Colors.Reset + "           skip printing the QR code",
	"",
	"  " + ui.Colors.Magenta + "receive" + ui.Colors.Reset + "  Download from a warp URL or PAKE code",
	"    " + ui.Colors.Yellow + "-o, --output" + ui.Colors.Reset + "      write to a specific file or directory",
	"    " + ui.Colors.Yellow + "-f, --force" + ui.Colors.Reset + "       overwrite existing files",
	"    " + ui.Colors.Yellow + "--workers" + ui.Colors.Reset + "         parallel upload workers (default 3)",
	"    " + ui.Colors.Yellow + "--chunk-size" + ui.Colors.Reset + "      chunk size in MB (default 2)",
	"    " + ui.Colors.Yellow + "--no-checksum" + ui.Colors.Reset + "     skip SHA256 verification",
	"    " + ui.Colors.Yellow + "--from-clipboard" + ui.Colors.Reset + "  scan QR code from clipboard",
	"",
	"  " + ui.Colors.Magenta + "search" + ui.Colors.Reset + "   Discover nearby warp hosts via mDNS",
	"    " + ui.Colors.Yellow + "--timeout" + ui.Colors.Reset + "          duration to wait for discovery (default 3s)",
	"",
	"  " + ui.Colors.Magenta + "history" + ui.Colors.Reset + "  View transfer history",
	"    " + ui.Colors.Yellow + "--cli" + ui.Colors.Reset + "               output in text mode",
	"",
	"  " + ui.Colors.Magenta + "resume" + ui.Colors.Reset + "   Manage interrupted transfers",
	"    " + ui.Colors.Yellow + "(no args)" + ui.Colors.Reset + "           list resumable sessions",
	"    " + ui.Colors.Yellow + "<session-id>" + ui.Colors.Reset + "        resume a specific session",
	"",
	"  " + ui.Colors.Magenta + "config" + ui.Colors.Reset + "   Manage configuration file",
	"    " + ui.Colors.Yellow + "init" + ui.Colors.Reset + "              create config interactively",
	"    " + ui.Colors.Yellow + "show" + ui.Colors.Reset + "              display current configuration",
	"    " + ui.Colors.Yellow + "edit" + ui.Colors.Reset + "              open config file in $EDITOR",
	"    " + ui.Colors.Yellow + "path" + ui.Colors.Reset + "              show config file path",
	"",
	ui.Colors.Bold + "Examples:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " ./photo.jpg " + ui.Colors.Dim + "             # Share a file" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --text \"hello\" " + ui.Colors.Dim + "             # Share text" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + " -d uploads " + ui.Colors.Dim + "                 # Save uploads to dir" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp search" + ui.Colors.Reset + " " + ui.Colors.Dim + "                    # Discover hosts" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " 7-apple-velocity " + ui.Colors.Dim + "          # Secure transfer via code" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp receive" + ui.Colors.Reset + " http://host:port/d/code " + ui.Colors.Dim + "    # Download via URL" + ui.Colors.Reset,
	"",
	ui.Colors.Dim + "Use \"warp <command> -h\" for command-specific help." + ui.Colors.Reset,
}

func PrintRoot() {
	for _, line := range RootHelpLines {
		fmt.Println(line)
	}
}

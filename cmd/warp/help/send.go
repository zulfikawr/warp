package help

import (
	"fmt"

	"github.com/zulfikawr/warp/internal/ui"
)

var SendHelpLines = []string{
	ui.Colors.Bold + ui.Colors.Green + "warp send" + ui.Colors.Reset + " - Share a file, directory, or text snippet",
	"",
	ui.Colors.Bold + "Usage:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " [flags] <path>",
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --text <text>",
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --stdin < file",
	"",
	ui.Colors.Bold + "Description:" + ui.Colors.Reset,
	"  Start a server and share a file, directory, or text with another device.",
	"  Generates a secure PAKE code for easy transfer and end-to-end encryption.",
	"",
	ui.Colors.Bold + "Flags:" + ui.Colors.Reset,
	"  " + ui.Colors.Yellow + "-p, --port" + ui.Colors.Reset + "        choose specific port (default: random)",
	"  " + ui.Colors.Yellow + "-i, --interface" + ui.Colors.Reset + "   bind to a specific network interface",
	"  " + ui.Colors.Yellow + "--text string" + ui.Colors.Reset + "     send a text snippet instead of a file",
	"  " + ui.Colors.Yellow + "--stdin" + ui.Colors.Reset + "           read text content from stdin",
	"  " + ui.Colors.Yellow + "--rate-limit" + ui.Colors.Reset + "      limit download bandwidth in Mbps (0 = unlimited)",
	"  " + ui.Colors.Yellow + "--cache-size" + ui.Colors.Reset + "      file cache size in MB (default: 100)",
	"  " + ui.Colors.Yellow + "--no-qr" + ui.Colors.Reset + "           skip printing the QR code",
	"  " + ui.Colors.Yellow + "--no-encrypt" + ui.Colors.Reset + "       disable encryption (not recommended)",
	"  " + ui.Colors.Yellow + "-v, --verbose" + ui.Colors.Reset + "     verbose logging (use -vv or -vvv for more detail)",
	"",
	ui.Colors.Bold + "Examples:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " ./photo.jpg                    " + ui.Colors.Dim + "# Share a file (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " ./documents/                   " + ui.Colors.Dim + "# Share a directory (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --text \"hello world\"           " + ui.Colors.Dim + "# Share text (encrypted)" + ui.Colors.Reset,
	"  echo \"hello\" | " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --stdin         " + ui.Colors.Dim + "# Read from stdin (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " -p 8080 ./file.zip             " + ui.Colors.Dim + "# Use specific port (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --rate-limit 10 ./video.mp4    " + ui.Colors.Dim + "# Limit to 10 Mbps (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp send" + ui.Colors.Reset + " --no-encrypt ./public.pdf      " + ui.Colors.Dim + "# Unencrypted transfer" + ui.Colors.Reset,
}

func PrintSend() {
	for _, line := range SendHelpLines {
		fmt.Println(line)
	}
}

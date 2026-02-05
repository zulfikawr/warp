package help

import (
	"fmt"

	"github.com/zulfikawr/warp/internal/ui"
)

var HostHelpLines = []string{
	ui.Colors.Bold + ui.Colors.Green + "warp host" + ui.Colors.Reset + " - Receive uploads into a directory you control",
	"",
	ui.Colors.Bold + "Usage:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + " [flags]",
	"",
	ui.Colors.Bold + "Description:" + ui.Colors.Reset,
	"  Start an upload server and receive files from other devices.",
	"  Uploaded files are saved to the specified directory.",
	"",
	ui.Colors.Bold + "Flags:" + ui.Colors.Reset,
	"  " + ui.Colors.Yellow + "-i, --interface" + ui.Colors.Reset + "   bind to a specific network interface",
	"  " + ui.Colors.Yellow + "-d, --dest" + ui.Colors.Reset + "        destination directory for uploads (default: .)",
	"  " + ui.Colors.Yellow + "--rate-limit" + ui.Colors.Reset + "      limit upload bandwidth in Mbps (0 = unlimited)",
	"  " + ui.Colors.Yellow + "--no-qr" + ui.Colors.Reset + "           skip printing the QR code",
	"  " + ui.Colors.Yellow + "--no-encrypt" + ui.Colors.Reset + "      disable encryption (not recommended)",
	"  " + ui.Colors.Yellow + "-v, --verbose" + ui.Colors.Reset + "     verbose logging (use -vv or -vvv for more detail)",
	"",
	ui.Colors.Bold + "Examples:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + "                                " + ui.Colors.Dim + "# Accept uploads to current directory (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + " -d ./uploads                   " + ui.Colors.Dim + "# Save uploads to ./uploads (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + " -d ./downloads -i eth0         " + ui.Colors.Dim + "# Bind to specific interface (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + " --rate-limit 50 -d ./uploads   " + ui.Colors.Dim + "# Limit to 50 Mbps (encrypted)" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp host" + ui.Colors.Reset + " --no-encrypt -d ./public       " + ui.Colors.Dim + "# Unencrypted uploads" + ui.Colors.Reset,
}

func PrintHost() {
	for _, line := range HostHelpLines {
		fmt.Println(line)
	}
}

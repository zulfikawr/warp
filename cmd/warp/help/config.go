package help

import (
	"fmt"

	"github.com/zulfikawr/warp/internal/ui"
)

var ConfigHelpLines = []string{
	ui.Colors.Bold + ui.Colors.Green + "warp config" + ui.Colors.Reset + " - Manage configuration file",
	"",
	ui.Colors.Bold + "Usage:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp config init" + ui.Colors.Reset + "  Initialize configuration interactively",
	"  " + ui.Colors.Green + "warp config show" + ui.Colors.Reset + "  Display current configuration",
	"  " + ui.Colors.Green + "warp config edit" + ui.Colors.Reset + "  Open config file in $EDITOR",
	"  " + ui.Colors.Green + "warp config path" + ui.Colors.Reset + "  Show config file path",
	"",
	ui.Colors.Bold + "Configuration File:" + ui.Colors.Reset,
	"  Location: ~/.config/warp/warp.yaml",
	"  Format:   YAML",
	"",
	ui.Colors.Bold + "Available Settings:" + ui.Colors.Reset,
	"  " + ui.Colors.Yellow + "default_interface" + ui.Colors.Reset + "  Network interface to bind to",
	"  " + ui.Colors.Yellow + "default_port" + ui.Colors.Reset + "       Port to use (0 = random)",
	"  " + ui.Colors.Yellow + "buffer_size" + ui.Colors.Reset + "        I/O buffer size in bytes",
	"  " + ui.Colors.Yellow + "max_upload_size" + ui.Colors.Reset + "    Maximum upload size in bytes",
	"  " + ui.Colors.Yellow + "rate_limit_mbps" + ui.Colors.Reset + "    Bandwidth limit in Mbps",
	"  " + ui.Colors.Yellow + "cache_size_mb" + ui.Colors.Reset + "      File cache size in MB",
	"  " + ui.Colors.Yellow + "chunk_size_mb" + ui.Colors.Reset + "      Chunk size for parallel uploads",
	"  " + ui.Colors.Yellow + "parallel_workers" + ui.Colors.Reset + "   Number of parallel upload workers",
	"  " + ui.Colors.Yellow + "no_qr" + ui.Colors.Reset + "              Skip QR code display",
	"  " + ui.Colors.Yellow + "no_checksum" + ui.Colors.Reset + "        Skip SHA256 verification",
	"  " + ui.Colors.Yellow + "upload_dir" + ui.Colors.Reset + "         Default upload directory",
	"",
	ui.Colors.Bold + "Examples:" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp config init" + ui.Colors.Reset + "              " + ui.Colors.Dim + "# Create config interactively" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp config show" + ui.Colors.Reset + "              " + ui.Colors.Dim + "# View current settings" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp config edit" + ui.Colors.Reset + "              " + ui.Colors.Dim + "# Edit configuration" + ui.Colors.Reset,
	"  " + ui.Colors.Green + "warp config path" + ui.Colors.Reset + "              " + ui.Colors.Dim + "# Show config location" + ui.Colors.Reset,
	"",
	ui.Colors.Dim + "Configuration values can also be set via environment variables:" + ui.Colors.Reset,
	ui.Colors.Dim + "  WARP_RATE_LIMIT_MBPS=10 warp send file.zip" + ui.Colors.Reset,
}

func PrintConfig() {
	for _, line := range ConfigHelpLines {
		fmt.Println(line)
	}
}

package commands

import (
	"fmt"
	"os"
	"syscall"

	"github.com/zulfikawr/warp/cmd/warp/ui"
	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/errors"
)

// Config executes the config command
func Config(args []string) error {
	if len(args) == 0 {
		configHelp()
		return nil
	}

	subcmd := args[0]
	switch subcmd {
	case "show":
		cfg, err := config.LoadConfig()
		if err != nil {
			return errors.ConfigError("Failed to load configuration", err)
		}
		configPath := config.GetConfigPath()
		fmt.Println(ui.C.Bold + "Current Configuration:" + ui.C.Reset)
		fmt.Printf("  Config file: %s\n", configPath)
		fmt.Println()
		fmt.Printf("  %-20s %v\n", "Default Interface:", cfg.DefaultInterface)
		fmt.Printf("  %-20s %d\n", "Default Port:", cfg.DefaultPort)
		fmt.Printf("  %-20s %d bytes\n", "Buffer Size:", cfg.BufferSize)
		fmt.Printf("  %-20s %d GB\n", "Max Upload Size:", cfg.MaxUploadSize/(1024*1024*1024))
		fmt.Printf("  %-20s %.1f Mbps\n", "Rate Limit:", cfg.RateLimitMbps)
		fmt.Printf("  %-20s %d MB\n", "Cache Size:", cfg.CacheSizeMB)
		fmt.Printf("  %-20s %d MB\n", "Chunk Size:", cfg.ChunkSizeMB)
		fmt.Printf("  %-20s %d\n", "Parallel Workers:", cfg.ParallelWorkers)
		fmt.Printf("  %-20s %v\n", "No QR Code:", cfg.NoQR)
		fmt.Printf("  %-20s %v\n", "No Checksum:", cfg.NoChecksum)
		fmt.Printf("  %-20s %s\n", "Upload Directory:", cfg.UploadDir)

	case "edit":
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}

		configPath := config.GetConfigPath()

		// Create config file if it doesn't exist
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			cfg := config.DefaultConfig()
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to create config file: %w", err)
			}
			fmt.Printf("Created new config file at: %s\n", configPath)
		}

		// Open editor
		cmd := fmt.Sprintf("%s %s", editor, configPath)
		fmt.Printf("Opening %s...\n", configPath)
		if err := syscall.Exec("/bin/sh", []string{"/bin/sh", "-c", cmd}, os.Environ()); err != nil {
			return fmt.Errorf("failed to open editor: %w", err)
		}

	case "path":
		fmt.Println(config.GetConfigPath())

	case "-h", "--help", "help":
		configHelp()

	default:
		fmt.Printf("Unknown config subcommand: %s\n", subcmd)
		configHelp()
		return fmt.Errorf("unknown subcommand: %s", subcmd)
	}

	return nil
}

func configHelp() {
	fmt.Println(ui.C.Bold + ui.C.Green + "warp config" + ui.C.Reset + " - Manage configuration file")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Usage:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config show" + ui.C.Reset + "  Display current configuration")
	fmt.Println("  " + ui.C.Green + "warp config edit" + ui.C.Reset + "  Open config file in $EDITOR")
	fmt.Println("  " + ui.C.Green + "warp config path" + ui.C.Reset + "  Show config file path")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Configuration File:" + ui.C.Reset)
	fmt.Println("  Location: ~/.config/warp/warp.yaml")
	fmt.Println("  Format:   YAML")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Available Settings:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Yellow + "default_interface" + ui.C.Reset + "  Network interface to bind to")
	fmt.Println("  " + ui.C.Yellow + "default_port" + ui.C.Reset + "       Port to use (0 = random)")
	fmt.Println("  " + ui.C.Yellow + "buffer_size" + ui.C.Reset + "        I/O buffer size in bytes")
	fmt.Println("  " + ui.C.Yellow + "max_upload_size" + ui.C.Reset + "    Maximum upload size in bytes")
	fmt.Println("  " + ui.C.Yellow + "rate_limit_mbps" + ui.C.Reset + "    Bandwidth limit in Mbps")
	fmt.Println("  " + ui.C.Yellow + "cache_size_mb" + ui.C.Reset + "      File cache size in MB")
	fmt.Println("  " + ui.C.Yellow + "chunk_size_mb" + ui.C.Reset + "      Chunk size for parallel uploads")
	fmt.Println("  " + ui.C.Yellow + "parallel_workers" + ui.C.Reset + "   Number of parallel upload workers")
	fmt.Println("  " + ui.C.Yellow + "no_qr" + ui.C.Reset + "              Skip QR code display")
	fmt.Println("  " + ui.C.Yellow + "no_checksum" + ui.C.Reset + "        Skip SHA256 verification")
	fmt.Println("  " + ui.C.Yellow + "upload_dir" + ui.C.Reset + "         Default upload directory")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Examples:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config show" + ui.C.Reset + "              " + ui.C.Dim + "# View current settings" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config edit" + ui.C.Reset + "              " + ui.C.Dim + "# Edit configuration" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config path" + ui.C.Reset + "              " + ui.C.Dim + "# Show config location" + ui.C.Reset)
	fmt.Println()
	fmt.Println(ui.C.Dim + "Configuration values can also be set via environment variables:" + ui.C.Reset)
	fmt.Println(ui.C.Dim + "  WARP_RATE_LIMIT_MBPS=10 warp send file.zip" + ui.C.Reset)
}

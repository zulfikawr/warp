package commands

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	case "init":
		return configInit()

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

func configInit() error {
	configPath := config.GetConfigPath()

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf(ui.C.Yellow+"Configuration file already exists at: %s\n"+ui.C.Reset, configPath)
		overwrite := promptYesNo("Do you want to overwrite it?", false)
		if !overwrite {
			fmt.Println(ui.C.Dim + "Configuration initialization cancelled." + ui.C.Reset)
			return nil
		}
	}

	fmt.Println(ui.C.Bold + ui.C.Green + "Initialize Warp Configuration" + ui.C.Reset)
	fmt.Println()
	fmt.Println(ui.C.Cyan + "Press Enter to use default values shown in " + ui.C.Dim + "[brackets]" + ui.C.Reset)
	fmt.Println()

	cfg := config.DefaultConfig()
	scanner := bufio.NewScanner(os.Stdin)

	// Default Interface
	cfg.DefaultInterface = promptString(scanner, ui.C.Cyan+"Network interface "+ui.C.Dim+"(leave empty for auto-detect)"+ui.C.Reset, cfg.DefaultInterface)

	// Default Port
	cfg.DefaultPort = promptInt(scanner, ui.C.Cyan+"Default port "+ui.C.Dim+"(0 for random)"+ui.C.Reset, cfg.DefaultPort)

	// Buffer Size
	bufferMB := cfg.BufferSize / (1024 * 1024)
	bufferMB = promptInt(scanner, ui.C.Cyan+"Buffer size (MB)"+ui.C.Reset, bufferMB)
	cfg.BufferSize = bufferMB * 1024 * 1024

	// Max Upload Size
	maxUploadGB := int(cfg.MaxUploadSize / (1024 * 1024 * 1024))
	maxUploadGB = promptInt(scanner, ui.C.Cyan+"Max upload size (GB)"+ui.C.Reset, maxUploadGB)
	cfg.MaxUploadSize = int64(maxUploadGB) * 1024 * 1024 * 1024

	// Rate Limit
	cfg.RateLimitMbps = promptFloat(scanner, ui.C.Cyan+"Rate limit "+ui.C.Dim+"(Mbps, 0 for no limit)"+ui.C.Reset, cfg.RateLimitMbps)

	// Cache Size
	cfg.CacheSizeMB = int64(promptInt(scanner, ui.C.Cyan+"Cache size (MB)"+ui.C.Reset, int(cfg.CacheSizeMB)))

	// Chunk Size
	cfg.ChunkSizeMB = promptInt(scanner, ui.C.Cyan+"Chunk size (MB)"+ui.C.Reset, cfg.ChunkSizeMB)

	// Parallel Workers
	cfg.ParallelWorkers = promptInt(scanner, ui.C.Cyan+"Number of parallel workers"+ui.C.Reset, cfg.ParallelWorkers)

	// No QR
	cfg.NoQR = promptYesNo(ui.C.Cyan+"Disable QR code display by default?"+ui.C.Reset, cfg.NoQR)

	// No Checksum
	cfg.NoChecksum = promptYesNo(ui.C.Cyan+"Disable SHA256 checksum verification by default?"+ui.C.Reset, cfg.NoChecksum)

	// Upload Directory
	cfg.UploadDir = promptString(scanner, ui.C.Cyan+"Default upload directory"+ui.C.Reset, cfg.UploadDir)

	// Save configuration
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println()
	fmt.Println(ui.C.Green + "✓ Configuration saved to: " + ui.C.Reset + ui.C.Dim + configPath + ui.C.Reset)
	fmt.Println()
	fmt.Println(ui.C.Dim + "You can edit the configuration anytime with:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config edit" + ui.C.Reset)

	return nil
}

func promptString(scanner *bufio.Scanner, prompt string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s "+ui.C.Dim+"[%s]"+ui.C.Reset+": ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return defaultValue
	}
	return input
}

func promptInt(scanner *bufio.Scanner, prompt string, defaultValue int) int {
	fmt.Printf("%s "+ui.C.Dim+"[%d]"+ui.C.Reset+": ", prompt, defaultValue)

	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(input)
	if err != nil {
		fmt.Printf(ui.C.Red+"Invalid number, using default: %d\n"+ui.C.Reset, defaultValue)
		return defaultValue
	}

	return value
}

func promptFloat(scanner *bufio.Scanner, prompt string, defaultValue float64) float64 {
	fmt.Printf("%s "+ui.C.Dim+"[%.1f]"+ui.C.Reset+": ", prompt, defaultValue)

	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(input, 64)
	if err != nil {
		fmt.Printf(ui.C.Red+"Invalid number, using default: %.1f\n"+ui.C.Reset, defaultValue)
		return defaultValue
	}

	return value
}

func promptYesNo(prompt string, defaultValue bool) bool {
	defaultStr := ui.C.Dim + "y/N" + ui.C.Reset
	if defaultValue {
		defaultStr = ui.C.Dim + "Y/n" + ui.C.Reset
	}

	fmt.Printf("%s [%s]: ", prompt, defaultStr)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(strings.ToLower(scanner.Text()))

	if input == "" {
		return defaultValue
	}

	return input == "y" || input == "yes"
}

func configHelp() {
	fmt.Println(ui.C.Bold + ui.C.Green + "warp config" + ui.C.Reset + " - Manage configuration file")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Usage:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config init" + ui.C.Reset + "  Initialize configuration interactively")
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
	fmt.Println("  " + ui.C.Green + "warp config init" + ui.C.Reset + "              " + ui.C.Dim + "# Create config interactively" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config show" + ui.C.Reset + "              " + ui.C.Dim + "# View current settings" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config edit" + ui.C.Reset + "              " + ui.C.Dim + "# Edit configuration" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp config path" + ui.C.Reset + "              " + ui.C.Dim + "# Show config location" + ui.C.Reset)
	fmt.Println()
	fmt.Println(ui.C.Dim + "Configuration values can also be set via environment variables:" + ui.C.Reset)
	fmt.Println(ui.C.Dim + "  WARP_RATE_LIMIT_MBPS=10 warp send file.zip" + ui.C.Reset)
}

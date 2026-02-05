package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/errors"
	"github.com/zulfikawr/warp/internal/ui"
)

// Config executes the config command
func Config(args []string) error {
	if len(args) == 0 {
		help.PrintConfig()
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
		fmt.Println(ui.Colors.Bold + "Current Configuration:" + ui.Colors.Reset)
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
		fmt.Printf("Opening %s...\n", configPath)
		if err := openEditor(configPath); err != nil {
			return fmt.Errorf("failed to open editor: %w", err)
		}

	case "path":
		fmt.Println(config.GetConfigPath())

	case "-h", "--help", "help":
		help.PrintConfig()

	default:
		fmt.Printf("Unknown config subcommand: %s\n", subcmd)
		help.PrintConfig()
		return fmt.Errorf("unknown subcommand: %s", subcmd)
	}

	return nil
}

func configInit() error {
	configPath := config.GetConfigPath()

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf(ui.Colors.Yellow+"Configuration file already exists at: %s\n"+ui.Colors.Reset, configPath)
		overwrite := promptYesNo("Do you want to overwrite it?", false)
		if !overwrite {
			fmt.Println(ui.Colors.Dim + "Configuration initialization cancelled." + ui.Colors.Reset)
			return nil
		}
	}

	fmt.Println(ui.Colors.Bold + ui.Colors.Green + "Initialize Warp Configuration" + ui.Colors.Reset)
	fmt.Println()
	fmt.Println(ui.Colors.Cyan + "Press Enter to use default values shown in " + ui.Colors.Dim + "[brackets]" + ui.Colors.Reset)
	fmt.Println()

	cfg := config.DefaultConfig()
	scanner := bufio.NewScanner(os.Stdin)

	// Default Interface
	cfg.DefaultInterface = promptString(scanner, ui.Colors.Cyan+"Network interface "+ui.Colors.Dim+"(leave empty for auto-detect)"+ui.Colors.Reset, cfg.DefaultInterface)

	// Default Port
	cfg.DefaultPort = promptInt(scanner, ui.Colors.Cyan+"Default port "+ui.Colors.Dim+"(0 for random)"+ui.Colors.Reset, cfg.DefaultPort)

	// Buffer Size
	bufferMB := cfg.BufferSize / (1024 * 1024)
	bufferMB = promptInt(scanner, ui.Colors.Cyan+"Buffer size (MB)"+ui.Colors.Reset, bufferMB)
	cfg.BufferSize = bufferMB * 1024 * 1024

	// Max Upload Size
	maxUploadGB := int(cfg.MaxUploadSize / (1024 * 1024 * 1024))
	maxUploadGB = promptInt(scanner, ui.Colors.Cyan+"Max upload size (GB)"+ui.Colors.Reset, maxUploadGB)
	cfg.MaxUploadSize = int64(maxUploadGB) * 1024 * 1024 * 1024

	// Rate Limit
	cfg.RateLimitMbps = promptFloat(scanner, ui.Colors.Cyan+"Rate limit "+ui.Colors.Dim+"(Mbps, 0 for no limit)"+ui.Colors.Reset, cfg.RateLimitMbps)

	// Cache Size
	cfg.CacheSizeMB = int64(promptInt(scanner, ui.Colors.Cyan+"Cache size (MB)"+ui.Colors.Reset, int(cfg.CacheSizeMB)))

	// Chunk Size
	cfg.ChunkSizeMB = promptInt(scanner, ui.Colors.Cyan+"Chunk size (MB)"+ui.Colors.Reset, cfg.ChunkSizeMB)

	// Parallel Workers
	cfg.ParallelWorkers = promptInt(scanner, ui.Colors.Cyan+"Number of parallel workers"+ui.Colors.Reset, cfg.ParallelWorkers)

	// No QR
	cfg.NoQR = promptYesNo(ui.Colors.Cyan+"Disable QR code display by default?"+ui.Colors.Reset, cfg.NoQR)

	// No Checksum
	cfg.NoChecksum = promptYesNo(ui.Colors.Cyan+"Disable SHA256 checksum verification by default?"+ui.Colors.Reset, cfg.NoChecksum)

	// Upload Directory
	cfg.UploadDir = promptString(scanner, ui.Colors.Cyan+"Default upload directory"+ui.Colors.Reset, cfg.UploadDir)

	// Save configuration
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println()
	fmt.Println(ui.Colors.Green + "✓ Configuration saved to: " + ui.Colors.Reset + ui.Colors.Dim + configPath + ui.Colors.Reset)
	fmt.Println()
	fmt.Println(ui.Colors.Dim + "You can edit the configuration anytime with:" + ui.Colors.Reset)
	fmt.Println("  " + ui.Colors.Green + "warp config edit" + ui.Colors.Reset)

	return nil
}

func promptString(scanner *bufio.Scanner, prompt string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s "+ui.Colors.Dim+"[%s]"+ui.Colors.Reset+": ", prompt, defaultValue)
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
	fmt.Printf("%s "+ui.Colors.Dim+"[%d]"+ui.Colors.Reset+": ", prompt, defaultValue)

	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(input)
	if err != nil {
		fmt.Printf(ui.Colors.Red+"Invalid number, using default: %d\n"+ui.Colors.Reset, defaultValue)
		return defaultValue
	}

	return value
}

func promptFloat(scanner *bufio.Scanner, prompt string, defaultValue float64) float64 {
	fmt.Printf("%s "+ui.Colors.Dim+"[%.1f]"+ui.Colors.Reset+": ", prompt, defaultValue)

	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(input, 64)
	if err != nil {
		fmt.Printf(ui.Colors.Red+"Invalid number, using default: %.1f\n"+ui.Colors.Reset, defaultValue)
		return defaultValue
	}

	return value
}

func promptYesNo(prompt string, defaultValue bool) bool {
	defaultStr := ui.Colors.Dim + "y/N" + ui.Colors.Reset
	if defaultValue {
		defaultStr = ui.Colors.Dim + "Y/n" + ui.Colors.Reset
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

// openEditor opens the specified file in the user's preferred editor.
// It uses the EDITOR environment variable, falling back to platform-specific defaults.
func openEditor(configPath string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}

	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

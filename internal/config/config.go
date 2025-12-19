package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	DefaultInterface string  `mapstructure:"default_interface"`
	DefaultPort      int     `mapstructure:"default_port"`
	BufferSize       int     `mapstructure:"buffer_size"`
	MaxUploadSize    int64   `mapstructure:"max_upload_size"`
	RateLimitMbps    float64 `mapstructure:"rate_limit_mbps"`
	CacheSizeMB      int64   `mapstructure:"cache_size_mb"`
	ChunkSizeMB      int     `mapstructure:"chunk_size_mb"`
	ParallelWorkers  int     `mapstructure:"parallel_workers"`
	NoQR             bool    `mapstructure:"no_qr"`
	NoChecksum       bool    `mapstructure:"no_checksum"`
	UploadDir        string  `mapstructure:"upload_dir"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultInterface: "",          // auto-detect
		DefaultPort:      0,           // random
		BufferSize:       1048576,     // 1MB
		MaxUploadSize:    10737418240, // 10GB
		RateLimitMbps:    0,           // no limit
		CacheSizeMB:      100,         // 100MB
		ChunkSizeMB:      2,           // 2MB
		ParallelWorkers:  3,           // 3 workers
		NoQR:             false,
		NoChecksum:       false,
		UploadDir:        ".",
	}
}

// LoadConfig loads configuration from file or creates default config
func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	// Set config file name and type
	viper.SetConfigName("warp")
	viper.SetConfigType("yaml")

	// Add config paths in order of priority
	if homeDir, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(homeDir, ".config", "warp"))
		viper.AddConfigPath(homeDir) // for .warp.yaml
	}
	viper.AddConfigPath("/etc/warp")
	viper.AddConfigPath(".")

	// Set environment variable prefix
	viper.SetEnvPrefix("WARP")
	viper.AutomaticEnv()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found - use defaults (not an error)
			return config, nil
		}
		// Config file was found but another error occurred (parse error, permission, etc.)
		// Return the actual error so users know their config is broken
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Unmarshal config
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	return config, nil
}

// SaveConfig saves the current configuration to file
func SaveConfig(config *Config) error {
	// Create config directory if it doesn't exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "warp")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "warp.yaml")

	// Set values in viper
	viper.Set("default_interface", config.DefaultInterface)
	viper.Set("default_port", config.DefaultPort)
	viper.Set("buffer_size", config.BufferSize)
	viper.Set("max_upload_size", config.MaxUploadSize)
	viper.Set("rate_limit_mbps", config.RateLimitMbps)
	viper.Set("cache_size_mb", config.CacheSizeMB)
	viper.Set("chunk_size_mb", config.ChunkSizeMB)
	viper.Set("parallel_workers", config.ParallelWorkers)
	viper.Set("no_qr", config.NoQR)
	viper.Set("no_checksum", config.NoChecksum)
	viper.Set("upload_dir", config.UploadDir)

	// Write config file
	if err := viper.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("cannot write config file: %w", err)
	}

	return nil
}

// GetConfigPath returns the path to the config file
func GetConfigPath() string {
	if viper.ConfigFileUsed() != "" {
		return viper.ConfigFileUsed()
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "~/.config/warp/warp.yaml"
	}

	return filepath.Join(homeDir, ".config", "warp", "warp.yaml")
}

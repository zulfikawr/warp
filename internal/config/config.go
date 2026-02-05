package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// runtimeConfig holds the global runtime configuration
var (
	runtimeConfig   *Config
	runtimeConfigMu sync.RWMutex
)

// GetConfig returns the runtime configuration. If not set, returns defaults.
func GetConfig() *Config {
	runtimeConfigMu.RLock()
	defer runtimeConfigMu.RUnlock()

	if runtimeConfig != nil {
		return runtimeConfig
	}
	return DefaultConfig()
}

// SetConfig sets the runtime configuration for the application
func SetConfig(cfg *Config) {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	runtimeConfig = cfg
}

// LoadAndSetConfig loads configuration from file and sets it as the runtime config
func LoadAndSetConfig() (*Config, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	SetConfig(cfg)
	return cfg, nil
}

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
	NoEncrypt        bool    `mapstructure:"no_encrypt"`
	NoChecksum       bool    `mapstructure:"no_checksum"`
	UploadDir        string  `mapstructure:"upload_dir"`

	// Resume configuration
	EnableResume          bool `mapstructure:"enable_resume"`
	AutoCheckpointSizeMB  int  `mapstructure:"auto_checkpoint_size_mb"`
	CheckpointIntervalMB  int  `mapstructure:"checkpoint_interval_mb"`
	CheckpointExpiryHours int  `mapstructure:"checkpoint_expiry_hours"`

	// Protocol tuning (exposes protocol-level settings for advanced users)
	Protocol ProtocolConfig `mapstructure:"protocol"`
}

// ProtocolConfig holds low-level protocol tuning parameters
type ProtocolConfig struct {
	BufferSizeSmall         int           `mapstructure:"buffer_size_small"`
	BufferSizeMedium        int           `mapstructure:"buffer_size_medium"`
	BufferSizeLarge         int           `mapstructure:"buffer_size_large"`
	BufferSizeVeryLarge     int           `mapstructure:"buffer_size_very_large"`
	SmallFileThreshold      int64         `mapstructure:"small_file_threshold"`
	MediumFileThreshold     int64         `mapstructure:"medium_file_threshold"`
	LargeFileThreshold      int64         `mapstructure:"large_file_threshold"`
	SendfileThreshold       int64         `mapstructure:"sendfile_threshold"`
	MaxCacheFileSize        int64         `mapstructure:"max_cache_file_size"`
	ProgressUpdateInterval  time.Duration `mapstructure:"progress_update_interval"`
	WebSocketUpdateInterval time.Duration `mapstructure:"websocket_update_interval"`
	ProgressRefreshRate     int           `mapstructure:"progress_refresh_rate"`
	ReadTimeout             time.Duration `mapstructure:"read_timeout"`
	WriteTimeout            time.Duration `mapstructure:"write_timeout"`
	IdleTimeout             time.Duration `mapstructure:"idle_timeout"`
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
		NoEncrypt:        false,
		NoChecksum:       false,
		UploadDir:        ".",

		// Resume defaults
		EnableResume:          true, // Enable by default
		AutoCheckpointSizeMB:  100,  // Auto-checkpoint for files > 100MB
		CheckpointIntervalMB:  5,    // Save checkpoint every 5MB
		CheckpointExpiryHours: 24,   // Checkpoints expire after 24 hours

		// Protocol defaults
		Protocol: ProtocolConfig{
			BufferSizeSmall:         8 * 1024,          // 8KB
			BufferSizeMedium:        64 * 1024,         // 64KB
			BufferSizeLarge:         1024 * 1024,       // 1MB
			BufferSizeVeryLarge:     4 * 1024 * 1024,   // 4MB
			SmallFileThreshold:      64 * 1024,         // 64KB
			MediumFileThreshold:     1024 * 1024,       // 1MB
			LargeFileThreshold:      100 * 1024 * 1024, // 100MB
			SendfileThreshold:       10 * 1024 * 1024,  // 10MB
			MaxCacheFileSize:        10 * 1024 * 1024,  // 10MB
			ProgressUpdateInterval:  200 * time.Millisecond,
			WebSocketUpdateInterval: 100 * time.Millisecond,
			ProgressRefreshRate:     10,
			ReadTimeout:             10 * time.Minute,
			WriteTimeout:            15 * time.Minute,
			IdleTimeout:             5 * time.Minute,
		},
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

	// Validate configuration values
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration values are valid
func (c *Config) Validate() error {
	if c.BufferSize <= 0 {
		return fmt.Errorf("buffer_size must be positive, got %d", c.BufferSize)
	}
	if c.BufferSize > 100*1024*1024 { // 100MB max
		return fmt.Errorf("buffer_size too large (max 100MB), got %d", c.BufferSize)
	}
	if c.DefaultPort < 0 || c.DefaultPort > 65535 {
		return fmt.Errorf("default_port must be 0-65535, got %d", c.DefaultPort)
	}
	if c.MaxUploadSize <= 0 {
		return fmt.Errorf("max_upload_size must be positive, got %d", c.MaxUploadSize)
	}
	if c.RateLimitMbps < 0 {
		return fmt.Errorf("rate_limit_mbps cannot be negative, got %f", c.RateLimitMbps)
	}
	if c.CacheSizeMB < 0 {
		return fmt.Errorf("cache_size_mb cannot be negative, got %d", c.CacheSizeMB)
	}
	if c.ChunkSizeMB <= 0 {
		return fmt.Errorf("chunk_size_mb must be positive, got %d", c.ChunkSizeMB)
	}
	if c.ChunkSizeMB > 100 { // 100MB max chunk
		return fmt.Errorf("chunk_size_mb too large (max 100MB), got %d", c.ChunkSizeMB)
	}
	if c.ParallelWorkers <= 0 {
		return fmt.Errorf("parallel_workers must be positive, got %d", c.ParallelWorkers)
	}
	if c.ParallelWorkers > 32 { // Reasonable max
		return fmt.Errorf("parallel_workers too large (max 32), got %d", c.ParallelWorkers)
	}

	// Validate resume configuration
	if c.AutoCheckpointSizeMB < 0 {
		return fmt.Errorf("auto_checkpoint_size_mb cannot be negative, got %d", c.AutoCheckpointSizeMB)
	}
	if c.CheckpointIntervalMB <= 0 {
		return fmt.Errorf("checkpoint_interval_mb must be positive, got %d", c.CheckpointIntervalMB)
	}
	if c.CheckpointIntervalMB > 100 {
		return fmt.Errorf("checkpoint_interval_mb too large (max 100MB), got %d", c.CheckpointIntervalMB)
	}
	if c.CheckpointExpiryHours <= 0 {
		return fmt.Errorf("checkpoint_expiry_hours must be positive, got %d", c.CheckpointExpiryHours)
	}
	if c.CheckpointExpiryHours > 168 { // 1 week max
		return fmt.Errorf("checkpoint_expiry_hours too large (max 168 hours), got %d", c.CheckpointExpiryHours)
	}

	// Validate protocol configuration
	if c.Protocol.BufferSizeSmall <= 0 {
		return fmt.Errorf("protocol.buffer_size_small must be positive, got %d", c.Protocol.BufferSizeSmall)
	}
	if c.Protocol.BufferSizeMedium <= 0 {
		return fmt.Errorf("protocol.buffer_size_medium must be positive, got %d", c.Protocol.BufferSizeMedium)
	}
	if c.Protocol.BufferSizeLarge <= 0 {
		return fmt.Errorf("protocol.buffer_size_large must be positive, got %d", c.Protocol.BufferSizeLarge)
	}
	if c.Protocol.BufferSizeVeryLarge <= 0 {
		return fmt.Errorf("protocol.buffer_size_very_large must be positive, got %d", c.Protocol.BufferSizeVeryLarge)
	}
	if c.Protocol.SmallFileThreshold <= 0 {
		return fmt.Errorf("protocol.small_file_threshold must be positive, got %d", c.Protocol.SmallFileThreshold)
	}
	if c.Protocol.MediumFileThreshold <= 0 {
		return fmt.Errorf("protocol.medium_file_threshold must be positive, got %d", c.Protocol.MediumFileThreshold)
	}
	if c.Protocol.LargeFileThreshold <= 0 {
		return fmt.Errorf("protocol.large_file_threshold must be positive, got %d", c.Protocol.LargeFileThreshold)
	}
	if c.Protocol.SendfileThreshold < 0 {
		return fmt.Errorf("protocol.sendfile_threshold cannot be negative, got %d", c.Protocol.SendfileThreshold)
	}
	if c.Protocol.MaxCacheFileSize < 0 {
		return fmt.Errorf("protocol.max_cache_file_size cannot be negative, got %d", c.Protocol.MaxCacheFileSize)
	}
	if c.Protocol.ProgressUpdateInterval <= 0 {
		return fmt.Errorf("protocol.progress_update_interval must be positive, got %v", c.Protocol.ProgressUpdateInterval)
	}
	if c.Protocol.WebSocketUpdateInterval <= 0 {
		return fmt.Errorf("protocol.websocket_update_interval must be positive, got %v", c.Protocol.WebSocketUpdateInterval)
	}
	if c.Protocol.ProgressRefreshRate <= 0 {
		return fmt.Errorf("protocol.progress_refresh_rate must be positive, got %d", c.Protocol.ProgressRefreshRate)
	}
	if c.Protocol.ReadTimeout <= 0 {
		return fmt.Errorf("protocol.read_timeout must be positive, got %v", c.Protocol.ReadTimeout)
	}
	if c.Protocol.WriteTimeout <= 0 {
		return fmt.Errorf("protocol.write_timeout must be positive, got %v", c.Protocol.WriteTimeout)
	}
	if c.Protocol.IdleTimeout <= 0 {
		return fmt.Errorf("protocol.idle_timeout must be positive, got %v", c.Protocol.IdleTimeout)
	}

	return nil
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
	viper.Set("no_encrypt", config.NoEncrypt)
	viper.Set("no_checksum", config.NoChecksum)
	viper.Set("upload_dir", config.UploadDir)
	viper.Set("enable_resume", config.EnableResume)
	viper.Set("auto_checkpoint_size_mb", config.AutoCheckpointSizeMB)
	viper.Set("checkpoint_interval_mb", config.CheckpointIntervalMB)
	viper.Set("checkpoint_expiry_hours", config.CheckpointExpiryHours)

	// Protocol settings
	viper.Set("protocol.buffer_size_small", config.Protocol.BufferSizeSmall)
	viper.Set("protocol.buffer_size_medium", config.Protocol.BufferSizeMedium)
	viper.Set("protocol.buffer_size_large", config.Protocol.BufferSizeLarge)
	viper.Set("protocol.buffer_size_very_large", config.Protocol.BufferSizeVeryLarge)
	viper.Set("protocol.small_file_threshold", config.Protocol.SmallFileThreshold)
	viper.Set("protocol.medium_file_threshold", config.Protocol.MediumFileThreshold)
	viper.Set("protocol.large_file_threshold", config.Protocol.LargeFileThreshold)
	viper.Set("protocol.sendfile_threshold", config.Protocol.SendfileThreshold)
	viper.Set("protocol.max_cache_file_size", config.Protocol.MaxCacheFileSize)
	viper.Set("protocol.progress_update_interval", config.Protocol.ProgressUpdateInterval)
	viper.Set("protocol.websocket_update_interval", config.Protocol.WebSocketUpdateInterval)
	viper.Set("protocol.progress_refresh_rate", config.Protocol.ProgressRefreshRate)
	viper.Set("protocol.read_timeout", config.Protocol.ReadTimeout)
	viper.Set("protocol.write_timeout", config.Protocol.WriteTimeout)
	viper.Set("protocol.idle_timeout", config.Protocol.IdleTimeout)

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

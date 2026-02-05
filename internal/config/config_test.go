package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BufferSize != 1048576 {
		t.Errorf("Expected BufferSize 1048576, got %d", cfg.BufferSize)
	}

	if cfg.MaxUploadSize != 10737418240 {
		t.Errorf("Expected MaxUploadSize 10737418240, got %d", cfg.MaxUploadSize)
	}

	if cfg.CacheSizeMB != 100 {
		t.Errorf("Expected CacheSizeMB 100, got %d", cfg.CacheSizeMB)
	}

	if cfg.ParallelWorkers != 3 {
		t.Errorf("Expected ParallelWorkers 3, got %d", cfg.ParallelWorkers)
	}

	if cfg.ChunkSizeMB != 2 {
		t.Errorf("Expected ChunkSizeMB 2, got %d", cfg.ChunkSizeMB)
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	// Create a temp directory with no config file
	tmpDir := t.TempDir()

	// Override home directory for this test
	originalHome := os.Getenv("HOME")
	originalUserProfile := os.Getenv("USERPROFILE") // Windows
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("USERPROFILE", tmpDir) // Windows
	defer func() {
		_ = os.Setenv("HOME", originalHome)
		_ = os.Setenv("USERPROFILE", originalUserProfile)
	}()

	// Load config when no file exists - should return defaults
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Should get default values
	defaultCfg := DefaultConfig()
	if cfg.BufferSize != defaultCfg.BufferSize {
		t.Errorf("Expected default BufferSize %d, got %d", defaultCfg.BufferSize, cfg.BufferSize)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Override home directory for this test
	originalHome := os.Getenv("HOME")
	originalUserProfile := os.Getenv("USERPROFILE") // Windows
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("USERPROFILE", tmpDir) // Windows
	defer func() {
		_ = os.Setenv("HOME", originalHome)
		_ = os.Setenv("USERPROFILE", originalUserProfile)
	}()

	// Create a custom config
	cfg := &Config{
		DefaultInterface:      "eth0",
		DefaultPort:           8080,
		BufferSize:            2097152,
		MaxUploadSize:         5368709120,
		RateLimitMbps:         100,
		CacheSizeMB:           200,
		ChunkSizeMB:           4,
		ParallelWorkers:       5,
		NoQR:                  true,
		NoChecksum:            false,
		UploadDir:             tmpDir, // Use tmpDir instead of /tmp/uploads for cross-platform
		EnableResume:          true,
		AutoCheckpointSizeMB:  50,
		CheckpointIntervalMB:  10,
		CheckpointExpiryHours: 48,
		Protocol:              DefaultConfig().Protocol, // Use default protocol config
	}

	// Create config directory
	configDir := filepath.Join(tmpDir, ".config", "warp")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Save config
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Check if file was created
	configPath := filepath.Join(configDir, "warp.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// List directory contents for debugging
		entries, _ := os.ReadDir(configDir)
		t.Logf("Config directory contents:")
		for _, e := range entries {
			t.Logf("  - %s", e.Name())
		}
		t.Fatalf("Config file was not created at %s", configPath)
	}

	// Load config - need to reset viper state
	loadedCfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify loaded config matches saved config
	if loadedCfg.DefaultInterface != cfg.DefaultInterface {
		t.Errorf("DefaultInterface mismatch: expected %s, got %s", cfg.DefaultInterface, loadedCfg.DefaultInterface)
	}

	if loadedCfg.DefaultPort != cfg.DefaultPort {
		t.Errorf("DefaultPort mismatch: expected %d, got %d", cfg.DefaultPort, loadedCfg.DefaultPort)
	}

	if loadedCfg.RateLimitMbps != cfg.RateLimitMbps {
		t.Errorf("RateLimitMbps mismatch: expected %.1f, got %.1f", cfg.RateLimitMbps, loadedCfg.RateLimitMbps)
	}

	if loadedCfg.ParallelWorkers != cfg.ParallelWorkers {
		t.Errorf("ParallelWorkers mismatch: expected %d, got %d", cfg.ParallelWorkers, loadedCfg.ParallelWorkers)
	}
}

func TestGetConfigPath(t *testing.T) {
	path := GetConfigPath()
	if path == "" {
		t.Error("GetConfigPath returned empty string")
	}

	// Should contain either .config/warp or warp.yaml
	if !filepath.IsAbs(path) && path != "~/.config/warp/warp.yaml" {
		t.Errorf("GetConfigPath returned unexpected relative path: %s", path)
	}
}

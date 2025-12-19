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
	// Load config when no file exists - should return defaults
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.BufferSize != 1048576 {
		t.Errorf("Expected default BufferSize, got %d", cfg.BufferSize)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Override home directory for this test
	originalHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", originalHome) }()

	// Create a custom config
	cfg := &Config{
		DefaultInterface: "eth0",
		DefaultPort:      8080,
		BufferSize:       2097152,
		MaxUploadSize:    5368709120,
		RateLimitMbps:    100,
		CacheSizeMB:      200,
		ChunkSizeMB:      4,
		ParallelWorkers:  5,
		NoQR:             true,
		NoChecksum:       false,
		UploadDir:        "/tmp/uploads",
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
	if _, err := os.Stat(filepath.Join(configDir, "warp.yaml")); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load config
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

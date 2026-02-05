package core

import (
	"testing"
	"time"
)

// TestSendOptions_Defaults verifies default values for SendOptions.
func TestSendOptions_Defaults(t *testing.T) {
	opts := SendOptions{}
	if opts.Port != 0 {
		t.Errorf("Expected default Port 0, got %d", opts.Port)
	}
	if opts.InterfaceName != "" {
		t.Errorf("Expected default InterfaceName empty, got %s", opts.InterfaceName)
	}
}

// TestReceiveOptions_Defaults verifies default values for ReceiveOptions.
func TestReceiveOptions_Defaults(t *testing.T) {
	opts := ReceiveOptions{}
	if opts.Code != "" {
		t.Errorf("Expected default Code empty, got %s", opts.Code)
	}
	if opts.Workers != 0 {
		t.Errorf("Expected default Workers 0, got %d", opts.Workers)
	}
}

// TestHostOptions_Defaults verifies default values for HostOptions.
func TestHostOptions_Defaults(t *testing.T) {
	opts := HostOptions{}
	if opts.DestDir != "" {
		t.Errorf("Expected default DestDir empty, got %s", opts.DestDir)
	}
	if opts.NoQR != false {
		t.Errorf("Expected default NoQR false, got %v", opts.NoQR)
	}
}

// TestSearchOptions_Defaults verifies default values for SearchOptions.
func TestSearchOptions_Defaults(t *testing.T) {
	opts := SearchOptions{}
	if opts.Timeout != 0 {
		t.Errorf("Expected default Timeout 0, got %v", opts.Timeout)
	}
}

// TestConfigOptions_Defaults verifies default values for ConfigOptions.
func TestConfigOptions_Defaults(t *testing.T) {
	opts := ConfigOptions{}
	if opts.Subcommand != "" {
		t.Errorf("Expected default Subcommand empty, got %s", opts.Subcommand)
	}
}

// TestOptions_FieldAssignment verifies that fields can be correctly assigned.
func TestOptions_FieldAssignment(t *testing.T) {
	sendOpts := SendOptions{
		Port:          8080,
		InterfaceName: "eth0",
		RateLimitMbps: 5.5,
		CacheSizeMB:   256,
		NoQR:          true,
		NoEncrypt:     true,
		TextContent:   "hello",
		FilePath:      "/tmp/test",
		Resume:        true,
		SessionID:     "sess123",
		Verbose:       2,
	}

	if sendOpts.Port != 8080 || sendOpts.InterfaceName != "eth0" || sendOpts.RateLimitMbps != 5.5 {
		t.Error("SendOptions field assignment failed")
	}

	receiveOpts := ReceiveOptions{
		Code:        "1-apple-banana",
		OutputPath:  "./downloads",
		Force:       true,
		Workers:     4,
		ChunkSizeMB: 8,
		NoChecksum:  true,
		Decrypt:     true,
		Resume:      true,
		SessionID:   "sess456",
		Verbose:     1,
	}

	if receiveOpts.Code != "1-apple-banana" || receiveOpts.Workers != 4 || !receiveOpts.Force {
		t.Error("ReceiveOptions field assignment failed")
	}

	hostOpts := HostOptions{
		InterfaceName: "wlan0",
		DestDir:       "./uploads",
		RateLimitMbps: 10.0,
		NoQR:          true,
		NoEncrypt:     true,
		Resume:        true,
		Verbose:       1,
	}

	if hostOpts.InterfaceName != "wlan0" || hostOpts.DestDir != "./uploads" || hostOpts.RateLimitMbps != 10.0 {
		t.Error("HostOptions field assignment failed")
	}

	searchOpts := SearchOptions{
		Timeout: 5 * time.Second,
		Verbose: 1,
	}

	if searchOpts.Timeout != 5*time.Second || searchOpts.Verbose != 1 {
		t.Error("SearchOptions field assignment failed")
	}

	configOpts := ConfigOptions{
		Subcommand:  "init",
		Interactive: true,
	}

	if configOpts.Subcommand != "init" || !configOpts.Interactive {
		t.Error("ConfigOptions field assignment failed")
	}
}

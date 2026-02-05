package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/progress"
)

// TestHostExecutor_Start verifies that the HostExecutor correctly starts the server.
func TestHostExecutor_Start(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    true,
	}

	var statusMsgs []string
	onStatus := func(msg string) {
		statusMsgs = append(statusMsgs, msg)
	}

	executor := NewHostExecutor(opts, onStatus, nil)
	info, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	if info == nil {
		t.Fatal("Expected ServerInfo, got nil")
	}

	if info.URL == "" {
		t.Error("Expected URL to be non-empty")
	}

	if info.Code == "" {
		t.Error("Expected Code to be non-empty")
	}

	if executor.Server() == nil {
		t.Error("Expected Server() to return non-nil")
	}

	if executor.DestDir() != tmpDir {
		t.Errorf("Expected DestDir %s, got %s", tmpDir, executor.DestDir())
	}

	// Check if status message was emitted
	found := false
	for _, msg := range statusMsgs {
		if strings.Contains(msg, "Starting host server") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected status message 'Starting host server...' not found")
	}

	// Test double start
	_, err = executor.Start(context.Background())
	if err == nil {
		t.Error("Expected error on double start, got nil")
	}
}

// TestHostExecutor_Start_QR verifies that QR code generation is handled correctly.
func TestHostExecutor_Start_QR(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    false,
	}

	executor := NewHostExecutor(opts, nil, nil)
	info, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	if info.QRCode == "" {
		t.Error("Expected QRCode to be non-empty when NoQR is false")
	}
}

// TestHostExecutor_Start_Options verifies that HostOptions are correctly applied.
func TestHostExecutor_Start_Options(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir:       tmpDir,
		InterfaceName: "lo",
		RateLimitMbps: 10,
		NoQR:          true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	if executor.Server().InterfaceName != "lo" {
		t.Errorf("Expected InterfaceName lo, got %s", executor.Server().InterfaceName)
	}

	if executor.Server().RateLimitMbps != 10 {
		t.Errorf("Expected RateLimitMbps 10, got %f", executor.Server().RateLimitMbps)
	}
}

// TestHostExecutor_Stop verifies that the HostExecutor stops gracefully.
func TestHostExecutor_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    true,
	}

	executor := NewHostExecutor(opts, nil, nil)

	// Stop before start
	if err := executor.Stop(); err != nil {
		t.Errorf("Stop before start returned error: %v", err)
	}

	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}

	if err := executor.Stop(); err != nil {
		t.Errorf("Failed to stop executor: %v", err)
	}

	// Stop again
	if err := executor.Stop(); err != nil {
		t.Errorf("Second stop returned error: %v", err)
	}
}

// TestHostExecutor_Wait verifies that Wait blocks until context cancellation.
func TestHostExecutor_Wait(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- executor.Wait(ctx)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait timed out")
	}
}

// TestHostExecutor_Wait_Interrupt verifies that Wait responds to interrupt signals.
func TestHostExecutor_Wait_Interrupt(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- executor.Wait(context.Background())
	}()

	// Simulate interrupt signal
	time.Sleep(100 * time.Millisecond)
	process, _ := os.FindProcess(os.Getpid())
	_ = process.Signal(os.Interrupt)

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Wait timed out waiting for interrupt")
	}
}

// TestHostExecutor_Wait_Stop verifies that Wait returns when Stop is called.
func TestHostExecutor_Wait_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- executor.Wait(context.Background())
	}()

	// Stop from another goroutine
	time.Sleep(100 * time.Millisecond)
	_ = executor.Stop()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait timed out after Stop()")
	}
}

// TestHostExecutor_ForwardProgress verifies that progress updates are correctly forwarded.
func TestHostExecutor_ForwardProgress(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    true,
	}

	progressReceived := make(chan progress.Progress, 1)
	onProg := func(p progress.Progress) {
		progressReceived <- p
	}

	executor := NewHostExecutor(opts, nil, onProg)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	testProg := progress.Progress{
		FileName:         "test.txt",
		TransferredBytes: 100,
		TotalBytes:       1000,
		IsComplete:       false,
	}

	// Manually send progress to the server's progress channel
	executor.Server().ProgressChan <- testProg

	select {
	case p := <-progressReceived:
		if p.FileName != testProg.FileName {
			t.Errorf("Expected FileName %s, got %s", testProg.FileName, p.FileName)
		}
		if p.TransferredBytes != testProg.TransferredBytes {
			t.Errorf("Expected TransferredBytes %d, got %d", testProg.TransferredBytes, p.TransferredBytes)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for progress update")
	}
}

// TestHostExecutor_ForwardProgress_NilCallback verifies that nil progress callback is handled.
func TestHostExecutor_ForwardProgress_NilCallback(t *testing.T) {
	tmpDir := t.TempDir()
	opts := HostOptions{
		DestDir: tmpDir,
		NoQR:    true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	testProg := progress.Progress{
		FileName: "test.txt",
	}

	// This should not crash even though onProg is nil
	executor.Server().ProgressChan <- testProg
	time.Sleep(100 * time.Millisecond)
}

// TestHostExecutor_DestDir verifies DestDir resolution.
func TestHostExecutor_DestDir(t *testing.T) {
	// Test default DestDir
	executor := NewHostExecutor(HostOptions{}, nil, nil)
	if executor.DestDir() != "." {
		t.Errorf("Expected default DestDir '.', got %s", executor.DestDir())
	}

	// Test custom DestDir
	tmpDir := "/tmp/warp-test"
	executorOpts := HostOptions{DestDir: tmpDir}
	executor2 := NewHostExecutor(executorOpts, nil, nil)
	if executor2.DestDir() != tmpDir {
		t.Errorf("Expected DestDir %s, got %s", tmpDir, executor2.DestDir())
	}
}

// TestHostExecutor_DestDir_Config verifies DestDir resolution from config.
func TestHostExecutor_DestDir_Config(t *testing.T) {
	// Save original config
	origCfg := config.GetConfig()
	defer config.SetConfig(origCfg)

	// Set custom config
	customCfg := config.DefaultConfig()
	customCfg.UploadDir = "/custom/upload/dir"
	config.SetConfig(customCfg)

	executor := NewHostExecutor(HostOptions{}, nil, nil)
	if executor.DestDir() != "/custom/upload/dir" {
		t.Errorf("Expected DestDir /custom/upload/dir, got %s", executor.DestDir())
	}
}

// TestHostExecutor_DestDir_EmptyConfig verifies DestDir resolution with empty config.
func TestHostExecutor_DestDir_EmptyConfig(t *testing.T) {
	// Save original config
	origCfg := config.GetConfig()
	defer config.SetConfig(origCfg)

	// Set custom config with empty UploadDir
	customCfg := config.DefaultConfig()
	customCfg.UploadDir = ""
	config.SetConfig(customCfg)

	executor := NewHostExecutor(HostOptions{}, nil, nil)
	if executor.DestDir() != "." {
		t.Errorf("Expected default DestDir '.', got %s", executor.DestDir())
	}
}

// TestHostExecutor_Start_ServerError verifies error handling during server start.
func TestHostExecutor_Start_ServerError(t *testing.T) {
	// Trying to bind to a privileged port or invalid interface should cause an error
	opts := HostOptions{
		InterfaceName: "nonexistent0",
		NoQR:          true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err == nil {
		t.Error("Expected error when starting server on nonexistent interface, got nil")
		executor.Stop()
	}
}

// TestHostExecutor_Start_MkdirError verifies error handling when directory creation fails.
func TestHostExecutor_Start_MkdirError(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file")
	_ = os.WriteFile(filePath, []byte("test"), 0644)

	// Trying to create a directory where a file exists should fail
	opts := HostOptions{
		DestDir: filepath.Join(filePath, "subdir"),
		NoQR:    true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err == nil {
		t.Error("Expected error when MkdirAll fails, got nil")
		executor.Stop()
	}
}

// TestHostExecutor_Start_CreateDestDir verifies that DestDir is created if it doesn't exist.
func TestHostExecutor_Start_CreateDestDir(t *testing.T) {
	parentDir := t.TempDir()
	destDir := filepath.Join(parentDir, "new-dir")

	opts := HostOptions{
		DestDir: destDir,
		NoQR:    true,
	}

	executor := NewHostExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		t.Errorf("Expected directory %s to be created", destDir)
	}
}

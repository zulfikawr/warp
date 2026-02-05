package core

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/progress"
)

// TestSendExecutor_Start verifies that the SendExecutor correctly starts the server with a file.
func TestSendExecutor_Start_File(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "warp-send-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	_ = tmpFile.Close()

	opts := SendOptions{
		FilePath: tmpFile.Name(),
		NoQR:     true,
	}

	executor := NewSendExecutor(opts, nil, nil)
	info, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	if info == nil || info.URL == "" {
		t.Fatal("Expected ServerInfo with URL")
	}

	if executor.Server().SrcPath != tmpFile.Name() {
		t.Errorf("Expected SrcPath %s, got %s", tmpFile.Name(), executor.Server().SrcPath)
	}
}

// TestSendExecutor_Start_Text verifies that the SendExecutor correctly starts the server with text content.
func TestSendExecutor_Start_Text(t *testing.T) {
	opts := SendOptions{
		TextContent: "hello world",
		NoQR:        true,
	}

	executor := NewSendExecutor(opts, nil, nil)
	info, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	if info == nil || info.URL == "" {
		t.Fatal("Expected ServerInfo with URL")
	}

	if executor.Server().TextContent != "hello world" {
		t.Errorf("Expected TextContent 'hello world', got %s", executor.Server().TextContent)
	}
}

// TestSendExecutor_Start_Options verifies that SendOptions are correctly applied.
func TestSendExecutor_Start_Options(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "warp-send-opts")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	_ = tmpFile.Close()

	opts := SendOptions{
		FilePath:      tmpFile.Name(),
		InterfaceName: "lo",
		RateLimitMbps: 5.0,
		CacheSizeMB:   10,
		NoQR:          true,
	}

	executor := NewSendExecutor(opts, nil, nil)
	_, err = executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	if executor.Server().InterfaceName != "lo" {
		t.Errorf("Expected InterfaceName lo, got %s", executor.Server().InterfaceName)
	}
	if executor.Server().RateLimitMbps != 5.0 {
		t.Errorf("Expected RateLimitMbps 5.0, got %f", executor.Server().RateLimitMbps)
	}
}

// TestSendExecutor_Start_ValidationError verifies input validation in Start.
func TestSendExecutor_Start_ValidationError(t *testing.T) {
	// No file and no text
	executor := NewSendExecutor(SendOptions{}, nil, nil)
	_, err := executor.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "requires a file path or text content") {
		t.Errorf("Expected validation error for missing input, got %v", err)
	}

	// File not found
	executor2 := NewSendExecutor(SendOptions{FilePath: "/nonexistent/file/path"}, nil, nil)
	_, err = executor2.Start(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "file not found") {
		t.Errorf("Expected validation error for nonexistent file, got %v", err)
	}
}

// TestSendExecutor_Stop verifies that the SendExecutor stops gracefully.
func TestSendExecutor_Stop(t *testing.T) {
	opts := SendOptions{TextContent: "test", NoQR: true}
	executor := NewSendExecutor(opts, nil, nil)

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
}

// TestSendExecutor_Wait verifies that Wait blocks until context cancellation.
func TestSendExecutor_Wait(t *testing.T) {
	opts := SendOptions{TextContent: "test", NoQR: true}
	executor := NewSendExecutor(opts, nil, nil)
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

// TestSendExecutor_Wait_Interrupt verifies that Wait responds to interrupt signals.
func TestSendExecutor_Wait_Interrupt(t *testing.T) {
	opts := SendOptions{TextContent: "test", NoQR: true}
	executor := NewSendExecutor(opts, nil, nil)
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

// TestSendExecutor_Wait_Stop verifies that Wait returns when Stop is called.
func TestSendExecutor_Wait_Stop(t *testing.T) {
	opts := SendOptions{TextContent: "test", NoQR: true}
	executor := NewSendExecutor(opts, nil, nil)
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

// TestSendExecutor_ForwardProgress verifies that progress updates are correctly forwarded.
func TestSendExecutor_ForwardProgress(t *testing.T) {
	progressReceived := make(chan progress.Progress, 1)
	onProg := func(p progress.Progress) {
		progressReceived <- p
	}

	opts := SendOptions{TextContent: "test", NoQR: true}
	executor := NewSendExecutor(opts, nil, onProg)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	testProg := progress.Progress{
		FileName:         "test.txt",
		TransferredBytes: 500,
		TotalBytes:       1000,
		IsComplete:       false,
	}

	executor.Server().ProgressChan <- testProg

	select {
	case p := <-progressReceived:
		if p.FileName != testProg.FileName {
			t.Errorf("Expected FileName %s, got %s", testProg.FileName, p.FileName)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for progress update")
	}
}

// TestSendExecutor_ForwardProgress_NilCallback verifies that nil progress callback is handled.
func TestSendExecutor_ForwardProgress_NilCallback(t *testing.T) {
	opts := SendOptions{TextContent: "test", NoQR: true}
	executor := NewSendExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start executor: %v", err)
	}
	defer executor.Stop()

	testProg := progress.Progress{
		FileName: "test.txt",
	}

	// This should not crash
	executor.Server().ProgressChan <- testProg
	time.Sleep(100 * time.Millisecond)
}

// TestSendExecutor_EmitStatus verifies that status messages are correctly emitted.
func TestSendExecutor_EmitStatus(t *testing.T) {
	var capturedMsg string
	onStatus := func(msg string) {
		capturedMsg = msg
	}

	executor := NewSendExecutor(SendOptions{}, onStatus, nil)
	executor.emitStatus("test message")

	if capturedMsg != "test message" {
		t.Errorf("Expected 'test message', got '%s'", capturedMsg)
	}

	// Test nil callback doesn't crash
	executorNil := NewSendExecutor(SendOptions{}, nil, nil)
	executorNil.emitStatus("should not crash")
}

// TestSendExecutor_Start_ServerError verifies error handling during server start.
func TestSendExecutor_Start_ServerError(t *testing.T) {
	opts := SendOptions{
		TextContent:   "test",
		InterfaceName: "nonexistent0",
		NoQR:          true,
	}

	executor := NewSendExecutor(opts, nil, nil)
	_, err := executor.Start(context.Background())
	if err == nil {
		t.Error("Expected error when starting server on nonexistent interface, got nil")
		executor.Stop()
	}
}

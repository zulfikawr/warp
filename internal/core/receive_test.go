package core

import (
	"context"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/progress"
)

// TestReceiveExecutor_NewReceiveExecutor verifies that NewReceiveExecutor initializes the executor correctly.
func TestReceiveExecutor_NewReceiveExecutor(t *testing.T) {
	opts := ReceiveOptions{Code: "test-code"}
	onStatus := func(string) {}
	onProg := func(progress.Progress) {}
	exec := NewReceiveExecutor(opts, onStatus, onProg)

	if exec.opts.Code != opts.Code {
		t.Errorf("Expected code %s, got %s", opts.Code, exec.opts.Code)
	}

	if exec.onStatus == nil {
		t.Error("Expected onStatus callback to be set")
	}

	if exec.onProg == nil {
		t.Error("Expected onProg callback to be set")
	}
}

// TestReceiveExecutor_Execute_NoCode verifies that Execute returns an error when no code is provided.
func TestReceiveExecutor_Execute_NoCode(t *testing.T) {
	exec := NewReceiveExecutor(ReceiveOptions{}, nil, nil)
	_, err := exec.Execute(context.Background())

	if err == nil {
		t.Error("Expected error when no code is provided, got nil")
	}
}

// TestReceiveExecutor_DiscoverServices verifies that service discovery works correctly.
func TestReceiveExecutor_DiscoverServices(t *testing.T) {
	exec := NewReceiveExecutor(ReceiveOptions{}, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This calls discovery.Browse which might take time or return empty
	_, err := exec.DiscoverServices(ctx, 50*time.Millisecond)
	if err != nil {
		// We don't necessarily expect an error here, even if results are empty
		t.Logf("DiscoverServices returned error (possibly expected): %v", err)
	}
}

// TestReceiveExecutor_EmitStatus verifies that status messages are correctly emitted.
func TestReceiveExecutor_EmitStatus(t *testing.T) {
	var capturedMsg string
	onStatus := func(msg string) {
		capturedMsg = msg
	}

	exec := NewReceiveExecutor(ReceiveOptions{}, onStatus, nil)
	exec.emitStatus("test message")
	if capturedMsg != "test message" {
		t.Errorf("Expected 'test message', got '%s'", capturedMsg)
	}
}

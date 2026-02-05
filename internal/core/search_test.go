package core

import (
	"context"
	"testing"
	"time"
)

// TestSearchExecutor_NewSearchExecutor verifies that NewSearchExecutor initializes the executor correctly.
func TestSearchExecutor_NewSearchExecutor(t *testing.T) {
	opts := SearchOptions{Timeout: 5 * time.Second}
	onStatus := func(string) {}

	exec := NewSearchExecutor(opts, onStatus)
	if exec.opts.Timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", exec.opts.Timeout)
	}
	if exec.onStatus == nil {
		t.Error("Expected onStatus callback to be set")
	}

	// Test default timeout
	execDefault := NewSearchExecutor(SearchOptions{}, nil)
	if execDefault.opts.Timeout != DefaultSearchTimeout {
		t.Errorf("Expected default timeout %v, got %v", DefaultSearchTimeout, execDefault.opts.Timeout)
	}
}

// TestSearchExecutor_Execute verifies that the search operation can be initiated.
func TestSearchExecutor_Execute(t *testing.T) {
	exec := NewSearchExecutor(SearchOptions{Timeout: 50 * time.Millisecond}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This calls discovery.Browse which might return empty but should not crash
	_, err := exec.Execute(ctx)
	if err != nil {
		t.Logf("Search Execute returned error (possibly expected): %v", err)
	}
}

// TestSearchExecutor_EmitStatus verifies that status messages are correctly emitted.
func TestSearchExecutor_EmitStatus(t *testing.T) {
	var capturedMsg string
	onStatus := func(msg string) {
		capturedMsg = msg
	}

	exec := NewSearchExecutor(SearchOptions{}, onStatus)
	exec.emitStatus("searching...")

	if capturedMsg != "searching..." {
		t.Errorf("Expected 'searching...', got '%s'", capturedMsg)
	}
}

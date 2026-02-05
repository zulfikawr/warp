package resume

import (
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/progress"
)

func TestNewProgressTracker(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)
	if p == nil {
		t.Fatal("NewProgressTracker returned nil")
	}
	if p.sessionID != "session-123" {
		t.Errorf("sessionID = %v, want session-123", p.sessionID)
	}
	if p.totalBytes != 1000 {
		t.Errorf("totalBytes = %v, want 1000", p.totalBytes)
	}
	if p.totalChunks != 10 {
		t.Errorf("totalChunks = %v, want 10", p.totalChunks)
	}
}

func TestProgressTracker_UpdateBytes(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Update bytes
	p.UpdateBytes(500)

	info := p.GetProgress()
	if info.TransferredBytes != 500 {
		t.Errorf("TransferredBytes = %v, want 500", info.TransferredBytes)
	}
	if info.Percent() != 50.0 {
		t.Errorf("Percent = %v, want 50.0", info.Percent())
	}

	// Update with smaller value (should not decrease - monotonic)
	p.UpdateBytes(400)
	info = p.GetProgress()
	if info.TransferredBytes != 500 {
		t.Errorf("TransferredBytes = %v, want 500 (monotonic)", info.TransferredBytes)
	}

	// Update with larger value
	p.UpdateBytes(800)
	info = p.GetProgress()
	if info.TransferredBytes != 800 {
		t.Errorf("TransferredBytes = %v, want 800", info.TransferredBytes)
	}
}

func TestProgressTracker_CompleteChunk(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Complete chunk
	p.CompleteChunk(0, 100)

	info := p.GetProgress()
	if info.CompletedChunks != 1 {
		t.Errorf("CompletedChunks = %v, want 1", info.CompletedChunks)
	}
	if info.TransferredBytes != 100 {
		t.Errorf("TransferredBytes = %v, want 100", info.TransferredBytes)
	}

	// Complete another chunk
	p.CompleteChunk(1, 100)
	info = p.GetProgress()
	if info.CompletedChunks != 2 {
		t.Errorf("CompletedChunks = %v, want 2", info.CompletedChunks)
	}
	if info.TransferredBytes != 200 {
		t.Errorf("TransferredBytes = %v, want 200", info.TransferredBytes)
	}
}

func TestProgressTracker_GetProgress(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Initial progress
	info := p.GetProgress()
	if info.TransferID != "session-123" {
		t.Errorf("SessionID = %v, want session-123", info.TransferID)
	}
	if info.TotalBytes != 1000 {
		t.Errorf("TotalBytes = %v, want 1000", info.TotalBytes)
	}
	if info.TransferredBytes != 0 {
		t.Errorf("TransferredBytes = %v, want 0", info.TransferredBytes)
	}
	if info.Percent() != 0.0 {
		t.Errorf("Percent = %v, want 0.0", info.Percent())
	}

	// After some progress
	p.UpdateBytes(500)
	info = p.GetProgress()
	if info.Percent() != 50.0 {
		t.Errorf("Percent = %v, want 50.0", info.Percent())
	}

	// Complete
	p.UpdateBytes(1000)
	info = p.GetProgress()
	if info.Percent() != 100.0 {
		t.Errorf("Percent = %v, want 100.0", info.Percent())
	}
}

func TestProgressTracker_GetSpeed(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Initial speed should be 0
	if speed := p.GetSpeed(); speed != 0.0 {
		t.Errorf("initial speed = %v, want 0.0", speed)
	}

	// Simulate some progress
	time.Sleep(10 * time.Millisecond)
	p.UpdateBytes(100)

	// Speed should be > 0
	if speed := p.GetSpeed(); speed <= 0 {
		t.Errorf("speed = %v, want > 0", speed)
	}
}

func TestProgressTracker_GetETA(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Initial ETA should be 0 (no speed data)
	if eta := p.GetETA(); eta != 0 {
		t.Errorf("initial ETA = %v, want 0", eta)
	}

	// Simulate progress
	time.Sleep(10 * time.Millisecond)
	p.UpdateBytes(500)

	// ETA should be > 0
	if eta := p.GetETA(); eta <= 0 {
		t.Errorf("ETA = %v, want > 0", eta)
	}

	// Complete transfer
	p.UpdateBytes(1000)
	if eta := p.GetETA(); eta != 0 {
		t.Errorf("ETA after completion = %v, want 0", eta)
	}
}

func TestProgressTracker_RestoreProgress(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Restore progress
	completedChunks := []int{0, 1, 2}
	p.RestoreProgress(completedChunks, 300)

	info := p.GetProgress()
	if info.CompletedChunks != 3 {
		t.Errorf("CompletedChunks = %v, want 3", info.CompletedChunks)
	}
	if info.TransferredBytes != 300 {
		t.Errorf("TransferredBytes = %v, want 300", info.TransferredBytes)
	}
}

func TestProgressTracker_OnProgress(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Register callback
	callbackCalled := false
	var receivedInfo progress.Progress

	p.OnProgress(func(info progress.Progress) {
		callbackCalled = true
		receivedInfo = info
	})

	// Trigger progress update
	p.UpdateBytes(500)

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	if !callbackCalled {
		t.Error("callback was not called")
	}
	if receivedInfo.TransferredBytes != 500 {
		t.Errorf("callback received TransferredBytes = %v, want 500", receivedInfo.TransferredBytes)
	}
}

func TestProgressTracker_Reset(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Make some progress
	p.UpdateBytes(500)
	p.CompleteChunk(0, 100)

	// Reset
	p.Reset()

	info := p.GetProgress()
	if info.TransferredBytes != 0 {
		t.Errorf("TransferredBytes after reset = %v, want 0", info.TransferredBytes)
	}
	if info.CompletedChunks != 0 {
		t.Errorf("CompletedChunks after reset = %v, want 0", info.CompletedChunks)
	}
}

func TestProgressTracker_MonotonicProgress(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Progress should never decrease
	p.UpdateBytes(500)
	info1 := p.GetProgress()

	p.UpdateBytes(300) // Try to decrease
	info2 := p.GetProgress()

	if info2.TransferredBytes < info1.TransferredBytes {
		t.Errorf("progress decreased: %v -> %v", info1.TransferredBytes, info2.TransferredBytes)
	}

	p.UpdateBytes(700) // Increase
	info3 := p.GetProgress()

	if info3.TransferredBytes < info2.TransferredBytes {
		t.Errorf("progress decreased: %v -> %v", info2.TransferredBytes, info3.TransferredBytes)
	}
}

func TestProgressTracker_MultipleCallbacks(t *testing.T) {
	p := NewProgressTracker("session-123", 1000, 10)

	// Register multiple callbacks
	callback1Called := false
	callback2Called := false

	p.OnProgress(func(info progress.Progress) {
		callback1Called = true
	})

	p.OnProgress(func(info progress.Progress) {
		callback2Called = true
	})

	// Trigger progress update
	p.UpdateBytes(500)

	// Give callbacks time to execute
	time.Sleep(10 * time.Millisecond)

	if !callback1Called {
		t.Error("callback 1 was not called")
	}
	if !callback2Called {
		t.Error("callback 2 was not called")
	}
}

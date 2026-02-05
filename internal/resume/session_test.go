package resume

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewTransferSession tests session initialization
func TestNewTransferSession(t *testing.T) {
	checkpoint := &Checkpoint{
		SessionID:   "test-session",
		TotalSize:   1024 * 1024,
		ChunkSize:   64 * 1024,
		TotalChunks: 16,
	}

	stateManager, cleanup := setupTestStateManager(t)
	defer cleanup()

	session := NewTransferSession(checkpoint, stateManager)

	if session.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, session.Status)
	}

	if session.Checkpoint != checkpoint {
		t.Error("Checkpoint not set correctly")
	}

	if session.Verifier == nil {
		t.Error("IntegrityVerifier not initialized")
	}

	if session.Progress == nil {
		t.Error("ProgressTracker not initialized")
	}
}

// TestNewTransferSession_WithExistingProgress tests session restoration
func TestNewTransferSession_WithExistingProgress(t *testing.T) {
	checkpoint := &Checkpoint{
		SessionID:       "test-session",
		TotalSize:       1024 * 1024,
		ChunkSize:       64 * 1024,
		TotalChunks:     16,
		CompletedChunks: []int{0, 1, 2, 3},
		ChunkChecksums: map[int]string{
			0: "hash0",
			1: "hash1",
			2: "hash2",
			3: "hash3",
		},
	}

	stateManager, cleanup := setupTestStateManager(t)
	defer cleanup()

	session := NewTransferSession(checkpoint, stateManager)

	// Check progress was restored
	progress := session.Progress.GetProgress()
	if progress.CompletedChunks != 4 {
		t.Errorf("Expected 4 completed chunks, got %d", progress.CompletedChunks)
	}

	expectedBytes := int64(4 * 64 * 1024)
	if progress.TransferredBytes != expectedBytes {
		t.Errorf("Expected %d bytes transferred, got %d", expectedBytes, progress.TransferredBytes)
	}
}

// TestTransferSession_GetStatus tests status retrieval
func TestTransferSession_GetStatus(t *testing.T) {
	session := createTestSession(t)

	if session.GetStatus() != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, session.GetStatus())
	}

	session.mu.Lock()
	session.Status = StatusActive
	session.mu.Unlock()

	if session.GetStatus() != StatusActive {
		t.Errorf("Expected status %s, got %s", StatusActive, session.GetStatus())
	}
}

// TestTransferSession_GetError tests error retrieval
func TestTransferSession_GetError(t *testing.T) {
	session := createTestSession(t)

	if session.GetError() != nil {
		t.Error("Expected no error initially")
	}

	testErr := errors.New("test error")
	session.mu.Lock()
	session.Error = testErr
	session.mu.Unlock()

	if session.GetError() != testErr {
		t.Error("Error not retrieved correctly")
	}
}

// TestTransferSession_ShouldAutoCheckpoint tests auto-checkpoint threshold
func TestTransferSession_ShouldAutoCheckpoint(t *testing.T) {
	tests := []struct {
		name      string
		totalSize int64
		expected  bool
	}{
		{
			name:      "Small file (10MB)",
			totalSize: 10 * 1024 * 1024,
			expected:  false,
		},
		{
			name:      "Exactly at threshold (100MB)",
			totalSize: AutoCheckpointThreshold,
			expected:  false,
		},
		{
			name:      "Just above threshold (100MB + 1)",
			totalSize: AutoCheckpointThreshold + 1,
			expected:  true,
		},
		{
			name:      "Large file (1GB)",
			totalSize: 1024 * 1024 * 1024,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkpoint := &Checkpoint{
				SessionID:   "test-session",
				TotalSize:   tt.totalSize,
				ChunkSize:   2 * 1024 * 1024,
				TotalChunks: int(tt.totalSize / (2 * 1024 * 1024)),
			}

			stateManager, cleanup := setupTestStateManager(t)
			defer cleanup()

			session := NewTransferSession(checkpoint, stateManager)

			if session.shouldAutoCheckpoint() != tt.expected {
				t.Errorf("Expected shouldAutoCheckpoint=%v, got %v", tt.expected, session.shouldAutoCheckpoint())
			}
		})
	}
}

// TestTransferSession_AdjustChunkSize tests chunk size adjustment
func TestTransferSession_AdjustChunkSize(t *testing.T) {
	tests := []struct {
		name              string
		totalSize         int64
		initialChunkSize  int64
		initialChunkCount int
		expectAdjustment  bool
	}{
		{
			name:              "Normal chunk count",
			totalSize:         1024 * 1024 * 1024,
			initialChunkSize:  2 * 1024 * 1024,
			initialChunkCount: 512,
			expectAdjustment:  false,
		},
		{
			name:              "Exactly at threshold",
			totalSize:         200 * 1024 * 1024 * 1024,
			initialChunkSize:  2 * 1024 * 1024,
			initialChunkCount: MaxChunksThreshold,
			expectAdjustment:  false,
		},
		{
			name:              "Above threshold",
			totalSize:         300 * 1024 * 1024 * 1024,
			initialChunkSize:  2 * 1024 * 1024,
			initialChunkCount: 150000,
			expectAdjustment:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkpoint := &Checkpoint{
				SessionID:   "test-session",
				TotalSize:   tt.totalSize,
				ChunkSize:   tt.initialChunkSize,
				TotalChunks: tt.initialChunkCount,
			}

			stateManager, cleanup := setupTestStateManager(t)
			defer cleanup()

			session := NewTransferSession(checkpoint, stateManager)
			session.adjustChunkSize()

			if tt.expectAdjustment {
				if session.Checkpoint.ChunkSize == tt.initialChunkSize {
					t.Error("Expected chunk size to be adjusted, but it wasn't")
				}
				if session.Checkpoint.TotalChunks > MaxChunksThreshold {
					t.Errorf("Chunk count %d still exceeds threshold %d", session.Checkpoint.TotalChunks, MaxChunksThreshold)
				}
			} else {
				if session.Checkpoint.ChunkSize != tt.initialChunkSize {
					t.Error("Chunk size was adjusted when it shouldn't have been")
				}
			}
		})
	}
}

// TestTransferSession_HandleError tests error recovery actions
func TestTransferSession_HandleError(t *testing.T) {
	session := createTestSession(t)

	tests := []struct {
		name           string
		err            error
		expectedAction RecoveryAction
	}{
		{
			name:           "Recoverable resumable error",
			err:            &ResumableError{Err: errors.New("network timeout"), Recoverable: true},
			expectedAction: ActionRetry,
		},
		{
			name:           "Non-recoverable resumable error",
			err:            &ResumableError{Err: errors.New("auth failed"), Recoverable: false},
			expectedAction: ActionPause,
		},
		{
			name:           "Integrity error",
			err:            &IntegrityError{Err: errors.New("checksum mismatch")},
			expectedAction: ActionRetry,
		},
		{
			name:           "Nonce exhausted",
			err:            &EncryptionResumeError{Err: errors.New("nonce exhausted"), Reason: "nonce_exhausted"},
			expectedAction: ActionRestart,
		},
		{
			name:           "Other encryption error",
			err:            &EncryptionResumeError{Err: errors.New("key mismatch"), Reason: "key_mismatch"},
			expectedAction: ActionAbort,
		},
		{
			name:           "Unknown error",
			err:            errors.New("unknown error"),
			expectedAction: ActionPause,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := session.HandleError(tt.err)
			if action != tt.expectedAction {
				t.Errorf("Expected action %v, got %v", tt.expectedAction, action)
			}
		})
	}
}

// TestTransferSession_IsRetryable tests retryable error detection
func TestTransferSession_IsRetryable(t *testing.T) {
	session := createTestSession(t)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Integrity error",
			err:      &IntegrityError{Err: errors.New("checksum mismatch")},
			expected: true,
		},
		{
			name:     "Resumable error",
			err:      &ResumableError{Err: errors.New("network timeout")},
			expected: true,
		},
		{
			name:     "Context canceled",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "Context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "Unknown error",
			err:      errors.New("unknown error"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := session.isRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("Expected isRetryable=%v, got %v", tt.expected, result)
			}
		})
	}
}

// TestTransferSession_RetryWithBackoff tests retry logic
func TestTransferSession_RetryWithBackoff(t *testing.T) {
	session := createTestSession(t)

	t.Run("Success on first attempt", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return nil
		}

		ctx := context.Background()
		err := session.RetryWithBackoff(ctx, operation)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("Success on second attempt", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts == 1 {
				return &ResumableError{Err: errors.New("temporary failure"), Recoverable: true}
			}
			return nil
		}

		ctx := context.Background()
		err := session.RetryWithBackoff(ctx, operation)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("Max retries exceeded", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return &ResumableError{Err: errors.New("persistent failure"), Recoverable: true}
		}

		ctx := context.Background()
		err := session.RetryWithBackoff(ctx, operation)

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !errors.Is(err, ErrMaxRetriesExceeded) {
			t.Errorf("Expected ErrMaxRetriesExceeded, got %v", err)
		}
		if attempts != MaxRetries+1 {
			t.Errorf("Expected %d attempts, got %d", MaxRetries+1, attempts)
		}
	})

	t.Run("Non-retryable error", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return context.Canceled
		}

		ctx := context.Background()
		err := session.RetryWithBackoff(ctx, operation)

		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("Context canceled during retry", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return &ResumableError{Err: errors.New("temporary failure"), Recoverable: true}
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := session.RetryWithBackoff(ctx, operation)

		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	})
}

// TestTransferSession_CompleteChunk tests chunk completion
func TestTransferSession_CompleteChunk(t *testing.T) {
	checkpoint := &Checkpoint{
		SessionID:      "test-session",
		TotalSize:      1024 * 1024,
		ChunkSize:      64 * 1024,
		TotalChunks:    16,
		ChunkChecksums: make(map[int]string),
	}

	stateManager, cleanup := setupTestStateManager(t)
	defer cleanup()

	session := NewTransferSession(checkpoint, stateManager)

	// Complete a chunk
	chunkData := make([]byte, 64*1024)
	err := session.CompleteChunk(0, chunkData, "hash0")

	if err != nil {
		t.Errorf("CompleteChunk failed: %v", err)
	}

	// Verify checkpoint was updated
	if len(session.Checkpoint.CompletedChunks) != 1 {
		t.Errorf("Expected 1 completed chunk, got %d", len(session.Checkpoint.CompletedChunks))
	}

	if session.Checkpoint.ChunkChecksums[0] != "hash0" {
		t.Error("Chunk checksum not recorded")
	}

	// Verify progress was updated
	progress := session.Progress.GetProgress()
	if progress.CompletedChunks != 1 {
		t.Errorf("Expected 1 completed chunk in progress, got %d", progress.CompletedChunks)
	}
}

// TestTransferSession_IsComplete tests completion detection
func TestTransferSession_IsComplete(t *testing.T) {
	checkpoint := &Checkpoint{
		SessionID:      "test-session",
		TotalSize:      1024 * 1024,
		ChunkSize:      64 * 1024,
		TotalChunks:    16,
		ChunkChecksums: make(map[int]string),
	}

	stateManager, cleanup := setupTestStateManager(t)
	defer cleanup()

	session := NewTransferSession(checkpoint, stateManager)

	if session.IsComplete() {
		t.Error("Session should not be complete initially")
	}

	// Mark all chunks as complete
	for i := 0; i < 16; i++ {
		session.Checkpoint.MarkChunkComplete(i, "hash")
	}

	if !session.IsComplete() {
		t.Error("Session should be complete after all chunks marked")
	}
}

// TestTransferSession_Pause tests pause functionality
func TestTransferSession_Pause(t *testing.T) {
	session := createTestSession(t)

	t.Run("Cannot pause pending transfer", func(t *testing.T) {
		err := session.Pause()
		if err == nil {
			t.Error("Expected error when pausing pending transfer")
		}
	})

	t.Run("Can pause active transfer", func(t *testing.T) {
		session.mu.Lock()
		session.Status = StatusActive
		session.mu.Unlock()

		err := session.Pause()
		if err != nil {
			t.Errorf("Pause failed: %v", err)
		}

		if !session.PauseRequested {
			t.Error("PauseRequested flag not set")
		}

		// Verify pause signal was sent
		select {
		case <-session.pauseCh:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Pause signal not sent")
		}
	})
}

// TestTransferSession_Resume tests resume functionality
func TestTransferSession_Resume(t *testing.T) {
	session := createTestSession(t)

	t.Run("Cannot resume non-paused transfer", func(t *testing.T) {
		err := session.Resume()
		if err == nil {
			t.Error("Expected error when resuming non-paused transfer")
		}
	})

	t.Run("Can resume paused transfer", func(t *testing.T) {
		session.mu.Lock()
		session.Status = StatusPaused
		session.mu.Unlock()

		err := session.Resume()
		if err != nil {
			t.Errorf("Resume failed: %v", err)
		}

		if session.GetStatus() != StatusActive {
			t.Errorf("Expected status %s, got %s", StatusActive, session.GetStatus())
		}

		if session.PauseRequested {
			t.Error("PauseRequested flag should be cleared")
		}

		// Verify resume signal was sent
		select {
		case <-session.resumeCh:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Resume signal not sent")
		}
	})
}

// TestTransferSession_Cancel tests cancel functionality
func TestTransferSession_Cancel(t *testing.T) {
	session := createTestSession(t)

	t.Run("Cannot cancel completed transfer", func(t *testing.T) {
		session.mu.Lock()
		session.Status = StatusCompleted
		session.mu.Unlock()

		err := session.Cancel()
		if err == nil {
			t.Error("Expected error when canceling completed transfer")
		}
	})

	t.Run("Can cancel active transfer", func(t *testing.T) {
		session.mu.Lock()
		session.Status = StatusActive
		session.mu.Unlock()

		err := session.Cancel()
		if err != nil {
			t.Errorf("Cancel failed: %v", err)
		}

		if !session.CancelRequested {
			t.Error("CancelRequested flag not set")
		}

		// Verify cancel signal was sent
		select {
		case <-session.cancelCh:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Cancel signal not sent")
		}
	})
}

// Helper functions

func setupTestStateManager(t *testing.T) (*TransferStateManager, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "warp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	stateManager, err := NewTransferStateManager(filepath.Join(tmpDir, "transfers"))
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create state manager: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return stateManager, cleanup
}

func createTestSession(t *testing.T) *TransferSession {
	t.Helper()
	checkpoint := &Checkpoint{
		SessionID:   "test-session",
		TotalSize:   1024 * 1024,
		ChunkSize:   64 * 1024,
		TotalChunks: 16,
	}

	stateManager, _ := setupTestStateManager(t)
	return NewTransferSession(checkpoint, stateManager)
}

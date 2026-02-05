package resume

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestNewTransferStateManager(t *testing.T) {
	tempDir := t.TempDir()

	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	if mgr.GetBaseDir() != tempDir {
		t.Errorf("BaseDir = %v, want %v", mgr.GetBaseDir(), tempDir)
	}

	// Verify directory was created
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("Checkpoint directory was not created")
	}
}

func TestTransferStateManager_CreateCheckpoint(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	opts := CheckpointOptions{
		SessionID:       "test-session-123",
		SourcePath:      "/source/file.txt",
		DestinationPath: "/dest/file.txt",
		Direction:       "upload",
		TotalSize:       1000,
		ChunkSize:       100,
		TotalChunks:     10,
		Encrypted:       false,
		ExpiresIn:       24 * time.Hour,
	}

	cp, err := mgr.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	if cp.SessionID != opts.SessionID {
		t.Errorf("SessionID = %v, want %v", cp.SessionID, opts.SessionID)
	}

	// Verify file was created with correct permissions
	checkpointPath := filepath.Join(tempDir, opts.SessionID+".json")
	info, err := os.Stat(checkpointPath)
	if err != nil {
		t.Fatalf("Checkpoint file not created: %v", err)
	}

	// Check permissions (0600) - skip on Windows as it doesn't support Unix permissions
	if runtime.GOOS != "windows" {
		mode := info.Mode()
		if mode.Perm() != 0600 {
			t.Errorf("File permissions = %o, want 0600", mode.Perm())
		}
	}
}

func TestTransferStateManager_LoadCheckpoint(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Create a checkpoint
	opts := CheckpointOptions{
		SessionID:  "test-load",
		SourcePath: "/source",
		TotalSize:  1000,
		ChunkSize:  100,
	}

	created, err := mgr.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Load it back
	loaded, err := mgr.LoadCheckpoint(opts.SessionID)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if loaded.SessionID != created.SessionID {
		t.Errorf("Loaded SessionID = %v, want %v", loaded.SessionID, created.SessionID)
	}
	if loaded.SourcePath != created.SourcePath {
		t.Errorf("Loaded SourcePath = %v, want %v", loaded.SourcePath, created.SourcePath)
	}
}

func TestTransferStateManager_LoadCheckpoint_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	_, err = mgr.LoadCheckpoint("nonexistent")
	if err != ErrCheckpointNotFound {
		t.Errorf("Expected ErrCheckpointNotFound, got %v", err)
	}
}

func TestTransferStateManager_UpdateCheckpoint(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Create checkpoint
	opts := CheckpointOptions{
		SessionID:   "test-update",
		SourcePath:  "/source",
		TotalSize:   1000,
		TotalChunks: 10,
	}

	cp, err := mgr.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	originalHash := cp.CheckpointHash

	// Modify checkpoint
	cp.MarkChunkComplete(0, "checksum-0")
	cp.MarkChunkComplete(1, "checksum-1")

	// Update
	if err := mgr.UpdateCheckpoint(cp); err != nil {
		t.Fatalf("UpdateCheckpoint failed: %v", err)
	}

	// Load and verify
	loaded, err := mgr.LoadCheckpoint(opts.SessionID)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if len(loaded.CompletedChunks) != 2 {
		t.Errorf("CompletedChunks length = %v, want 2", len(loaded.CompletedChunks))
	}

	// Hash should have changed
	if loaded.CheckpointHash == originalHash {
		t.Error("CheckpointHash should have changed after update")
	}
}

func TestTransferStateManager_DeleteCheckpoint(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Create checkpoint
	opts := CheckpointOptions{
		SessionID:  "test-delete",
		SourcePath: "/source",
		TotalSize:  1000,
	}

	_, err = mgr.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Delete
	if err := mgr.DeleteCheckpoint(opts.SessionID); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}

	// Verify file is gone
	checkpointPath := filepath.Join(tempDir, opts.SessionID+".json")
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Error("Checkpoint file should be deleted")
	}

	// Try to load (should fail)
	_, err = mgr.LoadCheckpoint(opts.SessionID)
	if err != ErrCheckpointNotFound {
		t.Errorf("Expected ErrCheckpointNotFound after delete, got %v", err)
	}
}

func TestTransferStateManager_ListResumable(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Create multiple checkpoints
	for i := 0; i < 3; i++ {
		opts := CheckpointOptions{
			SessionID:  fmt.Sprintf("session-%d", i),
			SourcePath: fmt.Sprintf("/source-%d", i),
			TotalSize:  1000,
		}
		if _, err := mgr.CreateCheckpoint(opts); err != nil {
			t.Fatalf("CreateCheckpoint failed: %v", err)
		}
	}

	// List
	summaries, err := mgr.ListResumable()
	if err != nil {
		t.Fatalf("ListResumable failed: %v", err)
	}

	if len(summaries) != 3 {
		t.Errorf("ListResumable returned %d summaries, want 3", len(summaries))
	}
}

func TestTransferStateManager_FindByPath(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Create checkpoint
	opts := CheckpointOptions{
		SessionID:       "test-find",
		SourcePath:      "/unique/source.txt",
		DestinationPath: "/unique/dest.txt",
		TotalSize:       1000,
	}

	_, err = mgr.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Find by path
	found, err := mgr.FindByPath(opts.SourcePath, opts.DestinationPath)
	if err != nil {
		t.Fatalf("FindByPath failed: %v", err)
	}

	if found.SessionID != opts.SessionID {
		t.Errorf("Found SessionID = %v, want %v", found.SessionID, opts.SessionID)
	}

	// Try to find non-existent
	_, err = mgr.FindByPath("/nonexistent", "/nonexistent")
	if err != ErrCheckpointNotFound {
		t.Errorf("Expected ErrCheckpointNotFound, got %v", err)
	}
}

func TestTransferStateManager_CleanupStale(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Create old checkpoint
	oldOpts := CheckpointOptions{
		SessionID:  "old-session",
		SourcePath: "/old",
		TotalSize:  1000,
		ExpiresIn:  48 * time.Hour, // Set expiry far in future so it doesn't interfere
	}
	oldCP, err := mgr.CreateCheckpoint(oldOpts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Manually set old timestamps
	oldCP.CreatedAt = time.Now().Add(-25 * time.Hour)
	oldCP.UpdatedAt = time.Now().Add(-25 * time.Hour)
	if err := mgr.UpdateCheckpoint(oldCP); err != nil {
		t.Fatalf("UpdateCheckpoint failed: %v", err)
	}

	// Create recent checkpoint
	recentOpts := CheckpointOptions{
		SessionID:  "recent-session",
		SourcePath: "/recent",
		TotalSize:  1000,
		ExpiresIn:  48 * time.Hour,
	}
	if _, err := mgr.CreateCheckpoint(recentOpts); err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Cleanup stale (older than 24 hours)
	if err := mgr.CleanupStale(24 * time.Hour); err != nil {
		t.Fatalf("CleanupStale failed: %v", err)
	}

	// Old should be gone
	_, err = mgr.LoadCheckpoint(oldOpts.SessionID)
	if err != ErrCheckpointNotFound {
		t.Errorf("Old checkpoint should be deleted, got error: %v", err)
	}

	// Recent should still exist
	_, err = mgr.LoadCheckpoint(recentOpts.SessionID)
	if err != nil {
		t.Errorf("Recent checkpoint should still exist: %v", err)
	}
}

func TestTransferStateManager_TamperDetection(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Create checkpoint
	opts := CheckpointOptions{
		SessionID:  "test-tamper",
		SourcePath: "/source",
		TotalSize:  1000,
	}

	cp, err := mgr.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Tamper with the file
	checkpointPath := filepath.Join(tempDir, opts.SessionID+".json")
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("Failed to read checkpoint: %v", err)
	}

	// Modify data (change source path)
	tamperedData := []byte(string(data)[:len(data)-100] + "TAMPERED" + string(data)[len(data)-92:])
	if err := os.WriteFile(checkpointPath, tamperedData, 0600); err != nil {
		t.Fatalf("Failed to write tampered data: %v", err)
	}

	// Clear memory cache to force reload from disk
	mgr.mu.Lock()
	delete(mgr.sessions, cp.SessionID)
	mgr.mu.Unlock()

	// Try to load (should detect tampering)
	_, err = mgr.LoadCheckpoint(opts.SessionID)
	if err == nil {
		t.Error("LoadCheckpoint should fail for tampered checkpoint")
	}

	var checkpointErr *CheckpointError
	if !errors.As(err, &checkpointErr) {
		t.Errorf("Expected CheckpointError, got %T", err)
	}
}

func TestTransferStateManager_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	opts := CheckpointOptions{
		SessionID:  "test-atomic",
		SourcePath: "/source",
		TotalSize:  1000,
	}

	cp, err := mgr.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Verify no .tmp file left behind
	tmpPath := filepath.Join(tempDir, opts.SessionID+".json.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("Temporary file should not exist after successful write")
	}

	// Verify checkpoint can be loaded
	loaded, err := mgr.LoadCheckpoint(opts.SessionID)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if loaded.SessionID != cp.SessionID {
		t.Error("Checkpoint data corrupted")
	}
}

func TestTransferStateManager_MaxSessions(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("NewTransferStateManager failed: %v", err)
	}

	// Set low limit for testing
	mgr.maxSessions = 3

	// Create up to limit
	for i := 0; i < 3; i++ {
		opts := CheckpointOptions{
			SessionID:  fmt.Sprintf("session-%d", i),
			SourcePath: fmt.Sprintf("/source-%d", i),
			TotalSize:  1000,
		}
		if _, err := mgr.CreateCheckpoint(opts); err != nil {
			t.Fatalf("CreateCheckpoint %d failed: %v", i, err)
		}
	}

	// Try to create one more (should fail)
	opts := CheckpointOptions{
		SessionID:  "session-overflow",
		SourcePath: "/overflow",
		TotalSize:  1000,
	}
	_, err = mgr.CreateCheckpoint(opts)
	if err == nil {
		t.Error("CreateCheckpoint should fail when max sessions reached")
	}
}

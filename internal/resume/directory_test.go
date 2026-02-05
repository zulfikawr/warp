package resume

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateDirectoryCheckpoint(t *testing.T) {
	// Create temporary directory with test files
	tempDir := t.TempDir()

	// Create test files
	testFiles := map[string]int64{
		"file1.txt":        1024,
		"file2.txt":        2048,
		"subdir/file3.txt": 4096,
	}

	for path, size := range testFiles {
		fullPath := filepath.Join(tempDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		data := make([]byte, size)
		if err := os.WriteFile(fullPath, data, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create directory checkpoint
	checkpoint, err := CreateDirectoryCheckpoint("test-session", tempDir, "/dest", 1024)
	if err != nil {
		t.Fatalf("Failed to create directory checkpoint: %v", err)
	}

	// Verify checkpoint
	if checkpoint.SessionID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got '%s'", checkpoint.SessionID)
	}

	if checkpoint.RootPath != tempDir {
		t.Errorf("Expected root path '%s', got '%s'", tempDir, checkpoint.RootPath)
	}

	if len(checkpoint.Files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(checkpoint.Files))
	}

	// Verify total size
	expectedSize := int64(1024 + 2048 + 4096)
	if checkpoint.TotalSize != expectedSize {
		t.Errorf("Expected total size %d, got %d", expectedSize, checkpoint.TotalSize)
	}

	// Verify total chunks
	expectedChunks := 1 + 2 + 4 // file1: 1 chunk, file2: 2 chunks, file3: 4 chunks
	if checkpoint.TotalChunks != expectedChunks {
		t.Errorf("Expected %d total chunks, got %d", expectedChunks, checkpoint.TotalChunks)
	}
}

func TestDirectoryTransferSession_MarkFileComplete(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create test checkpoint
	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:  "test-session",
			SourcePath: "/src",
			TotalSize:  1000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500},
			{Path: "file2.txt", Size: 500},
		},
		CompletedFiles: []string{},
		FailedFiles:    []string{},
	}

	session := NewDirectoryTransferSession(checkpoint, stateManager)

	// Mark file as complete
	if err := session.MarkFileComplete("file1.txt"); err != nil {
		t.Fatalf("Failed to mark file complete: %v", err)
	}

	// Verify file is marked complete
	if !session.IsFileComplete("file1.txt") {
		t.Error("File should be marked complete")
	}

	if session.IsFileComplete("file2.txt") {
		t.Error("File2 should not be marked complete")
	}

	// Verify completed files list
	if len(checkpoint.CompletedFiles) != 1 {
		t.Errorf("Expected 1 completed file, got %d", len(checkpoint.CompletedFiles))
	}
}

func TestDirectoryTransferSession_MarkFileFailed(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:  "test-session",
			SourcePath: "/src",
			TotalSize:  1000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500},
			{Path: "file2.txt", Size: 500},
		},
		CompletedFiles: []string{},
		FailedFiles:    []string{},
	}

	session := NewDirectoryTransferSession(checkpoint, stateManager)

	// Mark file as failed
	testErr := os.ErrPermission
	if err := session.MarkFileFailed("file1.txt", testErr); err != nil {
		t.Fatalf("Failed to mark file failed: %v", err)
	}

	// Verify file is marked failed
	if !session.IsFileFailed("file1.txt") {
		t.Error("File should be marked failed")
	}

	if session.IsFileFailed("file2.txt") {
		t.Error("File2 should not be marked failed")
	}

	// Verify failed files list
	if len(checkpoint.FailedFiles) != 1 {
		t.Errorf("Expected 1 failed file, got %d", len(checkpoint.FailedFiles))
	}

	// Verify error message is stored
	fileProgress, err := session.GetFileProgress("file1.txt")
	if err != nil {
		t.Fatalf("Failed to get file progress: %v", err)
	}

	if !fileProgress.Failed {
		t.Error("File progress should be marked as failed")
	}

	if fileProgress.ErrorMessage == "" {
		t.Error("Error message should be stored")
	}
}

func TestDirectoryTransferSession_GetPendingFiles(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:  "test-session",
			SourcePath: "/src",
			TotalSize:  1500,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500},
			{Path: "file2.txt", Size: 500},
			{Path: "file3.txt", Size: 500},
		},
		CompletedFiles: []string{"file1.txt"},
		FailedFiles:    []string{"file2.txt"},
	}

	session := NewDirectoryTransferSession(checkpoint, stateManager)

	// Get pending files
	pending := session.GetPendingFiles()

	// Should only return file3.txt (not complete and not failed)
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending file, got %d", len(pending))
	}

	if len(pending) > 0 && pending[0].Path != "file3.txt" {
		t.Errorf("Expected pending file 'file3.txt', got '%s'", pending[0].Path)
	}
}

func TestDirectoryTransferSession_RetryFailedFiles(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:  "test-session",
			SourcePath: "/src",
			TotalSize:  1000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500, Failed: true, ErrorMessage: "test error"},
			{Path: "file2.txt", Size: 500},
		},
		CompletedFiles: []string{},
		FailedFiles:    []string{"file1.txt"},
	}

	session := NewDirectoryTransferSession(checkpoint, stateManager)

	// Retry failed files
	if err := session.RetryFailedFiles(); err != nil {
		t.Fatalf("Failed to retry failed files: %v", err)
	}

	// Verify failed files list is cleared
	if len(checkpoint.FailedFiles) != 0 {
		t.Errorf("Expected 0 failed files after retry, got %d", len(checkpoint.FailedFiles))
	}

	// Verify file progress is cleared
	fileProgress, err := session.GetFileProgress("file1.txt")
	if err != nil {
		t.Fatalf("Failed to get file progress: %v", err)
	}

	if fileProgress.Failed {
		t.Error("File should not be marked as failed after retry")
	}

	if fileProgress.ErrorMessage != "" {
		t.Error("Error message should be cleared after retry")
	}
}

func TestDirectoryTransferSession_GetProgress(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:  "test-session",
			SourcePath: "/src",
			TotalSize:  2000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500},
			{Path: "file2.txt", Size: 500},
			{Path: "file3.txt", Size: 500},
			{Path: "file4.txt", Size: 500},
		},
		CompletedFiles: []string{"file1.txt", "file2.txt"},
		FailedFiles:    []string{},
	}

	session := NewDirectoryTransferSession(checkpoint, stateManager)

	// Get progress
	completed, total, percentage := session.GetProgress()

	if completed != 2 {
		t.Errorf("Expected 2 completed files, got %d", completed)
	}

	if total != 4 {
		t.Errorf("Expected 4 total files, got %d", total)
	}

	expectedPercentage := 50.0
	if percentage != expectedPercentage {
		t.Errorf("Expected %.1f%% progress, got %.1f%%", expectedPercentage, percentage)
	}
}

func TestDirectoryTransferSession_IsComplete(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:  "test-session",
			SourcePath: "/src",
			TotalSize:  1000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500},
			{Path: "file2.txt", Size: 500},
		},
		CompletedFiles: []string{},
		FailedFiles:    []string{},
	}

	session := NewDirectoryTransferSession(checkpoint, stateManager)

	// Should not be complete initially
	if session.IsComplete() {
		t.Error("Session should not be complete initially")
	}

	// Mark first file complete
	if err := session.MarkFileComplete("file1.txt"); err != nil {
		t.Fatalf("Failed to mark file complete: %v", err)
	}

	// Should still not be complete
	if session.IsComplete() {
		t.Error("Session should not be complete with only 1 of 2 files done")
	}

	// Mark second file complete
	if err := session.MarkFileComplete("file2.txt"); err != nil {
		t.Fatalf("Failed to mark file complete: %v", err)
	}

	// Should now be complete
	if !session.IsComplete() {
		t.Error("Session should be complete with all files done")
	}
}

func TestDirectoryTransferSession_Callbacks(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:  "test-session",
			SourcePath: "/src",
			TotalSize:  1000,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500},
			{Path: "file2.txt", Size: 500},
		},
		CompletedFiles: []string{},
		FailedFiles:    []string{},
	}

	session := NewDirectoryTransferSession(checkpoint, stateManager)

	// Track callback invocations
	fileCompleteCalled := false
	fileFailedCalled := false
	progressCalled := false

	session.OnFileComplete(func(filePath string) {
		fileCompleteCalled = true
		if filePath != "file1.txt" {
			t.Errorf("Expected file path 'file1.txt', got '%s'", filePath)
		}
	})

	session.OnFileFailed(func(filePath string, err error) {
		fileFailedCalled = true
		if filePath != "file2.txt" {
			t.Errorf("Expected file path 'file2.txt', got '%s'", filePath)
		}
	})

	session.OnProgress(func(completed, total int) {
		progressCalled = true
	})

	// Mark file complete
	if err := session.MarkFileComplete("file1.txt"); err != nil {
		t.Fatalf("Failed to mark file complete: %v", err)
	}

	// Mark file failed
	if err := session.MarkFileFailed("file2.txt", os.ErrPermission); err != nil {
		t.Fatalf("Failed to mark file failed: %v", err)
	}

	// Give callbacks time to execute (they run in goroutines)
	time.Sleep(100 * time.Millisecond)

	if !fileCompleteCalled {
		t.Error("OnFileComplete callback was not called")
	}

	if !fileFailedCalled {
		t.Error("OnFileFailed callback was not called")
	}

	if !progressCalled {
		t.Error("OnProgress callback was not called")
	}
}

func TestSaveAndLoadDirectoryCheckpoint(t *testing.T) {
	tempDir := t.TempDir()
	stateManager, err := NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create test checkpoint
	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:       "test-session",
			Version:         "1.0",
			SourcePath:      "/src",
			DestinationPath: "/dest",
			Direction:       "upload",
			TotalSize:       1000,
			ChunkSize:       512,
			TotalChunks:     4,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			ExpiresAt:       time.Now().Add(24 * time.Hour),
		},
		Files: []FileProgress{
			{Path: "file1.txt", Size: 500, TotalChunks: 2},
			{Path: "file2.txt", Size: 500, TotalChunks: 2},
		},
		CompletedFiles: []string{"file1.txt"},
		FailedFiles:    []string{},
		RootPath:       "/src",
	}

	// Save checkpoint
	if err := stateManager.SaveDirectoryCheckpoint(checkpoint); err != nil {
		t.Fatalf("Failed to save directory checkpoint: %v", err)
	}

	// Load checkpoint
	loaded, err := stateManager.LoadDirectoryCheckpoint("test-session")
	if err != nil {
		t.Fatalf("Failed to load directory checkpoint: %v", err)
	}

	// Verify loaded checkpoint
	if loaded.SessionID != checkpoint.SessionID {
		t.Errorf("Expected session ID '%s', got '%s'", checkpoint.SessionID, loaded.SessionID)
	}

	if loaded.RootPath != checkpoint.RootPath {
		t.Errorf("Expected root path '%s', got '%s'", checkpoint.RootPath, loaded.RootPath)
	}

	if len(loaded.Files) != len(checkpoint.Files) {
		t.Errorf("Expected %d files, got %d", len(checkpoint.Files), len(loaded.Files))
	}

	if len(loaded.CompletedFiles) != len(checkpoint.CompletedFiles) {
		t.Errorf("Expected %d completed files, got %d", len(checkpoint.CompletedFiles), len(loaded.CompletedFiles))
	}
}

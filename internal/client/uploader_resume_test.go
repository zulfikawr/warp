package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/resume"
)

func TestUploadSession_WithCheckpoint(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")
	testData := make([]byte, 10*1024*1024) // 10MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	if err := os.WriteFile(testFile, testData, 0o600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create state manager
	stateDir := filepath.Join(tmpDir, "checkpoints")
	stateManager, err := resume.NewTransferStateManager(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create a mock server that accepts chunks
	chunksReceived := make(map[int]bool)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunkID := r.Header.Get("X-Chunk-Id")
		if chunkID != "" {
			var id int
			fmt.Sscanf(chunkID, "%d", &id)
			chunksReceived[id] = true
		}
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	// Create upload session with checkpoint
	config := &UploadConfig{
		ChunkSize:     2 * 1024 * 1024, // 2MB chunks
		MaxConcurrent: 2,
		RetryAttempts: 1,
		RetryDelay:    100 * time.Millisecond,
	}

	session, err := NewUploadSession(server.URL, testFile, config, stateManager, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create upload session: %v", err)
	}

	// Verify checkpoint was created
	if session.Checkpoint == nil {
		t.Fatal("Checkpoint was not created")
	}

	// Verify checkpoint has correct metadata
	if session.Checkpoint.TotalSize != int64(len(testData)) {
		t.Errorf("Checkpoint total size mismatch: got %d, want %d", session.Checkpoint.TotalSize, len(testData))
	}

	if session.Checkpoint.Direction != "upload" {
		t.Errorf("Checkpoint direction mismatch: got %s, want upload", session.Checkpoint.Direction)
	}

	// Start upload
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = session.Upload(ctx)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify all chunks were uploaded
	expectedChunks := 5 // 10MB / 2MB = 5 chunks
	if len(chunksReceived) != expectedChunks {
		t.Errorf("Expected %d chunks, got %d", expectedChunks, len(chunksReceived))
	}

	// Verify checkpoint was deleted after successful upload
	checkpoints, err := stateManager.ListResumable()
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 0 {
		t.Errorf("Expected checkpoint to be deleted, but found %d checkpoints", len(checkpoints))
	}
}

func TestResumeUploadSession(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")
	testData := make([]byte, 10*1024*1024) // 10MB
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	if err := os.WriteFile(testFile, testData, 0o600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create state manager
	stateDir := filepath.Join(tmpDir, "checkpoints")
	stateManager, err := resume.NewTransferStateManager(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Track which chunks are received
	var mu sync.Mutex
	chunksReceived := make(map[int]bool)
	failAfter := 3 // Fail after receiving 3 chunks

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		chunkID := r.Header.Get("X-Chunk-Id")
		if chunkID != "" {
			var id int
			fmt.Sscanf(chunkID, "%d", &id)

			// Check if we should fail
			if failAfter > 0 && len(chunksReceived) >= failAfter {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.ReadAll(r.Body)
				return
			}

			chunksReceived[id] = true
		}
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	// Create upload session
	config := &UploadConfig{
		ChunkSize:     2 * 1024 * 1024, // 2MB chunks = 5 total chunks
		MaxConcurrent: 1,               // Sequential for predictable behavior
		RetryAttempts: 0,               // No retries
		RetryDelay:    100 * time.Millisecond,
	}

	session, err := NewUploadSession(server.URL, testFile, config, stateManager, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create upload session: %v", err)
	}

	sessionID := session.SessionID

	// Start upload (will fail after 3 chunks)
	ctx := context.Background()
	err = session.Upload(ctx)
	if err == nil {
		t.Fatal("Expected upload to fail, but it succeeded")
	}

	// Wait for async checkpoint save
	time.Sleep(300 * time.Millisecond)

	// Verify checkpoint exists with partial progress
	checkpoint, err := stateManager.LoadCheckpoint(sessionID)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	t.Logf("First attempt: %d chunks completed: %v", len(checkpoint.CompletedChunks), checkpoint.CompletedChunks)

	if len(checkpoint.CompletedChunks) != 3 {
		t.Errorf("Expected 3 completed chunks, got %d", len(checkpoint.CompletedChunks))
	}

	// Reset for resume - allow all chunks now
	mu.Lock()
	failAfter = 0
	chunksReceivedOnResume := make(map[int]bool)
	mu.Unlock()

	// Create new server that tracks only resume uploads
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunkID := r.Header.Get("X-Chunk-Id")
		if chunkID != "" {
			var id int
			fmt.Sscanf(chunkID, "%d", &id)
			chunksReceivedOnResume[id] = true
		}
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server2.Close()

	// Resume upload with new server URL
	resumedSession, err := ResumeUploadSession(checkpoint, server2.URL, config, stateManager, nil)
	if err != nil {
		t.Fatalf("Failed to resume upload session: %v", err)
	}

	// Verify resumed session has correct state
	if resumedSession.SessionID != sessionID {
		t.Errorf("Session ID mismatch: got %s, want %s", resumedSession.SessionID, sessionID)
	}

	// Complete the upload
	err = resumedSession.Upload(ctx)
	if err != nil {
		t.Fatalf("Resumed upload failed: %v", err)
	}

	t.Logf("Resume attempt: %d chunks uploaded: %v", len(chunksReceivedOnResume), chunksReceivedOnResume)

	// Verify only remaining chunks were uploaded (chunks 3 and 4, since 0,1,2 were completed)
	expectedRemaining := 2 // 5 total - 3 completed = 2 remaining
	if len(chunksReceivedOnResume) != expectedRemaining {
		t.Errorf("Expected %d chunks to be uploaded on resume, got %d", expectedRemaining, len(chunksReceivedOnResume))
	}

	// Verify already completed chunks (0, 1, 2) were NOT re-uploaded
	originalCompleted := []int{0, 1, 2} // Save the original completed chunks
	for _, completedChunk := range originalCompleted {
		if chunksReceivedOnResume[completedChunk] {
			t.Errorf("Already completed chunk %d was re-uploaded", completedChunk)
		}
	}

	// Verify the remaining chunks (3, 4) WERE uploaded
	if !chunksReceivedOnResume[3] || !chunksReceivedOnResume[4] {
		t.Error("Expected chunks 3 and 4 to be uploaded on resume")
	}

	// Note: Checkpoint deletion happens asynchronously and may not complete immediately
	// The important thing is that the resume worked correctly
}

func TestUploadSession_FileModificationDetection(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")
	testData := make([]byte, 1024*1024) // 1MB
	if err := os.WriteFile(testFile, testData, 0o600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create state manager
	stateDir := filepath.Join(tmpDir, "checkpoints")
	stateManager, err := resume.NewTransferStateManager(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create a checkpoint
	config := &UploadConfig{
		ChunkSize:     512 * 1024, // 512KB chunks
		MaxConcurrent: 1,
		RetryAttempts: 0,
	}

	opts := resume.CheckpointOptions{
		SessionID:   "test-session",
		SourcePath:  testFile,
		Direction:   "upload",
		TotalSize:   int64(len(testData)),
		ChunkSize:   config.ChunkSize,
		TotalChunks: 2,
	}

	checkpoint, err := stateManager.CreateCheckpoint(opts)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Wait a bit to ensure modification time will be different
	time.Sleep(10 * time.Millisecond)

	// Modify the file
	modifiedData := make([]byte, 2*1024*1024) // 2MB - different size
	if err := os.WriteFile(testFile, modifiedData, 0o600); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Try to resume - should fail due to size mismatch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err = ResumeUploadSession(checkpoint, server.URL, config, stateManager, nil)
	if err == nil {
		t.Fatal("Expected error when resuming with modified file, but got none")
	}

	expectedError := fmt.Sprintf("file size mismatch: checkpoint has %d bytes, file has %d bytes", len(testData), len(modifiedData))
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got: %v", expectedError, err)
	}
}

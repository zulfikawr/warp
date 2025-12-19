package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestParallelUpload(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-upload.bin")

	// Create 5MB test file
	testData := make([]byte, 5*1024*1024)
	_, err := rand.Read(testData)
	if err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create mock server
	var mu sync.Mutex
	receivedChunks := make(map[int][]byte)
	var sessionID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sessionID = r.Header.Get("X-Upload-Session")
		mu.Unlock()

		chunkIDStr := r.Header.Get("X-Chunk-Id")

		if chunkIDStr == "" {
			http.Error(w, "missing chunk id", http.StatusBadRequest)
			return
		}

		chunkID := 0
		if _, err := fmt.Sscanf(chunkIDStr, "%d", &chunkID); err != nil {
			http.Error(w, "invalid chunk id", http.StatusBadRequest)
			return
		}

		// Read chunk data
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		mu.Lock()
		receivedChunks[chunkID] = data
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	// Test parallel upload
	config := &UploadConfig{
		ChunkSize:     1 * 1024 * 1024, // 1MB chunks
		MaxConcurrent: 3,
		RetryAttempts: 2,
		RetryDelay:    100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = ParallelUpload(ctx, server.URL, testFile, config, nil)
	if err != nil {
		t.Fatalf("Parallel upload failed: %v", err)
	}

	// Verify all chunks were received
	mu.Lock()
	expectedChunks := 5 // 5MB / 1MB
	if len(receivedChunks) != expectedChunks {
		mu.Unlock()
		t.Errorf("Expected %d chunks, got %d", expectedChunks, len(receivedChunks))
	} else {
		mu.Unlock()
	}

	// Reassemble and verify data
	var reassembled bytes.Buffer
	for i := 0; i < expectedChunks; i++ {
		mu.Lock()
		chunk, ok := receivedChunks[i]
		mu.Unlock()

		if !ok {
			t.Errorf("Missing chunk %d", i)
			continue
		}
		reassembled.Write(chunk)
	}

	if !bytes.Equal(testData, reassembled.Bytes()) {
		t.Error("Reassembled data does not match original")
	}

	if sessionID == "" {
		t.Error("Session ID was not set")
	}
}

func TestUploadSessionRetry(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-retry.bin")
	testData := []byte("test data for retry")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create mock server that fails first 2 attempts
	attempts := make(map[int]int)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunkIDStr := r.Header.Get("X-Chunk-Id")
		chunkID := 0
		if _, err := fmt.Sscanf(chunkIDStr, "%d", &chunkID); err != nil {
			http.Error(w, "invalid chunk id", http.StatusBadRequest)
			return
		}
		attempts[chunkID]++

		// Fail first 2 attempts
		if attempts[chunkID] < 3 {
			http.Error(w, "simulated failure", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	// Test with retry enabled
	config := &UploadConfig{
		ChunkSize:     int64(len(testData)), // Single chunk
		MaxConcurrent: 1,
		RetryAttempts: 3,
		RetryDelay:    50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := ParallelUpload(ctx, server.URL, testFile, config, nil)
	if err != nil {
		t.Fatalf("Upload with retry failed: %v", err)
	}

	// Verify retries occurred
	if attempts[0] != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts[0])
	}
}

func TestUploadSessionCancel(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-cancel.bin")
	testData := make([]byte, 10*1024*1024) // 10MB
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Simulate slow upload
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	config := &UploadConfig{
		ChunkSize:     1 * 1024 * 1024,
		MaxConcurrent: 2,
		RetryAttempts: 0,
	}

	session, err := NewUploadSession(server.URL, testFile, config)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start upload in goroutine
	uploadErr := make(chan error, 1)
	go func() {
		uploadErr <- session.Upload(ctx)
	}()

	// Cancel after 500ms
	time.Sleep(500 * time.Millisecond)
	cancel()

	// Should return quickly with context canceled error
	select {
	case err := <-uploadErr:
		if err == nil {
			t.Error("Expected error after cancel, got nil")
		}
		if err != context.Canceled {
			t.Logf("Got error: %v (expected context.Canceled)", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Upload did not cancel in time")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", tt.bytes, result, tt.expected)
		}
	}
}

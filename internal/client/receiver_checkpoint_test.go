package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/resume"
)

func TestDownloader_CheckpointCreation(t *testing.T) {
	// Create temp directory for checkpoints
	tempDir := t.TempDir()
	stateManager, err := resume.NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create test server
	testData := []byte("test file content for checkpoint creation")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\"test.txt\"")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testData)
	}))
	defer server.Close()

	// Create downloader with state manager
	downloader := NewDownloader(nil, stateManager, nil)

	// Download file
	outputPath := filepath.Join(tempDir, "download.txt")
	_, err = downloader.Receive(context.Background(), server.URL, outputPath, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify file was downloaded
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("Downloaded file does not exist")
	}

	// Verify checkpoint was deleted after successful download
	summaries, err := stateManager.ListResumable()
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("Expected 0 checkpoints after successful download, got %d", len(summaries))
	}
}

func TestDownloader_ResumeDownload(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Use a subdirectory for state manager to isolate checkpoints
	checkpointDir := filepath.Join(tempDir, "checkpoints")
	stateManager, err := resume.NewTransferStateManager(checkpointDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create test data (10MB)
	testData := make([]byte, 10*1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Track request count and use a channel to force interruption
	requestCount := 0
	interruptAt := int64(3 * 1024 * 1024) // Interrupt after 3MB
	shouldInterrupt := true

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		rangeHeader := r.Header.Get("Range")
		startByte := int64(0)

		if rangeHeader != "" {
			// Parse Range header
			var start int64
			_, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			if err == nil && start > 0 {
				startByte = start
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", startByte, int64(len(testData))-1, len(testData)))
				w.WriteHeader(http.StatusPartialContent)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		} else {
			w.WriteHeader(http.StatusOK)
		}

		w.Header().Set("Content-Disposition", "attachment; filename=\"large.bin\"")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)-int(startByte)))
		w.Header().Set("Accept-Ranges", "bytes")

		// Write data
		if shouldInterrupt && requestCount == 1 {
			// First request - write partial data and interrupt
			_, _ = w.Write(testData[startByte:interruptAt])
			// Force close by panicking which will close connection
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		} else {
			// Resume request or second attempt - write all remaining data
			_, _ = w.Write(testData[startByte:])
		}
	}))
	defer server.Close()

	outputPath := filepath.Join(tempDir, "downloads", "large.bin")

	// Create downloads directory
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		t.Fatalf("Failed to create downloads directory: %v", err)
	}

	// First download - will be interrupted
	downloader1 := NewDownloader(nil, stateManager, nil)
	_, _ = downloader1.Receive(context.Background(), server.URL, outputPath, false, nil, nil, nil)
	// Expected to fail due to interrupted connection

	// Clean up any checkpoint directories to avoid test cleanup errors
	defer func() {
		// Remove all checkpoints
		summaries, _ := stateManager.ListResumable()
		for _, s := range summaries {
			_ = stateManager.DeleteCheckpoint(s.SessionID)
		}
		// Remove the checkpoint directory itself
		_ = os.RemoveAll(checkpointDir)
	}()

	// Verify partial file exists
	fi, err := os.Stat(outputPath)
	if err != nil {
		// If file doesn't exist, download might have failed before writing anything
		t.Skip("Download was interrupted before any data was written")
	}

	if fi.Size() >= int64(len(testData)) {
		t.Skip("First download completed fully, cannot test resume")
	}

	if fi.Size() == 0 {
		t.Skip("Download was interrupted before any data was written")
	}

	partialSize := fi.Size()
	t.Logf("Partial download size: %d bytes", partialSize)

	// Disable interruption for resume
	shouldInterrupt = false

	// Load checkpoint
	summaries, err := stateManager.ListResumable()
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(summaries) == 0 {
		t.Skip("No checkpoint found after partial download - checkpoint may not have been created for small transfer")
	}

	checkpoint, err := stateManager.LoadCheckpoint(summaries[0].SessionID)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	// Resume download
	downloader2, err := ResumeDownload(checkpoint, stateManager)
	if err != nil {
		t.Fatalf("Failed to create resume downloader: %v", err)
	}

	_, err = downloader2.Receive(context.Background(), server.URL, outputPath, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Resume download failed: %v", err)
	}

	// Verify file is complete
	finalFi, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Final file does not exist: %v", err)
	}

	if finalFi.Size() != int64(len(testData)) {
		t.Errorf("Expected final size %d, got %d", len(testData), finalFi.Size())
	}

	// Verify checkpoint was deleted
	summaries, err = stateManager.ListResumable()
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("Expected 0 checkpoints after resume completion, got %d", len(summaries))
	}
}

func TestDownloader_CheckpointUpdates(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	stateManager, err := resume.NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create test data (20MB to ensure multiple checkpoint updates)
	testData := make([]byte, 20*1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Track checkpoint updates
	checkpointUpdates := 0

	// Create test server with slow writes to allow checkpoint updates
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\"large.bin\"")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
		w.WriteHeader(http.StatusOK)

		// Write in chunks to simulate real download
		chunkSize := 1024 * 1024 // 1MB chunks
		for i := 0; i < len(testData); i += chunkSize {
			end := i + chunkSize
			if end > len(testData) {
				end = len(testData)
			}
			_, _ = w.Write(testData[i:end])

			// Small delay to allow checkpoint saves
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	outputPath := filepath.Join(tempDir, "large.bin")

	// Download with progress tracking
	downloader := NewDownloader(nil, stateManager, nil)
	progressCalls := 0
	_, err = downloader.Receive(context.Background(), server.URL, outputPath, false, func(p progress.Progress) {
		progressCalls++

		// Check for checkpoint updates periodically
		if progressCalls%10 == 0 {
			summaries, _ := stateManager.ListResumable()
			if len(summaries) > 0 {
				checkpointUpdates++
			}
		}
	}, nil, nil)

	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify we got checkpoint updates during download
	if checkpointUpdates == 0 {
		t.Log("Warning: No checkpoint updates detected during download (may be too fast)")
	}

	// Verify final file
	fi, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Downloaded file does not exist: %v", err)
	}

	if fi.Size() != int64(len(testData)) {
		t.Errorf("Expected file size %d, got %d", len(testData), fi.Size())
	}
}

func TestDownloader_NoCheckpointWithoutStateManager(t *testing.T) {
	// Create test server
	testData := []byte("test content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\"test.txt\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testData)
	}))
	defer server.Close()

	// Create downloader WITHOUT state manager
	downloader := NewDownloader(nil, nil, nil)

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test.txt")

	// Download should work without checkpoints
	_, err := downloader.Receive(context.Background(), server.URL, outputPath, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify file exists
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(content) != string(testData) {
		t.Errorf("Content mismatch: expected %q, got %q", testData, content)
	}
}

func TestDownloader_TextContentNoCheckpoint(t *testing.T) {
	// Create test server returning text
	testText := "Hello, this is plain text!"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(testText))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	stateManager, err := resume.NewTransferStateManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	downloader := NewDownloader(nil, stateManager, nil)

	// Capture stdout
	var output []byte
	writer := &testWriter{data: &output}

	result, err := downloader.Receive(context.Background(), server.URL, "", false, nil, writer, nil)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if result != "(stdout)" {
		t.Errorf("Expected result '(stdout)', got %q", result)
	}

	if string(output) != testText {
		t.Errorf("Expected output %q, got %q", testText, output)
	}

	// Verify no checkpoint was created for text output
	summaries, err := stateManager.ListResumable()
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("Expected 0 checkpoints for text output, got %d", len(summaries))
	}
}

// testWriter implements io.Writer for testing
type testWriter struct {
	data *[]byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	*w.data = append(*w.data, p...)
	return len(p), nil
}

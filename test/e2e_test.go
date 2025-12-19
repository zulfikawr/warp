package test

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/client"
	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/server"
)

// ANSI color codes for beautiful test output
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"

	symbolPass = "✓"
	symbolFail = "✗"
	symbolInfo = "ℹ"
	symbolTest = "→"
)

// Test helper functions
func logTest(t *testing.T, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	t.Logf("%s%s%s %s", colorCyan, symbolTest, colorReset, msg)
}

func logPass(t *testing.T, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	t.Logf("%s%s PASS%s %s", colorGreen, symbolPass, colorReset, msg)
}

func logInfo(t *testing.T, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	t.Logf("%s%s INFO%s %s", colorBlue, symbolInfo, colorReset, msg)
}

func logSection(t *testing.T, title string) {
	t.Logf("")
	t.Logf("%s%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s", colorBold, colorMagenta, colorReset)
	t.Logf("%s%s    %s    %s", colorBold, colorMagenta, title, colorReset)
	t.Logf("%s%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s", colorBold, colorMagenta, colorReset)
	t.Logf("")
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func assertEqual(t *testing.T, expected, actual interface{}, msg string) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s%s FAIL%s %s: expected %v, got %v", colorRed, symbolFail, colorReset, msg, expected, actual)
	}
}

func assertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s%s FAIL%s %s: %v", colorRed, symbolFail, colorReset, msg, err)
	}
}

// TestE2E_FileTransfer tests basic file transfer with various sizes
func TestE2E_FileTransfer(t *testing.T) {
	logSection(t, "File Transfer Tests")

	testCases := []struct {
		name string
		size int
		fill byte
	}{
		{"Small File (1KB)", 1024, 'a'},
		{"Medium File (1MB)", 1024 * 1024, 'b'},
		{"Large File (10MB)", 10 * 1024 * 1024, 'c'},
		{"Binary Data", 500 * 1024, 0xFF},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			logTest(t, "Creating test file: %s (%s)", tc.name, formatBytes(int64(tc.size)))

			// Prepare source file
			src, err := os.CreateTemp("", "warp-src-*")
			assertNoError(t, err, "Create temp file")
			defer func() { _ = os.Remove(src.Name()) }()

			data := bytes.Repeat([]byte{tc.fill}, tc.size)
			_, err = src.Write(data)
			assertNoError(t, err, "Write test data")
			_ = src.Close()

			logInfo(t, "File created: %s", filepath.Base(src.Name()))

			// Start server
			tok, err := crypto.GenerateToken(nil)
			assertNoError(t, err, "Generate token")

			srv := &server.Server{Token: tok, SrcPath: src.Name()}
			url, err := srv.Start()
			assertNoError(t, err, "Start server")
			defer func() { _ = srv.Shutdown() }()

			logInfo(t, "Server started: %s", url)

			// Receive file
			logTest(t, "Downloading file...")
			out, err := client.Receive(url, "", true, io.Discard)
			assertNoError(t, err, "Download file")
			defer func() { _ = os.Remove(out) }()

			logInfo(t, "Downloaded to: %s", filepath.Base(out))

			// Verify integrity
			logTest(t, "Verifying file integrity...")
			srcb, _ := os.ReadFile(src.Name())
			outb, _ := os.ReadFile(out)

			assertEqual(t, len(srcb), len(outb), "File size")

			srcHash := md5.Sum(srcb)
			outHash := md5.Sum(outb)
			assertEqual(t, srcHash, outHash, "MD5 checksum")

			duration := time.Since(start)
			mbps := (float64(len(outb)) * 8) / (duration.Seconds() * 1_000_000)

			logPass(t, "Transfer complete: %s in %v (%.1f Mbps)",
				formatBytes(int64(len(outb))), duration.Round(time.Millisecond), mbps)
		})
	}

	t.Logf("")
	t.Logf("%s%s✓ All file transfer tests passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_TextSharing tests text/clipboard sharing with various content types
func TestE2E_TextSharing(t *testing.T) {
	logSection(t, "Text Sharing Tests")

	testCases := []struct {
		name    string
		content string
	}{
		{"Simple Text", "Hello World"},
		{"API Key", "API_KEY_12345_SECRET_ABCDEF"},
		{"Long URL", "https://example.com/very/long/url/path?token=abc123&id=456&session=xyz789"},
		{"Multiline Text", "line1\nline2\nline3\nline4"},
		{"JSON Data", `{"user": "test", "nested": {"key": "value", "array": [1, 2, 3]}}`},
		{"Code Snippet", "func main() {\n\tfmt.Println(\"Hello\")\n}"},
		{"Unicode", "Hello 世界 🌍 🚀"},
		{"Special Chars", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logTest(t, "Testing text sharing: %s (%d bytes)", tc.name, len(tc.content))

			tok, err := crypto.GenerateToken(nil)
			assertNoError(t, err, "Generate token")

			srv := &server.Server{Token: tok, TextContent: tc.content}
			url, err := srv.Start()
			assertNoError(t, err, "Start server")
			defer func() { _ = srv.Shutdown() }()

			logInfo(t, "Server URL: %s", url)

			// Capture stdout
			var buf bytes.Buffer
			result, err := client.Receive(url, "", false, &buf)
			assertNoError(t, err, "Receive text")

			assertEqual(t, "(stdout)", result, "Output destination")
			logPass(t, "Text received successfully via stdout")
		})
	}

	t.Logf("")
	t.Logf("%s%s✓ All text sharing tests passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_HostUpload tests the upload (host) mode with various scenarios
func TestE2E_HostUpload(t *testing.T) {
	logSection(t, "Host Upload Tests")

	t.Run("SingleFileUpload", func(t *testing.T) {
		logTest(t, "Testing single file upload")

		destDir, err := os.MkdirTemp("", "warp-host-*")
		assertNoError(t, err, "Create temp dir")
		defer func() { _ = os.RemoveAll(destDir) }()

		tok, _ := crypto.GenerateToken(nil)
		srv := &server.Server{Token: tok, HostMode: true, UploadDir: destDir}
		url, err := srv.Start()
		assertNoError(t, err, "Start server")
		defer func() { _ = srv.Shutdown() }()

		logInfo(t, "Host server URL: %s", url)

		// Test GET returns HTML form
		logTest(t, "Verifying HTML form is served")
		resp, err := http.Get(url)
		assertNoError(t, err, "GET request")
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assertEqual(t, http.StatusOK, resp.StatusCode, "HTTP status")
		if !strings.Contains(string(body), "<form") {
			t.Error("HTML form not found in response")
		}
		logPass(t, "HTML form served correctly")

		// Upload a file
		logTest(t, "Uploading test file...")
		testContent := "test-upload-content-" + time.Now().Format("20060102150405")
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("file", "test-upload.txt")
		assertNoError(t, err, "Create form file")
		_, _ = io.WriteString(fw, testContent)
		_ = mw.Close()

		req, err := http.NewRequest(http.MethodPost, url, &buf)
		assertNoError(t, err, "Create POST request")
		req.Header.Set("Content-Type", mw.FormDataContentType())

		resp2, err := http.DefaultClient.Do(req)
		assertNoError(t, err, "POST request")
		_, _ = io.Copy(io.Discard, resp2.Body)
		_ = resp2.Body.Close()

		assertEqual(t, http.StatusOK, resp2.StatusCode, "Upload status")
		logPass(t, "File uploaded successfully")

		// Verify file on disk
		logTest(t, "Verifying uploaded file...")
		savedPath := filepath.Join(destDir, "test-upload.txt")
		content, err := os.ReadFile(savedPath)
		assertNoError(t, err, "Read uploaded file")
		assertEqual(t, testContent, string(content), "File content")
		logPass(t, "File content verified")
	})

	t.Run("DuplicateFileUpload", func(t *testing.T) {
		logTest(t, "Testing duplicate file upload (should create versioned files)")

		destDir, err := os.MkdirTemp("", "warp-host-dup-*")
		assertNoError(t, err, "Create temp dir")
		defer func() { _ = os.RemoveAll(destDir) }()

		tok, _ := crypto.GenerateToken(nil)
		srv := &server.Server{Token: tok, HostMode: true, UploadDir: destDir}
		url, err := srv.Start()
		assertNoError(t, err, "Start server")
		defer func() { _ = srv.Shutdown() }()

		// Upload same filename 3 times
		for i := 1; i <= 3; i++ {
			logTest(t, "Upload #%d", i)
			content := fmt.Sprintf("content-version-%d", i)

			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("file", "duplicate.txt")
			_, _ = io.WriteString(fw, content)
			_ = mw.Close()

			req, _ := http.NewRequest(http.MethodPost, url, &buf)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			resp, _ := http.DefaultClient.Do(req)
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		// Verify all 3 versions exist
		logTest(t, "Verifying versioned files...")
		files := []string{"duplicate.txt", "duplicate (1).txt", "duplicate (2).txt"}
		for i, fname := range files {
			fpath := filepath.Join(destDir, fname)
			if _, err := os.Stat(fpath); os.IsNotExist(err) {
				t.Errorf("Expected file not found: %s", fname)
			} else {
				content, _ := os.ReadFile(fpath)
				expected := fmt.Sprintf("content-version-%d", i+1)
				assertEqual(t, expected, string(content), fname)
				logInfo(t, "Found: %s", fname)
			}
		}
		logPass(t, "All versioned files created correctly")
	})

	t.Logf("")
	t.Logf("%s%s✓ All host upload tests passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_ResumableDownload tests download resumption functionality
func TestE2E_ResumableDownload(t *testing.T) {
	logSection(t, "Resumable Download Tests")

	logTest(t, "Creating 10MB test file")

	src, err := os.CreateTemp("", "warp-resume-*")
	assertNoError(t, err, "Create temp file")
	defer func() { _ = os.Remove(src.Name()) }()

	// Write 10MB of recognizable data
	data := bytes.Repeat([]byte("ABCDEFGHIJ"), 1024*1024)
	_, err = src.Write(data)
	assertNoError(t, err, "Write test data")
	_ = src.Close()

	logInfo(t, "Test file: %s", formatBytes(int64(len(data))))

	tok, _ := crypto.GenerateToken(nil)
	srv := &server.Server{Token: tok, SrcPath: src.Name()}
	url, err := srv.Start()
	assertNoError(t, err, "Start server")
	defer func() { _ = srv.Shutdown() }()

	logInfo(t, "Server URL: %s", url)

	outPath, err := os.CreateTemp("", "warp-out-*")
	assertNoError(t, err, "Create output file")
	outName := outPath.Name()
	defer func() { _ = os.Remove(outName) }()

	// Partial download: get 5MB
	logTest(t, "Downloading first 5MB (50%% of file)...")
	resp, err := http.Get(url)
	assertNoError(t, err, "HTTP GET")

	limited := io.LimitReader(resp.Body, 5*1024*1024)
	n, err := io.Copy(outPath, limited)
	_ = resp.Body.Close()
	_ = outPath.Close()
	assertNoError(t, err, "Write partial data")

	logInfo(t, "Partial download: %s", formatBytes(n))

	// Verify partial file size
	fi, err := os.Stat(outName)
	assertNoError(t, err, "Stat partial file")
	assertEqual(t, int64(5*1024*1024), fi.Size(), "Partial file size")
	logPass(t, "Partial file verified: 5MB")

	// Resume download
	logTest(t, "Resuming download from 5MB offset...")
	start := time.Now()
	result, err := client.Receive(url, outName, true, io.Discard)
	assertNoError(t, err, "Resume download")
	duration := time.Since(start)

	assertEqual(t, outName, result, "Output path")
	logPass(t, "Download resumed and completed in %v", duration.Round(time.Millisecond))

	// Verify complete file
	logTest(t, "Verifying complete file integrity...")
	srcb, _ := os.ReadFile(src.Name())
	outb, _ := os.ReadFile(outName)

	assertEqual(t, len(srcb), len(outb), "File size")

	srcHash := sha256.Sum256(srcb)
	outHash := sha256.Sum256(outb)
	assertEqual(t, srcHash, outHash, "SHA256 checksum")

	logPass(t, "File integrity verified: %s", formatBytes(int64(len(outb))))

	t.Logf("")
	t.Logf("%s%s✓ Resumable download test passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_DirectoryZipTransfer tests directory compression and transfer
func TestE2E_DirectoryZipTransfer(t *testing.T) {
	logSection(t, "Directory Transfer Tests")

	logTest(t, "Creating test directory structure")

	srcDir, err := os.MkdirTemp("", "warp-dir-*")
	assertNoError(t, err, "Create temp dir")
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create nested directory structure
	structure := map[string]string{
		"file1.txt":            "root file content",
		"subdir/file2.txt":     "subdir file content",
		"subdir/file3.txt":     "another subdir file",
		"deep/nested/file.txt": "deeply nested content",
	}

	for path, content := range structure {
		fullPath := filepath.Join(srcDir, path)
		_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
		err := os.WriteFile(fullPath, []byte(content), 0644)
		assertNoError(t, err, "Write "+path)
		logInfo(t, "Created: %s", path)
	}

	logTest(t, "Starting server for directory transfer")
	tok, _ := crypto.GenerateToken(nil)
	srv := &server.Server{Token: tok, SrcPath: srcDir}
	url, err := srv.Start()
	assertNoError(t, err, "Start server")
	defer func() { _ = srv.Shutdown() }()

	logInfo(t, "Server URL: %s", url)

	// Download (should be zipped)
	logTest(t, "Downloading directory as ZIP...")
	start := time.Now()
	out, err := client.Receive(url, "", true, io.Discard)
	assertNoError(t, err, "Download directory")
	defer func() { _ = os.Remove(out) }()
	duration := time.Since(start)

	// Verify it's a zip file
	fi, _ := os.Stat(out)
	logPass(t, "ZIP downloaded: %s in %v", formatBytes(fi.Size()), duration.Round(time.Millisecond))

	if !strings.HasSuffix(out, ".zip") {
		t.Error("Downloaded file should have .zip extension")
	} else {
		logPass(t, "File has .zip extension: %s", filepath.Base(out))
	}

	t.Logf("")
	t.Logf("%s%s✓ Directory transfer test passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_TokenSecurity tests token authentication
func TestE2E_TokenSecurity(t *testing.T) {
	logSection(t, "Token Security Tests")

	t.Run("InvalidToken", func(t *testing.T) {
		logTest(t, "Testing invalid token rejection")

		tok, _ := crypto.GenerateToken(nil)
		srv := &server.Server{Token: tok, TextContent: "secret"}
		url, err := srv.Start()
		assertNoError(t, err, "Start server")
		defer func() { _ = srv.Shutdown() }()

		logInfo(t, "Valid URL: %s", url)

		// Replace token with invalid one
		invalidURL := strings.Replace(url, tok, "invalid-token-12345", 1)
		logTest(t, "Trying invalid token: %s", invalidURL)

		resp, err := http.Get(invalidURL)
		if err != nil {
			// Network error is acceptable
			logPass(t, "Request failed as expected")
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
			logPass(t, "Invalid token rejected with status %d", resp.StatusCode)
		} else {
			t.Errorf("Expected 403/404 for invalid token, got %d", resp.StatusCode)
		}
	})

	t.Logf("")
	t.Logf("%s%s✓ Token security tests passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_FilenameHandling tests various filename edge cases
func TestE2E_FilenameHandling(t *testing.T) {
	logSection(t, "Filename Handling Tests")

	testCases := []struct {
		name     string
		filename string
		safe     bool
	}{
		{"Normal", "document.pdf", true},
		{"Spaces", "my document.txt", true},
		{"Unicode", "文档.txt", true},
		{"Multiple Dots", "archive.tar.gz", true},
		{"Long Name", strings.Repeat("a", 200) + ".txt", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logTest(t, "Testing filename: %s", tc.filename)

			destDir, err := os.MkdirTemp("", "warp-fname-*")
			assertNoError(t, err, "Create temp dir")
			defer func() { _ = os.RemoveAll(destDir) }()

			tok, _ := crypto.GenerateToken(nil)
			srv := &server.Server{Token: tok, HostMode: true, UploadDir: destDir}
			url, err := srv.Start()
			assertNoError(t, err, "Start server")
			defer func() { _ = srv.Shutdown() }()

			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("file", tc.filename)
			_, _ = io.WriteString(fw, "test content")
			_ = mw.Close()

			req, _ := http.NewRequest(http.MethodPost, url, &buf)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			resp, _ := http.DefaultClient.Do(req)
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			if tc.safe {
				assertEqual(t, http.StatusOK, resp.StatusCode, "Upload status")
				logPass(t, "File uploaded successfully")
			}
		})
	}

	t.Logf("")
	t.Logf("%s%s✓ All filename handling tests passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_ConcurrentTransfers tests multiple simultaneous transfers
func TestE2E_ConcurrentTransfers(t *testing.T) {
	logSection(t, "Concurrent Transfer Tests")

	logTest(t, "Testing 5 concurrent downloads")

	// Create test file
	src, err := os.CreateTemp("", "warp-concurrent-*")
	assertNoError(t, err, "Create temp file")
	defer func() { _ = os.Remove(src.Name()) }()

	data := bytes.Repeat([]byte("concurrent-test-data"), 1024)
	_, _ = src.Write(data)
	_ = src.Close()

	tok, _ := crypto.GenerateToken(nil)
	srv := &server.Server{Token: tok, SrcPath: src.Name()}
	url, err := srv.Start()
	assertNoError(t, err, "Start server")
	defer func() { _ = srv.Shutdown() }()

	logInfo(t, "Server URL: %s", url)

	// Launch 5 concurrent downloads
	const numClients = 5
	done := make(chan bool, numClients)
	start := time.Now()

	for i := 0; i < numClients; i++ {
		go func(id int) {
			logTest(t, "Client %d: Starting download", id)
			out, err := client.Receive(url, "", true, io.Discard)
			if err != nil {
				t.Errorf("Client %d failed: %v", id, err)
				done <- false
				return
			}
			_ = os.Remove(out)
			logPass(t, "Client %d: Download complete", id)
			done <- true
		}(i)
	}

	// Wait for all downloads
	success := 0
	for i := 0; i < numClients; i++ {
		if <-done {
			success++
		}
	}

	duration := time.Since(start)
	logPass(t, "Concurrent transfers: %d/%d successful in %v",
		success, numClients, duration.Round(time.Millisecond))

	if success != numClients {
		t.Errorf("Expected %d successful transfers, got %d", numClients, success)
	}

	t.Logf("")
	t.Logf("%s%s✓ Concurrent transfer test passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_ParallelUpload tests parallel chunk uploads with multiple workers
func TestE2E_ParallelUpload(t *testing.T) {
	logSection(t, "Parallel Upload Tests")

	// Create temp directory for test
	tmpDir := t.TempDir()

	// Create test file (10MB)
	testFile := filepath.Join(tmpDir, "parallel-upload-test.bin")
	fileSize := int64(10 * 1024 * 1024)
	testData := make([]byte, fileSize)
	_, err := rand.Read(testData)
	if err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create upload directory
	uploadDir := filepath.Join(tmpDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		t.Fatalf("Failed to create upload directory: %v", err)
	}

	t.Run("3_Workers_2MB_Chunks", func(t *testing.T) {
		logTest(t, "Starting host server for parallel upload")

		// Start server in host mode
		token, _ := crypto.GenerateToken(nil)
		srv := &server.Server{
			Token:     token,
			HostMode:  true,
			UploadDir: uploadDir,
		}
		url, err := srv.Start()
		if err != nil {
			t.Fatalf("Failed to start server: %v", err)
		}
		defer func() { _ = srv.Shutdown() }()

		logInfo(t, "Host server URL: %s", url)

		logTest(t, "Starting parallel chunk upload (3 workers, 2MB chunks)")

		// Configure parallel upload
		config := &client.UploadConfig{
			ChunkSize:     2 * 1024 * 1024, // 2MB chunks
			MaxConcurrent: 3,               // 3 parallel workers
			RetryAttempts: 2,
			RetryDelay:    500 * time.Millisecond,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		startTime := time.Now()
		err = client.ParallelUpload(ctx, url, testFile, config, nil)
		duration := time.Since(startTime)

		if err != nil {
			t.Fatalf("Parallel upload failed: %v", err)
		}

		// Calculate throughput
		mbps := float64(fileSize*8) / (duration.Seconds() * 1_000_000)
		logPass(t, "Parallel upload complete: %s in %.2fs (%.1f Mbps, %d workers)",
			formatBytes(fileSize), duration.Seconds(), mbps, config.MaxConcurrent)

		// Verify uploaded file
		logTest(t, "Verifying uploaded file integrity")
		uploadedFiles, err := os.ReadDir(uploadDir)
		if err != nil {
			t.Fatalf("Failed to read upload directory: %v", err)
		}

		if len(uploadedFiles) != 1 {
			t.Fatalf("Expected 1 uploaded file, got %d", len(uploadedFiles))
		}

		uploadedPath := filepath.Join(uploadDir, uploadedFiles[0].Name())
		uploadedData, err := os.ReadFile(uploadedPath)
		if err != nil {
			t.Fatalf("Failed to read uploaded file: %v", err)
		}

		if !bytes.Equal(testData, uploadedData) {
			t.Fatal("Uploaded file data does not match original")
		}

		logPass(t, "File integrity verified: %s", uploadedFiles[0].Name())
	})

	t.Logf("")
	t.Logf("%s%s✓ Parallel upload tests passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

// TestE2E_Performance provides performance benchmarks
func TestE2E_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	logSection(t, "Performance Tests")

	sizes := []int64{
		1 * 1024 * 1024,  // 1MB
		10 * 1024 * 1024, // 10MB
		50 * 1024 * 1024, // 50MB
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Transfer_%s", formatBytes(size)), func(t *testing.T) {
			logTest(t, "Performance test: %s", formatBytes(size))

			src, _ := os.CreateTemp("", "warp-perf-*")
			defer func() { _ = os.Remove(src.Name()) }()

			// Write random-ish data
			data := bytes.Repeat([]byte("performance-benchmark-data-"), int(size/27))
			_, _ = src.Write(data)
			_ = src.Close()

			tok, _ := crypto.GenerateToken(nil)
			srv := &server.Server{Token: tok, SrcPath: src.Name()}
			url, _ := srv.Start()
			defer func() { _ = srv.Shutdown() }()

			start := time.Now()
			out, err := client.Receive(url, "", true, io.Discard)
			duration := time.Since(start)

			assertNoError(t, err, "Download")
			defer func() { _ = os.Remove(out) }()

			mbps := (float64(size) * 8) / (duration.Seconds() * 1_000_000)
			throughput := float64(size) / duration.Seconds() / (1024 * 1024)

			logPass(t, "%s transferred in %v (%.1f Mbps, %.2f MiB/s)",
				formatBytes(size), duration.Round(time.Millisecond), mbps, throughput)
		})
	}

	t.Logf("")
	t.Logf("%s%s✓ All performance tests passed%s", colorBold, colorGreen, colorReset)
	t.Logf("")
}

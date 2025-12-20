package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/protocol"
	"github.com/zulfikawr/warp/internal/ui"
)

func TestServerValidAndInvalidToken(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "warp-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_, _ = tmpFile.Write([]byte("hello"))
	_ = tmpFile.Close()

	tok, _ := crypto.GenerateToken(nil)
	s := &Server{Token: tok, SrcPath: tmpFile.Name()}
	url, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown() }()

	// Valid token
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Invalid token
	resp2, err := http.Get(url + "x")
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp2.StatusCode)
	}
	_ = resp2.Body.Close()
}

func TestServerHealthEndpoint(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "warp-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	tok, _ := crypto.GenerateToken(nil)
	s := &Server{Token: tok, SrcPath: tmpFile.Name()}
	url, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown() }()

	// Extract base URL
	baseURL := url[:len(url)-len(tok)-3] // Remove /d/{token}

	// Test health endpoint
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}
}

func TestServerMetricsEndpoint(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "warp-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	tok, _ := crypto.GenerateToken(nil)
	s := &Server{Token: tok, SrcPath: tmpFile.Name()}
	url, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown() }()

	// Extract base URL
	baseURL := url[:len(url)-len(tok)-3]

	// Test metrics endpoint
	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", resp.StatusCode)
	}

	// Verify it contains Prometheus metrics
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if len(bodyStr) < 100 {
		t.Error("Metrics response too short")
	}
}

func TestHostMode(t *testing.T) {
	tmpDir := t.TempDir()

	tok, _ := crypto.GenerateToken(nil)
	s := &Server{
		Token:     tok,
		HostMode:  true,
		UploadDir: tmpDir,
	}

	url, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown() }()

	// Test GET returns HTML form
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) < 100 {
		t.Error("HTML response too short")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := ui.FormatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("ui.FormatBytes(%d) = %s, want %s", tt.bytes, got, tt.want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input     string
		wantError bool
	}{
		{"valid.txt", false},
		{"file-name_123.pdf", false},
		{"", true},
		{".", true},
		{"..", true},
		{"file\x00.txt", true}, // null byte
		{"file\n.txt", true},   // newline
	}

	for _, tt := range tests {
		result, err := sanitizeFilename(tt.input)
		if tt.wantError {
			if err == nil {
				t.Errorf("sanitizeFilename(%q) expected error, got nil (result: %s)", tt.input, result)
			}
		} else {
			if err != nil {
				t.Errorf("sanitizeFilename(%q) unexpected error: %v", tt.input, err)
			}
			if result == "" {
				t.Errorf("sanitizeFilename(%q) returned empty string", tt.input)
			}
		}
	}
}

func TestFindUniqueFilename(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file
	existingFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(existingFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	// First call should return the original name with (1) suffix
	unique := findUniqueFilename(tmpDir, "test.txt")
	if unique == existingFile {
		t.Error("Expected unique filename, got same as existing")
	}

	// Should contain " (1)" before extension
	expected := filepath.Join(tmpDir, "test (1).txt")
	if unique != expected {
		t.Errorf("Expected %s, got %s", expected, unique)
	}
}

func TestGetOptimalBufferSize(t *testing.T) {
	tests := []struct {
		fileSize int64
		wantSize int
	}{
		{1000, protocol.BufferSizeSmall},                  // Small file
		{100000, protocol.BufferSizeMedium},               // Medium file
		{10 * 1024 * 1024, protocol.BufferSizeLarge},      // Large file
		{500 * 1024 * 1024, protocol.BufferSizeVeryLarge}, // Very large file
	}

	for _, tt := range tests {
		got := protocol.GetOptimalBufferSize(tt.fileSize)
		if got != tt.wantSize {
			t.Errorf("protocol.GetOptimalBufferSize(%d) = %d, want %d", tt.fileSize, got, tt.wantSize)
		}
	}
}

func TestIsCompressible(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"file.txt", true},
		{"file.json", true},
		{"file.html", true},
		{"file.xml", true},
		{"file.csv", true},
		{"file.jpg", false},
		{"file.png", false},
		{"file.mp4", false},
		{"file.zip", false},
	}

	for _, tt := range tests {
		got := isCompressible(tt.path)
		if got != tt.want {
			t.Errorf("isCompressible(%s) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestChunkStat(t *testing.T) {
	cs := &chunkStat{}

	d1 := cs.add(100 * time.Millisecond)
	if d1 != 100*time.Millisecond {
		t.Errorf("First add returned %v, want 100ms", d1)
	}

	d2 := cs.add(50 * time.Millisecond)
	if d2 != 150*time.Millisecond {
		t.Errorf("Second add returned %v, want 150ms", d2)
	}
}

package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/ui"
)

// Downloader handles file downloads with configurable HTTP client
type Downloader struct {
	client *http.Client
}

// NewDownloader creates a new Downloader with the given HTTP client
// If client is nil, uses the default client with optimized settings
func NewDownloader(client *http.Client) *Downloader {
	if client == nil {
		client = defaultHTTPClient()
	}
	return &Downloader{client: client}
}

// defaultHTTPClient returns an HTTP client with optimized connection pooling and HTTP/2 support
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
			ForceAttemptHTTP2:     true,
			WriteBufferSize:       256 * 1024,
			ReadBufferSize:        256 * 1024,
			DisableCompression:    false, // Enable gzip compression
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
		Timeout: 5 * time.Minute,
	}
}

// getOptimalBufferSize determines the best buffer size based on file size
func getOptimalBufferSize(fileSize int64) int {
	switch {
	case fileSize < 64*1024: // < 64KB
		return 8192 // 8KB
	case fileSize < 1024*1024: // < 1MB
		return 65536 // 64KB
	case fileSize < 100*1024*1024: // < 100MB
		return 1048576 // 1MB
	default: // >= 100MB
		return 4194304 // 4MB
	}
}

// Receive downloads from url to outputPath. If outputPath is empty, derive from headers or URL.
// For text content (Content-Type: text/plain), outputs to stdout instead of saving to a file.
// Supports resumable downloads via HTTP Range headers if the file already partially exists.
func (d *Downloader) Receive(url string, outputPath string, force bool, progress io.Writer) (string, error) {
	// First, make a HEAD request or GET to determine filename and check for existing partial file
	var startByte int64 = 0

	// Try initial request to get headers
	resp, err := d.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w\n\nPossible solutions:\n  • Check if the server is running\n  • Verify the URL is correct\n  • Make sure you're on the same network\n  • Try: warp search (to find available servers)", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		if resp.StatusCode == 404 {
			return "", fmt.Errorf("file not found (HTTP 404)\n\nPossible solutions:\n  • The file may have expired\n  • Check if the URL is correct\n  • Try: warp search (to find available servers)")
		}
		return "", fmt.Errorf("server returned error: HTTP %d\n\nTip: Check if the server is still running", resp.StatusCode)
	}

	// Check if this is text content (text/plain without attachment disposition)
	contentType := resp.Header.Get("Content-Type")
	disposition := resp.Header.Get("Content-Disposition")
	isTextContent := strings.HasPrefix(contentType, "text/plain") && disposition == ""

	if isTextContent {
		// Output text to stdout
		_, err := io.Copy(os.Stdout, resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}
		return "(stdout)", nil
	}

	name := filenameFromResponse(resp)
	if name == "" {
		name = path.Base(resp.Request.URL.Path)
		if name == "" {
			name = "download.bin"
		}
	}
	if outputPath == "" {
		outputPath = name
	}

	totalSize := resp.ContentLength
	_ = resp.Body.Close()

	// Display download header
	if progress != nil {
		sizeStr := formatSize(totalSize)
		_, _ = fmt.Fprintf(progress, "Downloading: %s (%s)\n", name, sizeStr)
	}

	// Check if file already exists and can be resumed
	var f *os.File
	if fi, err := os.Stat(outputPath); err == nil {
		if !force && fi.Size() > 0 && fi.Size() < totalSize {
			// File exists and is incomplete - try to resume
			startByte = fi.Size()
			f, err = os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				return "", err
			}
		} else if !force {
			return "", fmt.Errorf("⚠️  File '%s' already exists\n\nUse --force or -f to overwrite", outputPath)
		} else {
			// Force overwrite
			f, err = os.Create(outputPath)
			if err != nil {
				return "", err
			}
		}
	} else {
		// File doesn't exist - create new
		f, err = os.Create(outputPath)
		if err != nil {
			return "", err
		}
	}
	defer func() { _ = f.Close() }()

	// Make the actual download request with Range header if resuming
	var downloadResp *http.Response
	if startByte > 0 {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
		downloadResp, err = d.client.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = downloadResp.Body.Close() }()

		if downloadResp.StatusCode != http.StatusPartialContent {
			// Server doesn't support resume, start over
			_ = f.Close()
			f, err = os.Create(outputPath)
			if err != nil {
				return "", err
			}
			defer func() { _ = f.Close() }()
			startByte = 0
			_ = downloadResp.Body.Close()
			downloadResp, err = d.client.Get(url)
			if err != nil {
				return "", err
			}
			defer func() { _ = downloadResp.Body.Close() }()
		}
	} else {
		downloadResp, err = d.client.Get(url)
		if err != nil {
			return "", err
		}
		defer func() { _ = downloadResp.Body.Close() }()
	}

	var src io.Reader = downloadResp.Body
	var startTime time.Time
	if progress != nil {
		// Use the improved progress reader with ETA calculation
		startTime = time.Now()
		src = &ui.ProgressReader{
			R:         downloadResp.Body,
			Total:     totalSize,
			Current:   startByte,
			Out:       progress,
			StartTime: startTime,
		}
	}

	// Use adaptive buffer sizing based on file size
	bufferSize := getOptimalBufferSize(totalSize)
	buf := make([]byte, bufferSize)

	// Compute checksum while downloading
	hash := sha256.New()
	teeReader := io.TeeReader(src, hash)

	if _, err := io.CopyBuffer(f, teeReader, buf); err != nil {
		return "", err
	}

	// Print completion message
	if progress != nil {
		_, _ = fmt.Fprintf(progress, "\n✓ Download complete\n")
	}

	// Verify checksum if server provided one
	expectedChecksum := downloadResp.Header.Get("X-Content-SHA256")
	if expectedChecksum != "" {
		actualChecksum := hex.EncodeToString(hash.Sum(nil))
		if actualChecksum != expectedChecksum {
			metrics.ChecksumVerifications.WithLabelValues("mismatch").Inc()
			_ = os.Remove(outputPath) // Delete corrupted file
			return "", fmt.Errorf("checksum verification failed: expected %s, got %s", expectedChecksum[:16]+"...", actualChecksum[:16]+"...")
		}
		metrics.ChecksumVerifications.WithLabelValues("match").Inc()
		if progress != nil {
			_, _ = fmt.Fprintf(progress, "✓ Checksum verified\n")
		}
	}

	// Print saved location
	if progress != nil {
		_, _ = fmt.Fprintf(progress, "\n\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\n")
		_, _ = fmt.Fprintf(progress, "\u2713 Transfer Complete\n\n")
		_, _ = fmt.Fprintf(progress, "Summary:\n")
		_, _ = fmt.Fprintf(progress, "  File:         %s\n", outputPath)
		_, _ = fmt.Fprintf(progress, "  Size:         %s\n", formatSize(totalSize))

		// Calculate transfer stats
		if !startTime.IsZero() {
			elapsed := time.Since(startTime)
			if elapsed.Seconds() > 0 {
				avgSpeed := float64(totalSize) / elapsed.Seconds()
				speedStr := formatSpeed(avgSpeed)
				_, _ = fmt.Fprintf(progress, "  Time:         %.1fs\n", elapsed.Seconds())
				_, _ = fmt.Fprintf(progress, "  Avg Speed:    %s\n", speedStr)
			}
		}

		_, _ = fmt.Fprintf(progress, "  Saved to:     %s\n", outputPath)
		if expectedChecksum != "" {
			_, _ = fmt.Fprintf(progress, "  Checksum:     Verified\n")
		}
		_, _ = fmt.Fprintf(progress, "\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\u2501\n")
	}

	return outputPath, nil
}

// formatSpeed formats bytes per second into a human-readable string
func formatSpeed(bytesPerSec float64) string {
	const unit = 1024
	if bytesPerSec < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}

	div := float64(unit)
	exp := 0
	units := []string{"KB/s", "MB/s", "GB/s", "TB/s"}

	for bytesPerSec >= div*unit && exp < len(units)-1 {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", bytesPerSec/div, units[exp])
}

// Package-level Receive function for backward compatibility
// Uses default HTTP client with optimized settings
var defaultDownloader = NewDownloader(nil)

// Receive downloads from url using the default HTTP client
// This is a convenience function that wraps Downloader.Receive
func Receive(url string, outputPath string, force bool, progress io.Writer) (string, error) {
	return defaultDownloader.Receive(url, outputPath, force, progress)
}

// filenameFromResponse extracts filename from Content-Disposition
func filenameFromResponse(resp *http.Response) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd == "" {
		return ""
	}
	// simplistic parsing: attachment; filename="name"
	parts := strings.Split(cd, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(strings.ToLower(p), "filename=") {
			v := strings.TrimPrefix(p, "filename=")
			v = strings.Trim(v, "\"")
			return v
		}
	}
	return ""
}

// formatSize formats bytes into a human-readable string with appropriate units
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div := int64(unit)
	exp := 0
	units := []string{"KB", "MB", "GB", "TB"}

	for bytes >= div*unit && exp < len(units)-1 {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

package client

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/protocol"
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

// readCloserAdapter adapts an io.Reader and a close func to an io.ReadCloser
type readCloserAdapter struct {
	r io.Reader
	c func()
}

func (a *readCloserAdapter) Read(p []byte) (int, error) { return a.r.Read(p) }
func (a *readCloserAdapter) Close() error {
	if a.c != nil {
		a.c()
		return nil
	}
	return nil
}

// Receive downloads from url to outputPath. If outputPath is empty, derive from headers or URL.
// For text content (Content-Type: text/plain), outputs to stdout instead of saving to a file.
// Supports resumable downloads via HTTP Range headers if the file already partially exists.
func (d *Downloader) Receive(url string, outputPath string, force bool, progress io.Writer, key []byte) (string, error) {
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

	// Handle Content-Encoding (zstd/gzip) before decryption
	var bodyReader io.ReadCloser = resp.Body
	enc := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if enc == "zstd" {
		zr, err := zstd.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return "", fmt.Errorf("failed to create zstd reader: %w", err)
		}
		// zstd.Decoder Close() signature doesn't match io.ReadCloser, adapt it
		bodyReader = &readCloserAdapter{r: zr, c: zr.Close}
	} else if enc == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return "", fmt.Errorf("failed to create gzip reader: %w", err)
		}
		bodyReader = gr
	}

	if key != nil {
		dr, err := crypto.NewDecryptReader(bodyReader, key)
		if err != nil {
			_ = bodyReader.Close()
			return "", fmt.Errorf("failed to create decrypt reader: %w", err)
		}
		bodyReader = io.NopCloser(dr)
	}

	// Check if this is text content (text/plain without attachment disposition)
	contentType := resp.Header.Get("Content-Type")
	disposition := resp.Header.Get("Content-Disposition")
	isTextContent := strings.HasPrefix(contentType, "text/plain") && disposition == ""

	if isTextContent {
		// Output text to stdout
		_, err := io.Copy(os.Stdout, bodyReader)
		_ = bodyReader.Close()
		if err != nil {
			return "", fmt.Errorf("failed to output text to stdout: %w", err)
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
				return "", fmt.Errorf("failed to open file for resume: %w", err)
			}
		} else if !force {
			return "", fmt.Errorf("%s⚠️  File '%s' already exists%s\n\nUse --force or -f to overwrite", ui.Colors.Yellow, outputPath, ui.Colors.Reset)
		} else {
			// Force overwrite
			f, err = os.Create(outputPath)
			if err != nil {
				return "", fmt.Errorf("failed to create file: %w", err)
			}
		}
	} else {
		// File doesn't exist - create new
		f, err = os.Create(outputPath)
		if err != nil {
			return "", fmt.Errorf("failed to create file: %w", err)
		}
	}
	defer func() { _ = f.Close() }()

	// Make the actual download request with Range header if resuming
	var downloadResp *http.Response
	if startByte > 0 {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
		downloadResp, err = d.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to execute resume request: %w", err)
		}

		if downloadResp.StatusCode != http.StatusPartialContent {
			// Server doesn't support resume, start over
			_ = f.Close()
			f, err = os.Create(outputPath)
			if err != nil {
				_ = downloadResp.Body.Close()
				return "", fmt.Errorf("failed to recreate file: %w", err)
			}
			defer func() { _ = f.Close() }()
			startByte = 0
			_ = downloadResp.Body.Close()
			downloadResp, err = d.client.Get(url)
			if err != nil {
				return "", fmt.Errorf("failed to restart download: %w", err)
			}
		}
	} else {
		downloadResp, err = d.client.Get(url)
		if err != nil {
			return "", fmt.Errorf("failed to start download: %w", err)
		}
	}
	defer func() { _ = downloadResp.Body.Close() }()

	var src io.Reader = downloadResp.Body
	if key != nil {
		dr, err := crypto.NewDecryptReader(downloadResp.Body, key)
		if err != nil {
			return "", fmt.Errorf("failed to create decrypt reader: %w", err)
		}
		src = dr
	}

	var startTime time.Time
	if progress != nil {
		// Use the improved progress reader with ETA calculation
		startTime = time.Now()
		src = &ui.ProgressReader{
			R:         src,
			Total:     totalSize,
			Current:   startByte,
			Out:       progress,
			StartTime: startTime,
		}
	}

	// Use adaptive buffer sizing based on file size
	bufferSize := protocol.GetOptimalBufferSize(totalSize)
	buf := make([]byte, bufferSize)

	// Compute checksum while downloading
	hash := sha256.New()
	teeReader := io.TeeReader(src, hash)

	if _, err := io.CopyBuffer(f, teeReader, buf); err != nil {
		return "", fmt.Errorf("failed to write file data: %w", err)
	}

	// Print completion message
	if progress != nil {
		_, _ = fmt.Fprintf(progress, "\n%s✓ Download complete%s\n", ui.Colors.Green, ui.Colors.Reset)
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
			_, _ = fmt.Fprintf(progress, "%s✓ Checksum verified%s\n", ui.Colors.Green, ui.Colors.Reset)
		}
	}

	// Print saved location
	if progress != nil {
		_, _ = fmt.Fprintf(progress, "\n%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ui.Colors.Dim, ui.Colors.Reset)
		_, _ = fmt.Fprintf(progress, "%s✓ Transfer Complete%s\n\n", ui.Colors.Green, ui.Colors.Reset)
		_, _ = fmt.Fprintf(progress, "%sSummary:%s\n", ui.Colors.Dim, ui.Colors.Reset)
		_, _ = fmt.Fprintf(progress, "  File:         %s\n", outputPath)
		_, _ = fmt.Fprintf(progress, "  Size:         %s\n", formatSize(totalSize))

		// Calculate transfer stats
		if !startTime.IsZero() {
			elapsed := time.Since(startTime)
			if elapsed.Seconds() > 0 {
				avgSpeed := float64(totalSize) / elapsed.Seconds()
				speedStr := ui.FormatSpeed(avgSpeed)
				_, _ = fmt.Fprintf(progress, "  Time:         %.1fs\n", elapsed.Seconds())
				_, _ = fmt.Fprintf(progress, "  Avg Speed:    %s\n", speedStr)
			}
		}

		_, _ = fmt.Fprintf(progress, "  Saved to:     %s\n", outputPath)
		if expectedChecksum != "" {
			_, _ = fmt.Fprintf(progress, "  Checksum:     %sVerified%s\n", ui.Colors.Green, ui.Colors.Reset)
		}
		_, _ = fmt.Fprintf(progress, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", ui.Colors.Dim, ui.Colors.Reset)
	}

	return outputPath, nil
}

// Package-level Receive function for backward compatibility
// Uses default HTTP client with optimized settings
var defaultDownloader = NewDownloader(nil)

// Receive downloads from url using the default HTTP client
// This is a convenience function that wraps Downloader.Receive
func Receive(url string, outputPath string, force bool, progress io.Writer, key []byte) (string, error) {
	return defaultDownloader.Receive(url, outputPath, force, progress, key)
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

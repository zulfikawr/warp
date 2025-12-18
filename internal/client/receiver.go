package client

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// httpClient with optimized connection pooling and HTTP/2 support
var httpClient = &http.Client{
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
func Receive(url string, outputPath string, force bool, progress io.Writer) (string, error) {
	// First, make a HEAD request or GET to determine filename and check for existing partial file
	var startByte int64 = 0
	var existingSize int64 = 0
	
	// Try initial request to get headers
	resp, err := httpClient.Get(url)
	if err != nil { return "", err }
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Check if this is text content (text/plain without attachment disposition)
	contentType := resp.Header.Get("Content-Type")
	disposition := resp.Header.Get("Content-Disposition")
	isTextContent := strings.HasPrefix(contentType, "text/plain") && disposition == ""

	if isTextContent {
		// Output text to stdout
		_, err := io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
		if err != nil { return "", err }
		return "(stdout)", nil
	}

	name := filenameFromResponse(resp)
	if name == "" {
		name = path.Base(resp.Request.URL.Path)
		if name == "" { name = "download.bin" }
	}
	if outputPath == "" {
		outputPath = name
	}
	
	totalSize := resp.ContentLength
	resp.Body.Close()
	
	// Display download header
	if progress != nil {
		fmt.Fprintf(progress, "Downloading: %s (%.1f MB)\n", name, float64(totalSize)/(1024*1024))
	}
	
	// Check if file already exists and can be resumed
	var f *os.File
	if fi, err := os.Stat(outputPath); err == nil {
		existingSize = fi.Size()
		if !force && existingSize > 0 && existingSize < totalSize {
			// File exists and is incomplete - try to resume
			startByte = existingSize
			f, err = os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil { return "", err }
		} else if !force {
			return "", errors.New("destination exists; use --force to overwrite")
		} else {
			// Force overwrite
			f, err = os.Create(outputPath)
			if err != nil { return "", err }
		}
	} else {
		// File doesn't exist - create new
		f, err = os.Create(outputPath)
		if err != nil { return "", err }
	}
	defer f.Close()
	
	// Make the actual download request with Range header if resuming
	var downloadResp *http.Response
	if startByte > 0 {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil { return "", err }
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
		downloadResp, err = httpClient.Do(req)
		if err != nil { return "", err }
		defer downloadResp.Body.Close()
		
		if downloadResp.StatusCode != http.StatusPartialContent {
			// Server doesn't support resume, start over
			f.Close()
			f, err = os.Create(outputPath)
			if err != nil { return "", err }
			defer f.Close()
			startByte = 0
			downloadResp.Body.Close()
			downloadResp, err = httpClient.Get(url)
			if err != nil { return "", err }
			defer downloadResp.Body.Close()
		}
	} else {
		downloadResp, err = httpClient.Get(url)
		if err != nil { return "", err }
		defer downloadResp.Body.Close()
	}
	
	var src io.Reader = downloadResp.Body
	if progress != nil {
		// Use the improved progress reader with ETA calculation
		src = &ui.ProgressReader{
			R:         downloadResp.Body,
			Total:     totalSize,
			Current:   startByte,
			Out:       progress,
			StartTime: time.Now(),
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
		fmt.Fprintf(progress, "\n✓ Download complete\n")
	}
	
	// Verify checksum if server provided one
	expectedChecksum := downloadResp.Header.Get("X-Content-SHA256")
	if expectedChecksum != "" {
		actualChecksum := hex.EncodeToString(hash.Sum(nil))
		if actualChecksum != expectedChecksum {
			metrics.ChecksumVerifications.WithLabelValues("mismatch").Inc()
			os.Remove(outputPath) // Delete corrupted file
			return "", fmt.Errorf("checksum verification failed: expected %s, got %s", expectedChecksum[:16]+"...", actualChecksum[:16]+"...")
		}
		metrics.ChecksumVerifications.WithLabelValues("match").Inc()
		if progress != nil {
			fmt.Fprintf(progress, "✓ Checksum verified\n")
		}
	}
	
	// Print saved location
	if progress != nil {
		fmt.Fprintf(progress, "Saved to: %s\n", outputPath)
	}
	
	return outputPath, nil
}

func filenameFromResponse(resp *http.Response) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd == "" { return "" }
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

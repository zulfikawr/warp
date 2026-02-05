package client

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/resume"
)

// Downloader handles file downloads with configurable HTTP client
type Downloader struct {
	client       *http.Client
	stateManager *resume.TransferStateManager
	checkpoint   *resume.Checkpoint

	// Pause support
	pauseChan  chan struct{}
	resumeChan chan struct{}
	isPaused   bool
	pauseMu    sync.Mutex
}

// NewDownloader creates a new Downloader with the given HTTP client
// If client is nil, uses the default client with optimized settings
// stateManager enables checkpoint-based resumable downloads
// checkpoint is optional - if provided, download will resume from checkpoint
func NewDownloader(client *http.Client, stateManager *resume.TransferStateManager, checkpoint *resume.Checkpoint) *Downloader {
	if client == nil {
		client = defaultHTTPClient()
	}
	return &Downloader{
		client:       client,
		stateManager: stateManager,
		checkpoint:   checkpoint,
		pauseChan:    make(chan struct{}, 1),
		resumeChan:   make(chan struct{}, 1),
		isPaused:     false,
	}
}

// Pause pauses the download
func (d *Downloader) Pause() {
	d.pauseMu.Lock()
	d.isPaused = true
	d.pauseMu.Unlock()

	select {
	case d.pauseChan <- struct{}{}:
	default:
	}
}

// Resume resumes a paused download
func (d *Downloader) Resume() {
	d.pauseMu.Lock()
	d.isPaused = false
	d.pauseMu.Unlock()

	select {
	case d.resumeChan <- struct{}{}:
	default:
	}
}

// IsPaused returns whether the download is paused
func (d *Downloader) IsPaused() bool {
	d.pauseMu.Lock()
	defer d.pauseMu.Unlock()
	return d.isPaused
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

// callbackProgressReader tracks progress and emits events via callback
type callbackProgressReader struct {
	r          io.Reader
	total      int64
	current    int64
	startTime  time.Time
	onProgress func(progress.Progress)
	lastEmit   time.Time
	filepath   string
}

func (p *callbackProgressReader) Read(b []byte) (int, error) {
	if p.startTime.IsZero() {
		p.startTime = time.Now()
	}

	n, err := p.r.Read(b)
	p.current += int64(n)

	// Emit progress update every 100ms
	if p.onProgress != nil && (time.Since(p.lastEmit) > 100*time.Millisecond || err != nil || p.current == p.total) {
		elapsed := time.Since(p.startTime)
		speed := float64(0)
		var eta time.Duration

		if elapsed.Seconds() > 0 {
			speed = float64(p.current*8) / (elapsed.Seconds() * 1_000_000) // Mbps
		}

		if speed > 0 && p.total > 0 {
			// speed is Mbps. (speed * 1_000_000 / 8) is bytes/sec
			bytesPerSec := (speed * 1_000_000) / 8
			if bytesPerSec > 0 {
				remaining := p.total - p.current
				etaSeconds := float64(remaining) / bytesPerSec
				eta = time.Duration(etaSeconds * float64(time.Second))
			}
		}

		p.onProgress(progress.Progress{
			TotalBytes:       p.total,
			TransferredBytes: p.current,
			SpeedBytesPerSec: (speed * 1_000_000) / 8, // Convert Mbps to bytes/sec
			StartTime:        p.startTime,
			ETA:              eta,
			IsComplete:       p.current == p.total,
			FileName:         p.filepath,
			Direction:        "download",
			LastUpdate:       time.Now(),
		})
		p.lastEmit = time.Now()
	}

	return n, err
}

// Receive downloads from url to outputPath. If outputPath is empty, derive from headers or URL.
// For text content (Content-Type: text/plain), outputs to stdoutWriter instead of saving to a file.
// If stdoutWriter is nil and content is text, it returns an error unless outputPath is specified.
// Supports checkpoint-based resumable downloads if StateManager is configured.
func (d *Downloader) Receive(ctx context.Context, url string, outputPath string, force bool, onProgress func(progress.Progress), stdoutWriter io.Writer, key []byte) (string, error) {
	// Check if resuming from checkpoint
	var startByte int64 = 0
	var sessionID string

	if d.checkpoint != nil {
		// Resuming from checkpoint - calculate bytes transferred from completed chunks
		startByte = int64(len(d.checkpoint.CompletedChunks)) * d.checkpoint.ChunkSize
		sessionID = d.checkpoint.SessionID
		if outputPath == "" {
			outputPath = d.checkpoint.DestinationPath
		}
	}

	// Try initial request to get headers
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		if resp.StatusCode == 404 {
			return "", fmt.Errorf("file not found (HTTP 404)")
		}
		return "", fmt.Errorf("server returned error: HTTP %d", resp.StatusCode)
	}

	// Handle Content-Encoding (zstd/gzip) before decryption
	var bodyReader io.ReadCloser = resp.Body
	enc := strings.ToLower(resp.Header.Get("Content-Encoding"))
	switch enc {
	case "zstd":
		zr, err := zstd.NewReader(resp.Body)
		if err != nil {
			_ = resp.Body.Close()
			return "", fmt.Errorf("failed to create zstd reader: %w", err)
		}
		// zstd.Decoder Close() signature doesn't match io.ReadCloser, adapt it
		bodyReader = &readCloserAdapter{r: zr, c: zr.Close}
	case "gzip":
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
		// Output text to stdoutWriter
		if stdoutWriter == nil {
			_ = bodyReader.Close()
			return "", fmt.Errorf("text content received but no output writer provided")
		}
		_, err := io.Copy(stdoutWriter, bodyReader)
		_ = bodyReader.Close()
		if err != nil {
			return "", fmt.Errorf("failed to output text: %w", err)
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

	// Create checkpoint if StateManager is configured and not resuming
	if d.stateManager != nil && d.checkpoint == nil {
		if sessionID == "" {
			sessionID = fmt.Sprintf("download-%d", time.Now().Unix())
		}
		if err := d.createCheckpoint(sessionID, url, outputPath, totalSize, key != nil); err != nil {
			return "", fmt.Errorf("failed to create checkpoint: %w", err)
		}
	}

	// Initial progress event
	if onProgress != nil {
		onProgress(progress.Progress{
			TotalBytes:       totalSize,
			TransferredBytes: startByte,
			FileName:         outputPath,
			Direction:        "download",
			IsResumable:      d.stateManager != nil,
			ResumedFrom:      float64(startByte) / float64(totalSize) * 100,
			StartTime:        time.Now(),
			LastUpdate:       time.Now(),
		})
	}

	// Check if file already exists and can be resumed
	var f *os.File
	if fi, err := os.Stat(outputPath); err == nil {
		// File exists
		if d.checkpoint != nil && fi.Size() > 0 && fi.Size() < totalSize {
			// Resuming from checkpoint - use existing file size as start point
			startByte = fi.Size()
			f, err = os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				return "", fmt.Errorf("failed to open file for resume: %w", err)
			}
		} else if !force && fi.Size() > 0 && fi.Size() < totalSize {
			// File exists and is incomplete - try to resume (no checkpoint)
			startByte = fi.Size()
			f, err = os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				return "", fmt.Errorf("failed to open file for resume: %w", err)
			}
		} else if !force {
			return "", fmt.Errorf("file '%s' already exists", outputPath)
		} else {
			// Force overwrite (but not if we have a checkpoint with progress)
			if d.checkpoint != nil && fi.Size() > 0 {
				// Resume from checkpoint
				startByte = fi.Size()
				f, err = os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND, 0o600)
				if err != nil {
					return "", fmt.Errorf("failed to open file for resume: %w", err)
				}
			} else {
				f, err = os.Create(outputPath)
				if err != nil {
					return "", fmt.Errorf("failed to create file: %w", err)
				}
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
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return "", fmt.Errorf("failed to create request: %w", err)
			}
			downloadResp, err = d.client.Do(req)
			if err != nil {
				return "", fmt.Errorf("failed to restart download: %w", err)
			}
		}
	} else {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		downloadResp, err = d.client.Do(req)
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

	if onProgress != nil {
		src = &callbackProgressReader{
			r:          src,
			total:      totalSize,
			current:    startByte,
			onProgress: onProgress,
			filepath:   outputPath,
		}
	}

	// Use adaptive buffer sizing based on file size
	// Use smaller buffer (32KB) to allow more frequent pause checks
	bufferSize := 32 * 1024 // 32KB for responsive pause
	buf := make([]byte, bufferSize)

	// Compute checksum while downloading
	hash := sha256.New()
	teeReader := io.TeeReader(src, hash)

	// Track chunks for checkpoint updates
	chunkSize := int64(5 * 1024 * 1024) // 5MB chunks
	var bytesInChunk int64
	chunkIndex := 0

	// If resuming, calculate starting chunk index
	if startByte > 0 {
		chunkIndex = int(startByte / chunkSize)
	}

	// Custom copy loop to update checkpoints periodically
	for {
		// Check context first
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Check if paused - wait for resume
		d.pauseMu.Lock()
		paused := d.isPaused
		d.pauseMu.Unlock()

		if paused {
			// Save checkpoint before waiting
			if bytesInChunk > 0 {
				_ = d.updateCheckpoint(startByte+bytesInChunk, chunkIndex)
			}

			// Wait for resume signal or context cancellation
			select {
			case <-d.resumeChan:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		n, err := teeReader.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return "", fmt.Errorf("failed to write file data: %w", writeErr)
			}

			bytesInChunk += int64(n)

			// Update checkpoint every 5MB
			if bytesInChunk >= chunkSize {
				startByte += bytesInChunk
				if updateErr := d.updateCheckpoint(startByte, chunkIndex); updateErr != nil {
					// Log but don't fail the download
					metrics.RecordError("checkpoint", "download")
				}
				bytesInChunk = 0
				chunkIndex++
			}
		}

		if err == io.EOF {
			// Final checkpoint update
			if bytesInChunk > 0 {
				startByte += bytesInChunk
				_ = d.updateCheckpoint(startByte, chunkIndex)
			}
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read data: %w", err)
		}
	}

	// Verify checksum if server provided one
	expectedChecksum := downloadResp.Header.Get("X-Content-SHA256")
	if expectedChecksum != "" {
		actualChecksum := hex.EncodeToString(hash.Sum(nil))
		if actualChecksum != expectedChecksum {
			metrics.ChecksumVerifications.WithLabelValues("mismatch").Inc()
			_ = os.Remove(outputPath) // Delete corrupted file
			return "", fmt.Errorf("checksum verification failed")
		}
		metrics.ChecksumVerifications.WithLabelValues("match").Inc()
	}

	// Delete checkpoint on successful completion
	if err := d.deleteCheckpoint(); err != nil {
		// Log but don't fail - download succeeded
		metrics.RecordError("checkpoint", "download")
	}

	// Final complete event
	if onProgress != nil {
		onProgress(progress.Progress{
			TotalBytes:       totalSize,
			TransferredBytes: totalSize,
			IsComplete:       true,
			FileName:         outputPath,
			SavedPath:        outputPath,
			Direction:        "download",
			LastUpdate:       time.Now(),
		})
	}

	return outputPath, nil
}

// Package-level Receive function for backward compatibility
// Uses default HTTP client with optimized settings
var defaultDownloader = NewDownloader(nil, nil, nil)

// Receive downloads from url using the default HTTP client
// This is a convenience function that wraps Downloader.Receive
func Receive(ctx context.Context, url string, outputPath string, force bool, onProgress func(progress.Progress), stdoutWriter io.Writer, key []byte) (string, error) {
	return defaultDownloader.Receive(ctx, url, outputPath, force, onProgress, stdoutWriter, key)
}

// createCheckpoint creates a new checkpoint for this download
func (d *Downloader) createCheckpoint(sessionID, url, outputPath string, totalSize int64, encrypted bool) error {
	if d.stateManager == nil {
		return nil // Checkpoints disabled
	}

	chunkSize := int64(5 * 1024 * 1024) // 5MB chunks
	totalChunks := int((totalSize + chunkSize - 1) / chunkSize)

	opts := resume.CheckpointOptions{
		SessionID:       sessionID,
		SourcePath:      url,
		DestinationPath: outputPath,
		Direction:       "download",
		TotalSize:       totalSize,
		ChunkSize:       chunkSize,
		TotalChunks:     totalChunks,
		Encrypted:       encrypted,
		ExpiresIn:       24 * time.Hour,
	}

	checkpoint, err := d.stateManager.CreateCheckpoint(opts)
	if err != nil {
		return err
	}

	d.checkpoint = checkpoint
	return nil
}

// updateCheckpoint updates the checkpoint with current progress
func (d *Downloader) updateCheckpoint(_ int64, chunkIndex int) error {
	if d.checkpoint == nil || d.stateManager == nil {
		return nil
	}

	// Mark chunk as complete
	d.checkpoint.MarkChunkComplete(chunkIndex, "")
	d.checkpoint.UpdatedAt = time.Now()

	// Non-blocking save
	go func() {
		if err := d.stateManager.UpdateCheckpoint(d.checkpoint); err != nil {
			metrics.RecordError("checkpoint", "download")
		}
	}()

	return nil
}

// deleteCheckpoint removes the checkpoint after successful download
func (d *Downloader) deleteCheckpoint() error {
	if d.checkpoint == nil || d.stateManager == nil {
		return nil
	}

	return d.stateManager.DeleteCheckpoint(d.checkpoint.SessionID)
}

// ResumeDownload creates a Downloader that resumes from a checkpoint
func ResumeDownload(checkpoint *resume.Checkpoint, stateManager *resume.TransferStateManager) (*Downloader, error) {
	if checkpoint.Direction != "download" {
		return nil, fmt.Errorf("checkpoint is not for download (direction: %s)", checkpoint.Direction)
	}

	// Calculate expected bytes from completed chunks
	expectedSize := int64(len(checkpoint.CompletedChunks)) * checkpoint.ChunkSize

	// Verify partial file exists
	fi, err := os.Stat(checkpoint.DestinationPath)
	if err != nil {
		return nil, fmt.Errorf("partial file not found: %w", err)
	}

	// Verify file size is reasonable (allow some variance for last chunk)
	if fi.Size() < expectedSize-checkpoint.ChunkSize || fi.Size() > expectedSize+checkpoint.ChunkSize {
		return nil, fmt.Errorf("partial file size mismatch: expected ~%d, got %d", expectedSize, fi.Size())
	}

	return &Downloader{
		client:       defaultHTTPClient(),
		stateManager: stateManager,
		checkpoint:   checkpoint,
	}, nil
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

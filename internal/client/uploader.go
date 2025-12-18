package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// UploadConfig configures parallel upload behavior
type UploadConfig struct {
	ChunkSize      int64         // Size of each chunk in bytes
	MaxConcurrent  int           // Maximum number of concurrent uploads
	RetryAttempts  int           // Number of retry attempts for failed chunks
	RetryDelay     time.Duration // Delay between retries
	ProgressWriter io.Writer     // Optional progress output
}

// DefaultUploadConfig returns sensible defaults for parallel uploads
func DefaultUploadConfig() *UploadConfig {
	return &UploadConfig{
		ChunkSize:      2 * 1024 * 1024, // 2MB chunks
		MaxConcurrent:  3,                // 3 parallel workers
		RetryAttempts:  3,                // 3 retries
		RetryDelay:     1 * time.Second,  // 1s between retries
		ProgressWriter: nil,
	}
}

// UploadSession tracks the state of a parallel upload
type UploadSession struct {
	SessionID      string
	URL            string
	File           *os.File
	TotalSize      int64
	Config         *UploadConfig
	uploadedBytes  atomic.Int64
	startTime      time.Time
	chunks         []chunkInfo
	chunkStatus    map[int]chunkState
	statusMu       sync.RWMutex
	progressTicker *time.Ticker
	cancel         context.CancelFunc
}

type chunkInfo struct {
	ID     int
	Offset int64
	Size   int64
}

type chunkState struct {
	Status    string // "pending", "uploading", "completed", "failed"
	Attempts  int
	Checksum  string
	BytesSent int64
}

// NewUploadSession creates a new parallel upload session
func NewUploadSession(url, filepath string, config *UploadConfig) (*UploadSession, error) {
	if config == nil {
		config = DefaultUploadConfig()
	}

	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Generate session ID
	sessionID := generateSessionID(filepath, stat.Size())

	// Calculate chunks
	totalChunks := int(math.Ceil(float64(stat.Size()) / float64(config.ChunkSize)))
	chunks := make([]chunkInfo, totalChunks)
	chunkStatus := make(map[int]chunkState, totalChunks)

	for i := 0; i < totalChunks; i++ {
		offset := int64(i) * config.ChunkSize
		size := config.ChunkSize
		if offset+size > stat.Size() {
			size = stat.Size() - offset
		}
		chunks[i] = chunkInfo{
			ID:     i,
			Offset: offset,
			Size:   size,
		}
		chunkStatus[i] = chunkState{Status: "pending", Attempts: 0}
	}

	return &UploadSession{
		SessionID:   sessionID,
		URL:         url,
		File:        file,
		TotalSize:   stat.Size(),
		Config:      config,
		chunks:      chunks,
		chunkStatus: chunkStatus,
		startTime:   time.Now(),
	}, nil
}

// generateSessionID creates a unique session identifier
func generateSessionID(filepath string, size int64) string {
	h := sha256.New()
	h.Write([]byte(filepath))
	h.Write([]byte(fmt.Sprintf("%d", size)))
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Upload performs the parallel upload with configurable concurrency
func (s *UploadSession) Upload(ctx context.Context) error {
	defer s.File.Close()

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	defer cancel()

	// Start progress reporting if configured
	if s.Config.ProgressWriter != nil {
		s.progressTicker = time.NewTicker(200 * time.Millisecond)
		defer s.progressTicker.Stop()
		go s.reportProgress()
	}

	// Create worker pool
	jobs := make(chan chunkInfo, len(s.chunks))
	results := make(chan error, len(s.chunks))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < s.Config.MaxConcurrent; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for chunk := range jobs {
				select {
				case <-ctx.Done():
					results <- ctx.Err()
					return
				default:
					err := s.uploadChunk(ctx, chunk)
					results <- err
				}
			}
		}(i)
	}

	// Queue all chunks
	for _, chunk := range s.chunks {
		jobs <- chunk
	}
	close(jobs)

	// Wait for all uploads to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var firstError error
	completedChunks := 0
	for err := range results {
		if err != nil && firstError == nil {
			firstError = err
			cancel() // Stop other uploads on first error
		}
		if err == nil {
			completedChunks++
		}
	}

	if firstError != nil {
		return fmt.Errorf("upload failed: %w", firstError)
	}

	// Final progress update
	if s.Config.ProgressWriter != nil {
		s.printFinalProgress()
	}

	return nil
}

// uploadChunk uploads a single chunk with retry logic
func (s *UploadSession) uploadChunk(ctx context.Context, chunk chunkInfo) error {
	var lastErr error

	for attempt := 0; attempt <= s.Config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := s.Config.RetryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		// Update status
		s.updateChunkStatus(chunk.ID, "uploading", attempt)

		// Read chunk data
		data := make([]byte, chunk.Size)
		_, err := s.File.ReadAt(data, chunk.Offset)
		if err != nil {
			lastErr = fmt.Errorf("read chunk %d: %w", chunk.ID, err)
			s.updateChunkStatus(chunk.ID, "failed", attempt)
			continue
		}

		// Calculate checksum
		checksum := sha256.Sum256(data)
		checksumHex := hex.EncodeToString(checksum[:])

		// Send chunk
		err = s.sendChunk(ctx, chunk, data, checksumHex)
		if err != nil {
			lastErr = err
			s.updateChunkStatus(chunk.ID, "failed", attempt)
			continue
		}

		// Success!
		s.updateChunkStatus(chunk.ID, "completed", attempt)
		s.setChunkChecksum(chunk.ID, checksumHex)
		s.uploadedBytes.Add(chunk.Size)
		return nil
	}

	return fmt.Errorf("chunk %d failed after %d attempts: %w", chunk.ID, s.Config.RetryAttempts+1, lastErr)
}

// sendChunk sends a single chunk to the server
func (s *UploadSession) sendChunk(ctx context.Context, chunk chunkInfo, data []byte, checksum string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", s.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers for chunk upload
	filename := s.File.Name()
	req.Header.Set("X-File-Name", url.QueryEscape(filename))
	req.Header.Set("X-Upload-Session", s.SessionID)
	req.Header.Set("X-Upload-Offset", fmt.Sprintf("%d", chunk.Offset))
	req.Header.Set("X-Upload-Total", fmt.Sprintf("%d", s.TotalSize))
	req.Header.Set("X-Chunk-Id", fmt.Sprintf("%d", chunk.ID))
	req.Header.Set("X-Chunk-Total", fmt.Sprintf("%d", len(s.chunks)))
	req.Header.Set("X-Chunk-Checksum", checksum)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Success  bool   `json:"success"`
		Filename string `json:"filename"`
		Received int64  `json:"received"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Non-fatal - server might not return JSON
		return nil
	}

	if !result.Success {
		return errors.New("server reported upload failure")
	}

	return nil
}

// updateChunkStatus updates the status of a chunk
func (s *UploadSession) updateChunkStatus(chunkID int, status string, attempts int) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	state := s.chunkStatus[chunkID]
	state.Status = status
	state.Attempts = attempts
	s.chunkStatus[chunkID] = state
}

// setChunkChecksum stores the checksum for a chunk
func (s *UploadSession) setChunkChecksum(chunkID int, checksum string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	state := s.chunkStatus[chunkID]
	state.Checksum = checksum
	s.chunkStatus[chunkID] = state
}

// getProgress returns current upload progress
func (s *UploadSession) getProgress() (completed, total int, bytesUploaded, bytesTotal int64, speed float64) {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()

	total = len(s.chunks)
	for _, state := range s.chunkStatus {
		if state.Status == "completed" {
			completed++
		}
	}

	bytesUploaded = s.uploadedBytes.Load()
	bytesTotal = s.TotalSize

	elapsed := time.Since(s.startTime).Seconds()
	if elapsed > 0 {
		speed = float64(bytesUploaded*8) / (elapsed * 1_000_000) // Mbps
	}

	return
}

// reportProgress periodically prints upload progress
func (s *UploadSession) reportProgress() {
	for range s.progressTicker.C {
		completed, total, bytesUploaded, bytesTotal, speed := s.getProgress()
		
		pct := float64(bytesUploaded) / float64(bytesTotal) * 100
		barWidth := 20
		filled := int(pct / 5)
		if filled > barWidth {
			filled = barWidth
		}
		bar := ""
		for i := 0; i < filled; i++ {
			bar += "="
		}
		for i := filled; i < barWidth; i++ {
			bar += " "
		}

		fmt.Fprintf(s.Config.ProgressWriter, "\r[%s] %3.0f%% | %s / %s | %.1f Mbps | Chunks: %d/%d",
			bar, pct,
			formatBytes(bytesUploaded), formatBytes(bytesTotal),
			speed,
			completed, total)
	}
}

// printFinalProgress prints the final progress line
func (s *UploadSession) printFinalProgress() {
	completed, total, bytesUploaded, bytesTotal, speed := s.getProgress()
	duration := time.Since(s.startTime).Seconds()

	fmt.Fprintf(s.Config.ProgressWriter, "\r[====================] 100%% | %s / %s | %.1f Mbps | %.2fs | Chunks: %d/%d\n",
		formatBytes(bytesUploaded), formatBytes(bytesTotal),
		speed, duration,
		completed, total)
}

// Cancel stops the upload
func (s *UploadSession) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// formatBytes formats bytes into human-readable string
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
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ParallelUpload is a convenience function for uploading a file with parallel chunks
func ParallelUpload(ctx context.Context, url, filepath string, config *UploadConfig, progress io.Writer) error {
	if config == nil {
		config = DefaultUploadConfig()
	}
	config.ProgressWriter = progress

	session, err := NewUploadSession(url, filepath, config)
	if err != nil {
		return err
	}

	return session.Upload(ctx)
}

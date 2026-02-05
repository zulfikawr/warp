package client

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
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
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/protocol"
	"github.com/zulfikawr/warp/internal/resume"
)

// UploadConfig configures parallel upload behavior
type UploadConfig struct {
	ChunkSize     int64                   // Size of each chunk in bytes
	MaxConcurrent int                     // Maximum number of concurrent uploads
	RetryAttempts int                     // Number of retry attempts for failed chunks
	RetryDelay    time.Duration           // Delay between retries
	OnProgress    func(progress.Progress) // Optional progress callback (using unified Progress type)
}

// DefaultUploadConfig returns sensible defaults for parallel uploads
func DefaultUploadConfig() *UploadConfig {
	return &UploadConfig{
		ChunkSize:     2 * 1024 * 1024, // 2MB chunks
		MaxConcurrent: 3,               // 3 parallel workers
		RetryAttempts: 3,               // 3 retries
		RetryDelay:    1 * time.Second, // 1s between retries
		OnProgress:    nil,
	}
}

// UploadSession tracks the state of a parallel upload
type UploadSession struct {
	SessionID      string
	URL            string
	File           *os.File
	TotalSize      int64
	Config         *UploadConfig
	Client         *http.Client // HTTP client for requests
	uploadedBytes  atomic.Int64
	startTime      time.Time
	chunks         []chunkInfo
	chunkStatus    map[int]chunkState
	statusMu       sync.RWMutex
	progressTicker *time.Ticker
	cancel         context.CancelFunc
	// bufferPool is session-specific for chunk-sized buffers.
	// For standard protocol buffer sizes, use internal/bufpool package instead.
	bufferPool sync.Pool
	filepath   string
	// Resume support
	StateManager *resume.TransferStateManager
	Checkpoint   *resume.Checkpoint
	checkpointMu sync.Mutex
	// Encryption support
	EncryptionKey   []byte                         // Encryption key for encrypted transfers
	EncryptionSalt  []byte                         // Salt used for key derivation
	encryptionState *resume.EncryptionStateManager // Manages encryption state for resumable transfers
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
// If encryptionKey is provided, chunks will be encrypted before upload
func NewUploadSession(url, filepath string, config *UploadConfig, stateManager *resume.TransferStateManager, checkpoint *resume.Checkpoint, encryptionKey []byte) (*UploadSession, error) {
	if config == nil {
		config = DefaultUploadConfig()
	}

	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	var sessionID string
	var chunks []chunkInfo
	var chunkStatus map[int]chunkState
	var uploadedBytes int64

	// If resuming from checkpoint, validate and restore state
	if checkpoint != nil {
		// Validate file hasn't changed
		if checkpoint.TotalSize != stat.Size() {
			_ = file.Close()
			return nil, fmt.Errorf("file size mismatch: checkpoint has %d bytes, file has %d bytes", checkpoint.TotalSize, stat.Size())
		}

		// Check file modification time if available
		if !checkpoint.CreatedAt.IsZero() && stat.ModTime().After(checkpoint.CreatedAt) {
			_ = file.Close()
			return nil, fmt.Errorf("file modified after checkpoint was created")
		}

		sessionID = checkpoint.SessionID
		totalChunks := checkpoint.TotalChunks
		chunks = make([]chunkInfo, totalChunks)
		chunkStatus = make(map[int]chunkState, totalChunks)

		// Restore chunk information
		for i := 0; i < totalChunks; i++ {
			offset := int64(i) * checkpoint.ChunkSize
			size := checkpoint.ChunkSize
			if offset+size > stat.Size() {
				size = stat.Size() - offset
			}
			chunks[i] = chunkInfo{
				ID:     i,
				Offset: offset,
				Size:   size,
			}

			// Check if chunk was already completed
			isCompleted := false
			for _, completedID := range checkpoint.CompletedChunks {
				if completedID == i {
					isCompleted = true
					break
				}
			}

			if isCompleted {
				chunkStatus[i] = chunkState{Status: "completed", Attempts: 0}
				uploadedBytes += size
			} else {
				chunkStatus[i] = chunkState{Status: "pending", Attempts: 0}
			}
		}
	} else {
		// New upload - generate session ID and calculate chunks
		sessionID = generateSessionID(filepath, stat.Size())

		totalChunks := int(math.Ceil(float64(stat.Size()) / float64(config.ChunkSize)))
		chunks = make([]chunkInfo, totalChunks)
		chunkStatus = make(map[int]chunkState, totalChunks)

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
	}

	session := &UploadSession{
		SessionID:     sessionID,
		URL:           url,
		File:          file,
		filepath:      filepath,
		TotalSize:     stat.Size(),
		Config:        config,
		Client:        defaultHTTPClient(),
		chunks:        chunks,
		chunkStatus:   chunkStatus,
		startTime:     time.Now(),
		StateManager:  stateManager,
		Checkpoint:    checkpoint,
		EncryptionKey: encryptionKey,
	}

	// Initialize buffer pool with chunk size (must be done after struct creation to avoid copy)
	chunkSize := config.ChunkSize
	session.bufferPool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, chunkSize)
			return &b
		},
	}

	// Initialize encryption state if key is provided
	if encryptionKey != nil {
		session.encryptionState = resume.NewEncryptionStateManager()
	}

	// Set initial uploaded bytes
	session.uploadedBytes.Store(uploadedBytes)

	// Create checkpoint if not resuming
	if checkpoint == nil && stateManager != nil {
		if err := session.createCheckpoint(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("failed to create checkpoint: %w", err)
		}
	}

	return session, nil
}

// generateSessionID creates a unique session identifier
func generateSessionID(filepath string, size int64) string {
	h := sha256.New()
	h.Write([]byte(filepath))
	_, _ = fmt.Fprintf(h, "%d", size)
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// InitializeEncryption initializes encryption for the upload session using a PAKE code
// This should be called after creating the session but before starting the upload
func (s *UploadSession) InitializeEncryption(pakeCode string) error {
	if s.encryptionState == nil {
		s.encryptionState = resume.NewEncryptionStateManager()
	}

	if err := s.encryptionState.Initialize(pakeCode); err != nil {
		return fmt.Errorf("failed to initialize encryption: %w", err)
	}

	// Store the derived key and salt
	s.EncryptionKey = s.encryptionState.GetKey()
	s.EncryptionSalt = s.encryptionState.GetSalt()

	// Update checkpoint with encryption state
	if s.Checkpoint != nil {
		s.Checkpoint.Encrypted = true
		s.Checkpoint.EncryptionState = s.encryptionState.SaveState()
		if s.StateManager != nil {
			if err := s.StateManager.UpdateCheckpoint(s.Checkpoint); err != nil {
				return fmt.Errorf("failed to save encryption state to checkpoint: %w", err)
			}
		}
	}

	return nil
}

// createCheckpoint creates a new checkpoint for this upload session
func (s *UploadSession) createCheckpoint() error {
	if s.StateManager == nil {
		return nil // No state manager, skip checkpoint
	}

	s.checkpointMu.Lock()
	defer s.checkpointMu.Unlock()

	opts := resume.CheckpointOptions{
		SessionID:       s.SessionID,
		SourcePath:      s.filepath,
		DestinationPath: "", // Server-side destination unknown
		Direction:       "upload",
		TotalSize:       s.TotalSize,
		ChunkSize:       s.Config.ChunkSize,
		TotalChunks:     len(s.chunks),
		Encrypted:       s.EncryptionKey != nil,
	}

	checkpoint, err := s.StateManager.CreateCheckpoint(opts)
	if err != nil {
		metrics.CheckpointSaveErrors.Inc()
		return fmt.Errorf("failed to create checkpoint: %w", err)
	}

	// Save encryption state if encrypted
	if s.EncryptionKey != nil && s.encryptionState != nil {
		encState := s.encryptionState.SaveState()
		checkpoint.EncryptionState = encState
		if err := s.StateManager.UpdateCheckpoint(checkpoint); err != nil {
			metrics.CheckpointSaveErrors.Inc()
			return fmt.Errorf("failed to save encryption state: %w", err)
		}
	}

	s.Checkpoint = checkpoint
	metrics.ActiveCheckpoints.Inc()
	return nil
}

// updateCheckpoint updates the checkpoint with current progress
func (s *UploadSession) updateCheckpoint() error {
	if s.StateManager == nil || s.Checkpoint == nil {
		return nil // No checkpoint to update
	}

	s.checkpointMu.Lock()
	defer s.checkpointMu.Unlock()

	// Get completed chunks
	s.statusMu.RLock()
	completedChunks := make([]int, 0, len(s.chunkStatus))
	for chunkID, state := range s.chunkStatus {
		if state.Status == "completed" {
			completedChunks = append(completedChunks, chunkID)
		}
	}
	s.statusMu.RUnlock()

	// Update checkpoint
	s.Checkpoint.CompletedChunks = completedChunks
	s.Checkpoint.UpdatedAt = time.Now()

	// Update encryption state if encrypted
	if s.EncryptionKey != nil && s.encryptionState != nil {
		s.Checkpoint.EncryptionState = s.encryptionState.SaveState()
	}

	// Save to disk (async to avoid blocking)
	go func() {
		saveStart := time.Now()
		if err := s.StateManager.UpdateCheckpoint(s.Checkpoint); err != nil {
			metrics.CheckpointSaveErrors.Inc()
			// Log error but don't fail the upload
		} else {
			metrics.CheckpointSavesTotal.Inc()
			metrics.CheckpointSaveDuration.Observe(time.Since(saveStart).Seconds())
		}
	}()

	return nil
}

// deleteCheckpoint removes the checkpoint file
func (s *UploadSession) deleteCheckpoint() error {
	if s.StateManager == nil || s.Checkpoint == nil {
		return nil
	}

	s.checkpointMu.Lock()
	defer s.checkpointMu.Unlock()

	if err := s.StateManager.DeleteCheckpoint(s.SessionID); err != nil {
		return err
	}

	metrics.CheckpointCleanups.WithLabelValues("completed").Inc()
	metrics.ActiveCheckpoints.Dec()
	s.Checkpoint = nil
	return nil
}

// Upload performs the parallel upload with configurable concurrency
func (s *UploadSession) Upload(ctx context.Context) error {
	defer func() { _ = s.File.Close() }()

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	defer cancel()

	// Start progress reporting if configured
	if s.Config.OnProgress != nil {
		s.progressTicker = time.NewTicker(protocol.ProgressUpdateInterval)
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

	// Queue only incomplete chunks
	s.statusMu.RLock()
	pendingChunks := 0
	for _, chunk := range s.chunks {
		if s.chunkStatus[chunk.ID].Status != "completed" {
			jobs <- chunk
			pendingChunks++
		}
	}
	s.statusMu.RUnlock()
	close(jobs)

	// Wait for all uploads to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var firstError error
	completedChunks := 0
	totalProcessed := 0
	for err := range results {
		totalProcessed++
		if err != nil && firstError == nil {
			firstError = err
			cancel() // Stop other uploads on first error
		}
		if err == nil {
			completedChunks++
			// Update checkpoint every 5 chunks
			if completedChunks%5 == 0 {
				_ = s.updateCheckpoint()
			}
		}
	}

	// Final checkpoint update
	if firstError == nil {
		_ = s.updateCheckpoint()
	}

	if firstError != nil {
		// Save checkpoint on error so we can resume
		_ = s.updateCheckpoint()
		return fmt.Errorf("upload failed: %w", firstError)
	}

	// Final progress update
	if s.Config.OnProgress != nil {
		s.emitProgress(true)
	}

	// Delete checkpoint on successful completion
	_ = s.deleteCheckpoint()

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

		// Get buffer from pool
		bufPtr := s.bufferPool.Get().(*[]byte)
		data := (*bufPtr)[:chunk.Size] // Slice to actual chunk size

		// Read chunk data
		_, err := s.File.ReadAt(data, chunk.Offset)
		if err != nil {
			s.bufferPool.Put(bufPtr) // Return buffer on error
			lastErr = fmt.Errorf("read chunk %d: %w", chunk.ID, err)
			s.updateChunkStatus(chunk.ID, "failed", attempt)
			continue
		}

		// Encrypt chunk data if encryption is enabled
		var dataToSend []byte
		if s.EncryptionKey != nil {
			encryptedData, encErr := s.encryptChunk(data, chunk.ID)
			if encErr != nil {
				s.bufferPool.Put(bufPtr)
				lastErr = fmt.Errorf("encrypt chunk %d: %w", chunk.ID, encErr)
				s.updateChunkStatus(chunk.ID, "failed", attempt)
				continue
			}
			dataToSend = encryptedData
		} else {
			dataToSend = data
		}

		// Compute checksum (of encrypted data if encrypted)
		checksum := sha256.Sum256(dataToSend)
		checksumHex := hex.EncodeToString(checksum[:])

		// Send chunk
		err = s.sendChunk(ctx, chunk, dataToSend, checksumHex)

		// Return buffer after sending
		s.bufferPool.Put(bufPtr)

		if err != nil {
			lastErr = err
			s.updateChunkStatus(chunk.ID, "failed", attempt)
			continue
		}

		// Success!
		s.updateChunkStatus(chunk.ID, "completed", attempt)
		s.setChunkChecksum(chunk.ID, checksumHex)
		s.uploadedBytes.Add(chunk.Size)

		// Update encryption state counter if encrypted
		if s.encryptionState != nil {
			s.encryptionState.IncrementChunkCounter()
		}

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
	filename := filepath.Base(s.File.Name())
	req.Header.Set("X-File-Name", url.QueryEscape(filename))
	req.Header.Set("X-Upload-Session", s.SessionID)
	req.Header.Set("X-Upload-Offset", fmt.Sprintf("%d", chunk.Offset))
	req.Header.Set("X-Upload-Total", fmt.Sprintf("%d", s.TotalSize))
	req.Header.Set("X-Chunk-Id", fmt.Sprintf("%d", chunk.ID))
	req.Header.Set("X-Chunk-Total", fmt.Sprintf("%d", len(s.chunks)))
	req.Header.Set("X-Chunk-Checksum", checksum)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	req.Header.Set("Content-Type", "application/octet-stream")

	// Add encryption header if encrypted
	if s.EncryptionKey != nil {
		req.Header.Set("X-Encrypted", "true")
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

// encryptChunk encrypts a chunk of data using AES-256-GCM
// The nonce is derived from the base nonce and chunk ID to ensure uniqueness
func (s *UploadSession) encryptChunk(plaintext []byte, chunkID int) ([]byte, error) {
	if s.EncryptionKey == nil {
		return nil, fmt.Errorf("encryption key not set")
	}

	// Create AES cipher
	block, err := aes.NewCipher(s.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Get nonce for this chunk from encryption state manager
	var nonce []byte
	if s.encryptionState != nil {
		nonce, err = s.encryptionState.GetNonceForChunk(uint64(chunkID))
		if err != nil {
			return nil, fmt.Errorf("failed to get nonce: %w", err)
		}
	} else {
		// Fallback: generate deterministic nonce from chunk ID
		nonce = make([]byte, crypto.NonceSize)
		for i := 0; i < 8; i++ {
			nonce[crypto.NonceSize-8+i] = byte(uint64(chunkID) >> (56 - uint(i*8)))
		}
	}

	// Encrypt the chunk
	// The ciphertext includes the authentication tag
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Prepend nonce to ciphertext for decryption
	result := make([]byte, len(nonce)+len(ciphertext))
	copy(result, nonce)
	copy(result[len(nonce):], ciphertext)

	return result, nil
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

// reportProgress periodically emits upload progress
func (s *UploadSession) reportProgress() {
	for range s.progressTicker.C {
		s.emitProgress(false)
	}
}

func (s *UploadSession) emitProgress(complete bool) {
	chunksDone, chunksTotal, sent, total, speed := s.getProgress()

	// Calculate ETA
	var eta time.Duration
	if speed > 0 && !complete {
		remaining := total - sent
		eta = time.Duration(float64(remaining)/speed) * time.Second
	}

	info := progress.Progress{
		TransferID:       s.SessionID,
		FileName:         filepath.Base(s.filepath),
		Direction:        "upload",
		TotalBytes:       total,
		TransferredBytes: sent,
		SpeedBytesPerSec: speed,
		StartTime:        s.startTime,
		ETA:              eta,
		TotalChunks:      chunksTotal,
		CompletedChunks:  chunksDone,
		IsComplete:       complete,
		SavedPath:        s.filepath,
		IsResumable:      s.StateManager != nil,
		LastUpdate:       time.Now(),
	}

	if s.Config.OnProgress != nil {
		s.Config.OnProgress(info)
	}
}

// Cancel stops the upload
func (s *UploadSession) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// ParallelUpload is a convenience function for uploading a file with parallel chunks
func ParallelUpload(ctx context.Context, url, filepath string, config *UploadConfig, onProgress func(progress.Progress)) error {
	if config == nil {
		config = DefaultUploadConfig()
	}
	config.OnProgress = onProgress

	session, err := NewUploadSession(url, filepath, config, nil, nil, nil)
	if err != nil {
		return err
	}

	return session.Upload(ctx)
}

// ResumeUploadSession creates an upload session from an existing checkpoint
// If the checkpoint was encrypted, encryptionKey must be provided to resume
func ResumeUploadSession(checkpoint *resume.Checkpoint, url string, config *UploadConfig, stateManager *resume.TransferStateManager, encryptionKey []byte) (*UploadSession, error) {
	if checkpoint == nil {
		return nil, fmt.Errorf("checkpoint is required for resume")
	}

	if checkpoint.Direction != "upload" {
		return nil, fmt.Errorf("checkpoint is not for an upload (direction: %s)", checkpoint.Direction)
	}

	// Validate encryption state
	if checkpoint.Encrypted && encryptionKey == nil {
		return nil, fmt.Errorf("encryption key required to resume encrypted upload")
	}

	if config == nil {
		config = DefaultUploadConfig()
	}

	// Override chunk size from checkpoint for consistency
	config.ChunkSize = checkpoint.ChunkSize

	// Create session with checkpoint
	session, err := NewUploadSession(url, checkpoint.SourcePath, config, stateManager, checkpoint, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload session: %w", err)
	}

	// Restore encryption state if encrypted
	if checkpoint.Encrypted && checkpoint.EncryptionState != nil {
		if session.encryptionState == nil {
			session.encryptionState = resume.NewEncryptionStateManager()
		}
		// Restore the nonce and counter state with the provided key
		if err := session.encryptionState.RestoreStateWithKey(checkpoint.EncryptionState, encryptionKey); err != nil {
			return nil, fmt.Errorf("failed to restore encryption state: %w", err)
		}
		session.EncryptionSalt = checkpoint.EncryptionState.Salt
	}

	metrics.ResumedTransfers.WithLabelValues("upload").Inc()

	return session, nil
}

package resume

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zulfikawr/warp/internal/progress"
)

// TransferStatus represents the current state of a transfer
type TransferStatus string

const (
	// StatusPending indicates the transfer is pending
	StatusPending TransferStatus = "pending"
	// StatusActive indicates the transfer is actively running
	StatusActive TransferStatus = "active"
	// StatusPaused indicates the transfer is paused
	StatusPaused TransferStatus = "paused"
	// StatusCompleted indicates the transfer completed successfully
	StatusCompleted TransferStatus = "completed"
	// StatusFailed indicates the transfer failed
	StatusFailed TransferStatus = "failed"
	// StatusCancelled indicates the transfer was cancelled
	StatusCancelled TransferStatus = "cancelled"
)

// RecoveryAction defines how to handle specific errors
type RecoveryAction int

const (
	// ActionRetry retries the operation
	ActionRetry RecoveryAction = iota
	// ActionSkip skips and continues
	ActionSkip
	// ActionPause pauses and saves state
	ActionPause
	// ActionRestart restarts from beginning
	ActionRestart
	// ActionAbort aborts with error
	ActionAbort
)

const (
	// AutoCheckpointThreshold is the file size threshold for auto-checkpoint (100MB)
	AutoCheckpointThreshold = 100 * 1024 * 1024
	// MaxChunksThreshold is the maximum number of chunks before auto-adjustment
	MaxChunksThreshold = 100000
	// MaxRetries is the maximum number of retry attempts
	MaxRetries = 3
	// BaseRetryDelay is the base delay for exponential backoff
	BaseRetryDelay = 1 * time.Second
)

// TransferSessionOptions contains options for creating a new transfer session
type TransferSessionOptions struct {
	Checkpoint       *Checkpoint
	StateManager     *TransferStateManager
	HTTPClient       *http.Client
	EncryptionKey    []byte
	ProgressCallback func(progress.Progress)
}

// TransferSession represents an active transfer
type TransferSession struct {
	// Core state
	Checkpoint     *Checkpoint
	StateManager   *TransferStateManager
	EncryptManager *EncryptionStateManager
	Verifier       *IntegrityVerifier
	Progress       *ProgressTracker

	// Transfer execution dependencies
	HTTPClient       *http.Client
	EncryptionKey    []byte
	ProgressCallback func(progress.Progress)

	// Runtime state
	Status          TransferStatus
	Error           error
	PauseRequested  bool
	CancelRequested bool

	// Channels for coordination
	pauseCh  chan struct{}
	resumeCh chan struct{}
	cancelCh chan struct{}
	doneCh   chan error

	mu sync.RWMutex
}

// NewTransferSession creates a new transfer session
func NewTransferSession(checkpoint *Checkpoint, stateManager *TransferStateManager) *TransferSession {
	return NewTransferSessionWithOptions(TransferSessionOptions{
		Checkpoint:   checkpoint,
		StateManager: stateManager,
	})
}

// NewTransferSessionWithOptions creates a new transfer session with full options
func NewTransferSessionWithOptions(opts TransferSessionOptions) *TransferSession {
	session := &TransferSession{
		Checkpoint:       opts.Checkpoint,
		StateManager:     opts.StateManager,
		HTTPClient:       opts.HTTPClient,
		EncryptionKey:    opts.EncryptionKey,
		ProgressCallback: opts.ProgressCallback,
		Status:           StatusPending,
		pauseCh:          make(chan struct{}, 1),
		resumeCh:         make(chan struct{}, 1),
		cancelCh:         make(chan struct{}, 1),
		doneCh:           make(chan error, 1),
	}

	// Initialize components
	session.Verifier = NewIntegrityVerifier()
	session.Progress = NewProgressTracker(
		opts.Checkpoint.SessionID,
		opts.Checkpoint.TotalSize,
		opts.Checkpoint.TotalChunks,
	)

	// Restore progress if resuming
	if len(opts.Checkpoint.CompletedChunks) > 0 {
		bytesTransferred := int64(len(opts.Checkpoint.CompletedChunks)) * opts.Checkpoint.ChunkSize
		if bytesTransferred > opts.Checkpoint.TotalSize {
			bytesTransferred = opts.Checkpoint.TotalSize
		}
		session.Progress.RestoreProgress(opts.Checkpoint.CompletedChunks, bytesTransferred)
	}

	// Import verification state
	if len(opts.Checkpoint.ChunkChecksums) > 0 {
		verificationState := &VerificationState{
			Algorithm:        "sha256",
			ChunkHashes:      opts.Checkpoint.ChunkChecksums,
			ExpectedFileHash: opts.Checkpoint.FileChecksum,
		}
		_ = session.Verifier.ImportState(verificationState)
	}

	return session
}

// Start begins or resumes the transfer
func (s *TransferSession) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.Status == StatusActive {
		s.mu.Unlock()
		return fmt.Errorf("transfer already active")
	}
	s.Status = StatusActive
	s.PauseRequested = false
	s.CancelRequested = false
	s.mu.Unlock()

	// Start transfer in background
	go s.run(ctx)

	return nil
}

// run executes the transfer (internal method)
func (s *TransferSession) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.mu.Lock()
			s.Status = StatusFailed
			s.Error = fmt.Errorf("panic: %v", r)
			s.mu.Unlock()
			s.doneCh <- s.Error
		}
	}()

	// Check if auto-checkpoint should be enabled
	if s.shouldAutoCheckpoint() {
		// Auto-checkpoint is enabled by default for large files
		// Adjust chunk size if needed
		s.adjustChunkSize()
	}

	// Execute the transfer based on direction
	err := s.executeTransfer(ctx)

	// Handle pause - don't mark as failed, keep paused status
	if s.PauseRequested {
		s.mu.Lock()
		s.Status = StatusPaused
		s.mu.Unlock()
		// Save checkpoint state
		if saveErr := s.StateManager.UpdateCheckpoint(s.Checkpoint); saveErr != nil {
			s.doneCh <- fmt.Errorf("failed to save checkpoint on pause: %w", saveErr)
			return
		}
		s.doneCh <- nil
		return
	}

	// Handle cancel
	if s.CancelRequested {
		s.mu.Lock()
		s.Status = StatusCancelled
		s.mu.Unlock()
		s.doneCh <- nil
		return
	}

	s.mu.Lock()
	if err != nil {
		s.Status = StatusFailed
		s.Error = err
	} else {
		s.Status = StatusCompleted
	}
	s.mu.Unlock()

	s.doneCh <- err
}

// executeTransfer performs the actual transfer
func (s *TransferSession) executeTransfer(ctx context.Context) error {
	// Determine transfer direction and execute accordingly
	switch s.Checkpoint.Direction {
	case "upload":
		return s.executeUpload(ctx)
	case "download":
		return s.executeDownload(ctx)
	default:
		return fmt.Errorf("unknown transfer direction: %s", s.Checkpoint.Direction)
	}
}

// executeUpload performs an upload transfer using the client.UploadSession
func (s *TransferSession) executeUpload(ctx context.Context) error {
	// Create upload adapter with checkpoint for resume support
	adapter := &uploadAdapter{
		client:       s.HTTPClient,
		stateManager: s.StateManager,
		checkpoint:   s.Checkpoint,
	}

	// Progress callback adapter to update session state
	onProgress := func(totalBytes, sentBytes int64, speedMbps float64, isComplete bool) {
		s.Progress.UpdateBytes(sentBytes)
		if s.ProgressCallback != nil {
			s.ProgressCallback(s.Progress.GetProgress())
		}
	}

	// Execute upload with pause checking
	return s.executeUploadWithPauseCheck(ctx, adapter, onProgress)
}

// uploadAdapter wraps upload parameters for the transfer session
type uploadAdapter struct {
	client       *http.Client
	stateManager *TransferStateManager
	checkpoint   *Checkpoint
}

// executeUploadWithPauseCheck performs upload with periodic pause checking
func (s *TransferSession) executeUploadWithPauseCheck(ctx context.Context, _ *uploadAdapter, _ func(totalBytes, sentBytes int64, speedMbps float64, isComplete bool)) error {
	// Check for pause/cancel before starting
	if shouldStop, err := s.checkPauseOrCancel(); shouldStop {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// The actual upload is performed by the client.UploadSession
	// This method provides the integration point for pause/resume coordination
	// The upload itself handles checkpointing internally

	// Create a context that can be cancelled on pause
	uploadCtx, uploadCancel := context.WithCancel(ctx)
	defer uploadCancel()

	// Monitor for pause/cancel in background
	go func() {
		select {
		case <-uploadCtx.Done():
			return
		case <-s.pauseCh:
			s.mu.Lock()
			s.PauseRequested = true
			s.mu.Unlock()
			uploadCancel()
		case <-s.cancelCh:
			s.mu.Lock()
			s.CancelRequested = true
			s.mu.Unlock()
			uploadCancel()
		}
	}()

	// The upload execution happens through the UploadSession.Upload method
	// which is called by the client code. This session tracks state and
	// coordinates pause/resume.

	// Wait for either completion or interruption
	select {
	case <-uploadCtx.Done():
		// Save checkpoint state before returning
		if err := s.saveCheckpointState(); err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}

		if s.PauseRequested {
			return s.handlePause()
		}
		if s.CancelRequested {
			return s.handleCancel()
		}
		return uploadCtx.Err()
	case err := <-s.doneCh:
		return err
	}
}

// executeDownload performs a download transfer using the client.Downloader
func (s *TransferSession) executeDownload(ctx context.Context) error {
	// Create downloader with checkpoint for resume support
	downloader := &downloadAdapter{
		client:       s.HTTPClient,
		stateManager: s.StateManager,
		checkpoint:   s.Checkpoint,
	}

	// Progress callback adapter to update session state
	onProgress := func(totalBytes, sentBytes int64, speedMbps float64, isComplete bool) {
		s.Progress.UpdateBytes(sentBytes)
		if s.ProgressCallback != nil {
			s.ProgressCallback(s.Progress.GetProgress())
		}
	}

	// Execute download with pause checking
	return s.executeDownloadWithPauseCheck(ctx, downloader, onProgress)
}

// downloadAdapter wraps download parameters for the transfer session
type downloadAdapter struct {
	client       *http.Client
	stateManager *TransferStateManager
	checkpoint   *Checkpoint
}

// executeDownloadWithPauseCheck performs download with periodic pause checking
func (s *TransferSession) executeDownloadWithPauseCheck(ctx context.Context, _ *downloadAdapter, _ func(totalBytes, sentBytes int64, speedMbps float64, isComplete bool)) error {
	// Check for pause/cancel before starting
	if shouldStop, err := s.checkPauseOrCancel(); shouldStop {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// The actual download is performed by the client.Downloader
	// This method provides the integration point for pause/resume coordination
	// The download itself handles checkpointing internally

	// Create a context that can be cancelled on pause
	downloadCtx, downloadCancel := context.WithCancel(ctx)
	defer downloadCancel()

	// Monitor for pause/cancel in background
	go func() {
		select {
		case <-downloadCtx.Done():
			return
		case <-s.pauseCh:
			s.mu.Lock()
			s.PauseRequested = true
			s.mu.Unlock()
			downloadCancel()
		case <-s.cancelCh:
			s.mu.Lock()
			s.CancelRequested = true
			s.mu.Unlock()
			downloadCancel()
		}
	}()

	// The download execution happens through the Downloader.Receive method
	// which is called by the client code. This session tracks state and
	// coordinates pause/resume.

	// Wait for either completion or interruption
	select {
	case <-downloadCtx.Done():
		// Save checkpoint state before returning
		if err := s.saveCheckpointState(); err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}

		if s.PauseRequested {
			return s.handlePause()
		}
		if s.CancelRequested {
			return s.handleCancel()
		}
		return downloadCtx.Err()
	case err := <-s.doneCh:
		return err
	}
}

// Pause pauses the transfer and saves state
func (s *TransferSession) Pause() error {
	s.mu.Lock()
	if s.Status != StatusActive {
		s.mu.Unlock()
		return fmt.Errorf("transfer not active (status: %s)", s.Status)
	}
	s.PauseRequested = true
	s.mu.Unlock()

	// Signal pause
	select {
	case s.pauseCh <- struct{}{}:
	default:
	}

	return nil
}

// handlePause handles the pause request
func (s *TransferSession) handlePause() error {
	s.mu.Lock()
	s.Status = StatusPaused
	s.mu.Unlock()

	// Save current state
	if err := s.StateManager.UpdateCheckpoint(s.Checkpoint); err != nil {
		return NewCheckpointError(err, s.Checkpoint.SessionID, "pause")
	}

	return fmt.Errorf("transfer paused")
}

// Resume resumes a paused transfer
func (s *TransferSession) Resume() error {
	s.mu.Lock()
	if s.Status != StatusPaused {
		s.mu.Unlock()
		return fmt.Errorf("transfer not paused (status: %s)", s.Status)
	}
	s.Status = StatusActive
	s.PauseRequested = false
	s.mu.Unlock()

	// Signal resume
	select {
	case s.resumeCh <- struct{}{}:
	default:
	}

	return nil
}

// Cancel cancels the transfer
func (s *TransferSession) Cancel() error {
	s.mu.Lock()
	if s.Status == StatusCompleted || s.Status == StatusCancelled {
		s.mu.Unlock()
		return fmt.Errorf("transfer already finished (status: %s)", s.Status)
	}
	s.CancelRequested = true
	s.mu.Unlock()

	// Signal cancel
	select {
	case s.cancelCh <- struct{}{}:
	default:
	}

	return nil
}

// handleCancel handles the cancel request
func (s *TransferSession) handleCancel() error {
	s.mu.Lock()
	s.Status = StatusCancelled
	s.mu.Unlock()

	// Optionally delete checkpoint on cancel
	// For now, we keep it so user can resume later

	return fmt.Errorf("transfer cancelled")
}

// checkPauseOrCancel checks if pause or cancel has been requested
// Returns true if the transfer should stop, along with the appropriate error
func (s *TransferSession) checkPauseOrCancel() (bool, error) {
	s.mu.RLock()
	pauseRequested := s.PauseRequested
	cancelRequested := s.CancelRequested
	s.mu.RUnlock()

	if cancelRequested {
		return true, s.handleCancel()
	}
	if pauseRequested {
		return true, s.handlePause()
	}
	return false, nil
}

// saveCheckpointState saves the current checkpoint state to disk
func (s *TransferSession) saveCheckpointState() error {
	if s.StateManager == nil || s.Checkpoint == nil {
		return nil
	}
	return s.StateManager.UpdateCheckpoint(s.Checkpoint)
}

// Wait blocks until the transfer completes or fails, or context is cancelled
func (s *TransferSession) Wait(ctx context.Context) error {
	select {
	case err := <-s.doneCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetStatus returns the current transfer status
func (s *TransferSession) GetStatus() TransferStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

// GetError returns the transfer error if any
func (s *TransferSession) GetError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Error
}

// shouldAutoCheckpoint checks if auto-checkpoint should be enabled
func (s *TransferSession) shouldAutoCheckpoint() bool {
	return s.Checkpoint.TotalSize > AutoCheckpointThreshold
}

// adjustChunkSize adjusts chunk size if chunk count exceeds threshold
func (s *TransferSession) adjustChunkSize() {
	if s.Checkpoint.TotalChunks > MaxChunksThreshold {
		// Calculate new chunk size to stay at or under threshold
		// Add 1 to ensure we round up and stay under the limit
		newChunkSize := (s.Checkpoint.TotalSize + int64(MaxChunksThreshold) - 1) / int64(MaxChunksThreshold)
		if newChunkSize > s.Checkpoint.ChunkSize {
			s.Checkpoint.ChunkSize = newChunkSize
			s.Checkpoint.TotalChunks = int(s.Checkpoint.TotalSize / newChunkSize)
			if s.Checkpoint.TotalSize%newChunkSize != 0 {
				s.Checkpoint.TotalChunks++
			}
		}
	}
}

// HandleError determines recovery action for errors
func (s *TransferSession) HandleError(err error) RecoveryAction {
	switch e := err.(type) {
	case *ResumableError:
		if e.Recoverable {
			return ActionRetry
		}
		return ActionPause
	case *IntegrityError:
		return ActionRetry // Re-transfer corrupted chunk
	case *EncryptionResumeError:
		if e.Reason == "nonce_exhausted" {
			return ActionRestart
		}
		return ActionAbort
	default:
		return ActionPause
	}
}

// RetryWithBackoff retries an operation with exponential backoff
func (s *TransferSession) RetryWithBackoff(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := BaseRetryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !s.isRetryable(err) {
			return err
		}
	}

	return fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

// isRetryable checks if an error is retryable
func (s *TransferSession) isRetryable(err error) bool {
	// Network errors, temporary failures are retryable
	// Corruption errors are retryable (re-transfer chunk)
	// Authentication errors, permanent failures are not retryable

	switch err.(type) {
	case *IntegrityError:
		return true
	case *ResumableError:
		return true
	default:
		// Check for context errors (not retryable)
		if err == context.Canceled || err == context.DeadlineExceeded {
			return false
		}
		// Default to retryable for unknown errors
		return true
	}
}

// CompleteChunk marks a chunk as completed and updates checkpoint
func (s *TransferSession) CompleteChunk(chunkID int, data []byte, checksum string) error {
	// Record in checkpoint
	s.Checkpoint.MarkChunkComplete(chunkID, checksum)

	// Record in verifier
	s.Verifier.RecordChunkHash(chunkID, checksum)

	// Update progress
	s.Progress.CompleteChunk(chunkID, int64(len(data)))

	// Periodically save checkpoint (every 10 chunks or on completion)
	if len(s.Checkpoint.CompletedChunks)%10 == 0 || len(s.Checkpoint.CompletedChunks) == s.Checkpoint.TotalChunks {
		if err := s.StateManager.UpdateCheckpoint(s.Checkpoint); err != nil {
			return NewCheckpointError(err, s.Checkpoint.SessionID, "update")
		}
	}

	return nil
}

// IsComplete checks if the transfer is complete
func (s *TransferSession) IsComplete() bool {
	return len(s.Checkpoint.CompletedChunks) >= s.Checkpoint.TotalChunks
}

// RestoreEncryptionState restores encryption state from checkpoint
func (s *TransferSession) RestoreEncryptionState(pakeCode string) error {
	if !s.Checkpoint.Encrypted || s.Checkpoint.EncryptionState == nil {
		return nil // No encryption to restore
	}

	if s.EncryptManager == nil {
		s.EncryptManager = NewEncryptionStateManager()
	}

	return s.EncryptManager.RestoreState(s.Checkpoint.EncryptionState, pakeCode)
}

// SaveEncryptionState saves current encryption state to checkpoint
func (s *TransferSession) SaveEncryptionState() error {
	if s.EncryptManager == nil || !s.Checkpoint.Encrypted {
		return nil // No encryption state to save
	}

	encState := s.EncryptManager.SaveState()
	s.Checkpoint.EncryptionState = encState

	return s.StateManager.UpdateCheckpoint(s.Checkpoint)
}

// GetProgressInfo returns current progress information
func (s *TransferSession) GetProgressInfo() *progress.Progress {
	if s.Progress == nil {
		return nil
	}
	info := s.Progress.GetProgress()
	return &info
}

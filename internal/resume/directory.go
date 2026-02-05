package resume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DirectoryCheckpoint extends Checkpoint for directory transfers
type DirectoryCheckpoint struct {
	Checkpoint
	Files          []FileProgress `json:"files"`
	CompletedFiles []string       `json:"completed_files"`
	FailedFiles    []string       `json:"failed_files"`
	RootPath       string         `json:"root_path"` // Original directory path
}

// FileProgress tracks progress for a single file in a directory transfer
type FileProgress struct {
	Path            string     `json:"path"`                    // Relative path from root
	Size            int64      `json:"size"`                    // File size in bytes
	CompletedChunks []int      `json:"completed_chunks"`        // Chunk IDs that are complete
	TotalChunks     int        `json:"total_chunks"`            // Total number of chunks
	Checksum        string     `json:"checksum"`                // SHA256 checksum
	StartedAt       time.Time  `json:"started_at"`              // When file transfer started
	CompletedAt     *time.Time `json:"completed_at,omitempty"`  // When file transfer completed
	Failed          bool       `json:"failed"`                  // Whether file transfer failed
	ErrorMessage    string     `json:"error_message,omitempty"` // Error message if failed
}

// DirectoryTransferSession manages a directory transfer with per-file progress
type DirectoryTransferSession struct {
	checkpoint   *DirectoryCheckpoint
	stateManager *TransferStateManager
	mu           sync.RWMutex

	// Callbacks
	onFileComplete func(filePath string)
	onFileFailed   func(filePath string, err error)
	onProgress     func(completedFiles, totalFiles int)
}

// NewDirectoryTransferSession creates a new directory transfer session
func NewDirectoryTransferSession(checkpoint *DirectoryCheckpoint, stateManager *TransferStateManager) *DirectoryTransferSession {
	return &DirectoryTransferSession{
		checkpoint:   checkpoint,
		stateManager: stateManager,
	}
}

// OnFileComplete registers a callback for when a file completes
func (s *DirectoryTransferSession) OnFileComplete(callback func(filePath string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFileComplete = callback
}

// OnFileFailed registers a callback for when a file fails
func (s *DirectoryTransferSession) OnFileFailed(callback func(filePath string, err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFileFailed = callback
}

// OnProgress registers a callback for overall progress updates
func (s *DirectoryTransferSession) OnProgress(callback func(completedFiles, totalFiles int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProgress = callback
}

// GetFileProgress returns the progress for a specific file
func (s *DirectoryTransferSession) GetFileProgress(filePath string) (*FileProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.checkpoint.Files {
		if s.checkpoint.Files[i].Path == filePath {
			return &s.checkpoint.Files[i], nil
		}
	}

	return nil, fmt.Errorf("file not found: %s", filePath)
}

// IsFileComplete checks if a file transfer is complete
func (s *DirectoryTransferSession) IsFileComplete(filePath string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, completedPath := range s.checkpoint.CompletedFiles {
		if completedPath == filePath {
			return true
		}
	}
	return false
}

// IsFileFailed checks if a file transfer has failed
func (s *DirectoryTransferSession) IsFileFailed(filePath string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, failedPath := range s.checkpoint.FailedFiles {
		if failedPath == filePath {
			return true
		}
	}
	return false
}

// MarkFileComplete marks a file as completed
func (s *DirectoryTransferSession) MarkFileComplete(filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already marked complete
	for _, completedPath := range s.checkpoint.CompletedFiles {
		if completedPath == filePath {
			return nil // Already complete
		}
	}

	// Add to completed files
	s.checkpoint.CompletedFiles = append(s.checkpoint.CompletedFiles, filePath)

	// Update file progress
	for i := range s.checkpoint.Files {
		if s.checkpoint.Files[i].Path == filePath {
			now := time.Now()
			s.checkpoint.Files[i].CompletedAt = &now
			break
		}
	}

	// Remove from failed files if present
	s.removeFromFailedFiles(filePath)

	// Save checkpoint
	if err := s.stateManager.UpdateCheckpoint(&s.checkpoint.Checkpoint); err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	// Trigger callback
	if s.onFileComplete != nil {
		go s.onFileComplete(filePath)
	}

	// Trigger progress callback
	if s.onProgress != nil {
		go s.onProgress(len(s.checkpoint.CompletedFiles), len(s.checkpoint.Files))
	}

	return nil
}

// MarkFileFailed marks a file as failed
func (s *DirectoryTransferSession) MarkFileFailed(filePath string, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already marked failed
	for _, failedPath := range s.checkpoint.FailedFiles {
		if failedPath == filePath {
			return nil // Already failed
		}
	}

	// Add to failed files
	s.checkpoint.FailedFiles = append(s.checkpoint.FailedFiles, filePath)

	// Update file progress
	for i := range s.checkpoint.Files {
		if s.checkpoint.Files[i].Path == filePath {
			s.checkpoint.Files[i].Failed = true
			if err != nil {
				s.checkpoint.Files[i].ErrorMessage = err.Error()
			}
			break
		}
	}

	// Save checkpoint
	if saveErr := s.stateManager.UpdateCheckpoint(&s.checkpoint.Checkpoint); saveErr != nil {
		return fmt.Errorf("failed to update checkpoint: %w", saveErr)
	}

	// Trigger callback
	if s.onFileFailed != nil {
		go s.onFileFailed(filePath, err)
	}

	return nil
}

// UpdateFileProgress updates the progress for a specific file
func (s *DirectoryTransferSession) UpdateFileProgress(filePath string, completedChunks []int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and update file progress
	found := false
	for i := range s.checkpoint.Files {
		if s.checkpoint.Files[i].Path == filePath {
			s.checkpoint.Files[i].CompletedChunks = completedChunks
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Save checkpoint
	if err := s.stateManager.UpdateCheckpoint(&s.checkpoint.Checkpoint); err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	return nil
}

// GetPendingFiles returns files that are not yet complete or failed
func (s *DirectoryTransferSession) GetPendingFiles() []FileProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pending := make([]FileProgress, 0)
	for _, file := range s.checkpoint.Files {
		if !s.IsFileComplete(file.Path) && !s.IsFileFailed(file.Path) {
			pending = append(pending, file)
		}
	}

	return pending
}

// GetFailedFiles returns files that have failed
func (s *DirectoryTransferSession) GetFailedFiles() []FileProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()

	failed := make([]FileProgress, 0)
	for _, file := range s.checkpoint.Files {
		if s.IsFileFailed(file.Path) {
			failed = append(failed, file)
		}
	}

	return failed
}

// RetryFailedFiles clears the failed status for all failed files
func (s *DirectoryTransferSession) RetryFailedFiles() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear failed files list
	s.checkpoint.FailedFiles = []string{}

	// Clear failed status from file progress
	for i := range s.checkpoint.Files {
		if s.checkpoint.Files[i].Failed {
			s.checkpoint.Files[i].Failed = false
			s.checkpoint.Files[i].ErrorMessage = ""
		}
	}

	// Save checkpoint
	if err := s.stateManager.UpdateCheckpoint(&s.checkpoint.Checkpoint); err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	return nil
}

// GetProgress returns overall directory transfer progress
func (s *DirectoryTransferSession) GetProgress() (completed, total int, percentage float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total = len(s.checkpoint.Files)
	completed = len(s.checkpoint.CompletedFiles)

	if total > 0 {
		percentage = float64(completed) / float64(total) * 100
	}

	return completed, total, percentage
}

// IsComplete checks if all files are complete
func (s *DirectoryTransferSession) IsComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.checkpoint.CompletedFiles) >= len(s.checkpoint.Files)
}

// HasFailures checks if any files have failed
func (s *DirectoryTransferSession) HasFailures() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.checkpoint.FailedFiles) > 0
}

// removeFromFailedFiles removes a file from the failed files list (must be called with lock held)
func (s *DirectoryTransferSession) removeFromFailedFiles(filePath string) {
	newFailed := make([]string, 0, len(s.checkpoint.FailedFiles))
	for _, failedPath := range s.checkpoint.FailedFiles {
		if failedPath != filePath {
			newFailed = append(newFailed, failedPath)
		}
	}
	s.checkpoint.FailedFiles = newFailed
}

// CreateDirectoryCheckpoint creates a new directory checkpoint by scanning a directory
func CreateDirectoryCheckpoint(sessionID, rootPath, destPath string, chunkSize int64) (*DirectoryCheckpoint, error) {
	// Scan directory for files
	files := make([]FileProgress, 0)

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Calculate total chunks for this file
		totalChunks := int(info.Size() / chunkSize)
		if info.Size()%chunkSize != 0 {
			totalChunks++
		}
		if totalChunks == 0 {
			totalChunks = 1
		}

		// Add file progress
		files = append(files, FileProgress{
			Path:            relPath,
			Size:            info.Size(),
			CompletedChunks: []int{},
			TotalChunks:     totalChunks,
			StartedAt:       time.Now(),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in directory: %s", rootPath)
	}

	// Calculate total size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
	}

	// Create base checkpoint
	now := time.Now()
	checkpoint := &DirectoryCheckpoint{
		Checkpoint: Checkpoint{
			SessionID:       sessionID,
			Version:         "1.0",
			SourcePath:      rootPath,
			DestinationPath: destPath,
			Direction:       "upload",
			TotalSize:       totalSize,
			ChunkSize:       chunkSize,
			TotalChunks:     0, // Will be sum of all file chunks
			CompletedChunks: []int{},
			Encrypted:       false,
			CreatedAt:       now,
			UpdatedAt:       now,
			ExpiresAt:       now.Add(24 * time.Hour),
		},
		Files:          files,
		CompletedFiles: []string{},
		FailedFiles:    []string{},
		RootPath:       rootPath,
	}

	// Calculate total chunks
	for _, file := range files {
		checkpoint.TotalChunks += file.TotalChunks
	}

	return checkpoint, nil
}

// SaveDirectoryCheckpoint saves a directory checkpoint to disk
func (m *TransferStateManager) SaveDirectoryCheckpoint(checkpoint *DirectoryCheckpoint) error {
	// Compute checkpoint hash
	checkpoint.UpdatedAt = time.Now()
	hash, err := checkpoint.ComputeHash()
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}
	checkpoint.CheckpointHash = hash

	// Marshal to JSON
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Write to file atomically
	checkpointPath := filepath.Join(m.baseDir, checkpoint.SessionID+".json")
	tempPath := checkpointPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, checkpointPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// LoadDirectoryCheckpoint loads a directory checkpoint from disk
func (m *TransferStateManager) LoadDirectoryCheckpoint(sessionID string) (*DirectoryCheckpoint, error) {
	checkpointPath := filepath.Join(m.baseDir, sessionID+".json")

	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var checkpoint DirectoryCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	// Verify checkpoint hash
	expectedHash := checkpoint.CheckpointHash
	actualHash, err := checkpoint.ComputeHash()
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash: %w", err)
	}

	if checkpoint.CheckpointHash != actualHash {
		return nil, &IntegrityError{
			Err:          fmt.Errorf("checkpoint hash mismatch"),
			ExpectedHash: expectedHash,
			ActualHash:   actualHash,
		}
	}

	return &checkpoint, nil
}

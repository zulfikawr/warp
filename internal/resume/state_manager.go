package resume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TransferStateManager handles checkpoint persistence and recovery
type TransferStateManager struct {
	baseDir     string // ~/.warp/transfers/
	sessions    map[string]*Checkpoint
	mu          sync.RWMutex
	maxSessions int // Default: 100
}

// NewTransferStateManager creates a new state manager
func NewTransferStateManager(baseDir string) (*TransferStateManager, error) {
	// Expand home directory if needed
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".warp", "transfers")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	return &TransferStateManager{
		baseDir:     baseDir,
		sessions:    make(map[string]*Checkpoint),
		maxSessions: 100,
	}, nil
}

// CreateCheckpoint initializes a new checkpoint for a transfer
func (m *TransferStateManager) CreateCheckpoint(opts CheckpointOptions) (*Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check session limit
	if len(m.sessions) >= m.maxSessions {
		return nil, fmt.Errorf("maximum sessions (%d) reached", m.maxSessions)
	}

	// Create checkpoint
	cp := NewCheckpoint(opts)

	// Compute and set hash
	if err := cp.UpdateHash(); err != nil {
		return nil, NewCheckpointError(err, cp.SessionID, "create")
	}

	// Save to disk
	if err := m.saveCheckpoint(cp); err != nil {
		return nil, NewCheckpointError(err, cp.SessionID, "create")
	}

	// Store in memory
	m.sessions[cp.SessionID] = cp

	return cp, nil
}

// LoadCheckpoint loads an existing checkpoint by session ID
func (m *TransferStateManager) LoadCheckpoint(sessionID string) (*Checkpoint, error) {
	m.mu.RLock()
	// Check memory cache first
	if cp, ok := m.sessions[sessionID]; ok {
		m.mu.RUnlock()
		return cp, nil
	}
	m.mu.RUnlock()

	// Load from disk
	checkpointPath := m.getCheckpointPath(sessionID)
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCheckpointNotFound
		}
		return nil, NewCheckpointError(err, sessionID, "load")
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, NewCheckpointError(err, sessionID, "load")
	}

	// Verify hash (tamper detection)
	valid, err := cp.VerifyHash()
	if err != nil {
		return nil, NewCheckpointError(err, sessionID, "load")
	}
	if !valid {
		return nil, NewCheckpointError(ErrCheckpointCorrupted, sessionID, "load")
	}

	// Check if expired
	if cp.IsExpired() {
		return nil, NewCheckpointError(ErrCheckpointExpired, sessionID, "load")
	}

	// Check version compatibility
	if cp.Version != CheckpointFormatVersion {
		return nil, NewCheckpointError(ErrInvalidCheckpointVersion, sessionID, "load")
	}

	// Store in memory cache
	m.mu.Lock()
	m.sessions[sessionID] = &cp
	m.mu.Unlock()

	return &cp, nil
}

// UpdateCheckpoint persists updated checkpoint state
func (m *TransferStateManager) UpdateCheckpoint(cp *Checkpoint) error {
	if cp == nil {
		return fmt.Errorf("checkpoint is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Update timestamp
	cp.UpdatedAt = time.Now()

	// Update hash
	if err := cp.UpdateHash(); err != nil {
		return NewCheckpointError(err, cp.SessionID, "update")
	}

	// Save to disk
	if err := m.saveCheckpoint(cp); err != nil {
		return NewCheckpointError(err, cp.SessionID, "update")
	}

	// Update memory cache
	m.sessions[cp.SessionID] = cp

	return nil
}

// DeleteCheckpoint removes a checkpoint after successful transfer
func (m *TransferStateManager) DeleteCheckpoint(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove from memory
	delete(m.sessions, sessionID)

	// Remove from disk
	checkpointPath := m.getCheckpointPath(sessionID)
	if err := os.Remove(checkpointPath); err != nil && !os.IsNotExist(err) {
		return NewCheckpointError(err, sessionID, "delete")
	}

	return nil
}

// ListResumable returns all resumable transfers
func (m *TransferStateManager) ListResumable() ([]*CheckpointSummary, error) {
	// Read all checkpoint files from disk
	files, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint directory: %w", err)
	}

	summaries := make([]*CheckpointSummary, 0)

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// Extract session ID from filename
		sessionID := file.Name()[:len(file.Name())-5] // Remove .json

		// Try to load checkpoint
		cp, err := m.LoadCheckpoint(sessionID)
		if err != nil {
			// Skip corrupted or expired checkpoints
			continue
		}

		summaries = append(summaries, cp.ToSummary())
	}

	return summaries, nil
}

// FindByPath finds a checkpoint matching source/destination paths
func (m *TransferStateManager) FindByPath(sourcePath, destPath string) (*Checkpoint, error) {
	summaries, err := m.ListResumable()
	if err != nil {
		return nil, err
	}

	for _, summary := range summaries {
		if summary.SourcePath == sourcePath && summary.DestinationPath == destPath {
			return m.LoadCheckpoint(summary.SessionID)
		}
	}

	return nil, ErrCheckpointNotFound
}

// CleanupStale removes checkpoints older than maxAge
func (m *TransferStateManager) CleanupStale(maxAge time.Duration) error {
	summaries, err := m.ListResumable()
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	cleaned := 0

	for _, summary := range summaries {
		if summary.UpdatedAt.Before(cutoff) || summary.CreatedAt.Before(cutoff) {
			if err := m.DeleteCheckpoint(summary.SessionID); err != nil {
				// Log error but continue cleanup
				continue
			}
			cleaned++
		}
	}

	return nil
}

// saveCheckpoint saves a checkpoint to disk with atomic write
func (m *TransferStateManager) saveCheckpoint(cp *Checkpoint) error {
	// Serialize to JSON
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Write to temporary file first (atomic write pattern)
	checkpointPath := m.getCheckpointPath(cp.SessionID)
	tempPath := checkpointPath + ".tmp"

	// Create temp file with restrictive permissions (0600)
	f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temp checkpoint file: %w", err)
	}

	// Write data
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}

	// Sync to disk
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to sync checkpoint: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to close checkpoint file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, checkpointPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename checkpoint file: %w", err)
	}

	return nil
}

// getCheckpointPath returns the file path for a checkpoint
func (m *TransferStateManager) getCheckpointPath(sessionID string) string {
	return filepath.Join(m.baseDir, sessionID+".json")
}

// GetBaseDir returns the base directory for checkpoints
func (m *TransferStateManager) GetBaseDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baseDir
}

// GetSessionCount returns the number of active sessions
func (m *TransferStateManager) GetSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

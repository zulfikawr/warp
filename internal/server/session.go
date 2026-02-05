package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/resume"
	"go.uber.org/zap"
)

// SessionManager handles the lifecycle of upload and download sessions
type SessionManager struct {
	uploadSessions sync.Map // sessionID -> *uploadSession
	activeUploads  sync.Map // filename -> *ProgressTracker
	chunkTimes     sync.Map // filename -> *chunkStat
	stateManager   *resume.TransferStateManager
	server         *Server // Back-reference to server for progress channel
}

// NewSessionManager creates a new SessionManager
func NewSessionManager(server *Server) *SessionManager {
	return &SessionManager{
		server: server,
	}
}

// SetStateManager sets the resume state manager
func (m *SessionManager) SetStateManager(sm *resume.TransferStateManager) {
	m.stateManager = sm
}

// GetStateManager returns the resume state manager
func (m *SessionManager) GetStateManager() *resume.TransferStateManager {
	return m.stateManager
}

// uploadSession represents a chunked upload session
type uploadSession struct {
	SessionID     string
	Filename      string
	TotalSize     int64
	TotalChunks   int
	ChunksWritten map[int]bool
	FilePath      string
	FileHandle    *os.File
	CreatedAt     time.Time
	StartTime     time.Time
	LastActivity  time.Time
	mu            sync.Mutex
	complete      bool
	server        *Server            // Reference to server for multi-file progress
	checkpoint    *resume.Checkpoint // Link to resume checkpoint
}

// isComplete checks if all chunks have been received
func (session *uploadSession) isComplete() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.complete
}

// chunkStat tracks chunk upload performance
type chunkStat struct {
	mu       sync.Mutex
	duration time.Duration
}

func (c *chunkStat) add(d time.Duration) time.Duration {
	c.mu.Lock()
	c.duration += d
	res := c.duration
	c.mu.Unlock()
	return res
}

// GetOrCreateSession retrieves an existing session or creates a new one
func (m *SessionManager) GetOrCreateSession(sessionID, filename string, totalSize int64, totalChunks int, destDir string) (*uploadSession, error) {
	// Check if session already exists (fast path)
	if val, ok := m.uploadSessions.Load(sessionID); ok {
		session := val.(*uploadSession)
		session.mu.Lock()
		session.LastActivity = time.Now()
		session.mu.Unlock()
		return session, nil
	}

	// Session doesn't exist - need to create it
	now := time.Now()
	session := &uploadSession{
		SessionID:     sessionID,
		Filename:      filename,
		TotalSize:     totalSize,
		TotalChunks:   totalChunks,
		ChunksWritten: make(map[int]bool),
		CreatedAt:     now,
		StartTime:     now,
		LastActivity:  now,
		server:        m.server,
	}

	sanitized, err := sanitizeFilename(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to sanitize filename: %w", err)
	}
	outPath := findUniqueFilename(destDir, sanitized)
	session.FilePath = outPath

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	if totalSize > 0 {
		if err := f.Truncate(totalSize); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("failed to pre-allocate space: %w", err)
		}
	}

	session.FileHandle = f

	// Atomically store the session - if another goroutine created it first, use theirs
	if actual, loaded := m.uploadSessions.LoadOrStore(sessionID, session); loaded {
		// Another goroutine created the session first, close our file and use theirs
		_ = f.Close()
		_ = os.Remove(outPath)
		return actual.(*uploadSession), nil
	}

	// Session was successfully stored by LoadOrStore above
	return session, nil
}

// GetSession retrieves an existing session
func (m *SessionManager) GetSession(sessionID string) (*uploadSession, bool) {
	val, ok := m.uploadSessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	return val.(*uploadSession), true
}

// RangeActiveUploads iterates over all active upload trackers
func (m *SessionManager) RangeActiveUploads(f func(key, value interface{}) bool) {
	m.activeUploads.Range(f)
}

// CleanupSession closes and removes an upload session
func (m *SessionManager) CleanupSession(sessionID string) {
	if val, ok := m.uploadSessions.LoadAndDelete(sessionID); ok {
		session := val.(*uploadSession)
		session.mu.Lock()
		if session.FileHandle != nil {
			_ = session.FileHandle.Close()
		}
		session.mu.Unlock()
	}
}

// CleanupStaleSessions removes sessions that haven't been active recently
func (m *SessionManager) CleanupStaleSessions() {
	staleThreshold := StaleSessionThreshold
	m.uploadSessions.Range(func(key, value interface{}) bool {
		session := value.(*uploadSession)
		session.mu.Lock()
		isStale := time.Since(session.LastActivity) > staleThreshold
		session.mu.Unlock()

		if isStale {
			sessionID := key.(string)
			logging.Info("Cleaning up stale session", zap.String("session_id", sessionID[:8]))
			m.CleanupSession(sessionID)
		}
		return true
	})
}

// AddChunkDuration adds chunk upload duration for performance tracking
func (m *SessionManager) AddChunkDuration(name string, d time.Duration) time.Duration {
	cs := m.GetChunkStat(name)
	return cs.add(d)
}

// GetChunkStat gets or creates chunk statistics for a file
func (m *SessionManager) GetChunkStat(name string) *chunkStat {
	val, _ := m.chunkTimes.LoadOrStore(name, &chunkStat{})
	return val.(*chunkStat)
}

// PersistUploadSession saves an upload session to a checkpoint file
func (m *SessionManager) PersistUploadSession(session *uploadSession, noEncrypt bool) error {
	if m.stateManager == nil {
		return nil // State manager not initialized
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Create or update checkpoint
	if session.checkpoint == nil {
		// Create new checkpoint
		opts := resume.CheckpointOptions{
			SessionID:       session.SessionID,
			SourcePath:      session.Filename, // Original filename
			DestinationPath: session.FilePath,
			Direction:       "upload",
			TotalSize:       session.TotalSize,
			ChunkSize:       int64(session.TotalSize / int64(session.TotalChunks)),
			TotalChunks:     session.TotalChunks,
			Encrypted:       !noEncrypt,
		}

		cp, err := m.stateManager.CreateCheckpoint(opts)
		if err != nil {
			return fmt.Errorf("failed to create checkpoint: %w", err)
		}
		session.checkpoint = cp
		metrics.ActiveCheckpoints.Inc()
	}

	// Update checkpoint with current progress
	completedChunks := make([]int, 0, len(session.ChunksWritten))
	for chunkID := range session.ChunksWritten {
		completedChunks = append(completedChunks, chunkID)
	}
	session.checkpoint.CompletedChunks = completedChunks
	session.checkpoint.UpdatedAt = time.Now()

	// Save checkpoint
	if err := m.stateManager.UpdateCheckpoint(session.checkpoint); err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	return nil
}

// LoadUploadSessions loads persisted upload sessions from checkpoint files on startup
func (m *SessionManager) LoadUploadSessions() error {
	if m.stateManager == nil {
		return nil // State manager not initialized
	}

	loadStart := time.Now()
	summaries, err := m.stateManager.ListResumable()
	if err != nil {
		metrics.CheckpointLoadErrors.Inc()
		return fmt.Errorf("failed to list resumable sessions: %w", err)
	}

	loaded := 0
	for _, summary := range summaries {
		// Only load upload sessions
		if summary.Direction != "upload" {
			continue
		}

		// Load full checkpoint
		cp, err := m.stateManager.LoadCheckpoint(summary.SessionID)
		if err != nil {
			logging.Warn("Failed to load checkpoint", zap.String("session_id", summary.SessionID[:8]), zap.Error(err))
			metrics.CheckpointLoadErrors.Inc()
			continue
		}

		// Check if checkpoint is expired (24 hours)
		if time.Since(cp.UpdatedAt) > 24*time.Hour {
			logging.Info("Skipping expired checkpoint", zap.String("session_id", summary.SessionID[:8]))
			_ = m.stateManager.DeleteCheckpoint(summary.SessionID)
			metrics.CheckpointCleanups.WithLabelValues("expired").Inc()
			continue
		}

		// Recreate upload session
		session := &uploadSession{
			SessionID:     cp.SessionID,
			Filename:      filepath.Base(cp.SourcePath),
			TotalSize:     cp.TotalSize,
			TotalChunks:   cp.TotalChunks,
			ChunksWritten: make(map[int]bool),
			FilePath:      cp.DestinationPath,
			CreatedAt:     cp.CreatedAt,
			StartTime:     cp.CreatedAt,
			LastActivity:  cp.UpdatedAt,
			server:        m.server,
			checkpoint:    cp,
		}

		// Restore completed chunks
		for _, chunkID := range cp.CompletedChunks {
			session.ChunksWritten[chunkID] = true
		}

		// Check if session is complete
		if len(session.ChunksWritten) >= session.TotalChunks {
			session.complete = true
		}

		// Reopen file handle if not complete
		if !session.complete {
			f, err := os.OpenFile(cp.DestinationPath, os.O_CREATE|os.O_RDWR, 0o600)
			if err != nil {
				logging.Warn("Failed to reopen file for session", zap.String("session_id", summary.SessionID[:8]), zap.Error(err))
				metrics.CheckpointLoadErrors.Inc()
				continue
			}
			session.FileHandle = f
		}

		// Store session
		m.uploadSessions.Store(cp.SessionID, session)
		loaded++
		metrics.CheckpointLoadsTotal.Inc()
		metrics.ActiveCheckpoints.Inc()
		metrics.ResumedTransfers.WithLabelValues("upload").Inc()
		logging.Info("Restored upload session",
			zap.String("session_id", summary.SessionID[:8]),
			zap.String("filename", session.Filename),
			zap.Int("completed_chunks", len(session.ChunksWritten)),
			zap.Int("total_chunks", session.TotalChunks))
	}

	if loaded > 0 {
		loadDuration := time.Since(loadStart).Seconds()
		metrics.CheckpointLoadDuration.Observe(loadDuration)
		logging.Info("Loaded upload sessions from checkpoints", zap.Int("count", loaded))
	}

	return nil
}

// CleanupExpiredCheckpoints removes checkpoint files older than 24 hours
func (m *SessionManager) CleanupExpiredCheckpoints() {
	if m.stateManager == nil {
		return
	}

	if err := m.stateManager.CleanupStale(24 * time.Hour); err != nil {
		logging.Warn("Failed to cleanup expired checkpoints", zap.Error(err))
	}
}

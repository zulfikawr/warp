package server

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/zulfikawr/warp/internal/logging"
	"go.uber.org/zap"
)

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
	server        *Server // Reference to server for multi-file progress
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

// getOrCreateSession retrieves an existing session or creates a new one
func (s *Server) getOrCreateSession(sessionID, filename string, totalSize int64, totalChunks int, destDir string) (*uploadSession, error) {
	// Check if session already exists (fast path)
	if val, ok := s.uploadSessions.Load(sessionID); ok {
		session := val.(*uploadSession)
		session.mu.Lock()
		session.LastActivity = time.Now()
		session.mu.Unlock()
		return session, nil
	}

	// Session doesn't exist - need to create it
	// Use sync.Map's LoadOrStore to prevent race condition
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
		server:        s,
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
	if actual, loaded := s.uploadSessions.LoadOrStore(sessionID, session); loaded {
		// Another goroutine created the session first, close our file and use theirs
		_ = f.Close()
		_ = os.Remove(outPath)
		return actual.(*uploadSession), nil
	}

	if s.multiFileDisplay == nil {
		s.multiFileDisplay = &MultiFileProgress{
			files:      make(map[string]*FileProgress),
			fileOrder:  make([]string, 0),
			startTime:  now,
			lastUpdate: now,
		}
	}

	s.multiFileDisplay.mu.Lock()
	if _, exists := s.multiFileDisplay.files[sessionID]; !exists {
		s.multiFileDisplay.files[sessionID] = &FileProgress{
			filename:  filename,
			size:      totalSize,
			startTime: now,
		}
		s.multiFileDisplay.fileOrder = append(s.multiFileDisplay.fileOrder, sessionID)
		// Accumulate total size for overall progress calculation
		s.multiFileDisplay.totalSize += totalSize
	}
	s.multiFileDisplay.mu.Unlock()

	// Session was successfully stored by LoadOrStore above
	return session, nil
}

// cleanupSession closes and removes an upload session
func (s *Server) cleanupSession(sessionID string) {
	if val, ok := s.uploadSessions.LoadAndDelete(sessionID); ok {
		session := val.(*uploadSession)
		session.mu.Lock()
		if session.FileHandle != nil {
			_ = session.FileHandle.Close()
		}
		session.mu.Unlock()
	}
}

// cleanupStaleSessions removes sessions that haven't been active recently
func (s *Server) cleanupStaleSessions() {
	staleThreshold := StaleSessionThreshold
	s.uploadSessions.Range(func(key, value interface{}) bool {
		session := value.(*uploadSession)
		session.mu.Lock()
		isStale := time.Since(session.LastActivity) > staleThreshold
		session.mu.Unlock()

		if isStale {
			sessionID := key.(string)
			logging.Info("Cleaning up stale session", zap.String("session_id", sessionID[:8]))
			s.cleanupSession(sessionID)
		}
		return true
	})
}

// addChunkDuration adds chunk upload duration for performance tracking
func (s *Server) addChunkDuration(name string, d time.Duration) time.Duration {
	cs := s.getChunkStat(name)
	return cs.add(d)
}

// getChunkStat gets or creates chunk statistics for a file
func (s *Server) getChunkStat(name string) *chunkStat {
	val, _ := s.chunkTimes.LoadOrStore(name, &chunkStat{})
	return val.(*chunkStat)
}

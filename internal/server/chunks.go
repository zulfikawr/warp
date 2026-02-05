package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/progress"
	"go.uber.org/zap"
)

// handleParallelChunk processes a single chunk in a parallel upload session
func (s *Server) handleParallelChunk(w http.ResponseWriter, r *http.Request, filename, sessionID, chunkIDStr, chunkTotalStr, offsetStr, dest string) {
	chunkStartTime := time.Now()
	metrics.ParallelUploadWorkers.Inc()
	defer metrics.ParallelUploadWorkers.Dec()

	// Validate session ID
	if err := ValidateSessionID(sessionID); err != nil {
		logging.Warn("Invalid session ID", zap.String("session_id", sessionID), zap.Error(err))
		http.Error(w, fmt.Sprintf("invalid session ID: %v", err), http.StatusBadRequest)
		return
	}

	// Parse chunk metadata
	chunkID, err := strconv.Atoi(chunkIDStr)
	if err != nil {
		http.Error(w, "invalid chunk id", http.StatusBadRequest)
		return
	}

	chunkTotal, err := strconv.Atoi(chunkTotalStr)
	if err != nil {
		http.Error(w, "invalid chunk total", http.StatusBadRequest)
		return
	}

	// Validate total chunks
	if err := ValidateTotalChunks(chunkTotal); err != nil {
		logging.Warn("Invalid total chunks", zap.Int("total", chunkTotal), zap.Error(err))
		http.Error(w, fmt.Sprintf("invalid total chunks: %v", err), http.StatusBadRequest)
		return
	}

	// Validate chunk ID
	if err := ValidateChunkID(chunkID, chunkTotal); err != nil {
		logging.Warn("Invalid chunk ID", zap.Int("chunk_id", chunkID), zap.Int("total", chunkTotal), zap.Error(err))
		http.Error(w, fmt.Sprintf("invalid chunk ID: %v", err), http.StatusBadRequest)
		return
	}

	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid offset", http.StatusBadRequest)
		return
	}

	totalSize := int64(0)
	if totalHeader := r.Header.Get("X-Upload-Total"); totalHeader != "" {
		totalSize, _ = strconv.ParseInt(totalHeader, 10, 64)
	}

	// Validate offset if we know the total size
	if totalSize > 0 {
		if err := ValidateOffset(offset, totalSize); err != nil {
			logging.Warn("Invalid offset", zap.Int64("offset", offset), zap.Int64("total_size", totalSize), zap.Error(err))
			http.Error(w, fmt.Sprintf("invalid offset: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Validate chunk size (content length)
	if r.ContentLength > 0 {
		if err := ValidateChunkSize(r.ContentLength); err != nil {
			logging.Warn("Invalid chunk size", zap.Int64("size", r.ContentLength), zap.Error(err))
			http.Error(w, fmt.Sprintf("invalid chunk size: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Get or create upload session
	session, err := s.SessionMgr.GetOrCreateSession(sessionID, filename, totalSize, chunkTotal, dest)
	if err != nil {
		logging.Error("Failed to create session", zap.String("session_id", sessionID[:8]), zap.String("filename", filename), zap.Error(err))
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	// Read chunk data
	chunkData, err := io.ReadAll(io.LimitReader(r.Body, r.ContentLength))
	if err != nil {
		logging.Error("Failed to read chunk", zap.Int("chunk_id", chunkID), zap.String("session_id", sessionID[:8]), zap.Error(err))
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	// Write chunk to file
	if err := session.writeChunk(chunkID, offset, chunkData); err != nil {
		logging.Error("Failed to write chunk", zap.Int("chunk_id", chunkID), zap.String("session_id", sessionID[:8]), zap.Error(err))
		metrics.ChunkUploadsTotal.WithLabelValues("error").Inc()
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}

	// Record chunk metrics
	chunkDuration := time.Since(chunkStartTime).Seconds()
	metrics.ChunkUploadDuration.Observe(chunkDuration)
	metrics.ChunkUploadsTotal.WithLabelValues("success").Inc()

	// Track cumulative chunk timing for this file
	s.SessionMgr.AddChunkDuration(filename, time.Since(chunkStartTime))

	// Persist session checkpoint (every 5 chunks or on completion)
	// Use non-blocking goroutine to avoid impacting upload performance
	shouldPersist := len(session.ChunksWritten)%5 == 0 || session.isComplete()
	if shouldPersist {
		// Clone session data for async persistence
		session.mu.Lock()
		completedChunks := make([]int, 0, len(session.ChunksWritten))
		for chunkID := range session.ChunksWritten {
			completedChunks = append(completedChunks, chunkID)
		}
		sessionCopy := &uploadSession{
			SessionID:     session.SessionID,
			Filename:      session.Filename,
			TotalSize:     session.TotalSize,
			TotalChunks:   session.TotalChunks,
			FilePath:      session.FilePath,
			CreatedAt:     session.CreatedAt,
			LastActivity:  session.LastActivity,
			checkpoint:    session.checkpoint,
			ChunksWritten: make(map[int]bool),
		}
		for _, id := range completedChunks {
			sessionCopy.ChunksWritten[id] = true
		}
		session.mu.Unlock()

		// Persist asynchronously
		go func() {
			persistStart := time.Now()
			if err := s.SessionMgr.PersistUploadSession(sessionCopy, s.NoEncrypt); err != nil {
				logging.Warn("Failed to persist upload session",
					zap.String("session_id", sessionID[:8]),
					zap.Error(err))
				metrics.CheckpointSaveErrors.Inc()
			} else {
				persistDuration := time.Since(persistStart).Seconds()
				metrics.CheckpointSaveDuration.Observe(persistDuration)
				metrics.CheckpointSavesTotal.Inc()
			}
		}()
	}

	// Build response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"success":  true,
		"filename": filepath.Base(session.FilePath),
		"received": len(chunkData),
		"chunk_id": chunkID,
		"complete": session.isComplete(),
	}

	_ = json.NewEncoder(w).Encode(response)

	// Cleanup if complete
	if session.isComplete() {
		// Close file handle but keep session for a bit (for late retries)
		session.mu.Lock()
		if session.FileHandle != nil {
			_ = session.FileHandle.Sync()
			_ = session.FileHandle.Close()
			session.FileHandle = nil
		}
		session.mu.Unlock()

		// Delete checkpoint since transfer is complete
		stateMgr := s.SessionMgr.GetStateManager()
		if stateMgr != nil && session.checkpoint != nil {
			if err := stateMgr.DeleteCheckpoint(sessionID); err != nil {
				logging.Warn("Failed to delete completed checkpoint", zap.String("session_id", sessionID[:8]), zap.Error(err))
			} else {
				metrics.CheckpointCleanups.WithLabelValues("completed").Inc()
				metrics.ActiveCheckpoints.Dec()
			}
		}

		// Force final progress update to ensure it reaches 100%
		if s.ProgressChan != nil {
			s.ProgressChan <- progress.Progress{
				FileName:         filepath.Base(session.FilePath),
				TransferredBytes: session.TotalSize,
				TotalBytes:       session.TotalSize,
				IsComplete:       true,
			}
		}

		// Schedule cleanup after a delay
		go func() {
			time.Sleep(30 * time.Second)
			s.SessionMgr.CleanupSession(sessionID)
		}()
	}
}

// writeChunk writes a chunk of data to the appropriate file position
func (session *uploadSession) writeChunk(chunkID int, offset int64, data []byte) error {
	session.mu.Lock()

	// Check if chunk was already written (idempotent)
	alreadyWritten := session.ChunksWritten[chunkID]
	if !alreadyWritten {
		n, err := session.FileHandle.WriteAt(data, offset)
		if err != nil {
			session.mu.Unlock()
			return fmt.Errorf("write failed: %w", err)
		}
		if n != len(data) {
			session.mu.Unlock()
			return fmt.Errorf("incomplete write: wrote %d of %d bytes", n, len(data))
		}

		session.ChunksWritten[chunkID] = true
		session.LastActivity = time.Now()
	}

	// Emit progress event
	if session.server != nil && session.server.ProgressChan != nil {
		isComplete := len(session.ChunksWritten) >= session.TotalChunks
		var receivedBytes int64
		if isComplete {
			receivedBytes = session.TotalSize
		} else {
			chunkSize := session.TotalSize / int64(session.TotalChunks)
			receivedBytes = int64(len(session.ChunksWritten)) * chunkSize
			if receivedBytes > session.TotalSize {
				receivedBytes = session.TotalSize
			}
		}

		// Calculate speed and ETA
		elapsed := time.Since(session.StartTime).Seconds()
		var speed float64
		var eta time.Duration
		if elapsed > 0 && receivedBytes > 0 {
			speed = float64(receivedBytes) / elapsed // bytes per second
			if speed > 0 && receivedBytes < session.TotalSize {
				remaining := session.TotalSize - receivedBytes
				etaSeconds := float64(remaining) / speed
				eta = time.Duration(etaSeconds * float64(time.Second))
			}
		}

		session.server.ProgressChan <- progress.Progress{
			FileName:         filepath.Base(session.FilePath),
			TransferredBytes: receivedBytes,
			TotalBytes:       session.TotalSize,
			IsComplete:       isComplete,
			SpeedBytesPerSec: speed,
			ETA:              eta,
			StartTime:        session.StartTime,
		}
	}

	session.mu.Unlock()
	return nil
}

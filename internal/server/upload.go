package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/protocol"
	"github.com/zulfikawr/warp/internal/ui"
	"go.uber.org/zap"
)

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	// Expect /u/{token} or /u/{token}/manifest
	seg := strings.TrimPrefix(r.URL.Path, protocol.UploadPathPrefix)
	seg = strings.TrimPrefix(seg, "/")
	parts := strings.Split(seg, "/")
	if len(parts) == 0 || parts[0] != s.Token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if len(parts) > 1 && parts[1] == "manifest" {
		s.handleManifest(w, r)
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, uploadPageHTML)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// FAST PATH: Raw binary stream (zero parsing overhead)
	if filename := r.Header.Get("X-File-Name"); filename != "" {
		s.handleRawUpload(w, r, filename)
		return
	}

	// Basic upload security and limits: limit request size if Content-Length present
	// and prevent caching of responses.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Ensure upload dir exists
	dest := s.UploadDir
	if dest == "" {
		dest = "."
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	// Set max upload size to 10GB for large file support
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize) // 10GB limit

	// Check available disk space (best effort)
	if r.ContentLength > 0 {
		if err := checkDiskSpace(dest, r.ContentLength); err != nil {
			logging.Warn("Disk space check failed", zap.Error(err))
			http.Error(w, "insufficient disk space", http.StatusInsufficientStorage)
			return
		}
	}

	// Use streaming multipart reader for true zero-copy I/O
	// This reads directly from network to disk without buffering entire files in RAM
	reader, err := r.MultipartReader()
	if err != nil {
		logging.Error("Failed to create multipart reader", zap.Error(err))
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	requestStart := time.Now()

	type savedInfo struct {
		Name string
		Size int64
	}
	var saved []savedInfo

	// Stream each file part directly to disk
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break // No more parts
		}
		if err != nil {
			logging.Error("Failed to read next part", zap.Error(err))
			http.Error(w, "upload error", http.StatusInternalServerError)
			return
		}

		// Skip non-file fields
		if part.FileName() == "" {
			_ = part.Close()
			continue
		}

		// Track active upload
		metrics.ActiveUploads.Inc()
		metrics.ActiveTransfers.Inc()

		// Limit each part to prevent memory exhaustion (DoS protection)
		const MaxPartSize = 10 << 30 // 10GB per part
		limitedPart := io.LimitReader(part, MaxPartSize)

		// Sanitize filename to prevent directory traversal
		name := filepath.Base(part.FileName())
		if name == "." || name == ".." {
			_ = part.Close()
			continue
		}

		// Use unique filename to prevent overwriting existing files
		outPath := findUniqueFilename(dest, name)
		filename := filepath.Base(outPath)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			logging.Error("Failed to create file", zap.String("filename", name), zap.Error(err))
			_ = part.Close()
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}

		// Use adaptive buffer sizing - default to 1MB for multipart uploads
		bufferSize := protocol.GetOptimalBufferSize(1024 * 1024) // Default to 1MB for unknown sizes
		bufPtr := getBuffer(bufferSize)
		defer putBuffer(bufPtr) // Ensure buffer is returned even on error
		buf := *bufPtr
		// Use limited reader to prevent memory exhaustion
		n, err := io.CopyBuffer(out, limitedPart, buf)
		cerr := out.Close()
		_ = part.Close()

		if err != nil || cerr != nil {
			logging.Error("Failed to write file", zap.String("filename", name), zap.NamedError("write_err", err), zap.NamedError("close_err", cerr))
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}

		duration := time.Since(requestStart).Seconds()
		mbps := 0.0
		if duration > 0 {
			mbps = (float64(n) * 8) / (duration * 1_000_000)
		}
		logging.Info("File received", zap.String("filename", filename), zap.String("size", ui.FormatBytes(n)), zap.Float64("duration", duration), zap.Float64("mbps", mbps))
		saved = append(saved, savedInfo{Name: filename, Size: n})

		// Record metrics for this file
		fileExt := strings.ToLower(filepath.Ext(filename))
		if fileExt == "" {
			fileExt = "no_ext"
		}
		metrics.UploadDuration.WithLabelValues(fileExt).Observe(duration)
		metrics.UploadSize.WithLabelValues(fileExt).Observe(float64(n))
		metrics.UploadThroughput.WithLabelValues(fileExt).Observe(mbps)
		metrics.UploadsTotal.WithLabelValues(fileExt, "success").Inc()

		// Decrement active counters
		metrics.ActiveUploads.Dec()
		metrics.ActiveTransfers.Dec()

		requestStart = time.Now() // Reset for next file
	}

	if len(saved) == 0 {
		http.Error(w, "no file provided", http.StatusBadRequest)
		return
	}

	// Simple success response (client already manages state)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// sanitizeFilename validates and cleans filenames to prevent security issues

func (s *Server) handleRawUpload(w http.ResponseWriter, r *http.Request, encodedFilename string) {
	const MaxUploadSize = 10 << 30 // 10GB
	if r.ContentLength > MaxUploadSize {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Decode filename from URL encoding
	filename, err := url.QueryUnescape(encodedFilename)
	if err != nil {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	name, err := sanitizeFilename(filename)
	if err != nil {
		logging.Warn("Invalid filename", zap.String("filename", filename), zap.Error(err))
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	dest := s.UploadDir
	if dest == "" {
		dest = "."
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	// Chunk/resume metadata
	sessionIDHeader := r.Header.Get("X-Upload-Session")
	offsetHeader := r.Header.Get("X-Upload-Offset")
	chunkIDHeader := r.Header.Get("X-Chunk-Id")
	chunkTotalHeader := r.Header.Get("X-Chunk-Total")

	// Validate session ID if provided
	if sessionIDHeader != "" {
		if err := ValidateSessionID(sessionIDHeader); err != nil {
			logging.Warn("Invalid session ID", zap.Error(err))
			http.Error(w, fmt.Sprintf("invalid session ID: %v", err), http.StatusBadRequest)
			return
		}
	}

	isParallelChunk := sessionIDHeader != "" && chunkIDHeader != "" && chunkTotalHeader != ""
	chunked := offsetHeader != ""
	var uploadOffset int64
	var totalSize int64

	// Validate Content-Length
	if r.ContentLength < 0 || r.ContentLength > MaxUploadSize {
		http.Error(w, "invalid or missing content length", http.StatusBadRequest)
		return
	}

	// Check available disk space
	if err := checkDiskSpace(dest, r.ContentLength); err != nil {
		logging.Warn("Disk space check failed", zap.Error(err))
		http.Error(w, "insufficient disk space", http.StatusInsufficientStorage)
		return
	}

	// Handle parallel chunk upload (new fast path)
	if isParallelChunk {
		s.handleParallelChunk(w, r, name, sessionIDHeader, chunkIDHeader, chunkTotalHeader, offsetHeader, dest)
		return
	}

	// Handle legacy sequential chunked upload
	if chunked {
		var err error
		uploadOffset, err = strconv.ParseInt(offsetHeader, 10, 64)
		if err != nil || uploadOffset < 0 {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		if totalHeader := r.Header.Get("X-Upload-Total"); totalHeader != "" {
			if total, err := strconv.ParseInt(totalHeader, 10, 64); err == nil && total > 0 {
				totalSize = total
				// Validate offset against total size
				if err := ValidateOffset(uploadOffset, totalSize); err != nil {
					logging.Warn("Invalid offset", zap.Error(err))
					http.Error(w, fmt.Sprintf("invalid offset: %v", err), http.StatusBadRequest)
					return
				}
			}
		}
	}

	var outPath string
	var actualFilename string
	if chunked {
		// For chunked uploads, use consistent filename
		outPath = filepath.Join(dest, name)
		actualFilename = filepath.Base(outPath)
		// Validate existing file size matches expected offset
		if fi, err := os.Stat(outPath); err == nil {
			if fi.Size() != uploadOffset {
				// File exists but offset doesn't match
				// This shouldn't happen with proper parallel chunk handling
				// Return error asking client to use proper session-based upload
				logging.Warn("Legacy chunk upload with offset mismatch",
					zap.Int64("file_offset", fi.Size()),
					zap.Int64("expected_offset", uploadOffset),
					zap.String("filename", actualFilename))
				http.Error(w, "offset mismatch - use session-based parallel upload", http.StatusConflict)
				return
			}
		}
	} else {
		outPath = findUniqueFilename(dest, name)
		actualFilename = filepath.Base(outPath)
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		logging.Error("Failed to open file", zap.String("filename", actualFilename), zap.Error(err))
		http.Error(w, "disk error", http.StatusInternalServerError)
		return
	}

	// Pre-allocate when total known and offset zero
	if chunked {
		if totalSize > 0 && uploadOffset == 0 {
			_ = f.Truncate(totalSize)
		}
		if _, err := f.Seek(uploadOffset, 0); err != nil {
			_ = f.Close()
			http.Error(w, "seek error", http.StatusInternalServerError)
			return
		}
	} else if r.ContentLength > 0 {
		_ = f.Truncate(r.ContentLength)
	}

	// Hijack connection for raw TCP copy
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = f.Close()
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		_ = f.Close()
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}
	// Consolidated cleanup to prevent race conditions
	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				logging.Warn("Failed to close file", zap.Error(err))
			}
		}
		if conn != nil {
			if err := conn.Close(); err != nil {
				logging.Warn("Failed to close connection", zap.Error(err))
			}
		}
	}()

	// Manual deadlines since http.Server timeouts no longer apply post-hijack
	_ = conn.SetReadDeadline(time.Now().Add(time.Hour))

	if chunked && uploadOffset == 0 {
		if totalSize > 0 {
			logging.Info("Receiving file", zap.String("filename", actualFilename), zap.String("size", ui.FormatBytes(totalSize)))
		} else {
			logging.Info("Receiving file", zap.String("filename", actualFilename))
		}
	}
	if !chunked {
		if r.ContentLength > 0 {
			logging.Info("Receiving file", zap.String("filename", actualFilename), zap.String("size", ui.FormatBytes(r.ContentLength)))
		} else {
			logging.Info("Receiving file", zap.String("filename", actualFilename))
		}
	}

	// Use adaptive buffer sizing based on expected file size
	expectedSize := totalSize
	if expectedSize <= 0 && r.ContentLength > 0 {
		expectedSize = r.ContentLength
	}
	bufferSize := protocol.GetOptimalBufferSize(expectedSize)
	bufPtr := getBuffer(bufferSize)
	buf := *bufPtr
	defer putBuffer(bufPtr)

	// Enforce size limit even when Content-Length is provided
	maxRead := r.ContentLength
	if maxRead <= 0 || maxRead > MaxUploadSize {
		maxRead = MaxUploadSize
	}

	// Limit reader to prevent over-reading
	reader := io.LimitReader(bufrw, maxRead)

	n, err := io.CopyBuffer(f, reader, buf)
	if err != nil && !errors.Is(err, io.EOF) {
		logging.Error("Upload stream failed", zap.String("filename", actualFilename), zap.Error(err))
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, _ = bufrw.WriteString("HTTP/1.1 500 Internal Server Error\r\nConnection: close\r\n\r\n")
		_ = bufrw.Flush()
		return
	}

	// Manual HTTP/1.1 response
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	response := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nConnection: close\r\n\r\n{\"success\":true,\"filename\":\"%s\",\"size\":%d}", actualFilename, n)
	_, _ = bufrw.WriteString(response)
	_ = bufrw.Flush()
}

// addChunkDuration adds chunk upload duration for performance tracking

package server

import (
	"compress/gzip"
	"fmt"
	"github.com/klauspost/compress/zstd"
	"github.com/zulfikawr/warp/internal/logging"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/protocol"
	"github.com/zulfikawr/warp/internal/ui"
)

// handleDownload serves the file or directory for download with various optimizations
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	metrics.ActiveDownloads.Inc()
	metrics.ActiveTransfers.Inc()
	defer func() {
		metrics.ActiveDownloads.Dec()
		metrics.ActiveTransfers.Dec()
	}()

	// Expect /d/{token}
	p := strings.TrimPrefix(r.URL.Path, protocol.PathPrefix)
	if p != s.Token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// If TextContent is set, serve text securely
	if s.TextContent != "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.TextContent)))
		// Prevent caching of sensitive text content
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		_, _ = w.Write([]byte(s.TextContent))
		return
	}

	fi, err := os.Stat(s.SrcPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fi.IsDir() {
		w.Header().Set("Content-Type", "application/zip")
		name := filepath.Base(s.SrcPath) + ".zip"
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
		// If client supports zstd or gzip, wrap the writer so the transmitted zip is compressed
		enc := strings.ToLower(r.Header.Get("Accept-Encoding"))
		if strings.Contains(enc, "zstd") {
			w.Header().Set("Content-Encoding", "zstd")
			// Let transfer encoding decide length
			w.Header().Del("Content-Length")
			zw, err := zstd.NewWriter(w)
			if err != nil {
				http.Error(w, "zip error", http.StatusInternalServerError)
				return
			}
			defer zw.Close()
			if err := ZipDirectoryWithProgress(zw, s.SrcPath, os.Stderr); err != nil {
				http.Error(w, "zip error", http.StatusInternalServerError)
			}
			return
		}
		if strings.Contains(enc, "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Del("Content-Length")
			gw := gzip.NewWriter(w)
			defer gw.Close()
			if err := ZipDirectoryWithProgress(gw, s.SrcPath, os.Stderr); err != nil {
				http.Error(w, "zip error", http.StatusInternalServerError)
			}
			return
		}
		// Default: no outer encoding, stream raw zip
		if err := ZipDirectoryWithProgress(w, s.SrcPath, os.Stderr); err != nil {
			http.Error(w, "zip error", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(s.SrcPath)))

	// Check if client supports compression and file is compressible
	encHeader := r.Header.Get("Accept-Encoding")
	acceptsZstd := strings.Contains(encHeader, "zstd")
	acceptsGzip := strings.Contains(encHeader, "gzip")
	// Prefer zstd when available
	shouldCompress := (acceptsZstd || acceptsGzip) && isCompressible(s.SrcPath) && fi.Size() > 1024 // Only compress files > 1KB

	// Support resumable downloads via Range headers
	f, err := os.Open(s.SrcPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()

	// Check if we have a shared key for this token
	var reader io.Reader = f
	var isEncrypted bool
	if val, ok := s.tokenKeys.Load(s.Token); ok {
		key := val.([]byte)
		logging.Info("Found key for token in tokenKeys", zap.String("token", s.Token), zap.Int("keyLen", len(key)))
		encReader, err := crypto.NewEncryptReader(f, key)
		if err != nil {
			logging.Error("Failed to create encrypt reader", zap.Error(err))
			http.Error(w, "encryption error", http.StatusInternalServerError)
			return
		}
		reader = encReader
		isEncrypted = true
		// Range requests and compression don't work with our chunked encryption
		shouldCompress = false
	} else {
		logging.Info("No key found for token", zap.String("token", s.Token))
	}

	// Apply rate limiting if configured
	clientIP := getClientIP(r)
	var writer io.Writer = w
	if limiter := s.getRateLimiter(clientIP); limiter != nil {
		writer = &RateLimitedWriter{w: w, limiter: limiter}
		metrics.RateLimitedRequests.WithLabelValues(clientIP).Inc()
	}

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" && strings.HasPrefix(rangeHeader, "bytes=") && reader == f {
		// Range requests only supported for unencrypted files
		// Parse Range: bytes=start-end
		rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
		var start int64
		if _, err := fmt.Sscanf(rangeSpec, "%d-", &start); err == nil && start > 0 {
			if _, err := f.Seek(start, 0); err == nil {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, fi.Size()-1, fi.Size()))
				w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()-start))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.Copy(writer, f)
				logging.Info("Resumed download", zap.Int64("start", start), zap.String("filename", filepath.Base(s.SrcPath)))
				return
			}
		}
	}

	// Serve with compression if applicable
	if shouldCompress {
		// Compute checksum first (with caching)
		checksum, err := s.getCachedChecksum(s.SrcPath)
		if err == nil {
			w.Header().Set("X-Content-SHA256", checksum)
		}

		// Decide encoder preference
		enc := strings.ToLower(r.Header.Get("Accept-Encoding"))
		// Reset file to beginning
		_, _ = f.Seek(0, 0)

		if strings.Contains(enc, "zstd") {
			w.Header().Set("Content-Encoding", "zstd")
			w.Header().Del("Content-Length")
			zw, err := zstd.NewWriter(writer)
			if err != nil {
				http.Error(w, "compression error", http.StatusInternalServerError)
				return
			}
			_, _ = io.Copy(zw, f)
			_ = zw.Close()
			if checksum != "" {
				logging.Info("Served file with zstd compression", zap.String("filename", filepath.Base(s.SrcPath)), zap.String("checksum", checksum[:16]+"..."))
			}
			return
		}

		// Fallback to gzip if supported
		if strings.Contains(enc, "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Del("Content-Length") // Let gzip set the length

			// Reset file to beginning (already reset above)
			gzipWriter := gzip.NewWriter(writer)
			_, _ = io.Copy(gzipWriter, f)
			_ = gzipWriter.Close()

			if checksum != "" {
				logging.Info("Served file with gzip compression", zap.String("filename", filepath.Base(s.SrcPath)), zap.String("checksum", checksum[:16]+"..."))
			}
			return
		}
	}

	// Use zero-copy sendfile for large binary files on Linux (>10MB and not compressible)
	// BUT: Skip sendfile for encrypted transfers since we need to stream through EncryptReader
	if runtime.GOOS == "linux" && fi.Size() > 10*1024*1024 && !isCompressible(s.SrcPath) && !isEncrypted {
		// Compute checksum before sending (with caching)
		checksum, err := s.getCachedChecksum(s.SrcPath)
		if err == nil {
			w.Header().Set("X-Content-SHA256", checksum)
		}

		// Set headers before attempting sendfile
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))

		if err := sendfileZeroCopy(w, f, 0, fi.Size()); err == nil {
			if checksum != "" {
				logging.Info("Served file using zero-copy sendfile", zap.String("filename", filepath.Base(s.SrcPath)), zap.String("size", ui.FormatBytes(fi.Size())), zap.String("checksum", checksum[:16]+"..."))
			} else {
				logging.Info("Served file using zero-copy sendfile", zap.String("filename", filepath.Base(s.SrcPath)), zap.String("size", ui.FormatBytes(fi.Size())))
			}
			return
		}
		// If sendfile fails, fall back to normal method
		logging.Warn("Sendfile failed, falling back to standard copy", zap.String("filename", filepath.Base(s.SrcPath)), zap.Error(err))
		// Need to reopen file since sendfile may have consumed it
		_ = f.Close()
		f, err = os.Open(s.SrcPath)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer func() { _ = f.Close() }()
		// After reopening, recreate encrypted reader if needed
		if isEncrypted {
			if val, ok := s.tokenKeys.Load(s.Token); ok {
				key := val.([]byte)
				encReader, err := crypto.NewEncryptReader(f, key)
				if err == nil {
					reader = encReader
				}
			}
		}
	}

	// Normal full file download without compression (fallback)
	// Compute checksum for integrity verification (with caching)
	checksum, err := s.getCachedChecksum(s.SrcPath)
	if err == nil {
		w.Header().Set("X-Content-SHA256", checksum)
	}

	if reader != f {
		// Encrypted transfer
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Encryption", "true")
		// For encrypted transfers, calculate and set Content-Length
		// Size = 12 byte nonce + plaintext + (16 byte GCM tag per 64KB chunk)
		encryptedSize := calculateEncryptedSize(fi.Size())
		w.Header().Set("Content-Length", fmt.Sprintf("%d", encryptedSize))
		_, _ = io.Copy(writer, reader)
		return
	}

	// Normal unencrypted transfer - use http.ServeFile for proper HTTP handling
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))

	if checksum := r.Header.Get("X-Content-SHA256"); checksum == "" {
		// Only compute checksum if not already set by other code paths
		if c, err := s.getCachedChecksum(s.SrcPath); err == nil {
			w.Header().Set("X-Content-SHA256", c)
		}
	}

	_, _ = io.Copy(writer, reader)

	// Record metrics after successful download
	duration := time.Since(startTime).Seconds()
	fileExt := strings.ToLower(filepath.Ext(s.SrcPath))
	if fileExt == "" {
		fileExt = "no_ext"
	}

	metrics.DownloadDuration.WithLabelValues(fileExt).Observe(duration)
	metrics.DownloadSize.WithLabelValues(fileExt).Observe(float64(fi.Size()))
	if duration > 0 {
		throughputMbps := float64(fi.Size()*8) / (duration * 1_000_000)
		metrics.DownloadThroughput.WithLabelValues(fileExt).Observe(throughputMbps)
	}
	metrics.DownloadsTotal.WithLabelValues(fileExt, "success").Inc()
}

// calculateEncryptedSize estimates the size of data after encryption
// Encryption adds: 12 bytes for nonce + 16 bytes GCM tag per encrypted chunk + 4 bytes length prefix per chunk
// Plaintext is encrypted in 64KB chunks
func calculateEncryptedSize(plainSize int64) int64 {
	const chunkSize = 64 * 1024
	const nonceSize = 12
	const tagSize = 16
	const lengthPrefixSize = 4 // Each encrypted chunk is prefixed with its length

	// Number of chunks needed to encrypt the plaintext
	numChunks := (plainSize + chunkSize - 1) / chunkSize

	// Total size = nonce + (length_prefix + plaintext + tag per chunk)
	encryptedSize := int64(nonceSize) + plainSize + (numChunks * int64(tagSize+lengthPrefixSize))
	return encryptedSize
}

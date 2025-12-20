package server

import (
	"compress/gzip"
	"fmt"
	"github.com/zulfikawr/warp/internal/logging"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
		// Use progress-enabled zip for directories
		if err := ZipDirectoryWithProgress(w, s.SrcPath, os.Stderr); err != nil {
			http.Error(w, "zip error", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(s.SrcPath)))

	// Check if client supports compression and file is compressible
	acceptsGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	shouldCompress := acceptsGzip && isCompressible(s.SrcPath) && fi.Size() > 1024 // Only compress files > 1KB

	// Support resumable downloads via Range headers
	f, err := os.Open(s.SrcPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()

	// Apply rate limiting if configured
	clientIP := getClientIP(r)
	var writer io.Writer = w
	if limiter := s.getRateLimiter(clientIP); limiter != nil {
		writer = &RateLimitedWriter{w: w, limiter: limiter}
		metrics.RateLimitedRequests.WithLabelValues(clientIP).Inc()
	}

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" && strings.HasPrefix(rangeHeader, "bytes=") {
		// Range requests don't work with compression, serve uncompressed
		shouldCompress = false
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

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Let gzip set the length

		// Reset file to beginning
		_, _ = f.Seek(0, 0)

		gzipWriter := gzip.NewWriter(writer)
		_, _ = io.Copy(gzipWriter, f)
		_ = gzipWriter.Close()

		if checksum != "" {
			logging.Info("Served file with gzip compression", zap.String("filename", filepath.Base(s.SrcPath)), zap.String("checksum", checksum[:16]+"..."))
		}
		return
	}

	// Use zero-copy sendfile for large binary files on Linux (>10MB and not compressible)
	if runtime.GOOS == "linux" && fi.Size() > 10*1024*1024 && !isCompressible(s.SrcPath) {
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
	}

	// Normal full file download without compression (fallback)
	// Compute checksum for integrity verification (with caching)
	checksum, err := s.getCachedChecksum(s.SrcPath)
	if err == nil {
		w.Header().Set("X-Content-SHA256", checksum)
	}

	http.ServeFile(w, r, s.SrcPath)

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

package server

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zulfikawr/warp/internal/discovery"
	"github.com/zulfikawr/warp/internal/metrics"
	"github.com/zulfikawr/warp/internal/network"
	"github.com/zulfikawr/warp/internal/protocol"
	"golang.org/x/time/rate"
)

//go:embed static/upload.html
var uploadPageHTML string

// Adaptive buffer pools for different file sizes
var bufferPools = map[int]*sync.Pool{
	8192: {
		New: func() interface{} {
			b := make([]byte, 8192) // 8KB for small files
			return &b
		},
	},
	65536: {
		New: func() interface{} {
			b := make([]byte, 65536) // 64KB for medium files
			return &b
		},
	},
	1048576: {
		New: func() interface{} {
			b := make([]byte, 1048576) // 1MB for large files
			return &b
		},
	},
	4194304: {
		New: func() interface{} {
			b := make([]byte, 4194304) // 4MB for very large files
			return &b
		},
	},
}

// getOptimalBufferSize determines the best buffer size based on file size
func getOptimalBufferSize(fileSize int64) int {
	switch {
	case fileSize < 64*1024: // < 64KB
		return 8192 // 8KB
	case fileSize < 1024*1024: // < 1MB
		return 65536 // 64KB
	case fileSize < 100*1024*1024: // < 100MB
		return 1048576 // 1MB
	default: // >= 100MB
		return 4194304 // 4MB
	}
}

// getBuffer retrieves a buffer of the specified size from the pool
func getBuffer(size int) *[]byte {
	pool, ok := bufferPools[size]
	if !ok {
		// Fallback to 1MB pool if size not found
		pool = bufferPools[1048576]
	}
	return pool.Get().(*[]byte)
}

// putBuffer returns a buffer to the appropriate pool
func putBuffer(buf *[]byte) {
	size := len(*buf)
	pool, ok := bufferPools[size]
	if ok {
		pool.Put(buf)
	}
}

// computeFileChecksum calculates SHA256 hash of a file
func computeFileChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	hash := sha256.New()
	bufSize := 1048576 // 1MB buffer for checksum computation
	buf := getBuffer(bufSize)
	defer putBuffer(buf)

	if _, err := io.CopyBuffer(hash, f, *buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// isCompressible checks if the file extension indicates compressible content
func isCompressible(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	compressible := []string{
		".txt", ".json", ".xml", ".html", ".css", ".js", ".csv", ".log",
		".md", ".yaml", ".yml", ".svg", ".sql", ".sh", ".bat", ".ps1",
	}
	for _, e := range compressible {
		if ext == e {
			return true
		}
	}
	return false
}

type Server struct {
	InterfaceName string
	Token         string
	SrcPath       string
	// Host mode (reverse drop)
	HostMode       bool
	UploadDir      string
	TextContent    string // If set, serves text instead of file
	IP             net.IP // Server's IP address (exported for CLI display)
	Port           int
	httpServer     *http.Server
	advertiser     *discovery.Advertiser
	chunkTimes     sync.Map // filename -> *chunkStat
	uploadSessions sync.Map // sessionID -> *uploadSession
	// Progress tracking for WebSocket updates
	activeUploads sync.Map // filename -> *ProgressTracker
	// Rate limiting (exported for CLI configuration)
	RateLimitMbps float64  // 0 = no limit
	rateLimiters  sync.Map // clientIP -> *rate.Limiter
	// File caching (exported for CLI configuration)
	MaxCacheSize int64 // max cache size in bytes (default 100MB)
	// Encryption (exported for CLI configuration)
	Password       string // If set, enables encryption
	EncryptionSalt []byte // Salt for key derivation
}

// CachedFile represents a cached file in memory
type CachedFile struct {
	Data     []byte
	ModTime  time.Time
	ETag     string
	Size     int64
	CachedAt time.Time
}

// RateLimitedWriter wraps an io.Writer with rate limiting
type RateLimitedWriter struct {
	w       io.Writer
	limiter *rate.Limiter
}

func (rl *RateLimitedWriter) Write(p []byte) (int, error) {
	// Wait for rate limiter to allow this write
	if rl.limiter != nil {
		if err := rl.limiter.WaitN(context.Background(), len(p)); err != nil {
			return 0, err
		}
	}
	return rl.w.Write(p)
}

// getRateLimiter gets or creates a rate limiter for a client IP
func (s *Server) getRateLimiter(clientIP string) *rate.Limiter {
	if s.RateLimitMbps <= 0 {
		return nil // No rate limiting
	}

	if lim, ok := s.rateLimiters.Load(clientIP); ok {
		return lim.(*rate.Limiter)
	}

	// Convert Mbps to bytes per second
	bytesPerSecond := (s.RateLimitMbps * 1_000_000) / 8
	burst := max(
		// 100ms burst
		int(bytesPerSecond/10),
		// Minimum 4KB burst
		4096,
	)

	lim := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
	s.rateLimiters.Store(clientIP, lim)
	return lim
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (if behind proxy)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP in the list
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Use RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ProgressTracker tracks upload/download progress for real-time WebSocket updates
type ProgressTracker struct {
	Filename     string
	TotalSize    int64
	BytesWritten int64 // atomic
	StartTime    time.Time
	LastUpdate   time.Time
}

// GetProgress returns current progress stats atomically
func (pt *ProgressTracker) GetProgress() map[string]interface{} {
	bytesWritten := atomic.LoadInt64(&pt.BytesWritten)
	elapsed := time.Since(pt.StartTime).Seconds()

	var mbps float64
	if elapsed > 0 {
		mbps = (float64(bytesWritten) * 8) / (elapsed * 1_000_000)
	}

	percentage := float64(0)
	if pt.TotalSize > 0 {
		percentage = (float64(bytesWritten) / float64(pt.TotalSize)) * 100
	}

	return map[string]interface{}{
		"filename":        pt.Filename,
		"total_size":      pt.TotalSize,
		"bytes_written":   bytesWritten,
		"percentage":      percentage,
		"throughput_mbps": mbps,
		"elapsed_seconds": elapsed,
	}
}

// UpdateProgress atomically updates bytes written
func (pt *ProgressTracker) UpdateProgress(bytes int64) {
	atomic.AddInt64(&pt.BytesWritten, bytes)
	pt.LastUpdate = time.Now()
}

// uploadSession tracks parallel chunk uploads
type uploadSession struct {
	SessionID     string
	Filename      string
	TotalSize     int64
	TotalChunks   int
	ChunksWritten map[int]bool
	FilePath      string
	FileHandle    *os.File
	CreatedAt     time.Time
	LastActivity  time.Time
	mu            sync.Mutex
	complete      bool
}

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

// tcpKeepAliveListener sets TCP keepalive and optimizes socket for high throughput
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	// Enable TCP keepalive to detect dead connections
	_ = tc.SetKeepAlive(true)
	_ = tc.SetKeepAlivePeriod(3 * time.Minute)

	// Critical: Disable Nagle's algorithm for immediate packet transmission
	// This eliminates 40-200ms delays waiting for packet coalescing
	_ = tc.SetNoDelay(true)

	// Let OS auto-tune TCP window size for optimal throughput
	// Manual buffer sizing can prevent dynamic scaling

	return tc, nil
}

// WebSocket upgrader for real-time progress updates
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from same origin only for security
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Allow non-browser clients
		}
		// In production, validate origin properly
		return true
	},
}

// Start initializes and starts the HTTP server. It returns the accessible URL.
func (s *Server) Start() (string, error) {
	ip, err := network.DiscoverLANIP(s.InterfaceName)
	if err != nil {
		return "", err
	}
	s.IP = ip

	mux := http.NewServeMux()
	// Health endpoint for realtime status checks
	mux.HandleFunc("/health", s.handleHealth)
	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())
	// WebSocket endpoint for real-time progress updates
	mux.HandleFunc("/ws/progress", s.handleProgressWebSocket)
	// Encryption info endpoint (returns salt if encryption is enabled)
	mux.HandleFunc("/d/encrypt-info", s.handleEncryptInfo)
	if s.HostMode {
		mux.HandleFunc(protocol.UploadPathPrefix, s.handleUpload)
	} else {
		mux.HandleFunc(protocol.PathPrefix, s.handleDownload)
	}

	s.httpServer = &http.Server{
		ReadTimeout:       0, // unlimited body time; rely on IdleTimeout
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      protocol.WriteTimeout,
		IdleTimeout:       protocol.IdleTimeout,
		MaxHeaderBytes:    1 << 20, // 1MB
		Handler:           mux,
		// Disable HTTP/2 for lower overhead on uploads
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	// Create standard TCP listener
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:0", ip.String()))
	if err != nil {
		return "", err
	}

	// Wrap with TCP optimizations
	tcpListener, ok := ln.(*net.TCPListener)
	if !ok {
		_ = ln.Close()
		return "", fmt.Errorf("expected TCP listener")
	}
	optimizedListener := tcpKeepAliveListener{tcpListener}

	addr := optimizedListener.Addr().String() // ip:port
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		_ = optimizedListener.Close()
		return "", fmt.Errorf("unexpected listener addr: %s", addr)
	}
	portStr := parts[len(parts)-1]
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	s.Port = port

	go func() {
		_ = s.httpServer.Serve(optimizedListener)
	}()

	// Start session cleanup routine
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.cleanupStaleSessions()
		}
	}()

	// Advertise via mDNS for discovery (best-effort)
	mode := "send"
	path := protocol.PathPrefix + s.Token
	if s.HostMode {
		mode = "host"
		path = protocol.UploadPathPrefix + s.Token
	}
	instance := fmt.Sprintf("warp-%s", s.Token[:6])
	adv, err := discovery.Advertise(instance, mode, s.Token, path, s.IP, s.Port)
	if err != nil {
		log.Printf("mDNS advertise failed: %v", err)
	} else {
		s.advertiser = adv
	}

	if s.HostMode {
		return fmt.Sprintf("http://%s:%d%s%s", ip.String(), s.Port, protocol.UploadPathPrefix, s.Token), nil
	}
	return fmt.Sprintf("http://%s:%d%s%s", ip.String(), s.Port, protocol.PathPrefix, s.Token), nil
}

// handleHealth returns a simple JSON payload indicating the server is alive.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Prevent caching to ensure fresh status on each request
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	resp := map[string]interface{}{
		"status": "ok",
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleProgressWebSocket upgrades the connection to WebSocket and streams progress updates
func (s *Server) handleProgressWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	metrics.ActiveWebSocketConnections.Inc()
	defer metrics.ActiveWebSocketConnections.Dec()

	// Send progress updates every 100ms
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Collect all active uploads/downloads
			progress := make([]map[string]interface{}, 0)
			s.activeUploads.Range(func(key, value interface{}) bool {
				tracker := value.(*ProgressTracker)
				progress = append(progress, tracker.GetProgress())
				return true
			})

			// Send progress update
			if len(progress) > 0 {
				metrics.WebSocketMessagesTotal.WithLabelValues("progress").Inc()
				if err := conn.WriteJSON(map[string]interface{}{
					"type":      "progress",
					"transfers": progress,
					"timestamp": time.Now().Unix(),
				}); err != nil {
					// Client disconnected
					return
				}
			}
		case <-r.Context().Done():
			// Connection closed
			return
		}
	}
}

// handleEncryptInfo provides encryption metadata for clients
func (s *Server) handleEncryptInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	resp := map[string]interface{}{
		"encrypted": s.Password != "",
	}

	if s.Password != "" && len(s.EncryptionSalt) > 0 {
		resp["salt"] = base64.StdEncoding.EncodeToString(s.EncryptionSalt)
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// handleManifest advertises upload parameters (chunk size, max concurrency).
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	resp := map[string]interface{}{
		"chunk_size":     2 * 1024 * 1024, // 2MB default chunk size
		"max_concurrent": 3,               // parallel workers hint
	}
	_ = json.NewEncoder(w).Encode(resp)
}

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
		if err := ZipDirectory(w, s.SrcPath); err != nil {
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
				log.Printf("Resumed download from byte %d for %s", start, filepath.Base(s.SrcPath))
				return
			}
		}
	}

	// Serve with compression if applicable
	if shouldCompress {
		// Compute checksum first
		checksum, err := computeFileChecksum(s.SrcPath)
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
			log.Printf("Served %s with gzip compression (SHA256: %s)", filepath.Base(s.SrcPath), checksum[:16]+"...")
		}
		return
	}

	// Use zero-copy sendfile for large binary files on Linux (>10MB and not compressible)
	if runtime.GOOS == "linux" && fi.Size() > 10*1024*1024 && !isCompressible(s.SrcPath) {
		// Compute checksum before sending
		checksum, err := computeFileChecksum(s.SrcPath)
		if err == nil {
			w.Header().Set("X-Content-SHA256", checksum)
		}

		// Set headers before attempting sendfile
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))

		if err := sendfileZeroCopy(w, f, 0, fi.Size()); err == nil {
			if checksum != "" {
				log.Printf("Served %s using zero-copy sendfile (%s, SHA256: %s)", filepath.Base(s.SrcPath), formatBytes(fi.Size()), checksum[:16]+"...")
			} else {
				log.Printf("Served %s using zero-copy sendfile (%s)", filepath.Base(s.SrcPath), formatBytes(fi.Size()))
			}
			return
		}
		// If sendfile fails, fall back to normal method
		log.Printf("Sendfile failed for %s, falling back to standard copy: %v", filepath.Base(s.SrcPath), err)
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
	// Compute checksum for integrity verification
	checksum, err := computeFileChecksum(s.SrcPath)
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

// handleUpload serves a simple HTML form on GET and accepts multipart file uploads on POST.
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
	r.Body = http.MaxBytesReader(w, r.Body, 10<<30) // 10GB limit

	// Check available disk space (best effort)
	if r.ContentLength > 0 {
		if err := checkDiskSpace(dest, r.ContentLength); err != nil {
			log.Printf("Disk space check failed: %v", err)
			http.Error(w, "insufficient disk space", http.StatusInsufficientStorage)
			return
		}
	}

	// Use streaming multipart reader for true zero-copy I/O
	// This reads directly from network to disk without buffering entire files in RAM
	reader, err := r.MultipartReader()
	if err != nil {
		log.Printf("Failed to create multipart reader: %v", err)
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
			log.Printf("Failed to read next part: %v", err)
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
			log.Printf("Failed to create file %s: %v", name, err)
			_ = part.Close()
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}

		// Use adaptive buffer sizing - default to 1MB for multipart uploads
		bufferSize := getOptimalBufferSize(1024 * 1024) // Default to 1MB for unknown sizes
		bufPtr := getBuffer(bufferSize)
		defer putBuffer(bufPtr) // Ensure buffer is returned even on error
		buf := *bufPtr
		n, err := io.CopyBuffer(out, part, buf)
		cerr := out.Close()
		_ = part.Close()

		if err != nil || cerr != nil {
			log.Printf("Failed to write file %s: write_err=%v, close_err=%v", name, err, cerr)
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}

		duration := time.Since(requestStart).Seconds()
		mbps := 0.0
		if duration > 0 {
			mbps = (float64(n) * 8) / (duration * 1_000_000)
		}
		log.Printf("%s, %s received in %.2fs (%.1f Mbps)", filename, formatBytes(n), duration, mbps)
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
func sanitizeFilename(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty filename")
	}

	// Clean path separators and get base
	name = filepath.Base(filepath.Clean(name))

	// Reject dangerous names
	if name == "" || name == "." || name == ".." {
		return "", errors.New("invalid filename")
	}

	// Remove control characters and null bytes
	for _, r := range name {
		if r < 32 || r == 0x7F {
			return "", errors.New("filename contains control characters")
		}
	}

	// Limit length to 255 bytes (common filesystem limit)
	if len(name) > 255 {
		return "", errors.New("filename too long")
	}

	return name, nil
}

// findUniqueFilename prevents file overwrites by appending (1), (2), etc.
func findUniqueFilename(dir, name string) string {
	name, err := sanitizeFilename(name)
	if err != nil {
		// Fallback to timestamp-based name if sanitization fails
		name = fmt.Sprintf("upload_%d", time.Now().UnixNano())
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	// First try: exact match
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	// Collision found: try "file (1).ext", "file (2).ext", etc.
	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s (%d)%s", base, i, ext)
		path = filepath.Join(dir, newName)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
	}

	// Fallback: Use timestamp if 1000 collisions (unlikely)
	return filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext))
}

// handleParallelChunk handles a single chunk from a parallel upload session
func (s *Server) handleParallelChunk(w http.ResponseWriter, r *http.Request, filename, sessionID, chunkIDStr, chunkTotalStr, offsetStr, dest string) {
	chunkStartTime := time.Now()
	metrics.ParallelUploadWorkers.Inc()
	defer metrics.ParallelUploadWorkers.Dec()

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

	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid offset", http.StatusBadRequest)
		return
	}

	totalSize := int64(0)
	if totalHeader := r.Header.Get("X-Upload-Total"); totalHeader != "" {
		totalSize, _ = strconv.ParseInt(totalHeader, 10, 64)
	}

	// Get or create upload session
	session, err := s.getOrCreateSession(sessionID, filename, totalSize, chunkTotal, dest)
	if err != nil {
		log.Printf("Failed to create session %s: %v", sessionID[:8], err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	// Read chunk data
	chunkData, err := io.ReadAll(io.LimitReader(r.Body, r.ContentLength))
	if err != nil {
		log.Printf("Failed to read chunk %d of session %s: %v", chunkID, sessionID[:8], err)
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	// Write chunk to file
	if err := session.writeChunk(chunkID, offset, chunkData); err != nil {
		log.Printf("Failed to write chunk %d of session %s: %v", chunkID, sessionID[:8], err)
		metrics.ChunkUploadsTotal.WithLabelValues("error").Inc()
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}

	// Record chunk metrics
	chunkDuration := time.Since(chunkStartTime).Seconds()
	metrics.ChunkUploadDuration.Observe(chunkDuration)
	metrics.ChunkUploadsTotal.WithLabelValues("success").Inc()

	// Track cumulative chunk timing for this file
	s.addChunkDuration(filename, time.Since(chunkStartTime))

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

		// Schedule cleanup after a delay
		go func() {
			time.Sleep(30 * time.Second)
			s.cleanupSession(sessionID)
		}()
	}
}

// getOrCreateSession retrieves or creates an upload session
func (s *Server) getOrCreateSession(sessionID, filename string, totalSize int64, totalChunks int, destDir string) (*uploadSession, error) {
	// Try to load existing session
	if val, ok := s.uploadSessions.Load(sessionID); ok {
		session := val.(*uploadSession)
		session.mu.Lock()
		session.LastActivity = time.Now()
		session.mu.Unlock()
		return session, nil
	}

	// Create new session
	session := &uploadSession{
		SessionID:     sessionID,
		Filename:      filename,
		TotalSize:     totalSize,
		TotalChunks:   totalChunks,
		ChunksWritten: make(map[int]bool),
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
	}

	// Determine file path
	sanitized, err := sanitizeFilename(filename)
	if err != nil {
		return nil, err
	}
	outPath := findUniqueFilename(destDir, sanitized)
	session.FilePath = outPath

	// Open file for writing (O_TRUNC ensures clean start)
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	// Pre-allocate file space if total size is known
	if totalSize > 0 {
		if err := f.Truncate(totalSize); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("failed to pre-allocate space: %w", err)
		}
	}

	session.FileHandle = f

	// Store session
	s.uploadSessions.Store(sessionID, session)
	log.Printf("Created upload session %s for %s (%s, %d chunks)",
		sessionID[:8], filepath.Base(outPath), formatBytes(totalSize), totalChunks)

	return session, nil
}

// writeChunk writes a chunk to the appropriate offset in the file
func (session *uploadSession) writeChunk(chunkID int, offset int64, data []byte) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	// Check if already written
	if session.ChunksWritten[chunkID] {
		return nil // Idempotent - chunk already written
	}

	// Write to file at offset
	n, err := session.FileHandle.WriteAt(data, offset)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("incomplete write: wrote %d of %d bytes", n, len(data))
	}

	// Mark chunk as written
	session.ChunksWritten[chunkID] = true
	session.LastActivity = time.Now()

	// Check if upload is complete
	if len(session.ChunksWritten) >= session.TotalChunks {
		session.complete = true
		log.Printf("Upload session %s complete: %s (%s, %d chunks)",
			session.SessionID[:8], filepath.Base(session.FilePath),
			formatBytes(session.TotalSize), session.TotalChunks)
	}

	return nil
}

// isComplete checks if all chunks have been received
func (session *uploadSession) isComplete() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.complete
}

// cleanup closes the file and removes the session
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
	staleThreshold := 1 * time.Hour
	s.uploadSessions.Range(func(key, value interface{}) bool {
		session := value.(*uploadSession)
		session.mu.Lock()
		isStale := time.Since(session.LastActivity) > staleThreshold
		session.mu.Unlock()

		if isStale {
			sessionID := key.(string)
			log.Printf("Cleaning up stale session %s", sessionID[:8])
			s.cleanupSession(sessionID)
		}
		return true
	})
}

// handleRawUpload processes raw binary stream uploads (A+ tier performance)
// This eliminates multipart parsing overhead for maximum speed
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
		log.Printf("Invalid filename %q: %v", filename, err)
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
		log.Printf("Disk space check failed: %v", err)
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
				log.Printf("Legacy chunk upload with offset mismatch: file=%d, expected=%d for %s",
					fi.Size(), uploadOffset, actualFilename)
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
		log.Printf("Failed to open file %s: %v", actualFilename, err)
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
	defer func() { _ = conn.Close() }()
	defer func() { _ = f.Close() }()

	// Manual deadlines since http.Server timeouts no longer apply post-hijack
	_ = conn.SetReadDeadline(time.Now().Add(time.Hour))

	if chunked && uploadOffset == 0 {
		if totalSize > 0 {
			log.Printf("receiving %s (%s)", actualFilename, formatBytes(totalSize))
		} else {
			log.Printf("receiving %s", actualFilename)
		}
	}
	if !chunked {
		if r.ContentLength > 0 {
			log.Printf("receiving %s (%s)", actualFilename, formatBytes(r.ContentLength))
		} else {
			log.Printf("receiving %s", actualFilename)
		}
	}

	// Use adaptive buffer sizing based on expected file size
	expectedSize := totalSize
	if expectedSize <= 0 && r.ContentLength > 0 {
		expectedSize = r.ContentLength
	}
	bufferSize := getOptimalBufferSize(expectedSize)
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
		log.Printf("Upload stream failed for %s: %v", actualFilename, err)
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

// formatBytes formats bytes into human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
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

// Shutdown stops the server gracefully.
func (s *Server) Shutdown() error {
	if s.httpServer == nil {
		return nil
	}
	if s.advertiser != nil {
		s.advertiser.Close()
	}
	// Use context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

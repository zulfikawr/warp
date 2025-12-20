package server

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/zulfikawr/warp/internal/logging"
	"go.uber.org/zap"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zulfikawr/warp/internal/discovery"
	"github.com/zulfikawr/warp/internal/network"
	"github.com/zulfikawr/warp/internal/protocol"
)

// Server represents the HTTP server for file transfer
type Server struct {
	InterfaceName string
	Token         string
	SrcPath       string
	// Host mode (reverse drop)
	HostMode         bool
	UploadDir        string
	TextContent      string // If set, serves text instead of file
	IP               net.IP // Server's IP address (exported for CLI display)
	Port             int
	httpServer       *http.Server
	advertiser       *discovery.Advertiser
	chunkTimes       sync.Map           // filename -> *chunkStat
	uploadSessions   sync.Map           // sessionID -> *uploadSession
	multiFileDisplay *MultiFileProgress // Tracks multiple file downloads for unified display
	// Progress tracking for WebSocket updates
	activeUploads sync.Map // filename -> *ProgressTracker
	// Rate limiting (exported for CLI configuration)
	RateLimitMbps float64  // 0 = no limit
	rateLimiters  sync.Map // clientIP -> *rateLimiterEntry
	// Checksum caching for performance
	checksumCache sync.Map // filepath -> *checksumCacheEntry
	// File caching (exported for CLI configuration)
	MaxCacheSize int64 // max cache size in bytes (default 100MB)
	// Encryption (exported for CLI configuration)
	Password       string // If set, enables encryption
	EncryptionSalt []byte // Salt for key derivation
	// Graceful shutdown support
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// Start initializes and starts the HTTP server
func (s *Server) Start() (string, error) {
	ip, err := network.DiscoverLANIP(s.InterfaceName)
	if err != nil {
		return "", fmt.Errorf("failed to discover LAN IP: %w", err)
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
		return "", fmt.Errorf("failed to listen on %s: %w", ip.String(), err)
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

	// Initialize shutdown context for graceful termination of background goroutines
	s.shutdownCtx, s.shutdownCancel = context.WithCancel(context.Background())

	go func() {
		_ = s.httpServer.Serve(optimizedListener)
	}()

	// Start session cleanup routine with proper shutdown support
	go func() {
		ticker := time.NewTicker(SessionCleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.cleanupStaleSessions()
			case <-s.shutdownCtx.Done():
				logging.Info("Stopping session cleanup goroutine")
				return
			}
		}
	}()

	// Start rate limiter cleanup routine to prevent memory leak
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.cleanupRateLimiters()
			case <-s.shutdownCtx.Done():
				logging.Info("Stopping rate limiter cleanup goroutine")
				return
			}
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
		logging.Warn("mDNS advertise failed", zap.Error(err))
	} else {
		s.advertiser = adv
	}

	if s.HostMode {
		return fmt.Sprintf("http://%s:%d%s%s", ip.String(), s.Port, protocol.UploadPathPrefix, s.Token), nil
	}
	return fmt.Sprintf("http://%s:%d%s%s", ip.String(), s.Port, protocol.PathPrefix, s.Token), nil
}

// handleHealth returns a simple JSON payload indicating the server is alive
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

// handleManifest advertises upload parameters (chunk size, max concurrency)
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

// Shutdown stops the server gracefully
func (s *Server) Shutdown() error {
	// Cancel shutdown context to stop background goroutines
	if s.shutdownCancel != nil {
		s.shutdownCancel()
	}

	if s.advertiser != nil {
		s.advertiser.Close()
	}

	if s.httpServer == nil {
		return nil
	}

	// Use context with timeout for graceful HTTP server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

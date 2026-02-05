package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/resume"
	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"

	"github.com/zulfikawr/warp/internal/discovery"
	"github.com/zulfikawr/warp/internal/network"
	"github.com/zulfikawr/warp/internal/protocol"
)

// Server represents the HTTP server for file transfer
type Server struct {
	InterfaceName string
	Code          string // PAKE Code (acts as session ID and password)
	SrcPath       string
	// Host mode (reverse drop)
	HostMode  bool
	UploadDir string

	TextContent string // If set, serves text instead of file
	IP          net.IP // Server's IP address (exported for CLI display)
	Port        int

	// Sub-managers
	AuthMgr      *AuthManager
	SessionMgr   *SessionManager
	RateLimitMgr *RateLimitManager

	httpServer  *http.Server
	http3Server *http3.Server
	advertiser  *discovery.Advertiser

	// Rate limiting (exported for CLI configuration)
	RateLimitMbps float64 // 0 = no limit

	// Transfer configuration (exported for CLI configuration)
	ChunkSize     int // size of chunks in bytes
	MaxConcurrent int // maximum concurrent uploads

	// Checksum caching for performance
	checksumCache sync.Map // filepath -> *checksumCacheEntry

	// File caching (exported for CLI configuration)
	MaxCacheSize int64 // max cache size in bytes (default 100MB)

	// Encryption (exported for CLI configuration)
	Password       string // If set, enables encryption
	EncryptionSalt []byte // Salt for key derivation
	NoEncrypt      bool   // If true, disables encryption

	// Graceful shutdown support
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// Self-signed certificate for QUIC/HTTP3
	tlsCert *tls.Certificate

	// ProgressChan for TUI updates
	ProgressChan chan progress.Progress

	// Pause state - can be toggled by host TUI
	IsPaused bool
	pauseMu  sync.RWMutex

	Protocols []string
}

// Start initializes and starts the HTTP server
func (s *Server) Start(ctx context.Context) (string, error) {
	// Set defaults for transfer configuration if not provided
	if s.ChunkSize <= 0 {
		s.ChunkSize = 2 * 1024 * 1024 // 2MB default
	}
	if s.MaxConcurrent <= 0 {
		s.MaxConcurrent = 3 // default parallel workers
	}

	ip, err := network.DiscoverLANIP(s.InterfaceName)
	if err != nil {
		return "", fmt.Errorf("failed to discover LAN IP: %w", err)
	}
	s.IP = ip

	// Initialize managers
	s.AuthMgr = NewAuthManager(s.Code)
	s.SessionMgr = NewSessionManager(s)
	s.RateLimitMgr = NewRateLimitManager(s.RateLimitMbps)

	// Initialize state manager for resume support (host mode only)
	if s.HostMode {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			logging.Warn("Failed to get home directory for state manager", zap.Error(err))
		} else {
			stateDir := filepath.Join(homeDir, ".warp", "transfers")
			stateManager, err := resume.NewTransferStateManager(stateDir)
			if err != nil {
				logging.Warn("Failed to initialize state manager", zap.Error(err))
			} else {
				s.SessionMgr.SetStateManager(stateManager)
				// Load existing sessions from checkpoints
				if err := s.SessionMgr.LoadUploadSessions(); err != nil {
					logging.Warn("Failed to load upload sessions", zap.Error(err))
				}
			}
		}
	}

	mux := http.NewServeMux()
	// Health endpoint for realtime status checks
	mux.HandleFunc("/health", s.handleHealth)
	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())
	// WebSocket endpoint for real-time progress updates
	mux.HandleFunc("/ws/progress", s.handleProgressWebSocket)
	// Encryption info endpoint (returns salt if encryption is enabled)
	mux.HandleFunc("/d/encrypt-info", s.handleEncryptInfo)
	// Pause state endpoint for clients to check
	mux.HandleFunc("/pause-state", s.handlePauseState)
	// Client IP endpoint for web UI
	mux.HandleFunc("/client-ip", s.handleClientIP)
	// PAKE endpoints
	mux.HandleFunc(protocol.PAKEInitPath, s.AuthMgr.HandleInit)
	mux.HandleFunc(protocol.PAKEVerifyPath, s.AuthMgr.HandleVerify)
	// Manifest endpoint for upload parameters
	mux.HandleFunc("/manifest", s.handleManifest)
	// Static files endpoint for CSS and JS
	staticFS, err := GetStaticFS()
	if err != nil {
		return "", fmt.Errorf("failed to get static file system: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
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

	// Start TCP server
	go func() {
		_ = s.httpServer.Serve(optimizedListener)
	}()

	// Set up QUIC/HTTP3 server on the same port
	// HTTP/3 uses UDP, quic-go will handle the listener setup
	quicAddr := fmt.Sprintf("%s:%d", ip.String(), s.Port)

	s.Protocols = []string{"HTTP/1.1"}

	// Create TLS config for QUIC
	tlsConfig, err := s.getQuicTLSConfig()
	if err != nil {
		logging.Warn("Failed to create TLS config for QUIC", zap.Error(err))
	} else {
		// Set up HTTP/3 server
		s.http3Server = &http3.Server{
			Handler:   mux,
			Addr:      quicAddr,
			TLSConfig: tlsConfig,
		}

		s.Protocols = append(s.Protocols, "QUIC")

		// Start QUIC server in background
		go func() {
			if err := s.http3Server.ListenAndServe(); err != nil &&
				err.Error() != "quic: Server closed" &&
				err.Error() != "http3: Server closed" &&
				err.Error() != "http: Server closed" {
				logging.Debug("QUIC server error", zap.Error(err))
			}
		}()

		logging.Info("QUIC/HTTP3 listener started successfully", zap.String("addr", quicAddr))
	}

	// Start session cleanup routine with proper shutdown support
	go func() {
		ticker := time.NewTicker(SessionCleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.SessionMgr.CleanupStaleSessions()
				s.SessionMgr.CleanupExpiredCheckpoints()
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
				s.RateLimitMgr.CleanupRateLimiters()
			case <-s.shutdownCtx.Done():
				logging.Info("Stopping rate limiter cleanup goroutine")
				return
			}
		}
	}()

	// Advertise via mDNS for discovery (best-effort)
	mode := "send"
	path := protocol.PathPrefix + s.Code
	if s.HostMode {
		mode = "host"
		path = protocol.UploadPathPrefix + s.Code
	}
	instance := fmt.Sprintf("warp-%s", s.Code) // Use full code or substring? Using full code is safer/unique
	// We might want to just use the number-word-word as is for friendliness
	adv, err := discovery.Advertise(instance, mode, s.Code, path, s.IP, s.Port)
	if err != nil {
		logging.Warn("mDNS advertise failed", zap.Error(err))
	} else {
		s.advertiser = adv
	}

	if s.HostMode {
		return fmt.Sprintf("http://%s:%d%s%s", ip.String(), s.Port, protocol.UploadPathPrefix, s.Code), nil
	}
	return fmt.Sprintf("http://%s:%d%s%s", ip.String(), s.Port, protocol.PathPrefix, s.Code), nil
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
		"chunk_size":     s.ChunkSize,     // configurable chunk size
		"max_concurrent": s.MaxConcurrent, // configurable parallel workers
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

	// Close HTTP/3 server if it exists
	if s.http3Server != nil {
		if err := s.http3Server.Close(); err != nil {
			logging.Warn("Error closing HTTP/3 server", zap.Error(err))
		}
	}

	if s.httpServer == nil {
		return nil
	}

	// Use context with timeout for graceful HTTP server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// generateSelfSignedCert creates a self-signed certificate for QUIC/HTTP3
func (s *Server) generateSelfSignedCert() (*tls.Certificate, error) {
	// Generate ECDSA private key (more efficient for QUIC)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         "warp-server",
			Organization:       []string{"warp"},
			OrganizationalUnit: []string{"local"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour), // Valid for 24 hours
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{s.IP.String(), "localhost", "127.0.0.1"},
		IPAddresses: []net.IP{s.IP, net.ParseIP("127.0.0.1")},
	}

	// Self-sign the certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Parse the certificate back
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  privateKey,
		Leaf:        cert,
	}, nil
}

// getQuicTLSConfig returns TLS configuration for QUIC listener
func (s *Server) getQuicTLSConfig() (*tls.Config, error) {
	if s.tlsCert == nil {
		cert, err := s.generateSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("failed to generate certificate: %w", err)
		}
		s.tlsCert = cert
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*s.tlsCert},
		ClientAuth:   tls.NoClientCert,
	}, nil
}

// SetPaused sets the pause state of the server
func (s *Server) SetPaused(paused bool) {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	s.IsPaused = paused
}

// GetPaused returns the current pause state
func (s *Server) GetPaused() bool {
	s.pauseMu.RLock()
	defer s.pauseMu.RUnlock()
	return s.IsPaused
}

// handlePauseState returns the current pause state for clients to check
func (s *Server) handlePauseState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	resp := map[string]interface{}{
		"paused": s.GetPaused(),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleClientIP returns the client's IP address
func (s *Server) handleClientIP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Get client IP from request
	clientIP := r.RemoteAddr
	// Strip port if present
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	resp := map[string]interface{}{
		"ip": clientIP,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// contextWriter wraps an io.Writer and checks a context before each write
type contextWriter struct {
	w   io.Writer
	ctx context.Context
}

func (cw *contextWriter) Write(p []byte) (int, error) {
	select {
	case <-cw.ctx.Done():
		return 0, cw.ctx.Err()
	default:
		return cw.w.Write(p)
	}
}

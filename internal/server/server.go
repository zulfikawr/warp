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
	"github.com/zulfikawr/warp/internal/logging"
	"go.uber.org/zap"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quic-go/quic-go/http3"

	"github.com/zulfikawr/warp/internal/crypto"
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
	http3Server      *http3.Server
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
	// PAKE (Password-Authenticated Key Exchange)
	PAKECode     string
	pakeSessions sync.Map // sessionID -> *pakeSession
	pakeAttempts sync.Map // clientIP -> int
	tokenKeys    sync.Map // token -> []byte (shared key)
	// Graceful shutdown support
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	// Self-signed certificate for QUIC/HTTP3
	tlsCert *tls.Certificate
}

type pakeSession struct {
	State         *crypto.PAKEState
	Key           []byte
	ClientMessage []byte
	ServerMessage []byte
	Expiry        time.Time
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
	// Speed test endpoints for network performance testing
	mux.HandleFunc("/speedtest/download", s.handleSpeedTestDownload)
	mux.HandleFunc("/speedtest/upload", s.handleSpeedTestUpload)
	// PAKE endpoints
	mux.HandleFunc(protocol.PAKEInitPath, s.handlePAKEInit)
	mux.HandleFunc(protocol.PAKEVerifyPath, s.handlePAKEVerify)
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

		// Start QUIC server in background
		go func() {
			if err := s.http3Server.ListenAndServe(); err != nil &&
				err.Error() != "quic: Server closed" &&
				err.Error() != "http3: Server closed" &&
				err.Error() != "http: Server closed" {
				logging.Warn("QUIC server error", zap.Error(err))
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

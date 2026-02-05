package core

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/errors"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/server"
	"github.com/zulfikawr/warp/internal/ui"
)

// HostExecutor handles the host command logic.
// It manages the lifecycle of an upload server that receives files from others.
type HostExecutor struct {
	opts     HostOptions
	server   *server.Server
	onStatus StatusCallback
	onProg   ProgressCallback

	mu       sync.Mutex
	started  bool
	stopChan chan struct{}
}

// NewHostExecutor creates a new HostExecutor with the given options and callbacks.
// The callbacks are optional and can be nil if progress/status updates are not needed.
func NewHostExecutor(opts HostOptions, onStatus StatusCallback, onProg ProgressCallback) *HostExecutor {
	return &HostExecutor{
		opts:     opts,
		onStatus: onStatus,
		onProg:   onProg,
		stopChan: make(chan struct{}),
	}
}

// Start initializes and starts the upload server.
// Returns server information including URL and PAKE code.
// The server runs until Stop() is called or an error occurs.
func (e *HostExecutor) Start(ctx context.Context) (*ServerInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return nil, fmt.Errorf("executor already started")
	}

	// Load configuration with defaults
	cfg := config.GetConfig()

	// Generate PAKE code for secure transfer
	code, err := crypto.GenerateCode(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PAKE code: %w", err)
	}

	// Apply options with config defaults
	iface := cfg.DefaultInterface
	if e.opts.InterfaceName != "" {
		iface = e.opts.InterfaceName
	}
	rateLimit := cfg.RateLimitMbps
	if e.opts.RateLimitMbps > 0 {
		rateLimit = e.opts.RateLimitMbps
	}
	destDir := e.opts.DestDir
	if destDir == "" {
		destDir = cfg.UploadDir
	}
	if destDir == "" {
		destDir = "."
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, errors.PermissionError("create directory", destDir, err)
	}

	// Create progress channel for transfer updates
	progressChan := make(chan progress.Progress, 10)

	// Create server instance in host mode
	e.server = &server.Server{
		InterfaceName: iface,
		Code:          code,
		RateLimitMbps: rateLimit,
		UploadDir:     destDir,
		HostMode:      true,
		NoEncrypt:     e.opts.NoEncrypt,
		MaxCacheSize:  cfg.CacheSizeMB * 1024 * 1024,
		ProgressChan:  progressChan,
	}

	// Start the server
	e.emitStatus("Starting host server...")
	url, err := e.server.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	e.started = true

	// Start progress forwarding goroutine
	go e.forwardProgress(progressChan)

	// Generate QR code if enabled
	var qrCode string
	if !e.opts.NoQR {
		qrCode, _ = ui.GenerateQR(url)
	}

	return &ServerInfo{
		URL:       url,
		Code:      code,
		Port:      e.server.Port,
		IP:        e.server.IP.String(),
		QRCode:    qrCode,
		Protocols: e.server.Protocols,
	}, nil
}

// Stop gracefully shuts down the server.
// Safe to call multiple times.
func (e *HostExecutor) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.started || e.server == nil {
		return nil
	}

	// Signal stop to any waiting goroutines
	select {
	case <-e.stopChan:
		// Already closed
	default:
		close(e.stopChan)
	}

	err := e.server.Shutdown()
	e.started = false
	return err
}

// Wait blocks until an interrupt signal is received or the context is cancelled.
// Useful for CLI mode where the server should run until Ctrl+C.
func (e *HostExecutor) Wait(ctx context.Context) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		e.emitStatus("Shutting down gracefully...")
		return e.Stop()
	case <-ctx.Done():
		return e.Stop()
	case <-e.stopChan:
		return nil
	}
}

// Server returns the underlying server instance.
// Returns nil if the executor hasn't been started.
func (e *HostExecutor) Server() *server.Server {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.server
}

// DestDir returns the directory where uploaded files are saved.
func (e *HostExecutor) DestDir() string {
	if e.opts.DestDir != "" {
		return e.opts.DestDir
	}
	cfg := config.GetConfig()
	if cfg != nil && cfg.UploadDir != "" {
		return cfg.UploadDir
	}
	return "."
}

// forwardProgress converts server progress updates to core Progress and forwards them.
func (e *HostExecutor) forwardProgress(ch <-chan progress.Progress) {
	for p := range ch {
		if e.onProg == nil {
			continue
		}

		progUpdate := progress.Progress{
			FileName:         p.FileName,
			TransferredBytes: p.TransferredBytes,
			TotalBytes:       p.TotalBytes,
			IsComplete:       p.IsComplete,
		}

		e.onProg(progUpdate)
	}
}

// emitStatus sends a status message if a callback is registered.
func (e *HostExecutor) emitStatus(msg string) {
	if e.onStatus != nil {
		e.onStatus(msg)
	}
}

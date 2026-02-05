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

// SendExecutor handles the send command logic.
// It manages the lifecycle of a file sharing server.
type SendExecutor struct {
	opts     SendOptions
	server   *server.Server
	onStatus StatusCallback
	onProg   ProgressCallback

	mu       sync.Mutex
	started  bool
	stopChan chan struct{}
}

// NewSendExecutor creates a new SendExecutor with the given options and callbacks.
// The callbacks are optional and can be nil if progress/status updates are not needed.
func NewSendExecutor(opts SendOptions, onStatus StatusCallback, onProg ProgressCallback) *SendExecutor {
	return &SendExecutor{
		opts:     opts,
		onStatus: onStatus,
		onProg:   onProg,
		stopChan: make(chan struct{}),
	}
}

// Start initializes and starts the file sharing server.
// Returns server information including URL and PAKE code.
// The server runs until Stop() is called or an error occurs.
func (e *SendExecutor) Start(ctx context.Context) (*ServerInfo, error) {
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
	port := e.opts.Port
	iface := cfg.DefaultInterface
	if e.opts.InterfaceName != "" {
		iface = e.opts.InterfaceName
	}
	rateLimit := cfg.RateLimitMbps
	if e.opts.RateLimitMbps > 0 {
		rateLimit = e.opts.RateLimitMbps
	}
	cacheSize := cfg.CacheSizeMB * 1024 * 1024
	if e.opts.CacheSizeMB > 0 {
		cacheSize = e.opts.CacheSizeMB * 1024 * 1024
	}

	// Combine text content sources
	textContent := e.opts.TextContent + e.opts.StdinContent

	// Validate input: need either file path or text content
	if e.opts.FilePath == "" && textContent == "" {
		return nil, fmt.Errorf("send requires a file path or text content")
	}

	// Verify file exists if specified
	if e.opts.FilePath != "" {
		if _, err := os.Stat(e.opts.FilePath); err != nil {
			return nil, errors.FileNotFoundError(e.opts.FilePath, err)
		}
	}

	// Create progress channel for transfer updates
	progressChan := make(chan progress.Progress, 10)

	// Create server instance
	e.server = &server.Server{
		Port:          port,
		InterfaceName: iface,
		Code:          code,
		SrcPath:       e.opts.FilePath,
		TextContent:   textContent,
		RateLimitMbps: rateLimit,
		MaxCacheSize:  cacheSize,
		NoEncrypt:     e.opts.NoEncrypt,
		ProgressChan:  progressChan,
	}

	// Start the server
	e.emitStatus("Starting server...")
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
func (e *SendExecutor) Stop() error {
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
func (e *SendExecutor) Wait(ctx context.Context) error {
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
func (e *SendExecutor) Server() *server.Server {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.server
}

// forwardProgress converts server progress updates to core Progress and forwards them.
func (e *SendExecutor) forwardProgress(ch <-chan progress.Progress) {
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

		// Calculate speed if we have timing info
		if p.TotalBytes > 0 && p.TransferredBytes > 0 {
			// Speed calculation would need timing data from server
			// For now, leave as 0 - can be enhanced later
		}

		e.onProg(progUpdate)
	}
}

// emitStatus sends a status message if a callback is registered.
func (e *SendExecutor) emitStatus(msg string) {
	if e.onStatus != nil {
		e.onStatus(msg)
	}
}

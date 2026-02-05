package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zulfikawr/warp/internal/client"
	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/discovery"
	"github.com/zulfikawr/warp/internal/errors"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/protocol"
	"github.com/zulfikawr/warp/internal/resume"
)

// ReceiveExecutor handles the receive command logic.
// It manages service discovery, PAKE handshake, and file download.
type ReceiveExecutor struct {
	opts     ReceiveOptions
	onStatus StatusCallback
	onProg   ProgressCallback
}

// NewReceiveExecutor creates a new ReceiveExecutor with the given options and callbacks.
// The callbacks are optional and can be nil if progress/status updates are not needed.
func NewReceiveExecutor(opts ReceiveOptions, onStatus StatusCallback, onProg ProgressCallback) *ReceiveExecutor {
	return &ReceiveExecutor{
		opts:     opts,
		onStatus: onStatus,
		onProg:   onProg,
	}
}

// Execute performs the receive operation: discovery, handshake, and download.
// Returns the path where the file was saved, or an error if the operation failed.
func (e *ReceiveExecutor) Execute(ctx context.Context) (string, error) {
	// Load configuration with defaults
	cfg := config.GetConfig()

	// Validate input
	if e.opts.Code == "" {
		return "", fmt.Errorf("receive requires a PAKE code")
	}

	// Apply config defaults
	workers := cfg.ParallelWorkers
	if e.opts.Workers > 0 {
		workers = e.opts.Workers
	}
	chunkSize := cfg.ChunkSizeMB
	if e.opts.ChunkSizeMB > 0 {
		chunkSize = e.opts.ChunkSizeMB
	}
	noChecksum := cfg.NoChecksum || e.opts.NoChecksum

	_ = workers    // Reserved for future parallel download support
	_ = chunkSize  // Reserved for future parallel download support
	_ = noChecksum // Used by client internally

	// Discover services on the network
	e.emitStatus("Searching for sender...")

	searchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	services, err := discovery.Browse(searchCtx, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to browse for servers: %w", err)
	}

	if len(services) == 0 {
		return "", fmt.Errorf("no warp services found on the network")
	}

	// Try to connect to each service with the provided code
	e.emitStatus("Performing PAKE handshake...")

	downloader := client.NewDownloader(nil, nil, nil)
	var url string
	var key []byte
	var found bool

	for _, svc := range services {
		baseURL := fmt.Sprintf("http://%s:%d", svc.IP, svc.Port)
		sharedKey, code, err := downloader.PerformPAKEHandshake(baseURL, e.opts.Code)
		if err == nil {
			e.emitStatus(fmt.Sprintf("Connected to %s", svc.Name))
			url = baseURL + protocol.PathPrefix + code
			key = sharedKey
			found = true
			break
		}
	}

	if !found {
		return "", errors.ConnectionError("any warp server", fmt.Errorf("no server found with the provided code"))
	}

	// Initialize state manager for resume support
	var stateManager *resume.TransferStateManager
	homeDir, err := os.UserHomeDir()
	if err == nil {
		stateDir := filepath.Join(homeDir, ".warp", "transfers")
		if sm, err := resume.NewTransferStateManager(stateDir); err == nil {
			stateManager = sm
		}
	}

	// Create downloader with state manager
	downloader = client.NewDownloader(nil, stateManager, nil)

	// Start download with progress callback
	e.emitStatus("Starting download...")

	onProgress := func(p progress.Progress) {
		if e.onProg == nil {
			return
		}
		e.onProg(p)
	}

	savedPath, err := downloader.Receive(ctx, url, e.opts.OutputPath, e.opts.Force, onProgress, nil, key)
	if err != nil {
		return "", err
	}

	return savedPath, nil
}

// DiscoverServices searches for warp services on the local network.
// Returns a list of discovered services without attempting to connect.
func (e *ReceiveExecutor) DiscoverServices(ctx context.Context, timeout time.Duration) ([]ServiceInfo, error) {
	e.emitStatus("Searching for services...")

	services, err := discovery.Browse(ctx, timeout)
	if err != nil {
		return nil, fmt.Errorf("service discovery failed: %w", err)
	}

	result := make([]ServiceInfo, 0, len(services))
	for _, svc := range services {
		result = append(result, ServiceInfo{
			Name:  svc.Name,
			Mode:  svc.Mode,
			IP:    svc.IP.String(),
			Port:  svc.Port,
			URL:   svc.URL,
			Token: svc.Token,
		})
	}

	return result, nil
}

// emitStatus sends a status message if a callback is registered.
func (e *ReceiveExecutor) emitStatus(msg string) {
	if e.onStatus != nil {
		e.onStatus(msg)
	}
}

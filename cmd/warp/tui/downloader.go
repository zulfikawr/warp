// Package tui provides the terminal user interface for warp.
// This file contains the TUI-specific downloader that bridges core receive
// functionality with the Bubble Tea event loop.
package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zulfikawr/warp/internal/client"
	"github.com/zulfikawr/warp/internal/discovery"
	"github.com/zulfikawr/warp/internal/progress"
	"github.com/zulfikawr/warp/internal/protocol"
	"github.com/zulfikawr/warp/internal/resume"
)

// DownloadStatusMsg is for status updates (Searching, Handshaking, etc.)
type DownloadStatusMsg string

// TUIDownloader handles downloads and emits tea.Msgs to the TUI event loop.
// It wraps the core receive functionality with TUI-specific progress reporting.
type TUIDownloader struct {
	client          *http.Client
	stateManager    *resume.TransferStateManager
	downloader      *client.Downloader
	isPaused        bool
	checkpoint      *resume.Checkpoint
	transferSession *resume.TransferSession

	// Channels for pause/resume coordination
	pauseChan  chan struct{}
	resumeChan chan struct{}
	cancelChan chan struct{}
	mu         sync.Mutex
}

// NewTUIDownloader creates a new TUI downloader with optimized HTTP settings.
func NewTUIDownloader() *TUIDownloader {
	// Initialize state manager for checkpoints
	homeDir, _ := os.UserHomeDir()
	stateManager, _ := resume.NewTransferStateManager(filepath.Join(homeDir, ".warp", "transfers"))

	return &TUIDownloader{
		client: &http.Client{
			Timeout: 0, // No timeout for large file transfers
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  true, // We handle compression manually
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		stateManager: stateManager,
		isPaused:     false,
		pauseChan:    make(chan struct{}, 1),
		resumeChan:   make(chan struct{}, 1),
		cancelChan:   make(chan struct{}, 1),
	}
}

// downloadProgressReader tracks progress and emits messages to the TUI.
type downloadProgressReader struct {
	r         io.Reader
	total     int64
	current   int64
	startTime time.Time
	filename  string
	progChan  chan<- tea.Msg
	lastEmit  time.Time
}

// Read implements io.Reader and emits progress updates.
func (p *downloadProgressReader) Read(b []byte) (int, error) {
	if p.startTime.IsZero() {
		p.startTime = time.Now()
	}

	n, err := p.r.Read(b)
	p.current += int64(n)

	// Emit progress update every 100ms to avoid flooding the event loop
	if time.Since(p.lastEmit) > 100*time.Millisecond || err != nil || p.current == p.total {
		elapsed := time.Since(p.startTime)
		speed := float64(p.current) / elapsed.Seconds()
		var eta time.Duration
		if speed > 0 {
			remaining := p.total - p.current
			eta = time.Duration(float64(remaining) / speed * float64(time.Second))
		}

		p.progChan <- progress.Progress{
			TotalBytes:       p.total,
			TransferredBytes: p.current,
			FileName:         p.filename,
			SpeedBytesPerSec: speed,
			ETA:              eta,
		}
		p.lastEmit = time.Now()
	}

	return n, err
}

// Receive performs the download operation and sends progress updates to the channel.
// This method runs in a goroutine and communicates with the TUI via the progress channel.
func (d *TUIDownloader) Receive(code string, outputPath string, progChan chan<- tea.Msg) {
	// Step 1: Discovery - find available warp services
	progChan <- DownloadStatusMsg("Searching for sender...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	services, err := discovery.Browse(ctx, 5*time.Second)
	if err != nil {
		progChan <- progress.Progress{Error: fmt.Errorf("search failed: %w", err)}
		return
	}

	// Step 2: PAKE Handshake - authenticate with the sender
	progChan <- DownloadStatusMsg("Performing PAKE handshake...")

	dl := client.NewDownloader(d.client, d.stateManager, d.checkpoint)
	d.downloader = dl

	var url string
	var key []byte
	found := false

	for _, s := range services {
		baseURL := fmt.Sprintf("http://%s:%d", s.IP, s.Port)
		sharedKey, handshakeCode, err := dl.PerformPAKEHandshake(baseURL, code)
		if err == nil {
			url = baseURL + protocol.PathPrefix + handshakeCode
			key = sharedKey
			found = true
			break
		}
	}

	if !found {
		progChan <- progress.Progress{Error: fmt.Errorf("no sender found with code '%s'", code)}
		return
	}

	// Step 3: Download - transfer the file with progress updates
	if d.checkpoint != nil {
		progChan <- DownloadStatusMsg(fmt.Sprintf("Resuming download from %.1f%%...", d.checkpoint.GetProgress()*100))
	} else {
		progChan <- DownloadStatusMsg("Starting download...")
	}

	var resumedFrom float64
	if d.checkpoint != nil {
		resumedFrom = d.checkpoint.GetProgress() * 100
	}

	onProgress := func(p progress.Progress) {
		chunksComplete := 0
		chunksTotal := 0
		if d.checkpoint != nil {
			chunksComplete = len(d.checkpoint.CompletedChunks)
			chunksTotal = d.checkpoint.TotalChunks
		}

		// Check actual downloader pause state
		isPaused := d.isPaused
		if d.downloader != nil {
			isPaused = d.downloader.IsPaused()
		}

		progChan <- progress.Progress{
			TotalBytes:       p.TotalBytes,
			TransferredBytes: p.TransferredBytes,
			FileName:         filepath.Base(p.FileName),
			SpeedBytesPerSec: p.SpeedBytesPerSec, // Convert Mbps to bytes/sec
			ETA:              p.ETA,
			IsComplete:       p.IsComplete,
			SavedPath:        p.FileName,
			Verified:         p.IsComplete,
			IsResumable:      d.stateManager != nil,
			ResumedFrom:      resumedFrom,
			CompletedChunks:  chunksComplete,
			TotalChunks:      chunksTotal,
			IsPaused:         isPaused,
		}
	}

	savedPath, err := dl.Receive(context.Background(), url, outputPath, false, onProgress, nil, key)
	if err != nil {
		progChan <- progress.Progress{Error: err}
		return
	}

	// The callback should have sent the final IsComplete message,
	// but we keep savedPath for potential future use
	_ = savedPath
}

// Pause pauses the current download
func (d *TUIDownloader) Pause() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isPaused {
		return nil // Already paused
	}

	d.isPaused = true

	// Signal pause to download goroutine
	select {
	case d.pauseChan <- struct{}{}:
	default:
	}

	// Pause the actual downloader
	if d.downloader != nil {
		d.downloader.Pause()
	}

	// Pause transfer session if exists
	if d.transferSession != nil {
		return d.transferSession.Pause()
	}

	return nil
}

// Resume resumes a paused download
func (d *TUIDownloader) Resume() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isPaused {
		return nil // Not paused
	}

	d.isPaused = false

	// Signal resume to download goroutine
	select {
	case d.resumeChan <- struct{}{}:
	default:
	}

	// Resume the actual downloader
	if d.downloader != nil {
		d.downloader.Resume()
	}

	// Resume transfer session if exists
	if d.transferSession != nil {
		return d.transferSession.Resume()
	}

	return nil
}

// TogglePause toggles between paused and active states
func (d *TUIDownloader) TogglePause() error {
	d.mu.Lock()
	isPaused := d.isPaused
	d.mu.Unlock()

	if isPaused {
		return d.Resume()
	}
	return d.Pause()
}

// Cancel cancels the current download
func (d *TUIDownloader) Cancel() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Signal cancel to download goroutine
	select {
	case d.cancelChan <- struct{}{}:
	default:
	}

	// Cancel transfer session if exists
	if d.transferSession != nil {
		return d.transferSession.Cancel()
	}

	return nil
}

// IsPaused returns whether the download is currently paused
func (d *TUIDownloader) IsPaused() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.isPaused
}

// LoadCheckpoint loads a checkpoint for resuming
func (d *TUIDownloader) LoadCheckpoint(sessionID string) error {
	if d.stateManager == nil {
		return fmt.Errorf("state manager not initialized")
	}

	checkpoint, err := d.stateManager.LoadCheckpoint(sessionID)
	if err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	d.checkpoint = checkpoint
	return nil
}

// ListResumable returns a list of resumable downloads
func (d *TUIDownloader) ListResumable() ([]*resume.CheckpointSummary, error) {
	if d.stateManager == nil {
		return nil, fmt.Errorf("state manager not initialized")
	}

	summaries, err := d.stateManager.ListResumable()
	if err != nil {
		return nil, err
	}

	// Filter for downloads only
	var downloads []*resume.CheckpointSummary
	for _, s := range summaries {
		if s.Direction == "download" {
			downloads = append(downloads, s)
		}
	}

	return downloads, nil
}

// ResumeFromCheckpoint resumes a download from a saved checkpoint
// This method requires the user to provide the PAKE code again since it's not stored
func (d *TUIDownloader) ResumeFromCheckpoint(sessionID string, code string, progChan chan<- tea.Msg) {
	// Load the checkpoint
	progChan <- DownloadStatusMsg("Loading checkpoint...")

	if d.stateManager == nil {
		progChan <- progress.Progress{Error: fmt.Errorf("state manager not initialized")}
		return
	}

	checkpoint, err := d.stateManager.LoadCheckpoint(sessionID)
	if err != nil {
		progChan <- progress.Progress{Error: fmt.Errorf("failed to load checkpoint: %w", err)}
		return
	}

	d.checkpoint = checkpoint

	// Step 1: Discovery - find available warp services
	progChan <- DownloadStatusMsg("Searching for sender...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	services, err := discovery.Browse(ctx, 5*time.Second)
	if err != nil {
		progChan <- progress.Progress{Error: fmt.Errorf("search failed: %w", err)}
		return
	}

	// Step 2: PAKE Handshake - authenticate with the sender using provided code
	progChan <- DownloadStatusMsg("Performing PAKE handshake...")

	dl := client.NewDownloader(d.client, d.stateManager, d.checkpoint)
	d.downloader = dl

	var url string
	var key []byte
	found := false

	for _, s := range services {
		baseURL := fmt.Sprintf("http://%s:%d", s.IP, s.Port)
		sharedKey, handshakeCode, err := dl.PerformPAKEHandshake(baseURL, code)
		if err == nil {
			url = baseURL + protocol.PathPrefix + handshakeCode
			key = sharedKey
			found = true
			break
		}
	}

	if !found {
		progChan <- progress.Progress{Error: fmt.Errorf("no sender found with code '%s' - sender may need to restart 'warp send'", code)}
		return
	}

	// Step 3: Resume download with progress updates
	resumedFrom := checkpoint.GetProgress() * 100
	progChan <- DownloadStatusMsg(fmt.Sprintf("Resuming download from %.1f%%...", resumedFrom))

	onProgress := func(p progress.Progress) {
		chunksComplete := len(checkpoint.CompletedChunks)
		chunksTotal := checkpoint.TotalChunks

		// Check actual downloader pause state
		isPaused := d.isPaused
		if d.downloader != nil {
			isPaused = d.downloader.IsPaused()
		}

		progChan <- progress.Progress{
			TotalBytes:       p.TotalBytes,
			TransferredBytes: p.TransferredBytes,
			FileName:         filepath.Base(p.FileName),
			SpeedBytesPerSec: p.SpeedBytesPerSec, // Convert Mbps to bytes/sec
			ETA:              p.ETA,
			IsComplete:       p.IsComplete,
			SavedPath:        p.FileName,
			Verified:         p.IsComplete,
			IsResumable:      true,
			ResumedFrom:      resumedFrom,
			CompletedChunks:  chunksComplete,
			TotalChunks:      chunksTotal,
			IsPaused:         isPaused,
		}
	}

	// Use force=true to allow overwriting/appending to existing file
	savedPath, err := dl.Receive(context.Background(), url, checkpoint.DestinationPath, true, onProgress, nil, key)
	if err != nil {
		progChan <- progress.Progress{Error: err}
		return
	}

	_ = savedPath
}

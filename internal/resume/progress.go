package resume

import (
	"sync"
	"time"

	"github.com/zulfikawr/warp/internal/progress"
)

// ProgressCallback is called when progress updates
type ProgressCallback func(info progress.Progress)

// ProgressTracker monitors transfer progress
type ProgressTracker struct {
	sessionID        string
	totalBytes       int64
	transferredBytes int64
	totalChunks      int
	completedChunks  int
	startTime        time.Time
	lastUpdate       time.Time
	speedSamples     []float64 // Rolling window for speed calculation
	callbacks        []ProgressCallback
	mu               sync.RWMutex
}

const (
	maxSpeedSamples = 10 // Keep last 10 speed samples for averaging
)

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(sessionID string, totalBytes int64, totalChunks int) *ProgressTracker {
	return &ProgressTracker{
		sessionID:    sessionID,
		totalBytes:   totalBytes,
		totalChunks:  totalChunks,
		startTime:    time.Now(),
		lastUpdate:   time.Now(),
		speedSamples: make([]float64, 0, maxSpeedSamples),
		callbacks:    make([]ProgressCallback, 0),
	}
}

// OnProgress registers a progress callback
func (p *ProgressTracker) OnProgress(cb ProgressCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callbacks = append(p.callbacks, cb)
}

// UpdateBytes records bytes transferred
func (p *ProgressTracker) UpdateBytes(bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Ensure monotonic progress
	if bytes > p.transferredBytes {
		p.transferredBytes = bytes
		p.lastUpdate = time.Now()
		p.updateSpeedSample()
	}

	// Trigger callbacks
	p.notifyCallbacks()
}

// CompleteChunk marks a chunk as completed
func (p *ProgressTracker) CompleteChunk(chunkID int, bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.completedChunks++
	p.transferredBytes += bytes
	p.lastUpdate = time.Now()
	p.updateSpeedSample()

	// Trigger callbacks
	p.notifyCallbacks()
}

// GetProgress returns current progress information
func (p *ProgressTracker) GetProgress() progress.Progress {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return progress.Progress{
		TransferID:       p.sessionID,
		TotalBytes:       p.totalBytes,
		TransferredBytes: p.transferredBytes,
		TotalChunks:      p.totalChunks,
		CompletedChunks:  p.completedChunks,
		SpeedBytesPerSec: p.getAverageSpeed(),
		ETA:              p.calculateETA(),
		StartTime:        p.startTime,
		LastUpdate:       p.lastUpdate,
	}
}

// GetETA calculates estimated time to completion
func (p *ProgressTracker) GetETA() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.calculateETA()
}

// GetSpeed returns current transfer speed in bytes/second
func (p *ProgressTracker) GetSpeed() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.getAverageSpeed()
}

// RestoreProgress restores progress from checkpoint
func (p *ProgressTracker) RestoreProgress(completedChunks []int, bytesTransferred int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.completedChunks = len(completedChunks)
	p.transferredBytes = bytesTransferred
	p.lastUpdate = time.Now()
}

// updateSpeedSample adds a new speed sample to the rolling window
// Must be called with lock held
func (p *ProgressTracker) updateSpeedSample() {
	elapsed := time.Since(p.startTime).Seconds()
	if elapsed > 0 {
		speed := float64(p.transferredBytes) / elapsed

		// Add to samples
		p.speedSamples = append(p.speedSamples, speed)

		// Keep only last N samples
		if len(p.speedSamples) > maxSpeedSamples {
			p.speedSamples = p.speedSamples[1:]
		}
	}
}

// getAverageSpeed calculates average speed from samples
// Must be called with lock held
func (p *ProgressTracker) getAverageSpeed() float64 {
	if len(p.speedSamples) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, speed := range p.speedSamples {
		sum += speed
	}
	return sum / float64(len(p.speedSamples))
}

// calculateETA calculates estimated time to completion
// Must be called with lock held
func (p *ProgressTracker) calculateETA() time.Duration {
	speed := p.getAverageSpeed()
	if speed <= 0 || p.transferredBytes >= p.totalBytes {
		return 0
	}

	remaining := p.totalBytes - p.transferredBytes
	etaSeconds := float64(remaining) / speed
	return time.Duration(etaSeconds * float64(time.Second))
}

// notifyCallbacks triggers all registered callbacks
// Must be called with lock held
func (p *ProgressTracker) notifyCallbacks() {
	if len(p.callbacks) == 0 {
		return
	}

	info := progress.Progress{
		TransferID:       p.sessionID,
		TotalBytes:       p.totalBytes,
		TransferredBytes: p.transferredBytes,
		TotalChunks:      p.totalChunks,
		CompletedChunks:  p.completedChunks,
		SpeedBytesPerSec: p.getAverageSpeed(),
		ETA:              p.calculateETA(),
		StartTime:        p.startTime,
		LastUpdate:       p.lastUpdate,
	}

	// Call callbacks without holding lock to avoid deadlocks
	callbacks := make([]ProgressCallback, len(p.callbacks))
	copy(callbacks, p.callbacks)

	// Release lock before calling callbacks
	p.mu.Unlock()
	for _, cb := range callbacks {
		cb(info)
	}
	p.mu.Lock()
}

// Reset resets the progress tracker
func (p *ProgressTracker) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.transferredBytes = 0
	p.completedChunks = 0
	p.startTime = time.Now()
	p.lastUpdate = time.Now()
	p.speedSamples = make([]float64, 0, maxSpeedSamples)
}

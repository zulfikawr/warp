// Package progress contains shared types for file transfer progress tracking.
// This package is intentionally small to avoid import cycles.
package progress

import "time"

// Progress represents the current state of a file transfer operation.
// This is the unified progress type used throughout the application for all transfers.
// It consolidates upload, download, and resume progress tracking into a single type.
type Progress struct {
	// Identity
	// TransferID is a unique identifier for this transfer (session ID, upload ID, etc.)
	TransferID string

	// FileName is the name of the file being transferred.
	FileName string

	// Direction indicates the transfer type: "upload", "download", or "host"
	Direction string

	// Progress tracking
	// TotalBytes is the total size of the file in bytes.
	// May be 0 if the total size is unknown.
	TotalBytes int64

	// TransferredBytes is the number of bytes transferred so far.
	// For uploads: bytes sent. For downloads: bytes received.
	TransferredBytes int64

	// Performance
	// SpeedBytesPerSec is the current transfer speed in bytes per second.
	SpeedBytesPerSec float64

	// StartTime is when the transfer began.
	StartTime time.Time

	// ETA is the estimated time remaining for the transfer to complete.
	ETA time.Duration

	// Chunking (for parallel uploads)
	// TotalChunks is the total number of chunks for chunked transfers.
	// 0 for non-chunked transfers.
	TotalChunks int

	// CompletedChunks is the number of chunks that have been successfully transferred.
	CompletedChunks int

	// State
	// IsComplete indicates whether the transfer has finished successfully.
	IsComplete bool

	// IsPaused indicates whether the transfer is currently paused.
	IsPaused bool

	// Error contains any error that occurred during the transfer.
	// If non-nil, the transfer has failed.
	Error error

	// Post-transfer
	// SavedPath is the final path where the file was saved (for downloads).
	SavedPath string

	// Verified indicates whether the file checksum was verified successfully.
	Verified bool

	// Resume support
	// IsResumable indicates whether this transfer supports resuming.
	IsResumable bool

	// ResumedFrom is the percentage at which the transfer was resumed (0-100).
	// 0 if this is a new transfer (not resumed).
	ResumedFrom float64

	// LastUpdate is the timestamp of the last progress update.
	LastUpdate time.Time
}

// Percent returns the transfer progress as a percentage (0-100).
// Returns 0 if TotalBytes is 0 or unknown.
func (p Progress) Percent() float64 {
	if p.TotalBytes == 0 {
		return 0
	}
	return float64(p.TransferredBytes) / float64(p.TotalBytes) * 100
}

// SpeedMbps returns the transfer speed in megabits per second.
func (p Progress) SpeedMbps() float64 {
	return p.SpeedBytesPerSec * 8 / 1_000_000
}

// Remaining returns the number of bytes remaining to be transferred.
func (p Progress) Remaining() int64 {
	if p.TotalBytes == 0 {
		return 0
	}
	remaining := p.TotalBytes - p.TransferredBytes
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Duration returns the time elapsed since the transfer started.
func (p Progress) Duration() time.Duration {
	if p.StartTime.IsZero() {
		return 0
	}
	return time.Since(p.StartTime)
}

// IsChunked returns true if this is a chunked transfer.
func (p Progress) IsChunked() bool {
	return p.TotalChunks > 0
}

// ChunksRemaining returns the number of chunks yet to be completed.
func (p Progress) ChunksRemaining() int {
	if !p.IsChunked() {
		return 0
	}
	remaining := p.TotalChunks - p.CompletedChunks
	if remaining < 0 {
		return 0
	}
	return remaining
}

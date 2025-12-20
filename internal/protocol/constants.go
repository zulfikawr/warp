package protocol

import "time"

// Buffer sizes for I/O operations
const (
	// BufferSizeSmall is used for small files (< 64KB)
	BufferSizeSmall = 8 * 1024 // 8KB

	// BufferSizeMedium is used for medium files (64KB - 1MB)
	BufferSizeMedium = 64 * 1024 // 64KB

	// BufferSizeLarge is used for large files (1MB - 100MB)
	BufferSizeLarge = 1024 * 1024 // 1MB

	// BufferSizeVeryLarge is used for very large files (> 100MB)
	BufferSizeVeryLarge = 4 * 1024 * 1024 // 4MB

	// DefaultBufferSize is the default buffer size when size is unknown
	DefaultBufferSize = BufferSizeLarge
)

// File size thresholds for buffer selection
const (
	// SmallFileThreshold is the size below which small buffers are used
	SmallFileThreshold = 64 * 1024 // 64KB

	// MediumFileThreshold is the size below which medium buffers are used
	MediumFileThreshold = 1024 * 1024 // 1MB

	// LargeFileThreshold is the size below which large buffers are used
	LargeFileThreshold = 100 * 1024 * 1024 // 100MB

	// SendfileThreshold is the minimum size for using zero-copy sendfile
	SendfileThreshold = 10 * 1024 * 1024 // 10MB

	// MaxCacheFileSize is the maximum file size to cache in memory
	MaxCacheFileSize = 10 * 1024 * 1024 // 10MB
)

// Progress update intervals
const (
	// ProgressUpdateInterval is how often progress is updated in the UI
	ProgressUpdateInterval = 200 * time.Millisecond

	// WebSocketUpdateInterval is how often WebSocket progress messages are sent
	WebSocketUpdateInterval = 100 * time.Millisecond

	// ProgressRefreshRate is the refresh rate in Hz
	ProgressRefreshRate = 10 // 10 updates per second
)

// UI constants
const (
	// ProgressBarWidth is the number of characters in the progress bar
	ProgressBarWidth = 20

	// ProgressBarFilled is the character used for the filled portion
	ProgressBarFilled = "="

	// ProgressBarEmpty is the character used for the empty portion
	ProgressBarEmpty = " "
)

// Timeouts
const (
	// ReadTimeout is the maximum time to read a request
	ReadTimeout = 10 * time.Minute

	// WriteTimeout is the maximum time to write a response
	WriteTimeout = 15 * time.Minute

	// IdleTimeout is the maximum time a connection can be idle
	IdleTimeout = 5 * time.Minute
)

// Path prefixes
const (
	// PathPrefix is the URL path prefix for downloads
	PathPrefix = "/d/"

	// UploadPathPrefix is the URL path prefix for uploads
	UploadPathPrefix = "/u/"
)

// GetOptimalBufferSize returns the best buffer size for a given file size
func GetOptimalBufferSize(fileSize int64) int {
	switch {
	case fileSize < SmallFileThreshold:
		return BufferSizeSmall
	case fileSize < MediumFileThreshold:
		return BufferSizeMedium
	case fileSize < LargeFileThreshold:
		return BufferSizeLarge
	default:
		return BufferSizeVeryLarge
	}
}

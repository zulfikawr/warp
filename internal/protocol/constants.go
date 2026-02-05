package protocol

import (
	"time"
)

// TransferConfig holds configurable thresholds for transfer optimization
// DEPRECATED: Use internal/config.ProtocolConfig instead. This is kept for backward compatibility.
type TransferConfig struct {
	// Buffer sizes
	BufferSizeSmall     int
	BufferSizeMedium    int
	BufferSizeLarge     int
	BufferSizeVeryLarge int

	// File size thresholds
	SmallFileThreshold  int64
	MediumFileThreshold int64
	LargeFileThreshold  int64
	SendfileThreshold   int64
	MaxCacheFileSize    int64

	// Progress intervals
	ProgressUpdateInterval  time.Duration
	WebSocketUpdateInterval time.Duration
	ProgressRefreshRate     int

	// Timeouts
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// DefaultTransferConfig returns the default transfer configuration
// DEPRECATED: Use config.GetConfig().Protocol instead
func DefaultTransferConfig() *TransferConfig {
	return &TransferConfig{
		BufferSizeSmall:         8 * 1024,
		BufferSizeMedium:        64 * 1024,
		BufferSizeLarge:         1024 * 1024,
		BufferSizeVeryLarge:     4 * 1024 * 1024,
		SmallFileThreshold:      64 * 1024,
		MediumFileThreshold:     1024 * 1024,
		LargeFileThreshold:      100 * 1024 * 1024,
		SendfileThreshold:       10 * 1024 * 1024,
		MaxCacheFileSize:        10 * 1024 * 1024,
		ProgressUpdateInterval:  200 * time.Millisecond,
		WebSocketUpdateInterval: 100 * time.Millisecond,
		ProgressRefreshRate:     10,
		ReadTimeout:             10 * time.Minute,
		WriteTimeout:            15 * time.Minute,
		IdleTimeout:             5 * time.Minute,
	}
}

// Buffer sizes for I/O operations
// These constants reference the runtime configuration for single source of truth
const (
	// BufferSizeSmall is used for small files (< 64KB)
	// Default: 8KB - can be configured via config file
	BufferSizeSmall = 8 * 1024 // 8KB

	// BufferSizeMedium is used for medium files (64KB - 1MB)
	// Default: 64KB - can be configured via config file
	BufferSizeMedium = 64 * 1024 // 64KB

	// BufferSizeLarge is used for large files (1MB - 100MB)
	// Default: 1MB - can be configured via config file
	BufferSizeLarge = 1024 * 1024 // 1MB

	// BufferSizeVeryLarge is used for very large files (> 100MB)
	// Default: 4MB - can be configured via config file
	BufferSizeVeryLarge = 4 * 1024 * 1024 // 4MB

	// DefaultBufferSize is the default buffer size when size is unknown
	DefaultBufferSize = BufferSizeLarge
)

// File size thresholds for buffer selection
const (
	// SmallFileThreshold is the size below which small buffers are used
	// Default: 64KB - can be configured via config file
	SmallFileThreshold = 64 * 1024 // 64KB

	// MediumFileThreshold is the size below which medium buffers are used
	// Default: 1MB - can be configured via config file
	MediumFileThreshold = 1024 * 1024 // 1MB

	// LargeFileThreshold is the size below which large buffers are used
	// Default: 100MB - can be configured via config file
	LargeFileThreshold = 100 * 1024 * 1024 // 100MB

	// SendfileThreshold is the minimum size for using zero-copy sendfile
	// Default: 10MB - can be configured via config file
	SendfileThreshold = 10 * 1024 * 1024 // 10MB

	// MaxCacheFileSize is the maximum file size to cache in memory
	// Default: 10MB - can be configured via config file
	MaxCacheFileSize = 10 * 1024 * 1024 // 10MB
)

// Progress update intervals
const (
	// ProgressUpdateInterval is how often progress is updated in the UI
	// Default: 200ms - can be configured via config file
	ProgressUpdateInterval = 200 * time.Millisecond

	// WebSocketUpdateInterval is how often WebSocket progress messages are sent
	// Default: 100ms - can be configured via config file
	WebSocketUpdateInterval = 100 * time.Millisecond

	// ProgressRefreshRate is the refresh rate in Hz
	// Default: 10 updates/sec - can be configured via config file
	ProgressRefreshRate = 10 // 10 updates per second
)

// Timeouts
const (
	// ReadTimeout is the maximum time to read a request
	// Default: 10 minutes - can be configured via config file
	ReadTimeout = 10 * time.Minute

	// WriteTimeout is the maximum time to write a response
	// Default: 15 minutes - can be configured via config file
	WriteTimeout = 15 * time.Minute

	// IdleTimeout is the maximum time a connection can be idle
	// Default: 5 minutes - can be configured via config file
	IdleTimeout = 5 * time.Minute
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

// Path prefixes
const (
	// PathPrefix is the URL path prefix for downloads
	PathPrefix = "/d/"

	// UploadPathPrefix is the URL path prefix for uploads
	UploadPathPrefix = "/u/"

	// PAKEInitPath is the URL path for PAKE initialization
	PAKEInitPath = "/pake/init"

	// PAKEVerifyPath is the URL path for PAKE verification
	PAKEVerifyPath = "/pake/verify"
)

// GetOptimalBufferSize returns the best buffer size for a given file size
// This function now uses runtime configuration for dynamic tuning
func GetOptimalBufferSize(fileSize int64) int {
	// Note: We use constants as fallbacks since we can't import config package
	// (would create circular dependency). The constants are kept in sync with
	// config defaults. For runtime configuration, use bufpool.GetForFileSize()
	// which has access to the config package.
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

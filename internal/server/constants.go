package server

import (
	"time"

	"github.com/zulfikawr/warp/internal/protocol"
)

// WebSocket configuration
const (
	// WebSocketUpdateInterval uses the protocol constant for consistency
	WebSocketUpdateInterval = protocol.WebSocketUpdateInterval
	WebSocketReadBuffer     = 1024
	WebSocketWriteBuffer    = 1024
)

// Session management
const (
	SessionCleanupInterval = 15 * time.Minute
	StaleSessionThreshold  = 1 * time.Hour
)

// Upload limits
const (
	MaxUploadSize     = 10 << 30 // 10GB
	MaxPartSize       = 10 << 30 // 10GB
	MaxFilenameLength = 255
)

// Buffer sizes
const (
	MinBufferSize     = protocol.BufferSizeSmall     // 8KB
	DefaultBufferSize = protocol.BufferSizeLarge     // 1MB
	MaxBufferSize     = protocol.BufferSizeVeryLarge // 4MB
)

// TCP tuning
const (
	TCPKeepAlivePeriod   = 3 * time.Minute
	TCPSendBufferSize    = protocol.BufferSizeVeryLarge // 4MB
	TCPReceiveBufferSize = protocol.BufferSizeVeryLarge // 4MB
)

// Timeouts
const (
	ShutdownTimeout         = 30 * time.Second
	ConnectionReadDeadline  = 1 * time.Hour
	ConnectionWriteDeadline = 5 * time.Second
)

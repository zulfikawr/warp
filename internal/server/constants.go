package server

import "time"

// WebSocket configuration
const (
	WebSocketUpdateInterval = 100 * time.Millisecond
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
	MinBufferSize     = 4096
	DefaultBufferSize = 1 << 20  // 1MB
	MaxBufferSize     = 16 << 20 // 16MB
)

// TCP tuning
const (
	TCPKeepAlivePeriod   = 3 * time.Minute
	TCPSendBufferSize    = 4 << 20 // 4MB
	TCPReceiveBufferSize = 4 << 20 // 4MB
)

// Timeouts
const (
	ShutdownTimeout         = 30 * time.Second
	ConnectionReadDeadline  = 1 * time.Hour
	ConnectionWriteDeadline = 5 * time.Second
)

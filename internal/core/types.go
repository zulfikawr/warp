// Package core provides the shared command logic and types for the warp CLI and TUI.
// It serves as the single source of truth for all command functionality, ensuring
// consistent behavior across different presentation layers.
package core

import (
	"time"

	"github.com/zulfikawr/warp/internal/progress"
)

// ProgressCallback is a function type for receiving progress updates.
type ProgressCallback func(progress.Progress)

// StatusCallback is a function type for receiving status messages.
type StatusCallback func(message string)

// ServiceInfo contains information about a discovered warp service.
type ServiceInfo struct {
	Name  string
	Mode  string
	IP    string
	Port  int
	URL   string
	Token string
}

// ServerInfo contains information about a running server.
type ServerInfo struct {
	URL       string
	Code      string
	Port      int
	IP        string
	QRCode    string
	Protocols []string
}

// SendOptions contains all configuration for the send command.
// These options control how files or text are shared over the network.
type SendOptions struct {
	// Port specifies a specific port to listen on. If 0, a random port is chosen.
	Port int

	// InterfaceName specifies the network interface to bind to.
	// If empty, the default interface is used.
	InterfaceName string

	// RateLimitMbps limits the transfer speed in megabits per second.
	// If 0, no rate limiting is applied.
	RateLimitMbps float64

	// CacheSizeMB sets the file cache size in megabytes.
	// If 0, the default cache size is used.
	CacheSizeMB int64

	// NoQR disables QR code generation for the transfer URL.
	NoQR bool

	// NoEncrypt disables PAKE encryption for the transfer.
	// Warning: This makes transfers insecure.
	NoEncrypt bool

	// TextContent contains text to share instead of a file.
	// If set, FilePath is ignored.
	TextContent string

	// StdinContent contains content read from stdin to share.
	// Combined with TextContent if both are set.
	StdinContent string

	// FilePath is the path to the file or directory to share.
	FilePath string

	// Resume enables resuming a paused transfer.
	Resume bool

	// SessionID specifies a specific session ID to resume.
	// If empty in resume mode, the user will be prompted to select.
	SessionID string

	// Verbose sets the verbosity level for logging (0=quiet, 1+=verbose).
	Verbose int
}

// ReceiveOptions contains all configuration for the receive command.
// These options control how files are downloaded from a sender.
type ReceiveOptions struct {
	// Code is the PAKE code for secure transfer authentication.
	Code string

	// OutputPath specifies where to save the received file.
	// If empty, the current directory is used with the original filename.
	OutputPath string

	// Force allows overwriting existing files without prompting.
	Force bool

	// Workers sets the number of parallel download workers.
	// If 0, the default number of workers is used.
	Workers int

	// ChunkSizeMB sets the chunk size for parallel downloads in megabytes.
	// If 0, the default chunk size is used.
	ChunkSizeMB int

	// NoChecksum skips SHA256 checksum verification after download.
	NoChecksum bool

	// Decrypt enables decryption of the received file.
	Decrypt bool

	// Resume enables resuming a paused download.
	Resume bool

	// SessionID specifies a specific session ID to resume.
	// If empty in resume mode, the user will be prompted to select.
	SessionID string

	// Verbose sets the verbosity level for logging (0=quiet, 1+=verbose).
	Verbose int
}

// HostOptions contains all configuration for the host command.
// These options control the upload server that receives files from others.
type HostOptions struct {
	// InterfaceName specifies the network interface to bind to.
	// If empty, the default interface is used.
	InterfaceName string

	// DestDir is the directory where uploaded files are saved.
	// If empty, the current directory is used.
	DestDir string

	// RateLimitMbps limits the transfer speed in megabits per second.
	// If 0, no rate limiting is applied.
	RateLimitMbps float64

	// NoQR disables QR code generation for the upload URL.
	NoQR bool

	// NoEncrypt disables PAKE encryption for uploads.
	// Warning: This makes transfers insecure.
	NoEncrypt bool

	// Resume enables restoring pending upload sessions.
	Resume bool

	// Verbose sets the verbosity level for logging (0=quiet, 1+=verbose).
	Verbose int
}

// SearchOptions contains all configuration for the search command.
// These options control network discovery of warp services.
type SearchOptions struct {
	// Timeout sets how long to search for services before returning.
	Timeout time.Duration

	// Verbose sets the verbosity level for logging (0=quiet, 1+=verbose).
	Verbose int
}

// ConfigOptions contains all configuration for the config command.
// These options control configuration management operations.
type ConfigOptions struct {
	// Subcommand specifies the config operation: "init", "show", "edit", "path".
	Subcommand string

	// Interactive enables interactive prompts for config initialization.
	Interactive bool
}

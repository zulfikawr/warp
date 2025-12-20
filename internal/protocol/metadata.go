package protocol

import (
	"fmt"
	"path/filepath"
)

// TransferType indicates the type of content being transferred
type TransferType int

const (
	// TransferTypeFile indicates a single file transfer
	TransferTypeFile TransferType = iota
	// TransferTypeDirectory indicates a directory transfer (will be zipped)
	TransferTypeDirectory
	// TransferTypeText indicates plain text content (displayed to stdout)
	TransferTypeText
	// TransferTypeStream indicates streaming data from stdin
	TransferTypeStream
)

// String returns the string representation of the transfer type
func (t TransferType) String() string {
	switch t {
	case TransferTypeFile:
		return "file"
	case TransferTypeDirectory:
		return "directory"
	case TransferTypeText:
		return "text"
	case TransferTypeStream:
		return "stream"
	default:
		return "unknown"
	}
}

// Metadata contains information about a transfer
type Metadata struct {
	// Name is the filename or identifier
	Name string
	// Size is the total size in bytes (0 for text/stream)
	Size int64
	// Type indicates what kind of transfer this is
	Type TransferType
	// Checksum is the SHA256 hash (hex encoded)
	Checksum string
	// ChunkSize for parallel uploads (0 for non-chunked)
	ChunkSize int64
	// Encrypted indicates if the content is encrypted
	Encrypted bool
	// ContentType is the MIME type (optional)
	ContentType string
}

// Validate checks if the metadata is valid
func (m *Metadata) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("metadata: name is required")
	}

	if m.Size < 0 {
		return fmt.Errorf("metadata: size cannot be negative: %d", m.Size)
	}

	if m.Type == TransferTypeFile || m.Type == TransferTypeDirectory {
		if m.Size == 0 {
			return fmt.Errorf("metadata: file/directory transfer requires non-zero size")
		}
	}

	if m.ChunkSize < 0 {
		return fmt.Errorf("metadata: chunk size cannot be negative: %d", m.ChunkSize)
	}

	// Validate checksum format if provided (should be 64 hex characters for SHA256)
	if m.Checksum != "" && len(m.Checksum) != 64 {
		return fmt.Errorf("metadata: invalid checksum length: expected 64, got %d", len(m.Checksum))
	}

	return nil
}

// IsChunked returns true if this transfer uses chunking
func (m *Metadata) IsChunked() bool {
	return m.ChunkSize > 0
}

// IsCompressible returns true if the file type is typically compressible
func (m *Metadata) IsCompressible() bool {
	if m.Type != TransferTypeFile {
		return false
	}

	ext := filepath.Ext(m.Name)
	compressibleExts := map[string]bool{
		".txt": true, ".json": true, ".xml": true, ".html": true,
		".css": true, ".js": true, ".csv": true, ".log": true,
		".md": true, ".yaml": true, ".yml": true, ".svg": true,
		".sql": true, ".sh": true, ".bat": true, ".ps1": true,
	}

	return compressibleExts[ext]
}

// ShouldUseZeroCopy returns true if zero-copy sendfile should be used (large uncompressed files)
func (m *Metadata) ShouldUseZeroCopy() bool {
	return m.Type == TransferTypeFile && m.Size >= SendfileThreshold && !m.IsCompressible()
}

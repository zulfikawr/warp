package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zulfikawr/warp/internal/bufpool"
	"github.com/zulfikawr/warp/internal/protocol"
)

// CachedFile represents a cached file in memory
type CachedFile struct {
	Data     []byte
	ModTime  time.Time
	ETag     string
	Size     int64
	CachedAt time.Time
}

// checksumCacheEntry caches file checksums with validation metadata
type checksumCacheEntry struct {
	checksum string
	modTime  time.Time
	size     int64
}

// computeFileChecksum calculates SHA256 hash of a file
func computeFileChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer func() { _ = f.Close() }()

	hash := sha256.New()
	buf := bufpool.Get(protocol.BufferSizeLarge)
	defer bufpool.Put(buf)

	if _, err := io.CopyBuffer(hash, f, *buf); err != nil {
		return "", fmt.Errorf("failed to compute checksum: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// getCachedChecksum retrieves or computes a file checksum with caching
func (s *Server) getCachedChecksum(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	// Check cache
	if val, ok := s.checksumCache.Load(path); ok {
		entry := val.(*checksumCacheEntry)
		// Verify file hasn't changed
		if entry.modTime.Equal(fi.ModTime()) && entry.size == fi.Size() {
			return entry.checksum, nil
		}
	}

	// Compute checksum
	checksum, err := computeFileChecksum(path)
	if err != nil {
		return "", fmt.Errorf("checksum computation failed: %w", err)
	}

	// Cache it
	s.checksumCache.Store(path, &checksumCacheEntry{
		checksum: checksum,
		modTime:  fi.ModTime(),
		size:     fi.Size(),
	})

	return checksum, nil
}

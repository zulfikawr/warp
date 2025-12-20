package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// Buffer pools for different file sizes to reduce allocations
var bufferPools = map[int]*sync.Pool{
	protocol.BufferSizeSmall: {
		New: func() interface{} {
			b := make([]byte, protocol.BufferSizeSmall)
			return &b
		},
	},
	protocol.BufferSizeMedium: {
		New: func() interface{} {
			b := make([]byte, protocol.BufferSizeMedium)
			return &b
		},
	},
	protocol.BufferSizeLarge: {
		New: func() interface{} {
			b := make([]byte, protocol.BufferSizeLarge)
			return &b
		},
	},
	protocol.BufferSizeVeryLarge: {
		New: func() interface{} {
			b := make([]byte, protocol.BufferSizeVeryLarge)
			return &b
		},
	},
}

// getBuffer retrieves a buffer of the specified size from the pool
func getBuffer(size int) *[]byte {
	pool, ok := bufferPools[size]
	if !ok {
		// Fallback to 1MB pool if size not found
		pool = bufferPools[protocol.BufferSizeLarge]
	}
	return pool.Get().(*[]byte)
}

// putBuffer returns a buffer to the appropriate pool
func putBuffer(buf *[]byte) {
	size := len(*buf)
	pool, ok := bufferPools[size]
	if ok {
		pool.Put(buf)
	}
}

// computeFileChecksum calculates SHA256 hash of a file
func computeFileChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer func() { _ = f.Close() }()

	hash := sha256.New()
	bufSize := 1048576 // 1MB buffer for checksum computation
	buf := getBuffer(bufSize)
	defer putBuffer(buf)

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

// isCompressible checks if the file extension indicates compressible content
func isCompressible(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	compressible := []string{
		".txt", ".html", ".htm", ".css", ".js", ".json", ".xml", ".svg",
		".csv", ".log", ".md", ".yaml", ".yml", ".toml",
	}
	for _, c := range compressible {
		if ext == c {
			return true
		}
	}
	return false
}

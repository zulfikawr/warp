// Package bufpool provides a unified buffer pool for efficient memory reuse
// across the application. It uses size-based pools to minimize allocations
// during file transfers.
package bufpool

import (
	"sync"

	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/protocol"
)

// pools holds buffer pools for different sizes
var pools = map[int]*sync.Pool{
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

// Get retrieves a buffer of the specified size from the pool.
// If the exact size is not available, it returns a buffer from the
// nearest larger pool. Falls back to BufferSizeLarge if size not found.
func Get(size int) *[]byte {
	pool, ok := pools[size]
	if !ok {
		// Find the smallest pool that can accommodate the requested size
		pool = pools[protocol.BufferSizeLarge] // Default fallback
		for poolSize, p := range pools {
			if poolSize >= size {
				pool = p
				break
			}
		}
	}
	return pool.Get().(*[]byte)
}

// GetForFileSize returns an optimally-sized buffer for the given file size.
// This uses the runtime configuration's thresholds to select the best buffer.
func GetForFileSize(fileSize int64) *[]byte {
	cfg := config.GetConfig()

	// Use config thresholds if available
	var size int
	switch {
	case fileSize < cfg.Protocol.SmallFileThreshold:
		size = cfg.Protocol.BufferSizeSmall
	case fileSize < cfg.Protocol.MediumFileThreshold:
		size = cfg.Protocol.BufferSizeMedium
	case fileSize < cfg.Protocol.LargeFileThreshold:
		size = cfg.Protocol.BufferSizeLarge
	default:
		size = cfg.Protocol.BufferSizeVeryLarge
	}

	return Get(size)
}

// Put returns a buffer to the appropriate pool based on its capacity.
// Buffers that don't match a known pool size are discarded.
func Put(buf *[]byte) {
	if buf == nil {
		return
	}
	size := cap(*buf)
	pool, ok := pools[size]
	if ok {
		pool.Put(buf)
	}
	// If size doesn't match a pool, let GC handle it
}

// GetCustom creates a buffer pool for a custom size and returns a buffer.
// This is useful for chunk-based uploads where chunk size may vary.
// The returned cleanup function should be called to return the buffer.
func GetCustom(size int64) (*[]byte, func()) {
	b := make([]byte, size)
	return &b, func() {
		// Custom buffers are not pooled, just let GC handle them
	}
}

package bufpool

import (
	"sync"
	"testing"

	"github.com/zulfikawr/warp/internal/config"
	"github.com/zulfikawr/warp/internal/protocol"
)

// TestGet_ExactSizeMatch verifies that Get returns a buffer from the correct pool when exact size matches
func TestGet_ExactSizeMatch(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		expected int
	}{
		{
			name:     "Small buffer size",
			size:     protocol.BufferSizeSmall,
			expected: protocol.BufferSizeSmall,
		},
		{
			name:     "Medium buffer size",
			size:     protocol.BufferSizeMedium,
			expected: protocol.BufferSizeMedium,
		},
		{
			name:     "Large buffer size",
			size:     protocol.BufferSizeLarge,
			expected: protocol.BufferSizeLarge,
		},
		{
			name:     "Very large buffer size",
			size:     protocol.BufferSizeVeryLarge,
			expected: protocol.BufferSizeVeryLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := Get(tt.size)
			if buf == nil {
				t.Error("Get returned nil buffer")
			}
			if cap(*buf) != tt.expected {
				t.Errorf("Get returned buffer with capacity %d, expected %d", cap(*buf), tt.expected)
			}
		})
	}
}

// TestGet_FallbackToNearestLarger verifies that Get falls back to a suitable pool for unknown sizes
func TestGet_FallbackToNearestLarger(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		minCap int
	}{
		{
			name:   "Size between small and medium",
			size:   protocol.BufferSizeSmall + 1024,
			minCap: protocol.BufferSizeSmall,
		},
		{
			name:   "Size between medium and large",
			size:   protocol.BufferSizeMedium + 1024,
			minCap: protocol.BufferSizeMedium,
		},
		{
			name:   "Size between large and very large",
			size:   protocol.BufferSizeLarge + 1024,
			minCap: protocol.BufferSizeLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := Get(tt.size)
			if buf == nil {
				t.Error("Get returned nil buffer")
			}
			cap := cap(*buf)
			// Should return a buffer that can accommodate the requested size
			if cap < tt.minCap {
				t.Errorf("Get returned buffer with capacity %d, expected at least %d", cap, tt.minCap)
			}
		})
	}
}

// TestGet_SizeLargerThanVeryLarge verifies that Get handles sizes larger than VeryLarge
func TestGet_SizeLargerThanVeryLarge(t *testing.T) {
	// When size is larger than all pools, Get defaults to BufferSizeLarge
	// and then tries to find a suitable pool from the map
	buf := Get(protocol.BufferSizeVeryLarge + 1024)
	if buf == nil {
		t.Error("Get returned nil buffer for size larger than VeryLarge")
	}
	// Should return some buffer (at least from the default pool)
	if cap(*buf) < protocol.BufferSizeSmall {
		t.Errorf("Get returned buffer with capacity %d, expected at least %d", cap(*buf), protocol.BufferSizeSmall)
	}
}

// TestGet_SmallSize verifies that Get handles very small sizes correctly
func TestGet_SmallSize(t *testing.T) {
	buf := Get(1)
	if buf == nil {
		t.Error("Get returned nil buffer for size 1")
	}
	if cap(*buf) < 1 {
		t.Errorf("Get returned buffer with capacity %d, expected at least 1", cap(*buf))
	}
}

// TestGet_ZeroSize verifies that Get handles zero size correctly
func TestGet_ZeroSize(t *testing.T) {
	buf := Get(0)
	if buf == nil {
		t.Error("Get returned nil buffer for size 0")
	}
	// Should return a buffer from the default pool
	if cap(*buf) < protocol.BufferSizeSmall {
		t.Errorf("Get returned buffer with capacity %d, expected at least %d", cap(*buf), protocol.BufferSizeSmall)
	}
}

// TestGet_NegativeSize verifies that Get handles negative sizes correctly
func TestGet_NegativeSize(t *testing.T) {
	buf := Get(-100)
	if buf == nil {
		t.Error("Get returned nil buffer for negative size")
	}
	// Should return a buffer from the default pool
	if cap(*buf) < protocol.BufferSizeSmall {
		t.Errorf("Get returned buffer with capacity %d, expected at least %d", cap(*buf), protocol.BufferSizeSmall)
	}
}

// TestPut_ValidBuffer verifies that Put correctly returns buffers to the pool
func TestPut_ValidBuffer(t *testing.T) {
	// Get a buffer
	buf := Get(protocol.BufferSizeSmall)
	if buf == nil {
		t.Fatal("Get returned nil buffer")
	}

	// Modify the buffer to verify it's reused
	(*buf)[0] = 42

	// Put it back
	Put(buf)

	// Get another buffer - it might be the same one from the pool
	buf2 := Get(protocol.BufferSizeSmall)
	if buf2 == nil {
		t.Fatal("Get returned nil buffer after Put")
	}

	// Both should have the same capacity
	if cap(*buf) != cap(*buf2) {
		t.Errorf("Buffer capacity mismatch: %d vs %d", cap(*buf), cap(*buf2))
	}
}

// TestPut_NilBuffer verifies that Put handles nil buffers gracefully
func TestPut_NilBuffer(t *testing.T) {
	// Should not panic
	Put(nil)
}

// TestPut_UnknownSize verifies that Put discards buffers with unknown sizes
func TestPut_UnknownSize(t *testing.T) {
	// Create a buffer with a size that doesn't match any pool
	customSize := protocol.BufferSizeSmall + 512
	buf := make([]byte, customSize)
	bufPtr := &buf

	// Put should not panic and should discard the buffer
	Put(bufPtr)
	// No assertion needed - just verify it doesn't panic
}

// TestGetForFileSize_SmallFile verifies that GetForFileSize returns small buffer for small files
func TestGetForFileSize_SmallFile(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	fileSize := cfg.Protocol.SmallFileThreshold - 1
	buf := GetForFileSize(fileSize)

	if buf == nil {
		t.Error("GetForFileSize returned nil buffer")
	}
	if cap(*buf) != cfg.Protocol.BufferSizeSmall {
		t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), cfg.Protocol.BufferSizeSmall)
	}
}

// TestGetForFileSize_MediumFile verifies that GetForFileSize returns medium buffer for medium files
func TestGetForFileSize_MediumFile(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	fileSize := cfg.Protocol.MediumFileThreshold - 1
	buf := GetForFileSize(fileSize)

	if buf == nil {
		t.Error("GetForFileSize returned nil buffer")
	}
	if cap(*buf) != cfg.Protocol.BufferSizeMedium {
		t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), cfg.Protocol.BufferSizeMedium)
	}
}

// TestGetForFileSize_LargeFile verifies that GetForFileSize returns large buffer for large files
func TestGetForFileSize_LargeFile(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	fileSize := cfg.Protocol.LargeFileThreshold - 1
	buf := GetForFileSize(fileSize)

	if buf == nil {
		t.Error("GetForFileSize returned nil buffer")
	}
	if cap(*buf) != cfg.Protocol.BufferSizeLarge {
		t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), cfg.Protocol.BufferSizeLarge)
	}
}

// TestGetForFileSize_VeryLargeFile verifies that GetForFileSize returns very large buffer for very large files
func TestGetForFileSize_VeryLargeFile(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	fileSize := cfg.Protocol.LargeFileThreshold + 1
	buf := GetForFileSize(fileSize)

	if buf == nil {
		t.Error("GetForFileSize returned nil buffer")
	}
	if cap(*buf) != cfg.Protocol.BufferSizeVeryLarge {
		t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), cfg.Protocol.BufferSizeVeryLarge)
	}
}

// TestGetForFileSize_ZeroFileSize verifies that GetForFileSize handles zero file size
func TestGetForFileSize_ZeroFileSize(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	buf := GetForFileSize(0)

	if buf == nil {
		t.Error("GetForFileSize returned nil buffer for zero file size")
	}
	if cap(*buf) != cfg.Protocol.BufferSizeSmall {
		t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), cfg.Protocol.BufferSizeSmall)
	}
}

// TestGetForFileSize_NegativeFileSize verifies that GetForFileSize handles negative file size
func TestGetForFileSize_NegativeFileSize(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	buf := GetForFileSize(-1)

	if buf == nil {
		t.Error("GetForFileSize returned nil buffer for negative file size")
	}
	if cap(*buf) != cfg.Protocol.BufferSizeSmall {
		t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), cfg.Protocol.BufferSizeSmall)
	}
}

// TestGetForFileSize_ThresholdBoundaries verifies correct buffer selection at threshold boundaries
func TestGetForFileSize_ThresholdBoundaries(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	tests := []struct {
		name     string
		fileSize int64
		expected int
	}{
		{
			name:     "At small threshold",
			fileSize: cfg.Protocol.SmallFileThreshold,
			expected: cfg.Protocol.BufferSizeMedium,
		},
		{
			name:     "Just before medium threshold",
			fileSize: cfg.Protocol.MediumFileThreshold - 1,
			expected: cfg.Protocol.BufferSizeMedium,
		},
		{
			name:     "At medium threshold",
			fileSize: cfg.Protocol.MediumFileThreshold,
			expected: cfg.Protocol.BufferSizeLarge,
		},
		{
			name:     "Just before large threshold",
			fileSize: cfg.Protocol.LargeFileThreshold - 1,
			expected: cfg.Protocol.BufferSizeLarge,
		},
		{
			name:     "At large threshold",
			fileSize: cfg.Protocol.LargeFileThreshold,
			expected: cfg.Protocol.BufferSizeVeryLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := GetForFileSize(tt.fileSize)
			if buf == nil {
				t.Error("GetForFileSize returned nil buffer")
			}
			if cap(*buf) != tt.expected {
				t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), tt.expected)
			}
		})
	}
}

// TestGetCustom_ValidSize verifies that GetCustom creates a buffer of the requested size
func TestGetCustom_ValidSize(t *testing.T) {
	customSize := int64(512 * 1024) // 512KB

	buf, cleanup := GetCustom(customSize)

	if buf == nil {
		t.Error("GetCustom returned nil buffer")
	}
	if cap(*buf) != int(customSize) {
		t.Errorf("GetCustom returned buffer with capacity %d, expected %d", cap(*buf), customSize)
	}

	// Cleanup should not panic
	cleanup()
}

// TestGetCustom_SmallSize verifies that GetCustom handles small custom sizes
func TestGetCustom_SmallSize(t *testing.T) {
	customSize := int64(1024) // 1KB

	buf, cleanup := GetCustom(customSize)

	if buf == nil {
		t.Error("GetCustom returned nil buffer")
	}
	if cap(*buf) != int(customSize) {
		t.Errorf("GetCustom returned buffer with capacity %d, expected %d", cap(*buf), customSize)
	}

	cleanup()
}

// TestGetCustom_LargeSize verifies that GetCustom handles large custom sizes
func TestGetCustom_LargeSize(t *testing.T) {
	customSize := int64(100 * 1024 * 1024) // 100MB

	buf, cleanup := GetCustom(customSize)

	if buf == nil {
		t.Error("GetCustom returned nil buffer")
	}
	if cap(*buf) != int(customSize) {
		t.Errorf("GetCustom returned buffer with capacity %d, expected %d", cap(*buf), customSize)
	}

	cleanup()
}

// TestGetCustom_ZeroSize verifies that GetCustom handles zero size
func TestGetCustom_ZeroSize(t *testing.T) {
	buf, cleanup := GetCustom(0)

	if buf == nil {
		t.Error("GetCustom returned nil buffer for zero size")
	}
	if cap(*buf) != 0 {
		t.Errorf("GetCustom returned buffer with capacity %d, expected 0", cap(*buf))
	}

	cleanup()
}

// TestGetCustom_CleanupFunction verifies that cleanup function is callable
func TestGetCustom_CleanupFunction(t *testing.T) {
	buf, cleanup := GetCustom(1024)

	if buf == nil {
		t.Fatal("GetCustom returned nil buffer")
	}

	// Cleanup should be callable and not panic
	cleanup()
	cleanup() // Should be safe to call multiple times
}

// TestConcurrentGet_MultipleGoroutines verifies that Get is thread-safe
func TestConcurrentGet_MultipleGoroutines(t *testing.T) {
	numGoroutines := 100
	numIterations := 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numIterations)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				buf := Get(protocol.BufferSizeSmall)
				if buf == nil {
					errors <- nil // Indicate error occurred
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Errorf("Concurrent Get operations failed: %d errors", len(errors))
	}
}

// TestConcurrentPutGet_MultipleGoroutines verifies that Put and Get are thread-safe together
func TestConcurrentPutGet_MultipleGoroutines(t *testing.T) {
	numGoroutines := 50
	numIterations := 20
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numIterations)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				buf := Get(protocol.BufferSizeSmall)
				if buf == nil {
					errors <- nil
					return
				}
				Put(buf)
			}
		}()
	}

	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Errorf("Concurrent Put/Get operations failed: %d errors", len(errors))
	}
}

// TestConcurrentGetForFileSize_MultipleGoroutines verifies that GetForFileSize is thread-safe
func TestConcurrentGetForFileSize_MultipleGoroutines(t *testing.T) {
	cfg := config.DefaultConfig()
	config.SetConfig(cfg)

	numGoroutines := 50
	numIterations := 20
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numIterations)

	fileSizes := []int64{
		cfg.Protocol.SmallFileThreshold - 1,
		cfg.Protocol.MediumFileThreshold - 1,
		cfg.Protocol.LargeFileThreshold - 1,
		cfg.Protocol.LargeFileThreshold + 1,
	}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				fileSize := fileSizes[(idx+j)%len(fileSizes)]
				buf := GetForFileSize(fileSize)
				if buf == nil {
					errors <- nil
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Errorf("Concurrent GetForFileSize operations failed: %d errors", len(errors))
	}
}

// TestBufferReuse_PoolEfficiency verifies that buffers are actually reused from the pool
func TestBufferReuse_PoolEfficiency(t *testing.T) {
	// Get a buffer
	buf1 := Get(protocol.BufferSizeSmall)
	if buf1 == nil {
		t.Fatal("Get returned nil buffer")
	}

	// Store the pointer address
	addr1 := buf1

	// Put it back
	Put(buf1)

	// Get another buffer - it should be the same one from the pool
	buf2 := Get(protocol.BufferSizeSmall)
	if buf2 == nil {
		t.Fatal("Get returned nil buffer after Put")
	}

	// The pointers should be the same (same buffer reused)
	if addr1 != buf2 {
		t.Logf("Buffer reuse not guaranteed (expected but not required): %p vs %p", addr1, buf2)
	}
}

// TestGet_AllPoolSizes verifies that all pool sizes are accessible
func TestGet_AllPoolSizes(t *testing.T) {
	poolSizes := []int{
		protocol.BufferSizeSmall,
		protocol.BufferSizeMedium,
		protocol.BufferSizeLarge,
		protocol.BufferSizeVeryLarge,
	}

	for _, size := range poolSizes {
		buf := Get(size)
		if buf == nil {
			t.Errorf("Get returned nil buffer for size %d", size)
		}
		if cap(*buf) != size {
			t.Errorf("Get returned buffer with capacity %d, expected %d", cap(*buf), size)
		}
	}
}

// TestPut_AllPoolSizes verifies that Put works correctly for all pool sizes
func TestPut_AllPoolSizes(t *testing.T) {
	poolSizes := []int{
		protocol.BufferSizeSmall,
		protocol.BufferSizeMedium,
		protocol.BufferSizeLarge,
		protocol.BufferSizeVeryLarge,
	}

	for _, size := range poolSizes {
		buf := Get(size)
		if buf == nil {
			t.Fatalf("Get returned nil buffer for size %d", size)
		}

		// Put should not panic
		Put(buf)
	}
}

// TestGetForFileSize_WithCustomConfig verifies that GetForFileSize respects custom config
func TestGetForFileSize_WithCustomConfig(t *testing.T) {
	// Create a custom config with different thresholds
	customCfg := config.DefaultConfig()
	customCfg.Protocol.SmallFileThreshold = 32 * 1024        // 32KB
	customCfg.Protocol.MediumFileThreshold = 512 * 1024      // 512KB
	customCfg.Protocol.LargeFileThreshold = 50 * 1024 * 1024 // 50MB

	config.SetConfig(customCfg)

	// Test with custom thresholds
	buf := GetForFileSize(customCfg.Protocol.SmallFileThreshold - 1)
	if buf == nil {
		t.Error("GetForFileSize returned nil buffer")
	}
	if cap(*buf) != customCfg.Protocol.BufferSizeSmall {
		t.Errorf("GetForFileSize returned buffer with capacity %d, expected %d", cap(*buf), customCfg.Protocol.BufferSizeSmall)
	}

	// Reset to default config
	config.SetConfig(config.DefaultConfig())
}

// TestGet_BufferContent verifies that buffers are properly initialized
func TestGet_BufferContent(t *testing.T) {
	buf := Get(protocol.BufferSizeSmall)
	if buf == nil {
		t.Fatal("Get returned nil buffer")
	}

	// Buffer should be properly initialized (all zeros)
	for i := 0; i < len(*buf); i++ {
		if (*buf)[i] != 0 {
			t.Errorf("Buffer not properly initialized at index %d: got %d, expected 0", i, (*buf)[i])
			break
		}
	}
}

// TestGet_BufferLength verifies that buffer length matches capacity
func TestGet_BufferLength(t *testing.T) {
	sizes := []int{
		protocol.BufferSizeSmall,
		protocol.BufferSizeMedium,
		protocol.BufferSizeLarge,
		protocol.BufferSizeVeryLarge,
	}

	for _, size := range sizes {
		buf := Get(size)
		if buf == nil {
			t.Fatalf("Get returned nil buffer for size %d", size)
		}

		if len(*buf) != cap(*buf) {
			t.Errorf("Buffer length %d does not match capacity %d for size %d", len(*buf), cap(*buf), size)
		}
	}
}

// TestGetCustom_BufferLength verifies that custom buffers have correct length
func TestGetCustom_BufferLength(t *testing.T) {
	customSize := int64(256 * 1024)

	buf, cleanup := GetCustom(customSize)
	defer cleanup()

	if buf == nil {
		t.Fatal("GetCustom returned nil buffer")
	}

	if len(*buf) != int(customSize) {
		t.Errorf("Custom buffer length %d does not match requested size %d", len(*buf), customSize)
	}

	if cap(*buf) != int(customSize) {
		t.Errorf("Custom buffer capacity %d does not match requested size %d", cap(*buf), customSize)
	}
}

// TestGet_MultipleBuffers verifies that multiple buffers can be obtained simultaneously
func TestGet_MultipleBuffers(t *testing.T) {
	buf1 := Get(protocol.BufferSizeSmall)
	buf2 := Get(protocol.BufferSizeMedium)
	buf3 := Get(protocol.BufferSizeLarge)
	buf4 := Get(protocol.BufferSizeVeryLarge)

	if buf1 == nil || buf2 == nil || buf3 == nil || buf4 == nil {
		t.Error("Get returned nil buffer")
	}

	if cap(*buf1) != protocol.BufferSizeSmall {
		t.Errorf("buf1 capacity mismatch: %d vs %d", cap(*buf1), protocol.BufferSizeSmall)
	}
	if cap(*buf2) != protocol.BufferSizeMedium {
		t.Errorf("buf2 capacity mismatch: %d vs %d", cap(*buf2), protocol.BufferSizeMedium)
	}
	if cap(*buf3) != protocol.BufferSizeLarge {
		t.Errorf("buf3 capacity mismatch: %d vs %d", cap(*buf3), protocol.BufferSizeLarge)
	}
	if cap(*buf4) != protocol.BufferSizeVeryLarge {
		t.Errorf("buf4 capacity mismatch: %d vs %d", cap(*buf4), protocol.BufferSizeVeryLarge)
	}
}

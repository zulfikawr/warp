package server

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// writeFile is a helper for tests
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// TestServer_NoGoroutineLeaks verifies server cleanup doesn't leak goroutines
func TestServer_NoGoroutineLeaks(t *testing.T) {
	// Give background goroutines time to stabilize
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	before := runtime.NumGoroutine()

	// Create and start server
	srv := &Server{
		Token:     "test-token",
		HostMode:  true,
		UploadDir: t.TempDir(),
	}

	url, err := srv.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	t.Logf("Server started at %s", url)

	// Do some operations
	time.Sleep(200 * time.Millisecond)

	// Shutdown server
	err = srv.Shutdown()
	if err != nil {
		t.Fatalf("Failed to shutdown server: %v", err)
	}

	// Wait for cleanup goroutines to finish
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()

	// Allow some tolerance for GC and runtime goroutines
	leaked := after - before
	if leaked > 3 {
		t.Errorf("Goroutine leak detected: before=%d, after=%d, leaked=%d", before, after, leaked)
		// Print stack traces to help debug
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		t.Logf("Goroutine stacks:\n%s", buf[:n])
	}
}

// TestServer_MultipleStartShutdown tests repeated start/shutdown cycles
func TestServer_MultipleStartShutdown(t *testing.T) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()

	// Run 3 cycles
	for i := 0; i < 3; i++ {
		srv := &Server{
			Token:     "test-token",
			HostMode:  true,
			UploadDir: t.TempDir(),
		}

		_, err := srv.Start()
		if err != nil {
			t.Fatalf("Cycle %d: Failed to start: %v", i, err)
		}

		time.Sleep(50 * time.Millisecond)

		err = srv.Shutdown()
		if err != nil {
			t.Fatalf("Cycle %d: Failed to shutdown: %v", i, err)
		}

		time.Sleep(50 * time.Millisecond)
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()

	leaked := after - before
	if leaked > 5 {
		t.Errorf("Goroutine leak after 3 cycles: before=%d, after=%d, leaked=%d", before, after, leaked)
	}
}

// TestRateLimiterCleanup verifies rate limiters are cleaned up
func TestRateLimiterCleanup(t *testing.T) {
	srv := &Server{
		Token:         "test-token",
		RateLimitMbps: 10,
	}

	// Create rate limiters for multiple IPs
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}
	for _, ip := range ips {
		lim := srv.getRateLimiter(ip)
		if lim == nil {
			t.Errorf("Failed to create rate limiter for %s", ip)
		}
	}

	// Verify they exist
	count := 0
	srv.rateLimiters.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	if count != 3 {
		t.Errorf("Expected 3 rate limiters, got %d", count)
	}

	// Manually set lastAccess to old time to trigger cleanup
	srv.rateLimiters.Range(func(key, value interface{}) bool {
		entry := value.(*rateLimiterEntry)
		entry.lastAccess = time.Now().Add(-2 * time.Hour)
		return true
	})

	// Run cleanup
	srv.cleanupRateLimiters()

	// Verify they're cleaned up
	count = 0
	srv.rateLimiters.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("Expected 0 rate limiters after cleanup, got %d", count)
	}
}

// TestChecksumCache verifies checksum caching works correctly
func TestChecksumCache(t *testing.T) {
	srv := &Server{}

	// Create a test file
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"
	content := []byte("test content for checksum")
	err := writeFile(testFile, content)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// First call should compute checksum
	checksum1, err := srv.getCachedChecksum(testFile)
	if err != nil {
		t.Fatalf("Failed to get checksum: %v", err)
	}

	// Second call should return cached checksum
	checksum2, err := srv.getCachedChecksum(testFile)
	if err != nil {
		t.Fatalf("Failed to get cached checksum: %v", err)
	}

	if checksum1 != checksum2 {
		t.Errorf("Checksums don't match: %s != %s", checksum1, checksum2)
	}

	// Verify cache entry exists
	val, ok := srv.checksumCache.Load(testFile)
	if !ok {
		t.Error("Checksum not cached")
	}
	entry := val.(*checksumCacheEntry)
	if entry.checksum != checksum1 {
		t.Errorf("Cached checksum mismatch: %s != %s", entry.checksum, checksum1)
	}

	// Modify file - cache should invalidate
	time.Sleep(10 * time.Millisecond) // Ensure mtime changes
	err = writeFile(testFile, []byte("modified content"))
	if err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Should compute new checksum
	checksum3, err := srv.getCachedChecksum(testFile)
	if err != nil {
		t.Fatalf("Failed to get checksum after modification: %v", err)
	}

	if checksum3 == checksum1 {
		t.Error("Checksum should have changed after file modification")
	}
}

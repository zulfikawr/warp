package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiterEntry tracks rate limiter with last access time
type rateLimiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// RateLimitedWriter wraps an io.Writer with rate limiting
type RateLimitedWriter struct {
	w       io.Writer
	limiter *rate.Limiter
}

func (rl *RateLimitedWriter) Write(p []byte) (int, error) {
	// Wait for rate limiter to allow this write
	if rl.limiter != nil {
		if err := rl.limiter.WaitN(context.Background(), len(p)); err != nil {
			return 0, err
		}
	}
	return rl.w.Write(p)
}

// getRateLimiter gets or creates a rate limiter for a client IP
func (s *Server) getRateLimiter(clientIP string) *rate.Limiter {
	if s.RateLimitMbps <= 0 {
		return nil // No rate limiting
	}

	if val, ok := s.rateLimiters.Load(clientIP); ok {
		entry := val.(*rateLimiterEntry)
		// Update last access time
		entry.lastAccess = time.Now()
		return entry.limiter
	}

	// Convert Mbps to bytes per second
	bytesPerSecond := (s.RateLimitMbps * 1_000_000) / 8
	burst := max(
		// 100ms burst
		int(bytesPerSecond/10),
		// Minimum 4KB burst
		4096,
	)

	lim := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)

	entry := &rateLimiterEntry{
		limiter:    lim,
		lastAccess: time.Now(),
	}

	s.rateLimiters.Store(clientIP, entry)
	return lim
}

// cleanupRateLimiters removes stale rate limiters to prevent memory leak
func (s *Server) cleanupRateLimiters() {
	staleThreshold := time.Now().Add(-1 * time.Hour)

	s.rateLimiters.Range(func(key, value interface{}) bool {
		entry := value.(*rateLimiterEntry)
		if entry.lastAccess.Before(staleThreshold) {
			s.rateLimiters.Delete(key)
		}
		return true
	})
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (if behind proxy)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP in the list
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Use RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ProgressTracker tracks upload/download progress for real-time WebSocket updates
type ProgressTracker struct {
	Filename     string
	TotalSize    int64
	BytesWritten int64 // atomic
	StartTime    time.Time
	LastUpdate   time.Time
}

// GetProgress returns current progress stats atomically
func (pt *ProgressTracker) GetProgress() map[string]interface{} {
	bytesWritten := atomic.LoadInt64(&pt.BytesWritten)
	elapsed := time.Since(pt.StartTime).Seconds()

	var mbps float64
	if elapsed > 0 {
		mbps = (float64(bytesWritten) * 8) / (elapsed * 1_000_000)
	}

	percentage := float64(0)
	if pt.TotalSize > 0 {
		percentage = (float64(bytesWritten) / float64(pt.TotalSize)) * 100
	}

	return map[string]interface{}{
		"filename":        pt.Filename,
		"total_size":      pt.TotalSize,
		"bytes_written":   bytesWritten,
		"percentage":      percentage,
		"throughput_mbps": mbps,
		"elapsed_seconds": elapsed,
	}
}

// UpdateProgress atomically updates bytes written
func (pt *ProgressTracker) UpdateProgress(bytes int64) {
	atomic.AddInt64(&pt.BytesWritten, bytes)
	pt.LastUpdate = time.Now()
}

// tcpKeepAliveListener sets TCP keepalive and optimizes socket for high throughput
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, fmt.Errorf("failed to accept TCP connection: %w", err)
	}
	// Enable TCP keepalive to detect dead connections
	_ = tc.SetKeepAlive(true)
	_ = tc.SetKeepAlivePeriod(3 * time.Minute)

	// Critical: Disable Nagle's algorithm for immediate packet transmission
	// This eliminates 40-200ms delays waiting for packet coalescing
	_ = tc.SetNoDelay(true)

	// Let OS auto-tune TCP window size for optimal throughput
	// Manual buffer sizing can prevent dynamic scaling

	return tc, nil
}

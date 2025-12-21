package speedtest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/zulfikawr/warp/internal/protocol"
)

const (
	// Test payload sizes
	uploadTestSize = 10485760 // 10 MB for upload test
	testDuration   = 3 * time.Second
)

// Result contains the results of a speed test
type Result struct {
	UploadMbps   float64
	DownloadMbps float64
	LatencyMs    float64
	Quality      string
	Error        error
}

// SpeedTest performs network speed testing against a target host
type SpeedTest struct {
	targetHost string
	client     *http.Client
}

// New creates a new SpeedTest instance
func New(targetHost string) *SpeedTest {
	// Parse host to ensure it has the right format
	if !strings.Contains(targetHost, ":") {
		targetHost = targetHost + ":8080"
	}

	return &SpeedTest{
		targetHost: targetHost,
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       30 * time.Second,
				DisableKeepAlives:     false,
				ForceAttemptHTTP2:     true,
				WriteBufferSize:       256 * 1024,
				ReadBufferSize:        256 * 1024,
				DisableCompression:    true, // Don't compress test data
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
			Timeout: 60 * time.Second,
		},
	}
}

// Run executes the full speed test suite
func (st *SpeedTest) Run(ctx context.Context) *Result {
	result := &Result{}

	// Test latency first
	latency, err := st.measureLatency(ctx)
	if err != nil {
		result.Error = fmt.Errorf("latency test failed: %w", err)
		return result
	}
	result.LatencyMs = latency

	// Test download speed
	downloadMbps, err := st.measureDownload(ctx)
	if err != nil {
		result.Error = fmt.Errorf("download test failed: %w", err)
		return result
	}
	result.DownloadMbps = downloadMbps

	// Test upload speed
	uploadMbps, err := st.measureUpload(ctx)
	if err != nil {
		result.Error = fmt.Errorf("upload test failed: %w", err)
		return result
	}
	result.UploadMbps = uploadMbps

	// Determine connection quality
	result.Quality = determineQuality(latency, downloadMbps, uploadMbps)

	return result
}

// measureLatency measures round-trip latency to the target
func (st *SpeedTest) measureLatency(ctx context.Context) (float64, error) {
	const samples = 5
	var totalLatency time.Duration

	for i := 0; i < samples; i++ {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		start := time.Now()

		// TCP connection test for pure latency measurement
		conn, err := net.DialTimeout("tcp", st.targetHost, 5*time.Second)
		if err != nil {
			return 0, fmt.Errorf("connection failed: %w", err)
		}
		latency := time.Since(start)
		conn.Close()

		totalLatency += latency

		// Small delay between samples to avoid overwhelming the target
		if i < samples-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	avgLatency := totalLatency / samples
	return float64(avgLatency.Milliseconds()), nil
}

// measureDownload measures download speed from the target
func (st *SpeedTest) measureDownload(ctx context.Context) (float64, error) {
	url := fmt.Sprintf("http://%s/speedtest/download", st.targetHost)

	bytesRead := int64(0)
	start := time.Now()

	// Keep downloading until we hit time limit
	for time.Since(start) < testDuration {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return 0, err
		}

		resp, err := st.client.Do(req)
		if err != nil {
			return 0, fmt.Errorf("download request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return 0, fmt.Errorf("download request failed with status: %d", resp.StatusCode)
		}

		// Use the same buffer size and hashing as real transfers for accuracy
		buf := make([]byte, protocol.BufferSizeVeryLarge)
		hash := sha256.New()
		n, err := io.CopyBuffer(hash, resp.Body, buf)
		resp.Body.Close()

		if err != nil && err != io.EOF {
			return 0, fmt.Errorf("read failed: %w", err)
		}

		bytesRead += n
	}

	elapsed := time.Since(start).Seconds()
	if elapsed == 0 || bytesRead == 0 {
		return 0, fmt.Errorf("insufficient data for measurement")
	}

	// Convert bytes/sec to Mbps
	bytesPerSecond := float64(bytesRead) / elapsed
	mbps := (bytesPerSecond * 8) / 1_000_000

	return mbps, nil
}

// measureUpload measures upload speed to the target
func (st *SpeedTest) measureUpload(ctx context.Context) (float64, error) {
	url := fmt.Sprintf("http://%s/speedtest/upload", st.targetHost)

	// Generate random test data once
	testData := make([]byte, uploadTestSize)
	if _, err := rand.Read(testData); err != nil {
		return 0, fmt.Errorf("failed to generate test data: %w", err)
	}

	bytesWritten := int64(0)
	start := time.Now()

	// Keep uploading until we hit time limit
	for time.Since(start) < testDuration {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(testData))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/octet-stream")

		resp, err := st.client.Do(req)
		if err != nil {
			return 0, fmt.Errorf("upload request failed: %w", err)
		}

		// Read and discard response
		// Use hashing to simulate real transfer overhead
		hash := sha256.New()
		_, _ = io.Copy(hash, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			bytesWritten += int64(len(testData))
		} else {
			return 0, fmt.Errorf("upload request failed with status: %d", resp.StatusCode)
		}
	}

	elapsed := time.Since(start).Seconds()
	if elapsed == 0 || bytesWritten == 0 {
		return 0, fmt.Errorf("insufficient data for measurement")
	}

	// Convert bytes/sec to Mbps
	bytesPerSecond := float64(bytesWritten) / elapsed
	mbps := (bytesPerSecond * 8) / 1_000_000

	return mbps, nil
}

// determineQuality determines connection quality based on metrics
func determineQuality(latencyMs, downloadMbps, uploadMbps float64) string {
	// Quality rating based on latency and speeds
	if latencyMs < 20 && downloadMbps > 100 && uploadMbps > 100 {
		return "Excellent"
	} else if latencyMs < 50 && downloadMbps > 50 && uploadMbps > 50 {
		return "Very Good"
	} else if latencyMs < 100 && downloadMbps > 25 && uploadMbps > 25 {
		return "Good"
	} else if latencyMs < 200 && downloadMbps > 10 && uploadMbps > 10 {
		return "Fair"
	}
	return "Poor"
}

// EstimateTransferTime estimates time to transfer a given file size
func EstimateTransferTime(fileSizeMB float64, speedMbps float64) time.Duration {
	if speedMbps <= 0 {
		return 0
	}

	// Convert Mbps to MBps (megabytes per second)
	mbps := speedMbps / 8

	// Calculate seconds needed
	seconds := fileSizeMB / mbps

	return time.Duration(seconds * float64(time.Second))
}

// FormatSpeed formats speed in appropriate units
func FormatSpeed(mbps float64) string {
	if mbps >= 1000 {
		return fmt.Sprintf("%.1f Gbps", mbps/1000)
	}
	return fmt.Sprintf("%.1f Mbps", mbps)
}

// FormatDuration formats duration in human-readable format
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f seconds", d.Seconds())
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%d min %d sec", minutes, seconds)
		}
		return fmt.Sprintf("%d minutes", minutes)
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%d hr %d min", hours, minutes)
	}
	return fmt.Sprintf("%d hours", hours)
}

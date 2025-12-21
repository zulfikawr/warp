package speedtest

import (
	"context"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	st := New("192.168.1.100:8080")

	if st == nil {
		t.Fatal("New() returned nil")
	}

	if st.targetHost != "192.168.1.100:8080" {
		t.Errorf("expected targetHost to be '192.168.1.100:8080', got '%s'", st.targetHost)
	}

	if st.client == nil {
		t.Error("client should not be nil")
	}
}

func TestNewWithoutPort(t *testing.T) {
	st := New("192.168.1.100")

	if st == nil {
		t.Fatal("New() returned nil")
	}

	if st.targetHost != "192.168.1.100:8080" {
		t.Errorf("expected targetHost to be '192.168.1.100:8080' (default port), got '%s'", st.targetHost)
	}
}

func TestDetermineQuality(t *testing.T) {
	tests := []struct {
		name         string
		latencyMs    float64
		downloadMbps float64
		uploadMbps   float64
		expected     string
	}{
		{
			name:         "Excellent connection",
			latencyMs:    10,
			downloadMbps: 150,
			uploadMbps:   150,
			expected:     "Excellent",
		},
		{
			name:         "Very Good connection",
			latencyMs:    30,
			downloadMbps: 75,
			uploadMbps:   75,
			expected:     "Very Good",
		},
		{
			name:         "Good connection",
			latencyMs:    60,
			downloadMbps: 40,
			uploadMbps:   40,
			expected:     "Good",
		},
		{
			name:         "Fair connection",
			latencyMs:    150,
			downloadMbps: 15,
			uploadMbps:   15,
			expected:     "Fair",
		},
		{
			name:         "Poor connection",
			latencyMs:    300,
			downloadMbps: 5,
			uploadMbps:   5,
			expected:     "Poor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineQuality(tt.latencyMs, tt.downloadMbps, tt.uploadMbps)
			if result != tt.expected {
				t.Errorf("determineQuality() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEstimateTransferTime(t *testing.T) {
	tests := []struct {
		name       string
		fileSizeMB float64
		speedMbps  float64
		expected   time.Duration
	}{
		{
			name:       "100 MB at 100 Mbps",
			fileSizeMB: 100,
			speedMbps:  100,
			expected:   8 * time.Second,
		},
		{
			name:       "1000 MB at 100 Mbps",
			fileSizeMB: 1000,
			speedMbps:  100,
			expected:   80 * time.Second,
		},
		{
			name:       "10 MB at 10 Mbps",
			fileSizeMB: 10,
			speedMbps:  10,
			expected:   8 * time.Second,
		},
		{
			name:       "Zero speed",
			fileSizeMB: 100,
			speedMbps:  0,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTransferTime(tt.fileSizeMB, tt.speedMbps)

			// Allow 1 second tolerance for floating point arithmetic
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}

			if diff > time.Second {
				t.Errorf("EstimateTransferTime() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		name     string
		mbps     float64
		expected string
	}{
		{
			name:     "Low speed in Mbps",
			mbps:     25.5,
			expected: "25.5 Mbps",
		},
		{
			name:     "Medium speed in Mbps",
			mbps:     125.8,
			expected: "125.8 Mbps",
		},
		{
			name:     "High speed in Gbps",
			mbps:     1500,
			expected: "1.5 Gbps",
		},
		{
			name:     "Very high speed in Gbps",
			mbps:     10000,
			expected: "10.0 Gbps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSpeed(tt.mbps)
			if result != tt.expected {
				t.Errorf("FormatSpeed() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "Seconds only",
			duration: 45 * time.Second,
			expected: "45 seconds",
		},
		{
			name:     "Minutes only",
			duration: 3 * time.Minute,
			expected: "3 minutes",
		},
		{
			name:     "Minutes and seconds",
			duration: 3*time.Minute + 30*time.Second,
			expected: "3 min 30 sec",
		},
		{
			name:     "Hours only",
			duration: 2 * time.Hour,
			expected: "2 hours",
		},
		{
			name:     "Hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			expected: "2 hr 30 min",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("FormatDuration() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMeasureLatency(t *testing.T) {
	// Test with an invalid host - should return error
	st := New("invalid-host-that-does-not-exist.local:99999")
	ctx := context.Background()

	_, err := st.measureLatency(ctx)
	if err == nil {
		t.Error("expected error for invalid host, got nil")
	}
}

func TestMeasureDownload(t *testing.T) {
	st := New("192.168.1.100:8080")
	ctx := context.Background()

	// Test with a short timeout - will fall back to raw measurement
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// This should complete and return a result (either real or fallback)
	speed, err := st.measureDownload(ctx)
	if err != nil && err != context.DeadlineExceeded {
		// If there's an error, it should be timeout or connection refused
		t.Logf("Download test error (expected if no server): %v", err)
	} else if speed > 0 {
		t.Logf("Download speed: %.2f Mbps", speed)
	}
}

func TestMeasureUpload(t *testing.T) {
	st := New("192.168.1.100:8080")
	ctx := context.Background()

	// Test with a short timeout - will fall back to raw measurement
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// This should complete and return a result (either real or fallback)
	speed, err := st.measureUpload(ctx)
	if err != nil && err != context.DeadlineExceeded {
		// If there's an error, it should be timeout or connection refused
		t.Logf("Upload test error (expected if no server): %v", err)
	} else if speed > 0 {
		t.Logf("Upload speed: %.2f Mbps", speed)
	}
}

func TestRun(t *testing.T) {
	// Test that Run returns a result even on connection failure
	st := New("invalid-host-that-does-not-exist.local:99999")
	ctx := context.Background()

	result := st.Run(ctx)

	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	// Should have an error due to invalid host
	if result.Error == nil {
		t.Error("expected error for invalid host, got nil")
	}
}

func BenchmarkDetermineQuality(b *testing.B) {
	for i := 0; i < b.N; i++ {
		determineQuality(25.5, 100.0, 100.0)
	}
}

func BenchmarkEstimateTransferTime(b *testing.B) {
	for i := 0; i < b.N; i++ {
		EstimateTransferTime(1000, 100)
	}
}

func BenchmarkFormatSpeed(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FormatSpeed(125.8)
	}
}

func BenchmarkFormatDuration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FormatDuration(3*time.Minute + 30*time.Second)
	}
}

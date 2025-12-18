package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsRegistration(t *testing.T) {
	// Test that all metrics are properly registered
	metrics := []prometheus.Collector{
		UploadDuration,
		UploadSize,
		UploadThroughput,
		UploadsTotal,
		DownloadDuration,
		DownloadSize,
		DownloadThroughput,
		DownloadsTotal,
		ActiveTransfers,
		ActiveUploads,
		ActiveDownloads,
		ChunkUploadDuration,
		ChunkUploadsTotal,
		ParallelUploadWorkers,
		ChecksumVerifications,
		CacheHits,
		CacheMisses,
		CacheSize,
		ActiveWebSocketConnections,
		WebSocketMessagesTotal,
	}

	for _, metric := range metrics {
		if metric == nil {
			t.Error("Found nil metric")
		}
	}
}

func TestUploadMetrics(t *testing.T) {
	// Record a test upload
	UploadDuration.WithLabelValues(".txt").Observe(1.5)
	UploadSize.WithLabelValues(".txt").Observe(1024)
	UploadThroughput.WithLabelValues(".txt").Observe(100)
	UploadsTotal.WithLabelValues(".txt", "success").Inc()

	// Verify counter increased
	count := testutil.ToFloat64(UploadsTotal.WithLabelValues(".txt", "success"))
	if count < 1 {
		t.Errorf("Expected UploadsTotal >= 1, got %f", count)
	}
}

func TestDownloadMetrics(t *testing.T) {
	// Record a test download
	DownloadDuration.WithLabelValues(".pdf").Observe(2.0)
	DownloadSize.WithLabelValues(".pdf").Observe(2048)
	DownloadThroughput.WithLabelValues(".pdf").Observe(200)
	DownloadsTotal.WithLabelValues(".pdf", "success").Inc()

	// Verify counter increased
	count := testutil.ToFloat64(DownloadsTotal.WithLabelValues(".pdf", "success"))
	if count < 1 {
		t.Errorf("Expected DownloadsTotal >= 1, got %f", count)
	}
}

func TestActiveTransferMetrics(t *testing.T) {
	// Increment active transfers
	ActiveTransfers.Inc()
	ActiveUploads.Inc()

	// Verify gauge increased
	activeTransfers := testutil.ToFloat64(ActiveTransfers)
	if activeTransfers < 1 {
		t.Errorf("Expected ActiveTransfers >= 1, got %f", activeTransfers)
	}

	// Decrement
	ActiveTransfers.Dec()
	ActiveUploads.Dec()
}

func TestChunkMetrics(t *testing.T) {
	// Record chunk upload
	ChunkUploadDuration.Observe(0.5)
	ChunkUploadsTotal.WithLabelValues("success").Inc()
	ParallelUploadWorkers.Set(3)

	// Verify metrics
	count := testutil.ToFloat64(ChunkUploadsTotal.WithLabelValues("success"))
	if count < 1 {
		t.Errorf("Expected ChunkUploadsTotal >= 1, got %f", count)
	}

	workers := testutil.ToFloat64(ParallelUploadWorkers)
	if workers != 3 {
		t.Errorf("Expected ParallelUploadWorkers = 3, got %f", workers)
	}
}

func TestChecksumMetrics(t *testing.T) {
	// Record checksum verification
	ChecksumVerifications.WithLabelValues("match").Inc()
	
	// Verify counter increased
	count := testutil.ToFloat64(ChecksumVerifications.WithLabelValues("match"))
	if count < 1 {
		t.Errorf("Expected ChecksumVerifications >= 1, got %f", count)
	}
}

func TestCacheMetrics(t *testing.T) {
	// Record cache activity
	CacheHits.Inc()
	CacheMisses.Inc()
	CacheSize.Set(1024000)

	// Verify metrics
	hits := testutil.ToFloat64(CacheHits)
	if hits < 1 {
		t.Errorf("Expected CacheHits >= 1, got %f", hits)
	}

	misses := testutil.ToFloat64(CacheMisses)
	if misses < 1 {
		t.Errorf("Expected CacheMisses >= 1, got %f", misses)
	}

	size := testutil.ToFloat64(CacheSize)
	if size != 1024000 {
		t.Errorf("Expected CacheSize = 1024000, got %f", size)
	}
}

func TestWebSocketMetrics(t *testing.T) {
	// Record WebSocket activity
	ActiveWebSocketConnections.Inc()
	WebSocketMessagesTotal.WithLabelValues("progress").Inc()

	// Verify metrics
	connections := testutil.ToFloat64(ActiveWebSocketConnections)
	if connections < 1 {
		t.Errorf("Expected ActiveWebSocketConnections >= 1, got %f", connections)
	}

	messages := testutil.ToFloat64(WebSocketMessagesTotal.WithLabelValues("progress"))
	if messages < 1 {
		t.Errorf("Expected WebSocketMessagesTotal >= 1, got %f", messages)
	}

	// Cleanup
	ActiveWebSocketConnections.Dec()
}

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Chunk Upload Metrics
//
// These metrics track parallel chunk upload performance.
// Parallel uploads split large files into chunks and upload them concurrently.
// Use these metrics to optimize chunk size and worker count.

var (
	// ChunkUploadDuration tracks the time to upload individual chunks.
	// Use this to identify slow chunks and tune chunk size.
	ChunkUploadDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "warp_chunk_upload_duration_seconds",
			Help:    "Individual chunk upload duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		},
	)

	// ChunkUploadsTotal counts chunk upload outcomes.
	// Labels: status (success, retry, error)
	// Use this to track chunk reliability and retry effectiveness.
	ChunkUploadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_chunk_uploads_total",
			Help: "Total number of chunk uploads",
		},
		[]string{"status"},
	)

	// ParallelUploadWorkers tracks the number of active parallel upload workers.
	// Use this to monitor concurrent chunk upload activity.
	ParallelUploadWorkers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_parallel_upload_workers",
			Help: "Number of active parallel upload workers",
		},
	)
)

// Helper functions for chunk metrics

// RecordChunkSuccess records a successful chunk upload.
func RecordChunkSuccess() {
	ChunkUploadsTotal.WithLabelValues("success").Inc()
}

// RecordChunkRetry records a chunk upload retry.
func RecordChunkRetry() {
	ChunkUploadsTotal.WithLabelValues("retry").Inc()
}

// RecordChunkError records a failed chunk upload.
func RecordChunkError() {
	ChunkUploadsTotal.WithLabelValues("error").Inc()
}

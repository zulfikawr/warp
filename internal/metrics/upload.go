package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Upload Metrics
//
// These metrics track file and directory uploads to the server.
// Use these metrics to monitor upload performance, success rates,
// and identify bottlenecks in the upload pipeline.

var (
	// UploadDuration tracks the time taken to complete file uploads.
	// Labels: file_ext (e.g., "txt", "pdf", "zip")
	// Use this to identify slow uploads by file type.
	UploadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_upload_duration_seconds",
			Help:    "Upload duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
		},
		[]string{"file_ext"},
	)

	// UploadSize tracks the size of uploaded files in bytes.
	// Labels: file_ext
	// Use this to understand upload size distribution.
	UploadSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_upload_size_bytes",
			Help:    "Upload size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 20), // 1KB to ~1GB
		},
		[]string{"file_ext"},
	)

	// UploadThroughput tracks upload speed in Mbps.
	// Labels: file_ext
	// Use this to monitor network performance and identify bandwidth issues.
	UploadThroughput = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_upload_throughput_mbps",
			Help:    "Upload throughput in Mbps",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1 Mbps to ~16Gbps
		},
		[]string{"file_ext"},
	)

	// UploadsTotal counts successful and failed uploads.
	// Labels: file_ext, status (success, error)
	// Use this to track upload success rate and identify problematic file types.
	UploadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_uploads_total",
			Help: "Total number of uploads",
		},
		[]string{"file_ext", "status"},
	)

	// ActiveUploads tracks the number of uploads currently in progress.
	// Use this to monitor concurrent upload load on the server.
	ActiveUploads = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_active_uploads",
			Help: "Number of active uploads",
		},
	)
)

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Download Metrics
//
// These metrics track file and directory downloads from the server.
// Use these metrics to monitor download performance, success rates,
// and identify network bottlenecks.

var (
	// DownloadDuration tracks the time taken to complete file downloads.
	// Labels: file_ext (e.g., "txt", "pdf", "zip")
	// Use this to identify slow downloads by file type.
	DownloadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_download_duration_seconds",
			Help:    "Download duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
		},
		[]string{"file_ext"},
	)

	// DownloadSize tracks the size of downloaded files in bytes.
	// Labels: file_ext
	// Use this to understand download size distribution.
	DownloadSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_download_size_bytes",
			Help:    "Download size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 20), // 1KB to ~1GB
		},
		[]string{"file_ext"},
	)

	// DownloadThroughput tracks download speed in Mbps.
	// Labels: file_ext
	// Use this to monitor network performance and identify bandwidth issues.
	DownloadThroughput = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_download_throughput_mbps",
			Help:    "Download throughput in Mbps",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1 Mbps to ~16Gbps
		},
		[]string{"file_ext"},
	)

	// DownloadsTotal counts successful and failed downloads.
	// Labels: file_ext, status (success, error)
	// Use this to track download success rate and identify problematic file types.
	DownloadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_downloads_total",
			Help: "Total number of downloads",
		},
		[]string{"file_ext", "status"},
	)

	// ActiveDownloads tracks the number of downloads currently in progress.
	// Use this to monitor concurrent download load on the server.
	ActiveDownloads = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_active_downloads",
			Help: "Number of active downloads",
		},
	)

	// ActiveTransfers tracks total active uploads and downloads combined.
	// Use this to monitor overall server load.
	ActiveTransfers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_active_transfers",
			Help: "Number of active transfers (uploads + downloads)",
		},
	)
)

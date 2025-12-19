package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var ( // Error classification
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_errors_total",
			Help: "Total errors by type and operation",
		},
		[]string{"type", "operation"},
	)

	// Retry tracking
	RetryAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_retry_attempts_total",
			Help: "Total retry attempts by operation and reason",
		},
		[]string{"operation", "reason"},
	)

	// Session lifecycle
	SessionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_session_duration_seconds",
			Help:    "Total session duration from start to completion",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~1 hour
		},
		[]string{"type"}, // upload or download
	)
	// Upload metrics
	UploadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_upload_duration_seconds",
			Help:    "Upload duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
		},
		[]string{"file_ext"},
	)

	UploadSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_upload_size_bytes",
			Help:    "Upload size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 20), // 1KB to ~1GB
		},
		[]string{"file_ext"},
	)

	UploadThroughput = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_upload_throughput_mbps",
			Help:    "Upload throughput in Mbps",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1 Mbps to ~16Gbps
		},
		[]string{"file_ext"},
	)

	UploadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_uploads_total",
			Help: "Total number of uploads",
		},
		[]string{"file_ext", "status"}, // status: success, error
	)

	// Download metrics
	DownloadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_download_duration_seconds",
			Help:    "Download duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		[]string{"file_ext"},
	)

	DownloadSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_download_size_bytes",
			Help:    "Download size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 20),
		},
		[]string{"file_ext"},
	)

	DownloadThroughput = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_download_throughput_mbps",
			Help:    "Download throughput in Mbps",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15),
		},
		[]string{"file_ext"},
	)

	DownloadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_downloads_total",
			Help: "Total number of downloads",
		},
		[]string{"file_ext", "status"},
	)

	// Active transfer tracking
	ActiveTransfers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_active_transfers",
			Help: "Number of active transfers (uploads + downloads)",
		},
	)

	ActiveUploads = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_active_uploads",
			Help: "Number of active uploads",
		},
	)

	ActiveDownloads = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_active_downloads",
			Help: "Number of active downloads",
		},
	)

	// Parallel chunk upload metrics
	ChunkUploadDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "warp_chunk_upload_duration_seconds",
			Help:    "Individual chunk upload duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
		},
	)

	ChunkUploadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_chunk_uploads_total",
			Help: "Total number of chunk uploads",
		},
		[]string{"status"}, // success, retry, error
	)

	ParallelUploadWorkers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_parallel_upload_workers",
			Help: "Number of active parallel upload workers",
		},
	)

	// Checksum verification metrics
	ChecksumVerifications = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_checksum_verifications_total",
			Help: "Total number of checksum verifications",
		},
		[]string{"status"}, // match, mismatch
	)

	// Cache metrics
	CacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "warp_cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	CacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "warp_cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	CacheSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_cache_size_bytes",
			Help: "Current cache size in bytes",
		},
	)

	// Rate limiting metrics
	RateLimitedRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_rate_limited_requests_total",
			Help: "Total number of rate limited requests",
		},
		[]string{"client_ip"},
	)

	// WebSocket metrics
	ActiveWebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	WebSocketMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_websocket_messages_total",
			Help: "Total number of WebSocket messages sent",
		},
		[]string{"type"}, // progress, error
	)

	// HTTP request metrics
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
)

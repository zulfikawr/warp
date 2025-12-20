package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Cache Metrics
//
// These metrics track checksum cache and buffer pool performance.
// The cache stores file checksums to avoid recomputation.
// Use these metrics to optimize cache size and hit rates.

var (
	// CacheHits counts successful checksum cache lookups.
	// Use this to monitor cache effectiveness.
	CacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "warp_cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	// CacheMisses counts failed checksum cache lookups.
	// Use this to identify cache sizing issues.
	CacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "warp_cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	// CacheSize tracks current cache memory usage in bytes.
	// Use this to monitor cache memory consumption.
	CacheSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_cache_size_bytes",
			Help: "Current cache size in bytes",
		},
	)

	// ChecksumVerifications tracks file integrity checks.
	// Labels: status (match, mismatch)
	// Use this to monitor data integrity and identify corruption issues.
	ChecksumVerifications = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_checksum_verifications_total",
			Help: "Total number of checksum verifications",
		},
		[]string{"status"},
	)
)

// Helper functions for cache metrics

// RecordCacheHit records a successful cache lookup.
func RecordCacheHit() {
	CacheHits.Inc()
}

// RecordCacheMiss records a failed cache lookup.
func RecordCacheMiss() {
	CacheMisses.Inc()
}

// RecordChecksumMatch records a successful checksum verification.
func RecordChecksumMatch() {
	ChecksumVerifications.WithLabelValues("match").Inc()
}

// RecordChecksumMismatch records a failed checksum verification.
func RecordChecksumMismatch() {
	ChecksumVerifications.WithLabelValues("mismatch").Inc()
}

// SetCacheSize updates the current cache size metric.
func SetCacheSize(bytes int64) {
	CacheSize.Set(float64(bytes))
}

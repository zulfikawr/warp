package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP Metrics
//
// These metrics track HTTP request performance and rate limiting.
// Use these metrics to monitor API endpoint performance and identify
// rate limiting effectiveness.

var (
	// HTTPRequestDuration tracks HTTP request processing time.
	// Labels: method (GET, POST, PUT), path (/d/, /u/, /health), status (200, 404, 500)
	// Use this to identify slow endpoints and optimize request handling.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestsTotal counts HTTP requests by endpoint and status.
	// Labels: method, path, status
	// Use this to track request volume and identify error patterns.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// RateLimitedRequests counts requests that exceeded rate limits.
	// Labels: limit_type (bandwidth, requests)
	// Use this to track rate limiting effectiveness.
	RateLimitedRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_rate_limited_requests_total",
			Help: "Total number of rate limited requests",
		},
		[]string{"limit_type"},
	)

	// RateLimitedClients tracks unique clients that have been rate limited.
	// This is a gauge that can be reset periodically.
	RateLimitedClients = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_rate_limited_clients",
			Help: "Number of unique clients currently rate limited",
		},
	)
)

// Helper functions for HTTP metrics

// RecordRateLimit records a rate-limited request by type.
func RecordRateLimit(limitType string) {
	RateLimitedRequests.WithLabelValues(limitType).Inc()
}

// RecordBandwidthRateLimit records a bandwidth rate limit event.
func RecordBandwidthRateLimit() {
	RateLimitedRequests.WithLabelValues("bandwidth").Inc()
}

// RecordRequestRateLimit records a request rate limit event.
func RecordRequestRateLimit() {
	RateLimitedRequests.WithLabelValues("requests").Inc()
}

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
	// Labels: client_ip
	// Use this to identify abusive clients and tune rate limiting.
	RateLimitedRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_rate_limited_requests_total",
			Help: "Total number of rate limited requests",
		},
		[]string{"client_ip"},
	)
)

// Helper functions for HTTP metrics

// RecordRateLimit records a rate-limited request for a client IP.
func RecordRateLimit(clientIP string) {
	RateLimitedRequests.WithLabelValues(clientIP).Inc()
}

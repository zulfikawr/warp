package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Session Metrics
//
// These metrics track transfer session lifecycle and reliability.
// Sessions manage the state of multi-part and chunked uploads.
// Use these metrics to monitor session management and identify issues.

var (
	// SessionDuration tracks the total time from session creation to completion.
	// Labels: type (upload, download)
	// Use this to understand end-to-end transfer time including retries.
	SessionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "warp_session_duration_seconds",
			Help:    "Total session duration from start to completion",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~1 hour
		},
		[]string{"type"},
	)

	// RetryAttemptsTotal counts retry attempts during transfers.
	// Labels: operation (upload, download, chunk), reason (network, timeout, server_error)
	// Use this to identify reliability issues and retry patterns.
	RetryAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_retry_attempts_total",
			Help: "Total retry attempts by operation and reason",
		},
		[]string{"operation", "reason"},
	)

	// ErrorsTotal counts errors by type and operation.
	// Labels: type (network, validation, permission, disk), operation (upload, download)
	// Use this to identify common error patterns and debugging priorities.
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_errors_total",
			Help: "Total errors by type and operation",
		},
		[]string{"type", "operation"},
	)
)

// Helper functions for session metrics

// RecordUploadSession records the duration of an upload session.
func RecordUploadSession(durationSeconds float64) {
	SessionDuration.WithLabelValues("upload").Observe(durationSeconds)
}

// RecordDownloadSession records the duration of a download session.
func RecordDownloadSession(durationSeconds float64) {
	SessionDuration.WithLabelValues("download").Observe(durationSeconds)
}

// RecordRetry records a retry attempt.
func RecordRetry(operation, reason string) {
	RetryAttemptsTotal.WithLabelValues(operation, reason).Inc()
}

// RecordError records an error by type and operation.
func RecordError(errorType, operation string) {
	ErrorsTotal.WithLabelValues(errorType, operation).Inc()
}

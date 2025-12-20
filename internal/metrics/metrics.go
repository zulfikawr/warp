// Package metrics provides Prometheus metrics for monitoring Warp transfers.
//
// The metrics package is organized into logical modules:
//
//   - upload.go: File upload performance and throughput metrics
//   - download.go: File download performance and throughput metrics
//   - chunks.go: Parallel chunk upload metrics for large file transfers
//   - session.go: Transfer session lifecycle, retries, and error tracking
//   - cache.go: Checksum cache and buffer pool performance metrics
//   - websocket.go: Real-time progress streaming metrics
//   - http.go: HTTP request performance and rate limiting metrics
//
// Usage Examples:
//
// Recording an upload:
//
//	start := time.Now()
//	metrics.ActiveUploads.Inc()
//	defer metrics.ActiveUploads.Dec()
//	// ... perform upload ...
//	metrics.UploadDuration.WithLabelValues("pdf").Observe(time.Since(start).Seconds())
//	metrics.UploadsTotal.WithLabelValues("pdf", "success").Inc()
//
// Recording a cache hit:
//
//	if cached, ok := cache.Get(key); ok {
//	    metrics.RecordCacheHit()
//	    return cached
//	}
//	metrics.RecordCacheMiss()
//
// Recording a WebSocket message:
//
//	metrics.WebSocketConnected()
//	defer metrics.WebSocketDisconnected()
//	metrics.RecordProgressMessage()
//
// All metrics are automatically registered with Prometheus and exposed
// via the /metrics endpoint when the server starts.
package metrics

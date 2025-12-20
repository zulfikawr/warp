package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// WebSocket Metrics
//
// These metrics track real-time progress streaming via WebSocket connections.
// WebSockets provide live upload progress updates to browser clients.
// Use these metrics to monitor connection health and message throughput.

var (
	// ActiveWebSocketConnections tracks currently connected WebSocket clients.
	// Use this to monitor concurrent real-time progress viewers.
	ActiveWebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "warp_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	// WebSocketMessagesTotal counts messages sent over WebSocket connections.
	// Labels: type (progress, error, complete)
	// Use this to track message patterns and identify chatty connections.
	WebSocketMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "warp_websocket_messages_total",
			Help: "Total number of WebSocket messages sent",
		},
		[]string{"type"},
	)
)

// Helper functions for WebSocket metrics

// WebSocketConnected increments the active connection counter.
// Call this when a new WebSocket connection is established.
func WebSocketConnected() {
	ActiveWebSocketConnections.Inc()
}

// WebSocketDisconnected decrements the active connection counter.
// Call this when a WebSocket connection is closed.
func WebSocketDisconnected() {
	ActiveWebSocketConnections.Dec()
}

// RecordProgressMessage records a progress update message.
func RecordProgressMessage() {
	WebSocketMessagesTotal.WithLabelValues("progress").Inc()
}

// RecordErrorMessage records an error message.
func RecordErrorMessage() {
	WebSocketMessagesTotal.WithLabelValues("error").Inc()
}

// RecordCompleteMessage records a completion message.
func RecordCompleteMessage() {
	WebSocketMessagesTotal.WithLabelValues("complete").Inc()
}

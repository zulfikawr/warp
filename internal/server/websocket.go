package server

import (
	"net/http"
	"time"

	"github.com/zulfikawr/warp/internal/logging"
	"go.uber.org/zap"

	"github.com/gorilla/websocket"
	"github.com/zulfikawr/warp/internal/metrics"
)

// WebSocket upgrader for real-time progress updates
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  WebSocketReadBuffer,
	WriteBufferSize: WebSocketWriteBuffer,
	CheckOrigin: func(r *http.Request) bool {
		// Allow non-browser clients (no Origin header)
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}

		// Parse the Origin header
		_, err := http.NewRequest("GET", origin, nil)
		if err != nil {
			return false
		}

		// Ensure the origin matches the request host
		return r.Header.Get("Origin") == "http://"+r.Host || r.Header.Get("Origin") == "https://"+r.Host
	},
}

// handleProgressWebSocket streams real-time progress updates via WebSocket
func (s *Server) handleProgressWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	metrics.ActiveWebSocketConnections.Inc()
	defer metrics.ActiveWebSocketConnections.Dec()

	// Send progress updates periodically
	ticker := time.NewTicker(WebSocketUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Collect all active uploads/downloads
			progress := make([]map[string]interface{}, 0)
			s.SessionMgr.RangeActiveUploads(func(key, value interface{}) bool {
				tracker := value.(*ProgressTracker)
				progress = append(progress, tracker.GetProgress())
				return true
			})

			// Send progress update
			if len(progress) > 0 {
				metrics.WebSocketMessagesTotal.WithLabelValues("progress").Inc()
				if err := conn.WriteJSON(map[string]interface{}{
					"type":      "progress",
					"transfers": progress,
					"timestamp": time.Now().Unix(),
				}); err != nil {
					// Client disconnected
					return
				}
			}
		case <-r.Context().Done():
			// Connection closed
			return
		}
	}
}

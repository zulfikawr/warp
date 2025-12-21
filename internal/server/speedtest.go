package server

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
)

const (
	// Size of test data to generate for download tests
	speedTestDownloadSize = 10485760 // 10 MB
)

// handleSpeedTestDownload serves random data for download speed testing
func (s *Server) handleSpeedTestDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set headers to prevent caching and compression
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Length", "10485760")

	// Generate random data once and reuse it
	buffer := make([]byte, 65536) // 64 KB buffer
	if _, err := rand.Read(buffer); err != nil {
		http.Error(w, "failed to generate test data", http.StatusInternalServerError)
		return
	}

	remaining := int64(speedTestDownloadSize)

	for remaining > 0 {
		toWrite := int64(len(buffer))
		if remaining < toWrite {
			toWrite = remaining
		}

		// Write to response
		n, err := w.Write(buffer[:toWrite])
		if err != nil {
			// Client disconnected or write error
			return
		}

		remaining -= int64(n)
	}
}

// handleSpeedTestUpload receives and discards data for upload speed testing
func (s *Server) handleSpeedTestUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read and hash all uploaded data to simulate real transfer overhead
	hash := sha256.New()
	bytesRead, err := io.Copy(hash, r.Body)
	if err != nil {
		http.Error(w, "failed to read upload data", http.StatusBadRequest)
		return
	}

	// Return success with bytes received
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","bytes_received":%d}`, bytesRead)
}

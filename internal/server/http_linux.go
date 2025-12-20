//go:build linux

package server

import (
	"errors"
	"fmt"
	"github.com/zulfikawr/warp/internal/logging"
	"github.com/zulfikawr/warp/internal/ui"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"
)

// sendfileZeroCopy uses the sendfile(2) syscall for zero-copy transfer on Linux
// This bypasses user-space copying and significantly improves performance for large files
func sendfileZeroCopy(w http.ResponseWriter, f *os.File, offset int64, length int64) error {
	// Get checksum header if it was set
	checksumHeader := w.Header().Get("X-Content-SHA256")

	// Try to hijack the connection to get the underlying socket
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("hijacking not supported")
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return fmt.Errorf("failed to hijack connection: %w", err)
	}
	// Clean up connection on exit
	defer func() {
		if conn != nil {
			if err := conn.Close(); err != nil {
				logging.Warn("Failed to close connection", zap.Error(err))
			}
		}
	}()

	// Write HTTP response headers manually
	headers := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nContent-Type: application/octet-stream\r\n", length)
	if checksumHeader != "" {
		headers += fmt.Sprintf("X-Content-SHA256: %s\r\n", checksumHeader)
	}
	headers += "\r\n"

	if _, err := bufrw.WriteString(headers); err != nil {
		return fmt.Errorf("failed to write headers: %w", err)
	}
	if err := bufrw.Flush(); err != nil {
		return fmt.Errorf("failed to flush headers: %w", err)
	}

	// Get the raw connection file descriptor
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return errors.New("not a TCP connection")
	}

	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to get raw connection: %w", err)
	}

	// Use sendfile(2) syscall for kernel-level zero-copy transfer
	var sendErr error
	var totalSent int64

	err = rawConn.Write(func(socketFD uintptr) bool {
		for totalSent < length {
			remaining := length - totalSent

			// sendfile can transfer up to ~2GB at a time
			chunkSize := remaining
			if chunkSize > 1<<30 { // 1GB chunks
				chunkSize = 1 << 30
			}

			// Calculate offset fresh each iteration to prevent corruption
			// sendfile modifies the offset pointer, so we must use a local copy
			useOffset := offset + totalSent
			n, err := syscall.Sendfile(int(socketFD), int(f.Fd()), &useOffset, int(chunkSize))
			if err != nil {
				if err == syscall.EAGAIN {
					// Would block, try again
					continue
				}
				sendErr = err
				return false
			}

			totalSent += int64(n)
			if n == 0 && totalSent < length {
				// EOF before expected length
				sendErr = io.ErrUnexpectedEOF
				return false
			}
		}
		return true
	})

	if err != nil {
		return fmt.Errorf("sendfile syscall failed: %w", err)
	}
	if sendErr != nil {
		return fmt.Errorf("sendfile transfer failed: %w", sendErr)
	}

	return nil
}

// checkDiskSpace verifies sufficient disk space is available before upload (Linux-specific)
func checkDiskSpace(path string, required int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		// If we can't check, allow the operation (fail gracefully later)
		return nil
	}
	available := int64(stat.Bavail) * int64(stat.Bsize)
	// Keep 1GB buffer for system operations
	if available-required < 1<<30 {
		return fmt.Errorf("insufficient disk space: need %s, have %s available",
			ui.FormatBytes(required), ui.FormatBytes(available))
	}
	return nil
}

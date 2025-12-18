//go:build !linux
// +build !linux

package server

import (
	"errors"
	"net/http"
	"os"
)

// sendfileZeroCopy is not available on non-Linux platforms
func sendfileZeroCopy(w http.ResponseWriter, f *os.File, offset int64, length int64) error {
	return errors.New("sendfile not supported on this platform")
}

// checkDiskSpace is a no-op on non-Linux platforms (graceful degradation)
// Returns nil to allow the operation to proceed
func checkDiskSpace(path string, required int64) error {
	// Cannot check disk space on this platform, allow operation to continue
	// If there's insufficient space, the write will fail naturally
	return nil
}

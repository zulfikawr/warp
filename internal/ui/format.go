package ui

import (
	"fmt"
	"time"
)

// FormatBytes formats bytes into human-readable string with appropriate units (e.g., "1.5 MB")
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatSpeed formats bytes per second into human-readable string (e.g., "10.5 MB/s")
func FormatSpeed(bytesPerSec float64) string {
	const unit = 1024
	if bytesPerSec < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}

	units := []string{"KB/s", "MB/s", "GB/s", "TB/s"}
	div := bytesPerSec
	unitIndex := -1

	for div >= unit && unitIndex < len(units)-1 {
		div /= unit
		unitIndex++
	}

	if unitIndex >= 0 && unitIndex < len(units) {
		return fmt.Sprintf("%.1f %s", div, units[unitIndex])
	}
	return fmt.Sprintf("%.1f B/s", bytesPerSec)
}

// FormatDuration formats a duration into human-readable string (e.g., "2m30s", "1h05m00s")
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	} else if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

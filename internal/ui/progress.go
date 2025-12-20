package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zulfikawr/warp/internal/protocol"
)

// Pre-computed progress bars to eliminate string allocations (~1000/sec during transfer)
var progressBars [protocol.ProgressBarWidth + 1]string

func init() {
	// Pre-compute progress bars
	for i := 0; i <= protocol.ProgressBarWidth; i++ {
		progressBars[i] = strings.Repeat(protocol.ProgressBarFilled, i) + strings.Repeat(protocol.ProgressBarEmpty, protocol.ProgressBarWidth-i)
	}
}

type ProgressReader struct {
	R         io.Reader
	Total     int64
	Current   int64
	Out       io.Writer
	StartTime time.Time
}

func (p *ProgressReader) Read(b []byte) (int, error) {
	// Initialize start time on first read
	if p.StartTime.IsZero() {
		p.StartTime = time.Now()
	}

	n, err := p.R.Read(b)
	p.Current += int64(n)

	if p.Total > 0 && p.Out != nil {
		elapsed := time.Since(p.StartTime)
		pct := float64(p.Current) / float64(p.Total) * 100.0

		// Calculate ETA
		var etaStr string
		if p.Current > 0 && elapsed.Seconds() > 0.5 { // Only show ETA after 500ms
			rate := float64(p.Current) / elapsed.Seconds() // bytes per second
			remaining := p.Total - p.Current
			if rate > 0 {
				etaSec := float64(remaining) / rate
				eta := time.Duration(etaSec * float64(time.Second))
				etaStr = FormatDuration(eta)
			}
		}

		// Calculate speed
		speedStr := ""
		if elapsed.Seconds() > 0 {
			bytesPerSec := float64(p.Current) / elapsed.Seconds()
			speedStr = FormatSpeed(bytesPerSec)
		}

		// Format sizes with smarter units
		currentSize := formatSize(p.Current)
		totalSize := formatSize(p.Total)
		elapsedStr := FormatDuration(elapsed)

		// Format progress bar with detailed information
		if etaStr != "" && speedStr != "" {
			_, _ = fmt.Fprintf(p.Out, "\r[%s%-*s%s] %s%3.0f%%%s | %s/%s | %s | Time: %s | ETA: %s",
				Colors.Green, protocol.ProgressBarWidth, bar(pct), Colors.Reset, Colors.Green, pct, Colors.Reset, currentSize, totalSize, speedStr, elapsedStr, etaStr)
		} else if speedStr != "" {
			_, _ = fmt.Fprintf(p.Out, "\r[%s%-*s%s] %s%3.0f%%%s | %s/%s | %s | Time: %s",
				Colors.Green, protocol.ProgressBarWidth, bar(pct), Colors.Reset, Colors.Green, pct, Colors.Reset, currentSize, totalSize, speedStr, elapsedStr)
		} else {
			_, _ = fmt.Fprintf(p.Out, "\r[%s%-*s%s] %s%3.0f%%%s | %s/%s",
				Colors.Green, protocol.ProgressBarWidth, bar(pct), Colors.Reset, Colors.Green, pct, Colors.Reset, currentSize, totalSize)
		}
	}
	return n, err
}

// bar creates a progress bar string more efficiently using strings.Repeat
func bar(pct float64) string {
	filled := int(pct / 5)
	if filled < 0 {
		filled = 0
	}
	if filled > protocol.ProgressBarWidth {
		filled = protocol.ProgressBarWidth
	}
	return progressBars[filled]
}

// formatSize formats bytes into a human-readable string with appropriate units
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div := int64(unit)
	exp := 0
	units := []string{"KB", "MB", "GB", "TB"}

	for bytes >= div*unit && exp < len(units)-1 {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

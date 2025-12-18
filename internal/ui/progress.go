package ui

import (
	"fmt"
	"io"
	"strings"
	"time"
)

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
				etaStr = formatDuration(eta)
			}
		}
		
		// Calculate speed
		speedStr := ""
		if elapsed.Seconds() > 0 {
			bytesPerSec := float64(p.Current) / elapsed.Seconds()
			speedStr = formatSpeed(bytesPerSec)
		}
		
		// Format sizes
		currentMB := float64(p.Current) / (1024 * 1024)
		totalMB := float64(p.Total) / (1024 * 1024)
		
		// Format progress bar with detailed information
		if etaStr != "" && speedStr != "" {
			fmt.Fprintf(p.Out, "\r[%-20s] %3.0f%% | %.1f MB/%.1f MB | %s | ETA: %s", 
				bar(pct), pct, currentMB, totalMB, speedStr, etaStr)
		} else if speedStr != "" {
			fmt.Fprintf(p.Out, "\r[%-20s] %3.0f%% | %.1f MB/%.1f MB | %s", 
				bar(pct), pct, currentMB, totalMB, speedStr)
		} else {
			fmt.Fprintf(p.Out, "\r[%-20s] %3.0f%%", bar(pct), pct)
		}
	}
	return n, err
}

// bar creates a progress bar string more efficiently using strings.Repeat
func bar(pct float64) string {
	filled := int(pct / 5)
	if filled < 0 { filled = 0 }
	if filled > 20 { filled = 20 }
	return strings.Repeat("=", filled) + strings.Repeat(" ", 20-filled)
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
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

// formatSpeed formats bytes per second into a human-readable string
func formatSpeed(bytesPerSec float64) string {
	const unit = 1024
	if bytesPerSec < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
	
	div := float64(unit)
	exp := 0
	units := []string{"KB/s", "MB/s", "GB/s", "TB/s"}
	
	for bytesPerSec >= div*unit && exp < len(units)-1 {
		div *= unit
		exp++
	}
	
	return fmt.Sprintf("%.1f %s", bytesPerSec/div, units[exp])
}

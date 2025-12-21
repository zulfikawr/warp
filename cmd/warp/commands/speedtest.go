package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zulfikawr/warp/cmd/warp/ui"
	"github.com/zulfikawr/warp/internal/speedtest"
)

// Speedtest executes the speedtest command
func Speedtest(args []string) error {
	fs := flag.NewFlagSet("speedtest", flag.ExitOnError)
	fs.Usage = speedtestHelp

	timeout := fs.Duration("timeout", 30*time.Second, "timeout for speed test")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	if fs.NArg() < 1 {
		speedtestHelp()
		return fmt.Errorf("target host required")
	}

	target := fs.Arg(0)

	// Ensure target has port if not specified
	if !strings.Contains(target, ":") {
		target = target + ":8080"
	}

	fmt.Printf("%sRunning network speed test to %s...%s\n\n", ui.C.Cyan, target, ui.C.Reset)

	// Create speed test instance
	st := speedtest.New(target)

	// Run test with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	result := st.Run(ctx)

	// Handle errors
	if result.Error != nil {
		return fmt.Errorf("speed test failed: %w", result.Error)
	}

	// Display results
	displayResults(result)

	return nil
}

func displayResults(result *speedtest.Result) {
	// Upload speed
	fmt.Printf("%sUpload:%s    %s  %s\n",
		ui.C.Bold, ui.C.Reset,
		formatSpeedWithColor(result.UploadMbps),
		createProgressBar(result.UploadMbps, 200))

	// Download speed
	fmt.Printf("%sDownload:%s  %s  %s\n",
		ui.C.Bold, ui.C.Reset,
		formatSpeedWithColor(result.DownloadMbps),
		createProgressBar(result.DownloadMbps, 200))

	// Latency with quality indicator - aligned with progress bars
	qualityColor := getQualityColor(result.Quality)
	fmt.Printf("%s✓ Latency:%s   %.0fms           %s%s%s\n\n",
		ui.C.Bold, ui.C.Reset,
		result.LatencyMs,
		qualityColor, result.Quality, ui.C.Reset)

	// Transfer time estimates
	fmt.Printf("%sYour network can transfer:%s\n", ui.C.Cyan, ui.C.Reset)

	estimateFileSizes := []float64{100, 1000, 10000} // MB

	for _, sizeMB := range estimateFileSizes {
		// Use the lower of upload/download for estimate (conservative)
		speed := result.DownloadMbps
		if result.UploadMbps < result.DownloadMbps {
			speed = result.UploadMbps
		}

		duration := speedtest.EstimateTransferTime(sizeMB, speed)
		sizeStr := formatFileSize(sizeMB)
		durationStr := speedtest.FormatDuration(duration)

		fmt.Printf("  • %s file in ~%s\n", sizeStr, durationStr)
	}
}

func formatSpeedWithColor(mbps float64) string {
	speed := speedtest.FormatSpeed(mbps)

	// Pad to align nicely
	padding := 12 - len(speed)
	if padding < 0 {
		padding = 0
	}

	// Color based on speed
	var color string
	if mbps >= 100 {
		color = ui.C.Green
	} else if mbps >= 50 {
		color = ui.C.Cyan
	} else if mbps >= 25 {
		color = ui.C.Yellow
	} else {
		color = ui.C.Red
	}

	return fmt.Sprintf("%s%s%s%s", color, speed, ui.C.Reset, strings.Repeat(" ", padding))
}

func createProgressBar(value, max float64) string {
	const barWidth = 20

	// Calculate filled portion
	filled := int((value / max) * barWidth)
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

	// Build progress bar using the same style as file transfers (=====)
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled)

	return fmt.Sprintf("[%s%s%s]", ui.C.Green, bar, ui.C.Reset)
}

func getQualityColor(quality string) string {
	switch quality {
	case "Excellent":
		return ui.C.Green
	case "Very Good":
		return ui.C.Cyan
	case "Good":
		return ui.C.Yellow
	case "Fair":
		return ui.C.Yellow
	default:
		return ui.C.Red
	}
}

func formatFileSize(mb float64) string {
	if mb >= 1000 {
		return fmt.Sprintf("%.0f GB", mb/1000)
	}
	return fmt.Sprintf("%.0f MB", mb)
}

func speedtestHelp() {
	fmt.Fprintf(os.Stderr, `%sUsage:%s warp speedtest [options] <host>

%sDescription:%s
  Test network speed (upload/download/latency) to a target host.
  This helps you understand your network performance and estimate transfer times.

%sArguments:%s
  <host>               Target host to test (e.g., 192.168.1.100 or example.com:8080)

%sOptions:%s
  --timeout <duration> Timeout for the speed test (default: 30s)
  -h, --help          Show this help message

%sExamples:%s
  warp speedtest 192.168.1.100
  warp speedtest 192.168.1.100:54321
  warp speedtest example.com:8080 --timeout 1m

%sOutput:%s
  The command displays:
  - Upload speed (Mbps/Gbps)
  - Download speed (Mbps/Gbps)
  - Network latency (milliseconds)
  - Connection quality rating
  - Estimated transfer times for common file sizes
`,
		ui.C.Bold, ui.C.Reset,
		ui.C.Bold, ui.C.Reset,
		ui.C.Bold, ui.C.Reset,
		ui.C.Bold, ui.C.Reset,
		ui.C.Bold, ui.C.Reset,
		ui.C.Bold, ui.C.Reset)
}

package server

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
)

// ZipProgress tracks compression progress for multi-file display
type ZipProgress struct {
	TotalFiles     int
	ProcessedFiles atomic.Int32
	TotalBytes     int64
	ProcessedBytes atomic.Int64
	CurrentFile    string
	Output         io.Writer
}

// Update prints the current progress
func (zp *ZipProgress) Update() {
	if zp.Output == nil {
		return
	}
	processed := zp.ProcessedFiles.Load()
	fmt.Fprintf(zp.Output, "\rCompressing: %d/%d files | %s",
		processed, zp.TotalFiles, zp.CurrentFile)
}

// ZipDirectory streams a zip of srcDir to w.
func ZipDirectory(w io.Writer, srcDir string) error {
	return ZipDirectoryWithProgress(w, srcDir, nil)
}

// ZipDirectoryWithProgress streams a zip of srcDir to w with progress tracking
func ZipDirectoryWithProgress(w io.Writer, srcDir string, progressOut io.Writer) error {
	// First pass: count files and total size
	var fileCount int
	var totalSize int64
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})

	progress := &ZipProgress{
		TotalFiles: fileCount,
		TotalBytes: totalSize,
		Output:     progressOut,
	}

	if progressOut != nil {
		fmt.Fprintf(progressOut, "\nPreparing %d files (%s total)...\n", fileCount, formatZipSize(totalSize))
	}

	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		progress.CurrentFile = rel
		progress.Update()

		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = rel
		fh.Method = zip.Deflate
		f, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		// Close immediately to prevent file handle exhaustion
		_, copyErr := io.Copy(f, file)
		closeErr := file.Close()

		progress.ProcessedFiles.Add(1)
		progress.ProcessedBytes.Add(info.Size())

		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})

	if progressOut != nil && err == nil {
		fmt.Fprintf(progressOut, "\r✓ Compressed %d files (%s total)          \n",
			fileCount, formatZipSize(totalSize))
	}

	return err
}

// formatZipSize formats bytes for zip progress display
func formatZipSize(bytes int64) string {
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

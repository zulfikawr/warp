package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sanitizeFilename validates and sanitizes a filename for secure filesystem operations
func sanitizeFilename(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty filename")
	}

	// Reject path separators immediately (directory traversal prevention)
	if strings.ContainsAny(name, "/\\") {
		return "", errors.New("filename contains path separators")
	}

	// Reject null bytes (can cause issues in some filesystems)
	if strings.Contains(name, "\x00") {
		return "", errors.New("filename contains null bytes")
	}

	// Reject ".." anywhere in the name (even as substring like "0..")
	if strings.Contains(name, "..") {
		return "", errors.New("filename contains directory traversal sequence")
	}

	// Clean and get base
	cleaned := filepath.Base(filepath.Clean(name))

	// Verify cleaning didn't change the name (indicates potential attack)
	if cleaned != name {
		return "", fmt.Errorf("filename normalization changed input: %q -> %q", name, cleaned)
	}

	// Reject dangerous names
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "", errors.New("invalid filename")
	}

	// Remove control characters and DEL
	for _, r := range cleaned {
		if r < 32 || r == 0x7F {
			return "", errors.New("filename contains control characters")
		}
	}

	// Reject filenames that are purely whitespace
	if strings.TrimSpace(cleaned) == "" {
		return "", errors.New("filename is only whitespace")
	}

	// Limit length to 255 bytes (common filesystem limit)
	if len(cleaned) > 255 {
		return "", errors.New("filename too long (max 255 bytes)")
	}

	return cleaned, nil
}

// findUniqueFilename prevents file overwrites by appending (1), (2), etc.
func findUniqueFilename(dir, name string) string {
	name, err := sanitizeFilename(name)
	if err != nil {
		// Fallback to timestamp-based name if sanitization fails
		name = fmt.Sprintf("upload_%d", time.Now().UnixNano())
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	// First try: exact match
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	// Collision found: try "file (1).ext", "file (2).ext", etc.
	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s (%d)%s", base, i, ext)
		path = filepath.Join(dir, newName)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
	}

	// Fallback: Use timestamp if 1000 collisions (unlikely)
	return filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext))
}

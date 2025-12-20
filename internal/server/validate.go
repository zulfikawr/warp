package server

import (
	"fmt"
	"regexp"
)

const (
	// MaxSessionIDLength is the maximum allowed length for session IDs
	MaxSessionIDLength = 64

	// MinSessionIDLength is the minimum allowed length for session IDs
	MinSessionIDLength = 8

	// MaxChunkSize is the maximum allowed chunk size (100MB)
	MaxChunkSize = 100 * 1024 * 1024

	// MinChunkSize is the minimum allowed chunk size (64KB)
	MinChunkSize = 64 * 1024

	// MaxChunkID is the maximum chunk index (prevent integer overflow)
	MaxChunkID = 100000

	// MaxTotalChunks is the maximum number of chunks allowed
	MaxTotalChunks = 100000
)

// sessionIDPattern validates session IDs (alphanumeric, hyphens, and underscores only)
var sessionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`)

// ValidateSessionID checks if a session ID is valid
func ValidateSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is empty")
	}

	if len(sessionID) < MinSessionIDLength {
		return fmt.Errorf("session ID too short: must be at least %d characters", MinSessionIDLength)
	}

	if len(sessionID) > MaxSessionIDLength {
		return fmt.Errorf("session ID too long: maximum %d characters", MaxSessionIDLength)
	}

	if !sessionIDPattern.MatchString(sessionID) {
		return fmt.Errorf("session ID contains invalid characters: only alphanumeric, hyphens, and underscores allowed")
	}

	return nil
}

// ValidateOffset checks if an upload offset is valid for the given file size
func ValidateOffset(offset, fileSize int64) error {
	if offset < 0 {
		return fmt.Errorf("offset cannot be negative: %d", offset)
	}

	if fileSize > 0 && offset > fileSize {
		return fmt.Errorf("offset %d exceeds file size %d", offset, fileSize)
	}

	return nil
}

// ValidateChunkID checks if a chunk ID is valid
func ValidateChunkID(chunkID, totalChunks int) error {
	if chunkID < 0 {
		return fmt.Errorf("chunk ID cannot be negative: %d", chunkID)
	}

	if chunkID > MaxChunkID {
		return fmt.Errorf("chunk ID too large: %d (max: %d)", chunkID, MaxChunkID)
	}

	if totalChunks > 0 && chunkID >= totalChunks {
		return fmt.Errorf("chunk ID %d exceeds total chunks %d", chunkID, totalChunks)
	}

	return nil
}

// ValidateTotalChunks checks if the total chunks count is reasonable
func ValidateTotalChunks(totalChunks int) error {
	if totalChunks <= 0 {
		return fmt.Errorf("total chunks must be positive: %d", totalChunks)
	}

	if totalChunks > MaxTotalChunks {
		return fmt.Errorf("total chunks too large: %d (max: %d)", totalChunks, MaxTotalChunks)
	}

	return nil
}

// ValidateChunkSize checks if a chunk size is reasonable
func ValidateChunkSize(chunkSize int64) error {
	if chunkSize < MinChunkSize {
		return fmt.Errorf("chunk size too small: %d bytes (min: %d)", chunkSize, MinChunkSize)
	}

	if chunkSize > MaxChunkSize {
		return fmt.Errorf("chunk size too large: %d bytes (max: %d)", chunkSize, MaxChunkSize)
	}

	return nil
}

// ValidateContentLength checks if content length is within acceptable bounds
func ValidateContentLength(contentLength, maxSize int64) error {
	if contentLength < 0 {
		return fmt.Errorf("content length cannot be negative")
	}

	if contentLength == 0 {
		return fmt.Errorf("content length is zero")
	}

	if contentLength > maxSize {
		return fmt.Errorf("content length %d exceeds maximum %d", contentLength, maxSize)
	}

	return nil
}

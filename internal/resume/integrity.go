package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
)

// IntegrityVerifier handles checksum verification for transfers
type IntegrityVerifier struct {
	algorithm       string         // "sha256"
	chunkHashes     map[int]string // Chunk ID -> hash
	fileHash        string         // Expected final file hash
	corruptedChunks []int          // List of corrupted chunks
	mu              sync.RWMutex
}

// ChunkReader interface for reading chunks
type ChunkReader interface {
	ReadChunk(chunkID int) ([]byte, error)
}

// NewIntegrityVerifier creates a new integrity verifier
func NewIntegrityVerifier() *IntegrityVerifier {
	return &IntegrityVerifier{
		algorithm:       "sha256",
		chunkHashes:     make(map[int]string),
		corruptedChunks: make([]int, 0),
	}
}

// SetExpectedFileHash sets the expected final file checksum
func (v *IntegrityVerifier) SetExpectedFileHash(hash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.fileHash = hash
}

// RecordChunkHash stores the hash for a completed chunk
func (v *IntegrityVerifier) RecordChunkHash(chunkID int, hash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.chunkHashes[chunkID] = hash
}

// VerifyChunk verifies a chunk's data against its stored hash
func (v *IntegrityVerifier) VerifyChunk(chunkID int, data []byte) error {
	v.mu.RLock()
	expectedHash, exists := v.chunkHashes[chunkID]
	v.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no hash recorded for chunk %d", chunkID)
	}

	// Compute actual hash
	hash := sha256.Sum256(data)
	actualHash := hex.EncodeToString(hash[:])

	if actualHash != expectedHash {
		v.mu.Lock()
		v.corruptedChunks = append(v.corruptedChunks, chunkID)
		v.mu.Unlock()

		return NewIntegrityError(
			fmt.Errorf("chunk %d hash mismatch", chunkID),
			chunkID,
			expectedHash,
			actualHash,
		)
	}

	return nil
}

// VerifyAllChunks verifies all stored chunk hashes against actual data
func (v *IntegrityVerifier) VerifyAllChunks(reader ChunkReader) ([]int, error) {
	v.mu.RLock()
	chunkIDs := make([]int, 0, len(v.chunkHashes))
	for id := range v.chunkHashes {
		chunkIDs = append(chunkIDs, id)
	}
	v.mu.RUnlock()

	corrupted := make([]int, 0)

	for _, chunkID := range chunkIDs {
		data, err := reader.ReadChunk(chunkID)
		if err != nil {
			return nil, fmt.Errorf("failed to read chunk %d: %w", chunkID, err)
		}

		if err := v.VerifyChunk(chunkID, data); err != nil {
			corrupted = append(corrupted, chunkID)
		}
	}

	return corrupted, nil
}

// VerifyFile verifies the complete file checksum
func (v *IntegrityVerifier) VerifyFile(filePath string) error {
	v.mu.RLock()
	expectedHash := v.fileHash
	v.mu.RUnlock()

	if expectedHash == "" {
		return fmt.Errorf("no expected file hash set")
	}

	// Open file
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	// Compute hash
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("failed to compute file hash: %w", err)
	}

	actualHash := hex.EncodeToString(hash.Sum(nil))

	if actualHash != expectedHash {
		return NewIntegrityError(
			fmt.Errorf("file hash mismatch"),
			-1, // -1 indicates file-level error
			expectedHash,
			actualHash,
		)
	}

	return nil
}

// GetCorruptedChunks returns list of chunks that failed verification
func (v *IntegrityVerifier) GetCorruptedChunks() []int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Return a copy
	corrupted := make([]int, len(v.corruptedChunks))
	copy(corrupted, v.corruptedChunks)
	return corrupted
}

// ClearCorruptedChunks clears the list of corrupted chunks
func (v *IntegrityVerifier) ClearCorruptedChunks() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.corruptedChunks = make([]int, 0)
}

// ExportState returns serializable verification state
func (v *IntegrityVerifier) ExportState() *VerificationState {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Create a copy of chunk hashes
	chunkHashes := make(map[int]string, len(v.chunkHashes))
	for k, v := range v.chunkHashes {
		chunkHashes[k] = v
	}

	return &VerificationState{
		Algorithm:        v.algorithm,
		ChunkHashes:      chunkHashes,
		ExpectedFileHash: v.fileHash,
	}
}

// ImportState restores verification state from checkpoint
func (v *IntegrityVerifier) ImportState(state *VerificationState) error {
	if state == nil {
		return fmt.Errorf("verification state is nil")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	v.algorithm = state.Algorithm
	v.chunkHashes = make(map[int]string, len(state.ChunkHashes))
	for k, hash := range state.ChunkHashes {
		v.chunkHashes[k] = hash
	}
	v.fileHash = state.ExpectedFileHash

	return nil
}

// GetChunkHash returns the hash for a specific chunk
func (v *IntegrityVerifier) GetChunkHash(chunkID int) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	hash, exists := v.chunkHashes[chunkID]
	return hash, exists
}

// GetChunkCount returns the number of chunks with recorded hashes
func (v *IntegrityVerifier) GetChunkCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.chunkHashes)
}

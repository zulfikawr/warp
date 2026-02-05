package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// CheckpointFormatVersion defines the current checkpoint format version
const CheckpointFormatVersion = "1.0.0"

// Checkpoint represents a persistent transfer state
type Checkpoint struct {
	// Identity
	SessionID string `json:"session_id"`
	Version   string `json:"version"` // Checkpoint format version

	// Transfer metadata
	SourcePath      string `json:"source_path"`
	DestinationPath string `json:"destination_path"`
	Direction       string `json:"direction"` // "upload" or "download"
	TotalSize       int64  `json:"total_size"`

	// Chunk tracking
	ChunkSize       int64 `json:"chunk_size"`
	TotalChunks     int   `json:"total_chunks"`
	CompletedChunks []int `json:"completed_chunks"`

	// Encryption state (no raw keys)
	Encrypted       bool             `json:"encrypted"`
	EncryptionState *EncryptionState `json:"encryption_state,omitempty"`

	// Integrity verification
	ChunkChecksums map[int]string `json:"chunk_checksums"`
	FileChecksum   string         `json:"file_checksum,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`

	// Checksum for tamper detection
	CheckpointHash string `json:"checkpoint_hash"`
}

// EncryptionState stores encryption metadata (no raw keys)
type EncryptionState struct {
	BaseNonce       []byte `json:"base_nonce"`        // Initial nonce
	ChunkCounter    uint64 `json:"chunk_counter"`     // Current counter
	Salt            []byte `json:"salt"`              // Key derivation salt
	KeyDerivationID string `json:"key_derivation_id"` // Reference for re-deriving key
}

// VerificationState stores integrity verification data
type VerificationState struct {
	Algorithm        string         `json:"algorithm"`
	ChunkHashes      map[int]string `json:"chunk_hashes"`
	ExpectedFileHash string         `json:"expected_file_hash,omitempty"`
}

// CheckpointSummary provides a lightweight view for listing
type CheckpointSummary struct {
	SessionID       string    `json:"session_id"`
	SourcePath      string    `json:"source_path"`
	DestinationPath string    `json:"destination_path"`
	Direction       string    `json:"direction"`
	Progress        float64   `json:"progress"` // 0.0 - 1.0
	TotalSize       int64     `json:"total_size"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Encrypted       bool      `json:"encrypted"`
}

// CheckpointOptions contains options for creating a new checkpoint
type CheckpointOptions struct {
	SessionID       string
	SourcePath      string
	DestinationPath string
	Direction       string
	TotalSize       int64
	ChunkSize       int64
	TotalChunks     int
	Encrypted       bool
	ExpiresIn       time.Duration
}

// NewCheckpoint creates a new checkpoint with the given options
func NewCheckpoint(opts CheckpointOptions) *Checkpoint {
	now := time.Now()
	expiresAt := now.Add(opts.ExpiresIn)
	if opts.ExpiresIn == 0 {
		expiresAt = now.Add(24 * time.Hour) // Default 24 hour expiry
	}

	cp := &Checkpoint{
		SessionID:       opts.SessionID,
		Version:         CheckpointFormatVersion,
		SourcePath:      opts.SourcePath,
		DestinationPath: opts.DestinationPath,
		Direction:       opts.Direction,
		TotalSize:       opts.TotalSize,
		ChunkSize:       opts.ChunkSize,
		TotalChunks:     opts.TotalChunks,
		CompletedChunks: make([]int, 0),
		Encrypted:       opts.Encrypted,
		ChunkChecksums:  make(map[int]string),
		CreatedAt:       now,
		UpdatedAt:       now,
		ExpiresAt:       expiresAt,
	}

	return cp
}

// ComputeHash computes the SHA256 hash of the checkpoint for tamper detection
// The hash is computed over all fields except CheckpointHash itself
func (c *Checkpoint) ComputeHash() (string, error) {
	// Create a copy without the hash field
	temp := *c
	temp.CheckpointHash = ""

	data, err := json.Marshal(temp)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// UpdateHash updates the checkpoint hash
func (c *Checkpoint) UpdateHash() error {
	hash, err := c.ComputeHash()
	if err != nil {
		return err
	}
	c.CheckpointHash = hash
	return nil
}

// VerifyHash verifies the checkpoint hasn't been tampered with
func (c *Checkpoint) VerifyHash() (bool, error) {
	expectedHash := c.CheckpointHash
	actualHash, err := c.ComputeHash()
	if err != nil {
		return false, err
	}
	return expectedHash == actualHash, nil
}

// IsExpired checks if the checkpoint has expired
func (c *Checkpoint) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// GetProgress calculates the current progress as a percentage (0.0 - 1.0)
func (c *Checkpoint) GetProgress() float64 {
	if c.TotalChunks == 0 {
		return 0.0
	}
	return float64(len(c.CompletedChunks)) / float64(c.TotalChunks)
}

// ToSummary converts a checkpoint to a summary for listing
func (c *Checkpoint) ToSummary() *CheckpointSummary {
	return &CheckpointSummary{
		SessionID:       c.SessionID,
		SourcePath:      c.SourcePath,
		DestinationPath: c.DestinationPath,
		Direction:       c.Direction,
		Progress:        c.GetProgress(),
		TotalSize:       c.TotalSize,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
		Encrypted:       c.Encrypted,
	}
}

// MarkChunkComplete marks a chunk as completed
func (c *Checkpoint) MarkChunkComplete(chunkID int, checksum string) {
	// Check if already completed
	for _, id := range c.CompletedChunks {
		if id == chunkID {
			return
		}
	}

	c.CompletedChunks = append(c.CompletedChunks, chunkID)
	c.ChunkChecksums[chunkID] = checksum
	c.UpdatedAt = time.Now()
}

// IsChunkComplete checks if a chunk has been completed
func (c *Checkpoint) IsChunkComplete(chunkID int) bool {
	for _, id := range c.CompletedChunks {
		if id == chunkID {
			return true
		}
	}
	return false
}

package resume

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/zulfikawr/warp/internal/crypto"
)

// EncryptionStateManager handles encryption state for resumable transfers
type EncryptionStateManager struct {
	baseNonce  []byte // Initial nonce from transfer start
	chunkCount uint64 // Current chunk counter
	salt       []byte // Salt for key derivation
	keyRef     string // Reference for key re-derivation (not the key itself)
	key        []byte // Derived key (not persisted)
}

// NewEncryptionStateManager creates a new encryption state manager
func NewEncryptionStateManager() *EncryptionStateManager {
	return &EncryptionStateManager{}
}

// Initialize sets up encryption state for a new transfer
func (e *EncryptionStateManager) Initialize(pakeCode string) error {
	// Generate base nonce
	baseNonce := make([]byte, crypto.NonceSize)
	if _, err := rand.Read(baseNonce); err != nil {
		return fmt.Errorf("failed to generate base nonce: %w", err)
	}

	// Generate salt for key derivation
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key from PAKE code
	key := crypto.DeriveKey(pakeCode, salt)

	// Create key reference (hash of PAKE code for verification)
	keyRefHash := sha256.Sum256([]byte(pakeCode))
	keyRef := hex.EncodeToString(keyRefHash[:])

	e.baseNonce = baseNonce
	e.chunkCount = 0
	e.salt = salt
	e.keyRef = keyRef
	e.key = key

	return nil
}

// SaveState returns serializable encryption state (no raw keys)
func (e *EncryptionStateManager) SaveState() *EncryptionState {
	return &EncryptionState{
		BaseNonce:       e.baseNonce,
		ChunkCounter:    e.chunkCount,
		Salt:            e.salt,
		KeyDerivationID: e.keyRef,
	}
}

// RestoreState restores encryption state from checkpoint
func (e *EncryptionStateManager) RestoreState(state *EncryptionState, pakeCode string) error {
	if state == nil {
		return fmt.Errorf("encryption state is nil")
	}

	// Derive key from PAKE code and salt
	key := crypto.DeriveKey(pakeCode, state.Salt)

	// Verify key matches by comparing key reference
	keyRefHash := sha256.Sum256([]byte(pakeCode))
	keyRef := hex.EncodeToString(keyRefHash[:])

	if keyRef != state.KeyDerivationID {
		return NewEncryptionResumeError(ErrKeyMismatch, "key_mismatch")
	}

	// Restore state
	e.baseNonce = state.BaseNonce
	e.chunkCount = state.ChunkCounter
	e.salt = state.Salt
	e.keyRef = state.KeyDerivationID
	e.key = key

	return nil
}

// RestoreStateWithKey restores encryption state from checkpoint using a pre-derived key
// This is used when resuming and the key is already available (not derived from PAKE code)
func (e *EncryptionStateManager) RestoreStateWithKey(state *EncryptionState, key []byte) error {
	if state == nil {
		return fmt.Errorf("encryption state is nil")
	}

	if key == nil {
		return fmt.Errorf("encryption key is nil")
	}

	// Restore state with provided key
	e.baseNonce = state.BaseNonce
	e.chunkCount = state.ChunkCounter
	e.salt = state.Salt
	e.keyRef = state.KeyDerivationID
	e.key = key

	return nil
}

// GetNonceForChunk returns the nonce for a specific chunk index
func (e *EncryptionStateManager) GetNonceForChunk(chunkIndex uint64) ([]byte, error) {
	if len(e.baseNonce) != crypto.NonceSize {
		return nil, fmt.Errorf("base nonce not initialized")
	}

	// Create a copy of the base nonce
	nonce := make([]byte, crypto.NonceSize)
	copy(nonce, e.baseNonce)

	// Encode chunk index into last 8 bytes of nonce (big-endian)
	for i := 0; i < 8; i++ {
		nonce[crypto.NonceSize-8+i] = byte(chunkIndex >> (56 - uint(i*8)))
	}

	return nonce, nil
}

// ValidateNonceSpace checks if there's sufficient nonce space remaining
func (e *EncryptionStateManager) ValidateNonceSpace(remainingChunks uint64) error {
	// Maximum safe chunks with 12-byte nonce and 8-byte counter
	const maxSafeChunks = 1 << 32 // 4 billion chunks

	if e.chunkCount+remainingChunks > maxSafeChunks {
		return NewEncryptionResumeError(
			ErrNonceExhausted,
			"nonce_exhausted",
		)
	}

	return nil
}

// IncrementChunkCounter increments the chunk counter
func (e *EncryptionStateManager) IncrementChunkCounter() {
	e.chunkCount++
}

// GetChunkCounter returns the current chunk counter
func (e *EncryptionStateManager) GetChunkCounter() uint64 {
	return e.chunkCount
}

// GetKey returns the derived encryption key
func (e *EncryptionStateManager) GetKey() []byte {
	return e.key
}

// GetSalt returns the salt used for key derivation
func (e *EncryptionStateManager) GetSalt() []byte {
	return e.salt
}

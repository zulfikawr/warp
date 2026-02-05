package resume

import (
	"bytes"
	"testing"

	"github.com/zulfikawr/warp/internal/crypto"
)

func TestNewEncryptionStateManager(t *testing.T) {
	em := NewEncryptionStateManager()
	if em == nil {
		t.Fatal("NewEncryptionStateManager returned nil")
	}
}

func TestEncryptionStateManager_Initialize(t *testing.T) {
	em := NewEncryptionStateManager()

	pakeCode := "test-pake-code-123"
	if err := em.Initialize(pakeCode); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Check base nonce was generated
	if len(em.baseNonce) != crypto.NonceSize {
		t.Errorf("baseNonce length = %v, want %v", len(em.baseNonce), crypto.NonceSize)
	}

	// Check salt was generated
	if len(em.salt) != crypto.SaltSize {
		t.Errorf("salt length = %v, want %v", len(em.salt), crypto.SaltSize)
	}

	// Check key was derived
	if len(em.key) != crypto.KeySize {
		t.Errorf("key length = %v, want %v", len(em.key), crypto.KeySize)
	}

	// Check key reference was set
	if em.keyRef == "" {
		t.Error("keyRef should not be empty")
	}

	// Check chunk counter initialized to 0
	if em.chunkCount != 0 {
		t.Errorf("chunkCount = %v, want 0", em.chunkCount)
	}
}

func TestEncryptionStateManager_SaveState(t *testing.T) {
	em := NewEncryptionStateManager()
	pakeCode := "test-pake-code"

	if err := em.Initialize(pakeCode); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Increment counter
	em.IncrementChunkCounter()
	em.IncrementChunkCounter()

	// Save state
	state := em.SaveState()
	if state == nil {
		t.Fatal("SaveState returned nil")
	}

	// Verify state
	if !bytes.Equal(state.BaseNonce, em.baseNonce) {
		t.Error("saved BaseNonce doesn't match")
	}
	if state.ChunkCounter != 2 {
		t.Errorf("saved ChunkCounter = %v, want 2", state.ChunkCounter)
	}
	if !bytes.Equal(state.Salt, em.salt) {
		t.Error("saved Salt doesn't match")
	}
	if state.KeyDerivationID != em.keyRef {
		t.Error("saved KeyDerivationID doesn't match")
	}
}

func TestEncryptionStateManager_RestoreState(t *testing.T) {
	// Create and initialize first manager
	em1 := NewEncryptionStateManager()
	pakeCode := "test-pake-code-456"

	if err := em1.Initialize(pakeCode); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	em1.IncrementChunkCounter()
	em1.IncrementChunkCounter()
	em1.IncrementChunkCounter()

	// Save state
	state := em1.SaveState()

	// Create new manager and restore
	em2 := NewEncryptionStateManager()
	if err := em2.RestoreState(state, pakeCode); err != nil {
		t.Fatalf("RestoreState failed: %v", err)
	}

	// Verify restored state
	if !bytes.Equal(em2.baseNonce, em1.baseNonce) {
		t.Error("restored baseNonce doesn't match")
	}
	if em2.chunkCount != em1.chunkCount {
		t.Errorf("restored chunkCount = %v, want %v", em2.chunkCount, em1.chunkCount)
	}
	if !bytes.Equal(em2.salt, em1.salt) {
		t.Error("restored salt doesn't match")
	}
	if em2.keyRef != em1.keyRef {
		t.Error("restored keyRef doesn't match")
	}

	// Verify keys match (key derivation is deterministic)
	if !bytes.Equal(em2.key, em1.key) {
		t.Error("restored key doesn't match (key derivation not deterministic)")
	}
}

func TestEncryptionStateManager_RestoreState_WrongPakeCode(t *testing.T) {
	// Create and initialize
	em1 := NewEncryptionStateManager()
	if err := em1.Initialize("correct-code"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	state := em1.SaveState()

	// Try to restore with wrong PAKE code
	em2 := NewEncryptionStateManager()
	if err := em2.RestoreState(state, "wrong-code"); err == nil {
		t.Error("RestoreState should fail with wrong PAKE code")
	}
}

func TestEncryptionStateManager_RestoreState_NilState(t *testing.T) {
	em := NewEncryptionStateManager()
	if err := em.RestoreState(nil, "test-code"); err == nil {
		t.Error("RestoreState should fail with nil state")
	}
}

func TestEncryptionStateManager_GetNonceForChunk(t *testing.T) {
	em := NewEncryptionStateManager()
	if err := em.Initialize("test-code"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Get nonce for chunk 0
	nonce0, err := em.GetNonceForChunk(0)
	if err != nil {
		t.Fatalf("GetNonceForChunk(0) failed: %v", err)
	}
	if len(nonce0) != crypto.NonceSize {
		t.Errorf("nonce length = %v, want %v", len(nonce0), crypto.NonceSize)
	}

	// Get nonce for chunk 1
	nonce1, err := em.GetNonceForChunk(1)
	if err != nil {
		t.Fatalf("GetNonceForChunk(1) failed: %v", err)
	}

	// Nonces should be different
	if bytes.Equal(nonce0, nonce1) {
		t.Error("nonces for different chunks should be different")
	}

	// Get nonce for chunk 0 again (should be same)
	nonce0Again, err := em.GetNonceForChunk(0)
	if err != nil {
		t.Fatalf("GetNonceForChunk(0) again failed: %v", err)
	}
	if !bytes.Equal(nonce0, nonce0Again) {
		t.Error("nonce for same chunk should be deterministic")
	}
}

func TestEncryptionStateManager_GetNonceForChunk_NotInitialized(t *testing.T) {
	em := NewEncryptionStateManager()
	// Don't initialize

	_, err := em.GetNonceForChunk(0)
	if err == nil {
		t.Error("GetNonceForChunk should fail when not initialized")
	}
}

func TestEncryptionStateManager_ValidateNonceSpace(t *testing.T) {
	em := NewEncryptionStateManager()
	if err := em.Initialize("test-code"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Should pass with reasonable chunk count
	if err := em.ValidateNonceSpace(1000); err != nil {
		t.Errorf("ValidateNonceSpace(1000) failed: %v", err)
	}

	// Should pass with large but safe chunk count
	if err := em.ValidateNonceSpace(1000000); err != nil {
		t.Errorf("ValidateNonceSpace(1000000) failed: %v", err)
	}

	// Should fail with excessive chunk count (> 4 billion)
	if err := em.ValidateNonceSpace(1 << 33); err == nil {
		t.Error("ValidateNonceSpace should fail with excessive chunk count")
	}
}

func TestEncryptionStateManager_IncrementChunkCounter(t *testing.T) {
	em := NewEncryptionStateManager()
	if err := em.Initialize("test-code"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Initial counter should be 0
	if counter := em.GetChunkCounter(); counter != 0 {
		t.Errorf("initial counter = %v, want 0", counter)
	}

	// Increment
	em.IncrementChunkCounter()
	if counter := em.GetChunkCounter(); counter != 1 {
		t.Errorf("counter after increment = %v, want 1", counter)
	}

	// Increment again
	em.IncrementChunkCounter()
	if counter := em.GetChunkCounter(); counter != 2 {
		t.Errorf("counter after 2 increments = %v, want 2", counter)
	}
}

func TestEncryptionStateManager_GetKey(t *testing.T) {
	em := NewEncryptionStateManager()
	pakeCode := "test-pake-code"

	if err := em.Initialize(pakeCode); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	key := em.GetKey()
	if len(key) != crypto.KeySize {
		t.Errorf("key length = %v, want %v", len(key), crypto.KeySize)
	}

	// Key should be deterministic for same PAKE code and salt
	expectedKey := crypto.DeriveKey(pakeCode, em.GetSalt())
	if !bytes.Equal(key, expectedKey) {
		t.Error("key doesn't match expected derived key")
	}
}

func TestEncryptionStateManager_GetSalt(t *testing.T) {
	em := NewEncryptionStateManager()
	if err := em.Initialize("test-code"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	salt := em.GetSalt()
	if len(salt) != crypto.SaltSize {
		t.Errorf("salt length = %v, want %v", len(salt), crypto.SaltSize)
	}
}

func TestEncryptionStateManager_KeyDerivationDeterminism(t *testing.T) {
	pakeCode := "determinism-test-code"

	// Create first manager
	em1 := NewEncryptionStateManager()
	if err := em1.Initialize(pakeCode); err != nil {
		t.Fatalf("Initialize em1 failed: %v", err)
	}
	state1 := em1.SaveState()

	// Create second manager and restore with same PAKE code
	em2 := NewEncryptionStateManager()
	if err := em2.RestoreState(state1, pakeCode); err != nil {
		t.Fatalf("RestoreState failed: %v", err)
	}

	// Keys should match
	if !bytes.Equal(em1.GetKey(), em2.GetKey()) {
		t.Error("key derivation is not deterministic")
	}

	// Create third manager with same salt but different PAKE code should fail
	em3 := NewEncryptionStateManager()
	if err := em3.RestoreState(state1, "different-code"); err == nil {
		t.Error("RestoreState should fail with different PAKE code")
	}
}

func TestEncryptionStateManager_NonceUniqueness(t *testing.T) {
	em := NewEncryptionStateManager()
	if err := em.Initialize("test-code"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Generate nonces for multiple chunks
	seen := make(map[string]bool)
	for i := uint64(0); i < 100; i++ {
		nonce, err := em.GetNonceForChunk(i)
		if err != nil {
			t.Fatalf("GetNonceForChunk(%d) failed: %v", i, err)
		}

		nonceStr := string(nonce)
		if seen[nonceStr] {
			t.Errorf("duplicate nonce detected for chunk %d", i)
		}
		seen[nonceStr] = true
	}
}

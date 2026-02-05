package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// mockChunkReader for testing
type mockChunkReader struct {
	chunks map[int][]byte
}

func (m *mockChunkReader) ReadChunk(chunkID int) ([]byte, error) {
	data, ok := m.chunks[chunkID]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestNewIntegrityVerifier(t *testing.T) {
	v := NewIntegrityVerifier()
	if v == nil {
		t.Fatal("NewIntegrityVerifier returned nil")
	}
	if v.algorithm != "sha256" {
		t.Errorf("algorithm = %v, want sha256", v.algorithm)
	}
	if v.GetChunkCount() != 0 {
		t.Errorf("initial chunk count = %v, want 0", v.GetChunkCount())
	}
}

func TestIntegrityVerifier_RecordAndGetChunkHash(t *testing.T) {
	v := NewIntegrityVerifier()

	// Record hash
	v.RecordChunkHash(0, "hash-0")
	v.RecordChunkHash(1, "hash-1")

	// Get hash
	hash, exists := v.GetChunkHash(0)
	if !exists {
		t.Error("chunk 0 hash should exist")
	}
	if hash != "hash-0" {
		t.Errorf("chunk 0 hash = %v, want hash-0", hash)
	}

	// Non-existent chunk
	_, exists = v.GetChunkHash(99)
	if exists {
		t.Error("chunk 99 should not exist")
	}

	// Check count
	if count := v.GetChunkCount(); count != 2 {
		t.Errorf("chunk count = %v, want 2", count)
	}
}

func TestIntegrityVerifier_VerifyChunk(t *testing.T) {
	v := NewIntegrityVerifier()

	// Test data
	data := []byte("test chunk data")
	hash := sha256.Sum256(data)
	expectedHash := hex.EncodeToString(hash[:])

	// Record hash
	v.RecordChunkHash(0, expectedHash)

	// Verify with correct data
	if err := v.VerifyChunk(0, data); err != nil {
		t.Errorf("VerifyChunk failed: %v", err)
	}

	// Verify with incorrect data
	wrongData := []byte("wrong data")
	if err := v.VerifyChunk(0, wrongData); err == nil {
		t.Error("VerifyChunk should fail with wrong data")
	}

	// Verify non-existent chunk
	if err := v.VerifyChunk(99, data); err == nil {
		t.Error("VerifyChunk should fail for non-existent chunk")
	}
}

func TestIntegrityVerifier_VerifyAllChunks(t *testing.T) {
	v := NewIntegrityVerifier()

	// Create mock chunks
	chunks := map[int][]byte{
		0: []byte("chunk 0 data"),
		1: []byte("chunk 1 data"),
		2: []byte("chunk 2 data"),
	}

	reader := &mockChunkReader{chunks: chunks}

	// Record hashes
	for id, data := range chunks {
		hash := sha256.Sum256(data)
		v.RecordChunkHash(id, hex.EncodeToString(hash[:]))
	}

	// Verify all chunks (should pass)
	corrupted, err := v.VerifyAllChunks(reader)
	if err != nil {
		t.Fatalf("VerifyAllChunks failed: %v", err)
	}
	if len(corrupted) != 0 {
		t.Errorf("corrupted chunks = %v, want empty", corrupted)
	}

	// Corrupt one chunk
	chunks[1] = []byte("corrupted data")

	// Verify again (should detect corruption)
	corrupted, err = v.VerifyAllChunks(reader)
	if err != nil {
		t.Fatalf("VerifyAllChunks failed: %v", err)
	}
	if len(corrupted) != 1 {
		t.Errorf("corrupted chunks count = %v, want 1", len(corrupted))
	}
	if len(corrupted) > 0 && corrupted[0] != 1 {
		t.Errorf("corrupted chunk = %v, want 1", corrupted[0])
	}
}

func TestIntegrityVerifier_VerifyFile(t *testing.T) {
	v := NewIntegrityVerifier()

	// Create temp file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("test file content")

	if err := os.WriteFile(filePath, content, 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Compute expected hash
	hash := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(hash[:])

	// Set expected hash
	v.SetExpectedFileHash(expectedHash)

	// Verify file (should pass)
	if err := v.VerifyFile(filePath); err != nil {
		t.Errorf("VerifyFile failed: %v", err)
	}

	// Set wrong hash
	v.SetExpectedFileHash("wrong-hash")

	// Verify file (should fail)
	if err := v.VerifyFile(filePath); err == nil {
		t.Error("VerifyFile should fail with wrong hash")
	}
}

func TestIntegrityVerifier_GetCorruptedChunks(t *testing.T) {
	v := NewIntegrityVerifier()

	// Initially empty
	if corrupted := v.GetCorruptedChunks(); len(corrupted) != 0 {
		t.Errorf("initial corrupted chunks = %v, want empty", corrupted)
	}

	// Record and verify corrupted chunk
	v.RecordChunkHash(0, "expected-hash")
	_ = v.VerifyChunk(0, []byte("wrong data"))

	corrupted := v.GetCorruptedChunks()
	if len(corrupted) != 1 {
		t.Errorf("corrupted chunks count = %v, want 1", len(corrupted))
	}

	// Clear corrupted chunks
	v.ClearCorruptedChunks()
	if corrupted := v.GetCorruptedChunks(); len(corrupted) != 0 {
		t.Errorf("corrupted chunks after clear = %v, want empty", corrupted)
	}
}

func TestIntegrityVerifier_ExportImportState(t *testing.T) {
	v := NewIntegrityVerifier()

	// Set up state
	v.RecordChunkHash(0, "hash-0")
	v.RecordChunkHash(1, "hash-1")
	v.SetExpectedFileHash("file-hash")

	// Export state
	state := v.ExportState()
	if state == nil {
		t.Fatal("ExportState returned nil")
	}
	if state.Algorithm != "sha256" {
		t.Errorf("exported algorithm = %v, want sha256", state.Algorithm)
	}
	if len(state.ChunkHashes) != 2 {
		t.Errorf("exported chunk hashes count = %v, want 2", len(state.ChunkHashes))
	}
	if state.ExpectedFileHash != "file-hash" {
		t.Errorf("exported file hash = %v, want file-hash", state.ExpectedFileHash)
	}

	// Create new verifier and import state
	v2 := NewIntegrityVerifier()
	if err := v2.ImportState(state); err != nil {
		t.Fatalf("ImportState failed: %v", err)
	}

	// Verify imported state
	hash, exists := v2.GetChunkHash(0)
	if !exists || hash != "hash-0" {
		t.Errorf("imported chunk 0 hash = %v, want hash-0", hash)
	}

	state2 := v2.ExportState()
	if state2.ExpectedFileHash != "file-hash" {
		t.Errorf("imported file hash = %v, want file-hash", state2.ExpectedFileHash)
	}
}

func TestIntegrityVerifier_ImportNilState(t *testing.T) {
	v := NewIntegrityVerifier()
	if err := v.ImportState(nil); err == nil {
		t.Error("ImportState should fail with nil state")
	}
}

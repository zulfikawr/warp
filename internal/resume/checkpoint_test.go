package resume

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNewCheckpoint(t *testing.T) {
	opts := CheckpointOptions{
		SessionID:       "test-session-123",
		SourcePath:      "/path/to/source.txt",
		DestinationPath: "/path/to/dest.txt",
		Direction:       "upload",
		TotalSize:       1024 * 1024 * 100, // 100MB
		ChunkSize:       2 * 1024 * 1024,   // 2MB
		TotalChunks:     50,
		Encrypted:       true,
		ExpiresIn:       24 * time.Hour,
	}

	cp := NewCheckpoint(opts)

	if cp.SessionID != opts.SessionID {
		t.Errorf("SessionID = %v, want %v", cp.SessionID, opts.SessionID)
	}
	if cp.Version != CheckpointFormatVersion {
		t.Errorf("Version = %v, want %v", cp.Version, CheckpointFormatVersion)
	}
	if cp.SourcePath != opts.SourcePath {
		t.Errorf("SourcePath = %v, want %v", cp.SourcePath, opts.SourcePath)
	}
	if cp.Direction != opts.Direction {
		t.Errorf("Direction = %v, want %v", cp.Direction, opts.Direction)
	}
	if cp.TotalSize != opts.TotalSize {
		t.Errorf("TotalSize = %v, want %v", cp.TotalSize, opts.TotalSize)
	}
	if cp.Encrypted != opts.Encrypted {
		t.Errorf("Encrypted = %v, want %v", cp.Encrypted, opts.Encrypted)
	}
	if len(cp.CompletedChunks) != 0 {
		t.Errorf("CompletedChunks should be empty, got %v", cp.CompletedChunks)
	}
	if len(cp.ChunkChecksums) != 0 {
		t.Errorf("ChunkChecksums should be empty, got %v", cp.ChunkChecksums)
	}
}

func TestCheckpoint_ComputeHash(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID:       "test-session",
		SourcePath:      "/source",
		DestinationPath: "/dest",
		Direction:       "upload",
		TotalSize:       1000,
		ChunkSize:       100,
		TotalChunks:     10,
	})

	hash1, err := cp.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash1 == "" {
		t.Error("ComputeHash returned empty string")
	}

	// Same checkpoint should produce same hash
	hash2, err := cp.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("ComputeHash not deterministic: %v != %v", hash1, hash2)
	}

	// Modifying checkpoint should change hash
	cp.MarkChunkComplete(0, "checksum-0")
	hash3, err := cp.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash failed: %v", err)
	}
	if hash1 == hash3 {
		t.Error("ComputeHash should change when checkpoint is modified")
	}
}

func TestCheckpoint_UpdateHash(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID:  "test-session",
		SourcePath: "/source",
		TotalSize:  1000,
		ChunkSize:  100,
	})

	if err := cp.UpdateHash(); err != nil {
		t.Fatalf("UpdateHash failed: %v", err)
	}

	if cp.CheckpointHash == "" {
		t.Error("CheckpointHash should be set after UpdateHash")
	}
}

func TestCheckpoint_VerifyHash(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID:  "test-session",
		SourcePath: "/source",
		TotalSize:  1000,
	})

	// Update hash
	if err := cp.UpdateHash(); err != nil {
		t.Fatalf("UpdateHash failed: %v", err)
	}

	// Verify should pass
	valid, err := cp.VerifyHash()
	if err != nil {
		t.Fatalf("VerifyHash failed: %v", err)
	}
	if !valid {
		t.Error("VerifyHash should return true for unmodified checkpoint")
	}

	// Tamper with checkpoint
	originalHash := cp.CheckpointHash
	cp.SourcePath = "/tampered"
	cp.CheckpointHash = originalHash // Keep old hash

	// Verify should fail
	valid, err = cp.VerifyHash()
	if err != nil {
		t.Fatalf("VerifyHash failed: %v", err)
	}
	if valid {
		t.Error("VerifyHash should return false for tampered checkpoint")
	}
}

func TestCheckpoint_IsExpired(t *testing.T) {
	// Create expired checkpoint
	expiredCP := NewCheckpoint(CheckpointOptions{
		SessionID: "expired",
		ExpiresIn: -1 * time.Hour, // Expired 1 hour ago
	})
	if !expiredCP.IsExpired() {
		t.Error("IsExpired should return true for expired checkpoint")
	}

	// Create valid checkpoint
	validCP := NewCheckpoint(CheckpointOptions{
		SessionID: "valid",
		ExpiresIn: 1 * time.Hour, // Expires in 1 hour
	})
	if validCP.IsExpired() {
		t.Error("IsExpired should return false for valid checkpoint")
	}
}

func TestCheckpoint_GetProgress(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID:   "test",
		TotalChunks: 10,
	})

	// No chunks completed
	if progress := cp.GetProgress(); progress != 0.0 {
		t.Errorf("GetProgress = %v, want 0.0", progress)
	}

	// 5 chunks completed
	for i := 0; i < 5; i++ {
		cp.MarkChunkComplete(i, "checksum")
	}
	if progress := cp.GetProgress(); progress != 0.5 {
		t.Errorf("GetProgress = %v, want 0.5", progress)
	}

	// All chunks completed
	for i := 5; i < 10; i++ {
		cp.MarkChunkComplete(i, "checksum")
	}
	if progress := cp.GetProgress(); progress != 1.0 {
		t.Errorf("GetProgress = %v, want 1.0", progress)
	}
}

func TestCheckpoint_MarkChunkComplete(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID:   "test",
		TotalChunks: 10,
	})

	// Mark chunk 0 complete
	cp.MarkChunkComplete(0, "checksum-0")
	if len(cp.CompletedChunks) != 1 {
		t.Errorf("CompletedChunks length = %v, want 1", len(cp.CompletedChunks))
	}
	if cp.ChunkChecksums[0] != "checksum-0" {
		t.Errorf("ChunkChecksums[0] = %v, want checksum-0", cp.ChunkChecksums[0])
	}

	// Mark same chunk again (should be idempotent)
	cp.MarkChunkComplete(0, "checksum-0")
	if len(cp.CompletedChunks) != 1 {
		t.Errorf("CompletedChunks length = %v, want 1 (idempotent)", len(cp.CompletedChunks))
	}

	// Mark another chunk
	cp.MarkChunkComplete(5, "checksum-5")
	if len(cp.CompletedChunks) != 2 {
		t.Errorf("CompletedChunks length = %v, want 2", len(cp.CompletedChunks))
	}
}

func TestCheckpoint_IsChunkComplete(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID:   "test",
		TotalChunks: 10,
	})

	// Chunk not completed
	if cp.IsChunkComplete(0) {
		t.Error("IsChunkComplete(0) should return false")
	}

	// Mark chunk complete
	cp.MarkChunkComplete(0, "checksum")
	if !cp.IsChunkComplete(0) {
		t.Error("IsChunkComplete(0) should return true after marking complete")
	}

	// Other chunks still not complete
	if cp.IsChunkComplete(1) {
		t.Error("IsChunkComplete(1) should return false")
	}
}

func TestCheckpoint_ToSummary(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID:       "test-session",
		SourcePath:      "/source",
		DestinationPath: "/dest",
		Direction:       "upload",
		TotalSize:       1000,
		TotalChunks:     10,
		Encrypted:       true,
	})

	// Mark some chunks complete
	cp.MarkChunkComplete(0, "checksum-0")
	cp.MarkChunkComplete(1, "checksum-1")

	summary := cp.ToSummary()

	if summary.SessionID != cp.SessionID {
		t.Errorf("Summary.SessionID = %v, want %v", summary.SessionID, cp.SessionID)
	}
	if summary.Progress != 0.2 {
		t.Errorf("Summary.Progress = %v, want 0.2", summary.Progress)
	}
	if summary.Encrypted != cp.Encrypted {
		t.Errorf("Summary.Encrypted = %v, want %v", summary.Encrypted, cp.Encrypted)
	}
}

// TestCheckpointRoundTrip tests JSON serialization round-trip
func TestCheckpointRoundTrip(t *testing.T) {
	original := NewCheckpoint(CheckpointOptions{
		SessionID:       "test-session-123",
		SourcePath:      "/path/to/source.txt",
		DestinationPath: "/path/to/dest.txt",
		Direction:       "upload",
		TotalSize:       1024 * 1024 * 100,
		ChunkSize:       2 * 1024 * 1024,
		TotalChunks:     50,
		Encrypted:       true,
		ExpiresIn:       24 * time.Hour,
	})

	// Add some state
	original.MarkChunkComplete(0, "checksum-0")
	original.MarkChunkComplete(1, "checksum-1")
	original.EncryptionState = &EncryptionState{
		BaseNonce:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		ChunkCounter:    2,
		Salt:            []byte{1, 2, 3, 4},
		KeyDerivationID: "test-key-id",
	}
	if err := original.UpdateHash(); err != nil {
		t.Fatalf("UpdateHash failed: %v", err)
	}

	// Serialize
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Deserialize
	var restored Checkpoint
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Compare (excluding computed fields like UpdatedAt which may differ slightly)
	if restored.SessionID != original.SessionID {
		t.Errorf("SessionID mismatch: %v != %v", restored.SessionID, original.SessionID)
	}
	if restored.Version != original.Version {
		t.Errorf("Version mismatch: %v != %v", restored.Version, original.Version)
	}
	if restored.SourcePath != original.SourcePath {
		t.Errorf("SourcePath mismatch: %v != %v", restored.SourcePath, original.SourcePath)
	}
	if restored.TotalSize != original.TotalSize {
		t.Errorf("TotalSize mismatch: %v != %v", restored.TotalSize, original.TotalSize)
	}
	if !reflect.DeepEqual(restored.CompletedChunks, original.CompletedChunks) {
		t.Errorf("CompletedChunks mismatch: %v != %v", restored.CompletedChunks, original.CompletedChunks)
	}
	if !reflect.DeepEqual(restored.ChunkChecksums, original.ChunkChecksums) {
		t.Errorf("ChunkChecksums mismatch: %v != %v", restored.ChunkChecksums, original.ChunkChecksums)
	}
	if restored.Encrypted != original.Encrypted {
		t.Errorf("Encrypted mismatch: %v != %v", restored.Encrypted, original.Encrypted)
	}

	// Verify encryption state
	if restored.EncryptionState == nil {
		t.Fatal("EncryptionState is nil after deserialization")
	}
	if !reflect.DeepEqual(restored.EncryptionState.BaseNonce, original.EncryptionState.BaseNonce) {
		t.Errorf("BaseNonce mismatch")
	}
	if restored.EncryptionState.ChunkCounter != original.EncryptionState.ChunkCounter {
		t.Errorf("ChunkCounter mismatch: %v != %v", restored.EncryptionState.ChunkCounter, original.EncryptionState.ChunkCounter)
	}

	// Verify hash
	valid, err := restored.VerifyHash()
	if err != nil {
		t.Fatalf("VerifyHash failed: %v", err)
	}
	if !valid {
		t.Error("Hash verification failed after round-trip")
	}
}

func TestCheckpointOptions_DefaultExpiry(t *testing.T) {
	cp := NewCheckpoint(CheckpointOptions{
		SessionID: "test",
		ExpiresIn: 0, // Should default to 24 hours
	})

	expectedExpiry := time.Now().Add(24 * time.Hour)
	diff := cp.ExpiresAt.Sub(expectedExpiry)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("Default expiry not set correctly, got %v, want ~%v", cp.ExpiresAt, expectedExpiry)
	}
}

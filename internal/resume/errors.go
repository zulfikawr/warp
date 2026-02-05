package resume

import (
	"errors"
	"fmt"
)

// Common errors
var (
	// ErrCheckpointNotFound indicates a checkpoint file was not found
	ErrCheckpointNotFound = errors.New("checkpoint not found")

	// ErrCheckpointCorrupted indicates a checkpoint file is corrupted or tampered with
	ErrCheckpointCorrupted = errors.New("checkpoint corrupted or tampered with")

	// ErrCheckpointExpired indicates a checkpoint has expired
	ErrCheckpointExpired = errors.New("checkpoint expired")

	// ErrInvalidCheckpointVersion indicates an unsupported checkpoint version
	ErrInvalidCheckpointVersion = errors.New("invalid checkpoint version")

	// ErrNonceExhausted indicates the nonce space has been exhausted
	ErrNonceExhausted = errors.New("nonce space exhausted")

	// ErrKeyMismatch indicates encryption key derivation mismatch
	ErrKeyMismatch = errors.New("encryption key mismatch")

	// ErrMaxRetriesExceeded indicates maximum retry attempts exceeded
	ErrMaxRetriesExceeded = errors.New("maximum retry attempts exceeded")
)

// ResumableError indicates the transfer can be resumed
type ResumableError struct {
	Err         error
	SessionID   string
	LastChunk   int
	Recoverable bool
}

// Error implements the error interface
func (e *ResumableError) Error() string {
	return fmt.Sprintf("resumable error (session=%s, last_chunk=%d, recoverable=%v): %v",
		e.SessionID, e.LastChunk, e.Recoverable, e.Err)
}

// Unwrap returns the underlying error
func (e *ResumableError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target
func (e *ResumableError) Is(target error) bool {
	_, ok := target.(*ResumableError)
	return ok
}

// CheckpointError indicates a checkpoint-related failure
type CheckpointError struct {
	Err       error
	SessionID string
	Operation string // "create", "load", "update", "delete"
}

// Error implements the error interface
func (e *CheckpointError) Error() string {
	return fmt.Sprintf("checkpoint error (session=%s, operation=%s): %v",
		e.SessionID, e.Operation, e.Err)
}

// Unwrap returns the underlying error
func (e *CheckpointError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target
func (e *CheckpointError) Is(target error) bool {
	_, ok := target.(*CheckpointError)
	return ok
}

// IntegrityError indicates data corruption
type IntegrityError struct {
	Err          error
	ChunkID      int
	ExpectedHash string
	ActualHash   string
}

// Error implements the error interface
func (e *IntegrityError) Error() string {
	return fmt.Sprintf("integrity error (chunk=%d, expected=%s, actual=%s): %v",
		e.ChunkID, e.ExpectedHash, e.ActualHash, e.Err)
}

// Unwrap returns the underlying error
func (e *IntegrityError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target
func (e *IntegrityError) Is(target error) bool {
	_, ok := target.(*IntegrityError)
	return ok
}

// EncryptionResumeError indicates encryption state cannot be restored
type EncryptionResumeError struct {
	Err    error
	Reason string // "nonce_exhausted", "key_mismatch", "state_corrupted"
}

// Error implements the error interface
func (e *EncryptionResumeError) Error() string {
	return fmt.Sprintf("encryption resume error (reason=%s): %v", e.Reason, e.Err)
}

// Unwrap returns the underlying error
func (e *EncryptionResumeError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target
func (e *EncryptionResumeError) Is(target error) bool {
	_, ok := target.(*EncryptionResumeError)
	return ok
}

// NewResumableError creates a new ResumableError
func NewResumableError(err error, sessionID string, lastChunk int, recoverable bool) *ResumableError {
	return &ResumableError{
		Err:         err,
		SessionID:   sessionID,
		LastChunk:   lastChunk,
		Recoverable: recoverable,
	}
}

// NewCheckpointError creates a new CheckpointError
func NewCheckpointError(err error, sessionID, operation string) *CheckpointError {
	return &CheckpointError{
		Err:       err,
		SessionID: sessionID,
		Operation: operation,
	}
}

// NewIntegrityError creates a new IntegrityError
func NewIntegrityError(err error, chunkID int, expectedHash, actualHash string) *IntegrityError {
	return &IntegrityError{
		Err:          err,
		ChunkID:      chunkID,
		ExpectedHash: expectedHash,
		ActualHash:   actualHash,
	}
}

// NewEncryptionResumeError creates a new EncryptionResumeError
func NewEncryptionResumeError(err error, reason string) *EncryptionResumeError {
	return &EncryptionResumeError{
		Err:    err,
		Reason: reason,
	}
}

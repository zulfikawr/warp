package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// KeySize is the size of the encryption key in bytes (AES-256)
	KeySize = 32
	// SaltSize is the size of the salt for key derivation
	SaltSize = 32
	// NonceSize is the size of the GCM nonce
	NonceSize = 12
	// PBKDF2Iterations is the number of iterations for key derivation
	PBKDF2Iterations = 100000
)

// DeriveKey derives an encryption key from a password using PBKDF2
func DeriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, PBKDF2Iterations, KeySize, sha256.New)
}

// GenerateSalt generates a random salt for key derivation
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the provided key
// Returns: nonce + ciphertext
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext and tag to nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with the provided key
// Expects: nonce + ciphertext format
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < NonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:NonceSize], ciphertext[NonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// EncryptReader wraps an io.Reader to encrypt data on-the-fly
type EncryptReader struct {
	reader     io.Reader
	gcm        cipher.AEAD
	nonce      []byte
	buffer     []byte
	offset     int
	chunkCount uint64
	maxChunks  uint64 // Safety limit to prevent nonce reuse
}

// NewEncryptReader creates a new encrypting reader
func NewEncryptReader(reader io.Reader, key []byte) (*EncryptReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	return &EncryptReader{
		reader:     reader,
		gcm:        gcm,
		nonce:      nonce,
		buffer:     nonce, // First read returns the nonce
		offset:     0,
		chunkCount: 0,
		maxChunks:  1 << 32, // 4 billion chunks max (safe for 12-byte nonce)
	}, nil
}

// Read implements io.Reader, encrypting data from the underlying reader
func (er *EncryptReader) Read(p []byte) (int, error) {
	// First, drain any buffered data (starting with nonce)
	if er.offset < len(er.buffer) {
		n := copy(p, er.buffer[er.offset:])
		er.offset += n
		if er.offset >= len(er.buffer) {
			er.buffer = nil
			er.offset = 0
		}
		return n, nil
	}

	// Check nonce space exhaustion
	if er.chunkCount >= er.maxChunks {
		return 0, fmt.Errorf("encryption limit reached: maximum chunks exceeded (processed %d chunks)", er.chunkCount)
	}

	// Read a chunk from the underlying reader
	chunk := make([]byte, 64*1024) // 64KB chunks
	n, err := er.reader.Read(chunk)
	if n > 0 {
		// Use deterministic nonce construction: first 4 bytes stay as random, last 8 bytes are counter
		// This provides 2^64 unique nonces with the same base nonce
		if len(er.nonce) == NonceSize {
			// Store counter in last 8 bytes (big-endian)
			for i := 0; i < 8; i++ {
				er.nonce[NonceSize-8+i] = byte(er.chunkCount >> (56 - uint(i*8)))
			}
		}
		er.chunkCount++

		// Encrypt the chunk
		encrypted := er.gcm.Seal(nil, er.nonce, chunk[:n], nil)
		er.buffer = encrypted
		er.offset = 0

		// Copy as much as possible to output
		copied := copy(p, er.buffer)
		er.offset = copied
		return copied, nil
	}

	if err != nil {
		return 0, err
	}

	return 0, io.EOF
}

// DecryptReader wraps an io.Reader to decrypt data on-the-fly
type DecryptReader struct {
	reader     io.Reader
	gcm        cipher.AEAD
	nonce      []byte
	buffer     []byte
	offset     int
	first      bool
	chunkCount uint64
	maxChunks  uint64
}

// NewDecryptReader creates a new decrypting reader
func NewDecryptReader(reader io.Reader, key []byte) (*DecryptReader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &DecryptReader{
		reader:     reader,
		gcm:        gcm,
		first:      true,
		chunkCount: 0,
		maxChunks:  1 << 32,
	}, nil
}

// Read implements io.Reader, decrypting data from the underlying reader
func (dr *DecryptReader) Read(p []byte) (int, error) {
	// First read extracts the nonce
	if dr.first {
		dr.nonce = make([]byte, NonceSize)
		if _, err := io.ReadFull(dr.reader, dr.nonce); err != nil {
			return 0, fmt.Errorf("failed to read nonce: %w", err)
		}
		dr.first = false
	}

	// Drain buffered plaintext
	if dr.offset < len(dr.buffer) {
		n := copy(p, dr.buffer[dr.offset:])
		dr.offset += n
		if dr.offset >= len(dr.buffer) {
			dr.buffer = nil
			dr.offset = 0
		}
		return n, nil
	}

	// Check nonce space exhaustion
	if dr.chunkCount >= dr.maxChunks {
		return 0, fmt.Errorf("decryption limit reached: maximum chunks exceeded (processed %d chunks)", dr.chunkCount)
	}

	// Read encrypted chunk
	// Size = 64KB plaintext + 16 bytes tag
	chunk := make([]byte, 64*1024+dr.gcm.Overhead())
	n, err := dr.reader.Read(chunk)
	if n > 0 {
		// Reconstruct nonce using same deterministic scheme as encryption
		if len(dr.nonce) == NonceSize {
			for i := 0; i < 8; i++ {
				dr.nonce[NonceSize-8+i] = byte(dr.chunkCount >> (56 - uint(i*8)))
			}
		}
		dr.chunkCount++

		// Decrypt the chunk
		plaintext, decErr := dr.gcm.Open(nil, dr.nonce, chunk[:n], nil)
		if decErr != nil {
			return 0, fmt.Errorf("decryption failed: %w", decErr)
		}

		dr.buffer = plaintext
		dr.offset = 0

		// Copy to output
		copied := copy(p, dr.buffer)
		dr.offset = copied
		return copied, nil
	}

	if err != nil {
		return 0, err
	}

	return 0, io.EOF
}

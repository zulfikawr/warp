package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

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
	// State for sending framed chunks
	phase     int    // 0: sending length prefix, 1: sending encrypted data
	lengthBuf []byte // 4-byte length prefix
	lengthOff int    // Current offset in length prefix
	chunkData []byte // Encrypted chunk data
	chunkOff  int    // Current offset in chunk data
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
		phase:      -1,      // -1: sending nonce, 0: sending length, 1: sending data
	}, nil
}

// Read implements io.Reader, encrypting data from the underlying reader
func (er *EncryptReader) Read(p []byte) (int, error) {
	if os.Getenv("WARP_DEBUG") != "" && er.chunkCount == 0 && er.phase == -1 {
		fmt.Fprintf(os.Stderr, "[EncryptReader] First Read() call with buffer size %d\n", len(p))
	}
	// Phase -1: Send the nonce first
	if er.phase == -1 {
		if er.offset < len(er.buffer) {
			n := copy(p, er.buffer[er.offset:])
			er.offset += n
			if os.Getenv("WARP_DEBUG") != "" && n > 0 {
				fmt.Fprintf(os.Stderr, "[EncryptReader] Sending nonce: %d bytes, first 4 hex: %x\n", n, er.buffer[er.offset-n:er.offset])
			}
			if er.offset >= len(er.buffer) {
				er.buffer = nil
				er.offset = 0
				er.phase = 0 // Move to length sending phase
				er.lengthOff = 0
			}
			return n, nil
		}
		er.phase = 0
	}

	// Check nonce space exhaustion
	if er.chunkCount >= er.maxChunks {
		return 0, fmt.Errorf("encryption limit reached: maximum chunks exceeded (processed %d chunks)", er.chunkCount)
	}

	// Phase 0: Send length prefix bytes
	if er.phase == 0 && er.lengthBuf != nil && er.lengthOff < len(er.lengthBuf) {
		n := copy(p, er.lengthBuf[er.lengthOff:])
		er.lengthOff += n
		if os.Getenv("WARP_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[EncryptReader] Sending length prefix: %x (bytes %d-%d of 4)\n", er.lengthBuf, er.lengthOff-n, er.lengthOff)
		}
		if er.lengthOff >= len(er.lengthBuf) {
			er.phase = 1 // Move to chunk data phase
			er.chunkOff = 0
		}
		return n, nil
	}

	// Phase 1: Send encrypted chunk data
	if er.phase == 1 && er.chunkData != nil && er.chunkOff < len(er.chunkData) {
		n := copy(p, er.chunkData[er.chunkOff:])
		er.chunkOff += n
		if er.chunkOff >= len(er.chunkData) {
			// Done with this chunk, prepare for next
			er.lengthBuf = nil
			er.chunkData = nil
			er.phase = 0 // Back to reading a new chunk
		}
		return n, nil
	}

	// Phase 0: Read a new chunk if needed (only if we've finished previous chunk data)
	if er.phase == 0 && er.lengthBuf == nil {
		// Time to read a new chunk from underlying reader
		chunk := make([]byte, 64*1024) // 64KB chunks
		n, err := er.reader.Read(chunk)
		if n > 0 {
			if os.Getenv("WARP_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[EncryptReader] Read %d bytes from plaintext, encrypting chunk #%d\n", n, er.chunkCount)
			}
			// Create a working copy of the nonce for this chunk
			workingNonce := make([]byte, NonceSize)
			copy(workingNonce, er.nonce)

			// Use counter in last 8 bytes
			if len(workingNonce) == NonceSize {
				for i := 0; i < 8; i++ {
					workingNonce[NonceSize-8+i] = byte(er.chunkCount >> (56 - uint(i*8)))
				}
			}
			er.chunkCount++

			// Encrypt the chunk
			encrypted := er.gcm.Seal(nil, workingNonce, chunk[:n], nil)

			// Prepare length prefix (4 bytes, big-endian)
			er.lengthBuf = make([]byte, 4)
			er.lengthBuf[0] = byte(len(encrypted) >> 24)
			er.lengthBuf[1] = byte(len(encrypted) >> 16)
			er.lengthBuf[2] = byte(len(encrypted) >> 8)
			er.lengthBuf[3] = byte(len(encrypted))

			er.chunkData = encrypted
			er.lengthOff = 0
			er.chunkOff = 0
			er.phase = 0

			if os.Getenv("WARP_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[EncryptReader] Encrypted chunk #%d: length prefix = %x (%d bytes total encrypted)\n", er.chunkCount-1, er.lengthBuf, len(encrypted))
			}

			// Now try to send some data in this same call
			return er.Read(p)
		}

		if err != nil {
			return 0, err
		}
	}

	return 0, io.EOF
}

// Close closes the underlying reader if it implements io.Closer
func (er *EncryptReader) Close() error {
	if closer, ok := er.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
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
	// For buffering encrypted chunks
	chunkBuf []byte // Buffer for reading encrypted chunk data
	chunkLen int    // Expected length of current chunk
	chunkOff int    // Offset in current chunk read
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
	// First read extracts the nonce from the stream
	if dr.first {
		dr.nonce = make([]byte, NonceSize)
		if _, err := io.ReadFull(dr.reader, dr.nonce); err != nil {
			return 0, fmt.Errorf("failed to read nonce: %w", err)
		}
		if os.Getenv("WARP_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DecryptReader] Read nonce: %x\n", dr.nonce)
		}
		dr.first = false
	}

	// Drain buffered plaintext first
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

	// Read length prefix if we don't have a current chunk
	if dr.chunkLen == 0 {
		lengthBytes := make([]byte, 4)
		n, err := dr.reader.Read(lengthBytes)
		if n < 4 {
			if err == io.EOF && n == 0 {
				return 0, io.EOF
			}
			// Try to read the remaining bytes
			for n < 4 && err == nil {
				var b [1]byte
				m, e := dr.reader.Read(b[:])
				if m > 0 {
					lengthBytes[n] = b[0]
					n += m
				}
				err = e
				if err != nil && err != io.EOF {
					return 0, err
				}
			}
			if n < 4 {
				return 0, io.EOF
			}
		}
		// Parse big-endian length
		dr.chunkLen = (int(lengthBytes[0]) << 24) | (int(lengthBytes[1]) << 16) | (int(lengthBytes[2]) << 8) | int(lengthBytes[3])
		dr.chunkBuf = make([]byte, dr.chunkLen)
		dr.chunkOff = 0
		if os.Getenv("WARP_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[DecryptReader] Read length prefix: %x -> chunk length = %d bytes (chunk #%d)\n", lengthBytes, dr.chunkLen, dr.chunkCount)
		}
	}

	// Read the encrypted chunk data
	for dr.chunkOff < dr.chunkLen {
		n, err := dr.reader.Read(dr.chunkBuf[dr.chunkOff:])
		if n > 0 {
			dr.chunkOff += n
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
		if err == io.EOF {
			if dr.chunkOff < dr.chunkLen {
				return 0, fmt.Errorf("unexpected EOF while reading encrypted chunk: expected %d bytes, got %d", dr.chunkLen, dr.chunkOff)
			}
			break
		}
	}

	// Decrypt the buffered chunk
	workingNonce := make([]byte, NonceSize)
	copy(workingNonce, dr.nonce)
	if len(workingNonce) == NonceSize {
		// Add counter to last 8 bytes
		for i := 0; i < 8; i++ {
			workingNonce[NonceSize-8+i] = byte(dr.chunkCount >> (56 - uint(i*8)))
		}
	}
	dr.chunkCount++

	// Decrypt the chunk
	plaintext, decErr := dr.gcm.Open(nil, workingNonce, dr.chunkBuf, nil)
	if decErr != nil {
		return 0, fmt.Errorf("decryption failed: %w", decErr)
	}

	// Reset for next chunk
	dr.chunkLen = 0
	dr.chunkOff = 0
	dr.chunkBuf = nil

	dr.buffer = plaintext
	dr.offset = 0

	// Copy to output
	copied := copy(p, dr.buffer)
	dr.offset = copied
	return copied, nil
}

// Close closes the underlying reader if it implements io.Closer
func (dr *DecryptReader) Close() error {
	if closer, ok := dr.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

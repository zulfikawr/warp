package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"io"
)

// GenerateToken returns a secure 32-byte hex string token.
func GenerateToken(randReader io.Reader) (string, error) {
	if randReader == nil {
		randReader = rand.Reader
	}
	b := make([]byte, 32)
	if _, err := io.ReadFull(randReader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

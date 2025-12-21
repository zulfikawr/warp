package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
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

// GenerateCode returns a human-readable PAKE code in the format N-word-word.
func GenerateCode(randReader io.Reader) (string, error) {
	if randReader == nil {
		randReader = rand.Reader
	}

	// Generate a number between 1 and 99
	n, err := rand.Int(randReader, big.NewInt(99))
	if err != nil {
		return "", err
	}
	num := n.Int64() + 1

	// Pick two words from the wordlist
	w1Idx, err := rand.Int(randReader, big.NewInt(int64(len(WordList))))
	if err != nil {
		return "", err
	}
	w2Idx, err := rand.Int(randReader, big.NewInt(int64(len(WordList))))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d-%s-%s", num, WordList[w1Idx.Int64()], WordList[w2Idx.Int64()]), nil
}

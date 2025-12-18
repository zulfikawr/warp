package crypto

import (
	"testing"
	"crypto/rand"
)

func TestGenerateTokenUniquenessAndLength(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		tok, err := GenerateToken(rand.Reader)
		if err != nil {
			t.Fatalf("error generating token: %v", err)
		}
		if len(tok) != 64 { // 32 bytes hex = 64 chars
			t.Fatalf("unexpected token length: %d", len(tok))
		}
		if _, ok := seen[tok]; ok {
			t.Fatalf("duplicate token generated: %s", tok)
		}
		seen[tok] = struct{}{}
	}
}

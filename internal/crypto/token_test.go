package crypto

import (
	"crypto/rand"
	"testing"
)

func TestGenerateCodeUniqueness(t *testing.T) {
	seen := make(map[string]struct{})
	for range 100 { // 100 iterations is plenty for PAKE codes which are random enough to avoid collisions in small tests
		code, err := GenerateCode(rand.Reader)
		if err != nil {
			t.Fatalf("error generating code: %v", err)
		}
		if len(code) == 0 {
			t.Fatal("generated empty code")
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("duplicate code generated: %s", code)
		}
		seen[code] = struct{}{}
	}
}

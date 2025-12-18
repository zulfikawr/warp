package protocol

import (
	"testing"
)

func TestConstants(t *testing.T) {
	if PathPrefix != "/d/" {
		t.Fatalf("unexpected PathPrefix: %s", PathPrefix)
	}
}

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManifestConfiguration(t *testing.T) {
	// Test case 1: Default values
	s1 := &Server{
		Code: "test-defaults",
	}
	// Start triggers default initialization
	_, _ = s1.Start(context.Background())
	defer func() { _ = s1.Shutdown() }()

	req1, _ := http.NewRequest(http.MethodGet, "/manifest", nil)
	w1 := httptest.NewRecorder()
	s1.handleManifest(w1, req1)

	var resp1 map[string]int
	if err := json.NewDecoder(w1.Body).Decode(&resp1); err != nil {
		t.Fatal(err)
	}

	if resp1["chunk_size"] != 2*1024*1024 {
		t.Errorf("Default chunk_size = %d, want %d", resp1["chunk_size"], 2*1024*1024)
	}
	if resp1["max_concurrent"] != 3 {
		t.Errorf("Default max_concurrent = %d, want %d", resp1["max_concurrent"], 3)
	}

	// Test case 2: Custom values
	s2 := &Server{
		Code:          "test-custom",
		ChunkSize:     4 * 1024 * 1024,
		MaxConcurrent: 5,
	}
	// Start checks defaults but shouldn't override non-zero values
	_, _ = s2.Start(context.Background())
	defer func() { _ = s2.Shutdown() }()

	req2, _ := http.NewRequest(http.MethodGet, "/manifest", nil)
	w2 := httptest.NewRecorder()
	s2.handleManifest(w2, req2)

	var resp2 map[string]int
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatal(err)
	}

	if resp2["chunk_size"] != 4*1024*1024 {
		t.Errorf("Custom chunk_size = %d, want %d", resp2["chunk_size"], 4*1024*1024)
	}
	if resp2["max_concurrent"] != 5 {
		t.Errorf("Custom max_concurrent = %d, want %d", resp2["max_concurrent"], 5)
	}
}

package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestReceiveCreatesFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\"hello.txt\"")
		_, _ = w.Write([]byte("data"))
	}))
	defer ts.Close()

	out, err := Receive(ts.URL, "", true, io.Discard, nil)
	if err != nil {
		t.Fatalf("Receive error: %v", err)
	}
	defer func() { _ = os.Remove(out) }()

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "data" {
		t.Fatalf("content = %q, want %q", string(b), "data")
	}
}

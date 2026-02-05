package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/zulfikawr/warp/internal/progress"
)

func TestReceiveCreatesFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\"hello.txt\"")
		_, _ = w.Write([]byte("data"))
	}))
	defer ts.Close()

	out, err := Receive(context.Background(), ts.URL, "", true, func(pi progress.Progress) {}, nil, nil)
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

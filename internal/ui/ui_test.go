package ui

import (
	"bytes"
	"testing"
)

func TestProgressReaderPercentages(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, 100)
	buf := bytes.NewBuffer(data)
	out := &bytes.Buffer{}
	pr := &ProgressReader{R: buf, Total: int64(len(data)), Out: out}
	b := make([]byte, 50)
	_, _ = pr.Read(b)
	if pr.Current != 50 {
		t.Fatalf("Current=%d want 50", pr.Current)
	}
	_, _ = pr.Read(b)
	if pr.Current != 100 {
		t.Fatalf("Current=%d want 100", pr.Current)
	}
}

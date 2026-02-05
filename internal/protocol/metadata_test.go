package protocol

import "testing"

func TestIsCompressible(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"file.txt", true},
		{"file.json", true},
		{"file.html", true},
		{"file.xml", true},
		{"file.csv", true},
		{"file.jpg", false},
		{"file.png", false},
		{"file.mp4", false},
		{"file.zip", false},
		{"file.TXT", true}, // Case insensitivity check
	}

	for _, tt := range tests {
		got := IsCompressible(tt.path)
		if got != tt.want {
			t.Errorf("IsCompressible(%s) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

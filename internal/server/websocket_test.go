package server

import (
	"net/http/httptest"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name      string
		origin    string
		host      string
		wantAllow bool
	}{
		{
			name:      "No Origin",
			origin:    "",
			host:      "example.com",
			wantAllow: true,
		},
		{
			name:      "Matching Origin HTTP",
			origin:    "http://example.com",
			host:      "example.com",
			wantAllow: true,
		},
		{
			name:      "Matching Origin HTTPS",
			origin:    "https://example.com",
			host:      "example.com",
			wantAllow: true,
		},
		{
			name:      "Mismatched Origin Host",
			origin:    "http://attacker.com",
			host:      "example.com",
			wantAllow: false,
		},
		{
			name:      "Mismatched Origin Port",
			origin:    "http://example.com:8080",
			host:      "example.com",
			wantAllow: false,
		},
		{
			name:      "Matching Origin With Port",
			origin:    "http://example.com:8080",
			host:      "example.com:8080",
			wantAllow: true,
		},
		{
			name:      "Invalid Origin URL",
			origin:    "://invalid",
			host:      "example.com",
			wantAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Host = tt.host

			allow := wsUpgrader.CheckOrigin(req)
			if allow != tt.wantAllow {
				t.Errorf("CheckOrigin() = %v, want %v", allow, tt.wantAllow)
			}
		})
	}
}

package client

import (
	"net/http"
	"time"
)

// defaultHTTPClient returns an HTTP client with optimized connection pooling and HTTP/2 support.
// This is used by both Downloader and UploadSession for consistent HTTP client configuration.
func defaultHTTPClient() *http.Client {
	base := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     true,
		WriteBufferSize:       256 * 1024,
		ReadBufferSize:        256 * 1024,
		DisableCompression:    true, // we'll handle compression negotiation (zstd/gzip) manually
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	return &http.Client{
		Transport: &acceptEncodingTransport{base},
		Timeout:   5 * time.Minute,
	}
}

// acceptEncodingTransport injects Accept-Encoding headers for zstd and gzip
type acceptEncodingTransport struct {
	base http.RoundTripper
}

func (t *acceptEncodingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "zstd, gzip")
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

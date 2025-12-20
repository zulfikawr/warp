package client

import (
	"net/http"
	"time"
)

// defaultHTTPClient returns an HTTP client with optimized connection pooling and HTTP/2 support.
// This is used by both Downloader and UploadSession for consistent HTTP client configuration.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
			ForceAttemptHTTP2:     true,
			WriteBufferSize:       256 * 1024,
			ReadBufferSize:        256 * 1024,
			DisableCompression:    false, // Enable gzip compression
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
		Timeout: 5 * time.Minute,
	}
}

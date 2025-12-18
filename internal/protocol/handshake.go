package protocol

import "time"

const (
	PathPrefix = "/d/"
	UploadPathPrefix = "/u/"
)

var (
	// Generous timeouts for large file transfers (gigabytes over slower connections)
	ReadTimeout  = 10 * time.Minute
	WriteTimeout = 15 * time.Minute
	IdleTimeout  = 5 * time.Minute
)

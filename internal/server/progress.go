package server

// This file previously contained a duplicate TransferProgress type.
// Now we use the unified progress.Progress type from internal/progress.
// The server.Server.ProgressChan now uses progress.Progress directly.

package core

import (
	"context"
	"fmt"
	"time"

	"github.com/zulfikawr/warp/internal/discovery"
)

// DefaultSearchTimeout is the default duration for service discovery.
const DefaultSearchTimeout = 3 * time.Second

// SearchExecutor handles the search command logic.
// It discovers warp services on the local network using mDNS.
type SearchExecutor struct {
	opts     SearchOptions
	onStatus StatusCallback
}

// NewSearchExecutor creates a new SearchExecutor with the given options and callbacks.
// The callback is optional and can be nil if status updates are not needed.
func NewSearchExecutor(opts SearchOptions, onStatus StatusCallback) *SearchExecutor {
	// Apply default timeout if not specified
	if opts.Timeout == 0 {
		opts.Timeout = DefaultSearchTimeout
	}

	return &SearchExecutor{
		opts:     opts,
		onStatus: onStatus,
	}
}

// Execute performs service discovery on the local network.
// Returns a list of discovered warp services.
func (e *SearchExecutor) Execute(ctx context.Context) ([]ServiceInfo, error) {
	e.emitStatus("Searching for warp services on local network...")

	// Create a timeout context if the parent context doesn't have one
	searchCtx, cancel := context.WithTimeout(ctx, e.opts.Timeout)
	defer cancel()

	// Perform mDNS discovery
	services, err := discovery.Browse(searchCtx, e.opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("service discovery failed: %w", err)
	}

	// Convert to core ServiceInfo type
	result := make([]ServiceInfo, 0, len(services))
	for _, svc := range services {
		result = append(result, ServiceInfo{
			Name:  svc.Name,
			Mode:  svc.Mode,
			IP:    svc.IP.String(),
			Port:  svc.Port,
			URL:   svc.URL,
			Token: svc.Token,
		})
	}

	if len(result) == 0 {
		e.emitStatus("No warp services found")
	} else {
		e.emitStatus(fmt.Sprintf("Found %d service(s)", len(result)))
	}

	return result, nil
}

// emitStatus sends a status message if a callback is registered.
func (e *SearchExecutor) emitStatus(msg string) {
	if e.onStatus != nil {
		e.onStatus(msg)
	}
}

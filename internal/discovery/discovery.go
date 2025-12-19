package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/grandcat/zeroconf"
)

// Advertiser represents an active mDNS advertisement.
type Advertiser struct {
	server *zeroconf.Server
}

// Service describes a discovered warp endpoint.
type Service struct {
	Name  string
	Mode  string // send|host
	Token string
	IP    net.IP
	Port  int
	URL   string
}

// Advertise publishes the service over mDNS.
// mode: "send" or "host"
// token: transfer token
// path: URL path including leading slash (e.g., "/d/{token}")
func Advertise(instance, mode, token, path string, ip net.IP, port int) (*Advertiser, error) {
	if ip == nil {
		return nil, fmt.Errorf("ip is required")
	}

	txt := []string{
		"mode=" + mode,
		"token=" + token,
		"path=" + path,
		"ip=" + ip.String(),
	}

	srv, err := zeroconf.Register(instance, "_warp._tcp", "local.", port, txt, nil)
	if err != nil {
		return nil, err
	}

	return &Advertiser{server: srv}, nil
}

// Close stops advertising.
func (a *Advertiser) Close() {
	if a != nil && a.server != nil {
		a.server.Shutdown()
	}
}

// Browse discovers warp services via mDNS.
// timeout defines how long to wait for responses.
func Browse(ctx context.Context, timeout time.Duration) ([]Service, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}

	entries := make(chan *zeroconf.ServiceEntry)
	results := []Service{}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use done channel to properly wait for goroutine completion
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entries {
			if len(e.AddrIPv4) == 0 {
				continue
			}
			ip := e.AddrIPv4[0]
			mode := attr(e, "mode")
			token := attr(e, "token")
			path := attr(e, "path")
			url := fmt.Sprintf("http://%s:%d%s", ip.String(), e.Port, path)
			results = append(results, Service{
				Name:  e.Instance,
				Mode:  mode,
				Token: token,
				IP:    ip,
				Port:  e.Port,
				URL:   url,
			})
		}
	}()

	err = resolver.Browse(ctx, "_warp._tcp", "local.", entries)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		// Real error, not just timeout
		return nil, err
	}

	// Wait for timeout/cancellation
	<-ctx.Done()

	// Wait for processing goroutine to finish
	// The entries channel will be closed by zeroconf when Browse returns
	<-done

	return results, nil
}

func attr(e *zeroconf.ServiceEntry, key string) string {
	prefix := key + "="
	for _, t := range e.Text {
		if len(t) >= len(prefix) && t[:len(prefix)] == prefix {
			return t[len(prefix):]
		}
	}
	return ""
}

package discovery

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestAdvertiseAndBrowse(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	port := 54321
	token := "tokendiscovery"
	path := "/d/" + token

	adv, err := Advertise("warp-test-"+token[:6], "send", token, path, ip, port)
	if err != nil {
		t.Fatalf("advertise failed: %v", err)
	}
	defer adv.Close()

	// Give the responder a moment to announce
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	services, err := Browse(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("browse failed: %v", err)
	}

	found := false
	for _, svc := range services {
		if svc.Token == token && svc.Mode == "send" {
			found = true
			if svc.URL == "" {
				t.Fatalf("expected URL to be set")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected to find advertised service")
	}
}

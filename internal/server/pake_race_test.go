package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zulfikawr/warp/internal/crypto"
)

func TestPAKEAttemptsRaceCondition(t *testing.T) {
	// Initialize AuthManager
	code := "1234-warp-share"
	am := NewAuthManager(code)
	clientIP := "127.0.0.1"

	// Initialize attempts to 0
	am.pakeAttempts.Store(clientIP, &clientState{})

	// Number of concurrent requests
	concurrency := 100

	// Create a wait group
	var wg sync.WaitGroup
	wg.Add(concurrency)

	// We need to preload sessions because HandleVerify requires them.
	// We'll simulate 100 different ports from the same IP.
	for i := 0; i < concurrency; i++ {
		sessionID := fmt.Sprintf("%s:%d", clientIP, 50000+i)

		// Create a dummy session
		// We need a valid session object but the content can be dummy since we expect verification to fail
		// VerifyConfirmation checks HMAC. We can just provide garbage keys/messages so it fails.
		state, _ := crypto.InitializePAKE(code, true)
		am.pakeSessions.Store(sessionID, &pakeSession{
			State:         state,
			Key:           []byte("dummykey"),
			ClientMessage: []byte("clientmsg"),
			ServerMessage: []byte("servermsg"),
			Expiry:        time.Now().Add(time.Minute),
		})
	}

	// Perform concurrent Verify requests
	for i := 0; i < concurrency; i++ {
		go func(port int) {
			defer wg.Done()

			sessionID := fmt.Sprintf("%s:%d", clientIP, 50000+port)

			// Create request body
			reqBody := pakeVerifyRequest{
				Confirmation: []byte("invalid-confirmation"),
			}
			bodyBytes, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader(bodyBytes))
			req.RemoteAddr = sessionID

			w := httptest.NewRecorder()

			am.HandleVerify(w, req)
		}(i)
	}

	wg.Wait()

	// Check final attempt count
	val, ok := am.pakeAttempts.Load(clientIP)
	if !ok {
		t.Fatalf("Expected attempts entry for %s", clientIP)
	}
	state := val.(*clientState)
	state.mu.Lock()
	attempts := state.attempts
	state.mu.Unlock()

	if attempts != concurrency {
		t.Errorf("Race condition detected! Expected %d attempts, got %d", concurrency, attempts)
	} else {
		t.Logf("Attempts count: %d (Matches expected)", attempts)
	}
}

package crypto

import (
	"bytes"
	"testing"
)

// TestPAKEHandshake_Success verifies that client and server derive the same 32-byte shared key
// using the same PAKE code while following the production message exchange order.
func TestPAKEHandshake_Success(t *testing.T) {
	code := "test-code-123"

	server, err := InitializePAKE(code, true)
	if err != nil {
		t.Fatalf("InitializePAKE(server) failed: %v", err)
	}
	client, err := InitializePAKE(code, false)
	if err != nil {
		t.Fatalf("InitializePAKE(client) failed: %v", err)
	}

	// Sequence: client -> server -> client
	clientMsg := client.Bytes()
	keyServer, err := server.ComputeSharedKey(clientMsg)
	if err != nil {
		t.Fatalf("server.ComputeSharedKey failed: %v", err)
	}
	serverMsg := server.Bytes()
	keyClient, err := client.ComputeSharedKey(serverMsg)
	if err != nil {
		t.Fatalf("client.ComputeSharedKey failed: %v", err)
	}

	if len(keyServer) != 32 || len(keyClient) != 32 {
		t.Fatalf("expected 32-byte keys, got %d and %d", len(keyServer), len(keyClient))
	}
	if !bytes.Equal(keyServer, keyClient) {
		t.Fatalf("shared keys differ: server=%x client=%x", keyServer, keyClient)
	}
}

// TestPAKEConfirmationFlow_Success verifies the mutual confirmation HMAC exchange.
func TestPAKEConfirmationFlow_Success(t *testing.T) {
	code := "pake-confirmation-code"

	server, err := InitializePAKE(code, true)
	if err != nil {
		t.Fatalf("InitializePAKE(server) failed: %v", err)
	}
	client, err := InitializePAKE(code, false)
	if err != nil {
		t.Fatalf("InitializePAKE(client) failed: %v", err)
	}

	// Sequence: client -> server -> client
	clientMsg := client.Bytes()
	keyServer, err := server.ComputeSharedKey(clientMsg)
	if err != nil {
		t.Fatalf("server.ComputeSharedKey failed: %v", err)
	}
	serverMsg := server.Bytes()
	keyClient, err := client.ComputeSharedKey(serverMsg)
	if err != nil {
		t.Fatalf("client.ComputeSharedKey failed: %v", err)
	}

	// Client sends HMAC(key, serverMsg)
	clientConfirmation := GenerateConfirmation(keyClient, serverMsg)
	if err := VerifyConfirmation(keyServer, serverMsg, clientConfirmation); err != nil {
		t.Fatalf("server VerifyConfirmation failed: %v", err)
	}

	// Server sends HMAC(key, clientMsg)
	serverConfirmation := GenerateConfirmation(keyServer, clientMsg)
	if err := VerifyConfirmation(keyClient, clientMsg, serverConfirmation); err != nil {
		t.Fatalf("client VerifyConfirmation failed: %v", err)
	}
}

// TestPAKEConfirmation_FailsWithWrongKey ensures HMAC verification fails when using a different key.
func TestPAKEConfirmation_FailsWithWrongKey(t *testing.T) {
	// First handshake with codeA
	serverA, err := InitializePAKE("codeA", true)
	if err != nil {
		t.Fatalf("InitializePAKE(serverA) failed: %v", err)
	}
	clientA, err := InitializePAKE("codeA", false)
	if err != nil {
		t.Fatalf("InitializePAKE(clientA) failed: %v", err)
	}
	clientMsgA := clientA.Bytes()
	keyServerA, err := serverA.ComputeSharedKey(clientMsgA)
	if err != nil {
		t.Fatalf("serverA.ComputeSharedKey failed: %v", err)
	}
	serverMsgA := serverA.Bytes()
	keyClientA, err := clientA.ComputeSharedKey(serverMsgA)
	if err != nil {
		t.Fatalf("clientA.ComputeSharedKey failed: %v", err)
	}

	// Second handshake with codeB to produce a different key
	serverB, err := InitializePAKE("codeB", true)
	if err != nil {
		t.Fatalf("InitializePAKE(serverB) failed: %v", err)
	}
	clientB, err := InitializePAKE("codeB", false)
	if err != nil {
		t.Fatalf("InitializePAKE(clientB) failed: %v", err)
	}
	clientMsgB := clientB.Bytes()
	keyServerB, err := serverB.ComputeSharedKey(clientMsgB)
	if err != nil {
		t.Fatalf("serverB.ComputeSharedKey failed: %v", err)
	}
	serverMsgB := serverB.Bytes()
	keyClientB, err := clientB.ComputeSharedKey(serverMsgB)
	if err != nil {
		t.Fatalf("clientB.ComputeSharedKey failed: %v", err)
	}

	// Sanity: ensure keys from different codes are not equal
	if bytes.Equal(keyServerA, keyServerB) || bytes.Equal(keyClientA, keyClientB) {
		t.Fatalf("expected different keys for different codes")
	}

	// Use confirmation from A but verify with key from B
	conf := GenerateConfirmation(keyClientA, serverMsgA)
	if err := VerifyConfirmation(keyServerB, serverMsgA, conf); err == nil {
		t.Fatalf("expected VerifyConfirmation to fail with wrong key")
	}
}

// TestBytes_NonEmptyAndDifferentBetweenRoles confirms both sides produce non-empty and distinct initial messages.
func TestBytes_NonEmptyAndDifferentBetweenRoles(t *testing.T) {
	server, err := InitializePAKE("msg-role-code", true)
	if err != nil {
		t.Fatalf("InitializePAKE(server) failed: %v", err)
	}
	client, err := InitializePAKE("msg-role-code", false)
	if err != nil {
		t.Fatalf("InitializePAKE(client) failed: %v", err)
	}

	// Proper sequence: client -> server
	clientMsg := client.Bytes()
	if _, err := server.ComputeSharedKey(clientMsg); err != nil {
		t.Fatalf("server.ComputeSharedKey failed: %v", err)
	}
	serverMsg := server.Bytes()

	if len(serverMsg) == 0 || len(clientMsg) == 0 {
		t.Fatalf("expected non-empty messages: server=%d client=%d", len(serverMsg), len(clientMsg))
	}
	if bytes.Equal(serverMsg, clientMsg) {
		t.Fatalf("expected different initial messages between roles")
	}
}

// TestVerifyConfirmation_FailsOnWrongMessage ensures HMAC validation fails with an altered message.
func TestVerifyConfirmation_FailsOnWrongMessage(t *testing.T) {
	code := "altered-message-code"

	server, err := InitializePAKE(code, true)
	if err != nil {
		t.Fatalf("InitializePAKE(server) failed: %v", err)
	}
	client, err := InitializePAKE(code, false)
	if err != nil {
		t.Fatalf("InitializePAKE(client) failed: %v", err)
	}

	// Sequence: client -> server -> client
	clientMsg := client.Bytes()
	keyServer, err := server.ComputeSharedKey(clientMsg)
	if err != nil {
		t.Fatalf("server.ComputeSharedKey failed: %v", err)
	}
	serverMsg := server.Bytes()
	keyClient, err := client.ComputeSharedKey(serverMsg)
	if err != nil {
		t.Fatalf("client.ComputeSharedKey failed: %v", err)
	}

	conf := GenerateConfirmation(keyClient, serverMsg)

	// Alter the message slightly
	badMsg := make([]byte, len(serverMsg))
	copy(badMsg, serverMsg)
	if len(badMsg) > 0 {
		badMsg[0] ^= 0xFF
	}

	if err := VerifyConfirmation(keyServer, badMsg, conf); err == nil {
		t.Fatalf("expected VerifyConfirmation to fail with altered message")
	}

	// Also ensure correct message succeeds
	if err := VerifyConfirmation(keyServer, serverMsg, conf); err != nil {
		t.Fatalf("expected VerifyConfirmation to succeed with correct message: %v", err)
	}
}

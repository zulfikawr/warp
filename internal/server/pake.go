package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/zulfikawr/warp/internal/crypto"
)

// AuthManager handles PAKE-based authentication and shared key management
type AuthManager struct {
	Code         string
	pakeSessions sync.Map // sessionID -> *pakeSession
	pakeAttempts sync.Map // clientIP -> *clientState
	tokenKeys    sync.Map // code -> []byte (shared key)
}

// clientState holds the state for a client IP with atomic access
type clientState struct {
	mu       sync.Mutex
	attempts int
}

// NewAuthManager creates a new AuthManager with the given PAKE code
func NewAuthManager(code string) *AuthManager {
	return &AuthManager{
		Code: code,
	}
}

type pakeSession struct {
	State         *crypto.PAKEState
	Key           []byte
	ClientMessage []byte
	ServerMessage []byte
	Expiry        time.Time
}

type pakeInitRequest struct {
	Message []byte `json:"message"`
}

type pakeInitResponse struct {
	Message []byte `json:"message"`
}

type pakeVerifyRequest struct {
	Confirmation []byte `json:"confirmation"`
}

type pakeVerifyResponse struct {
	Confirmation []byte `json:"confirmation"`
	Code         string `json:"code,omitempty"`
}

// HandleInit handles the initial PAKE message from the client
func (m *AuthManager) HandleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := getClientIP(r)
	val, _ := m.pakeAttempts.LoadOrStore(clientIP, &clientState{})
	state := val.(*clientState)

	state.mu.Lock()
	if state.attempts >= 5 {
		state.mu.Unlock()
		http.Error(w, "Too many attempts", http.StatusTooManyRequests)
		return
	}
	state.mu.Unlock()

	var req pakeInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	pakeState, err := crypto.InitializePAKE(m.Code, true)
	if err != nil {
		http.Error(w, "Failed to initialize PAKE", http.StatusInternalServerError)
		return
	}

	// Server (Role 1) updates with Client's message (X)
	// This computes Y and the shared key
	key, err := pakeState.ComputeSharedKey(req.Message)
	if err != nil {
		http.Error(w, "Failed to compute shared key", http.StatusBadRequest)
		return
	}

	serverMessage := pakeState.Bytes()

	// Store session
	sessionID := r.RemoteAddr
	m.pakeSessions.Store(sessionID, &pakeSession{
		State:         pakeState,
		Key:           key,
		ClientMessage: req.Message,
		ServerMessage: serverMessage,
		Expiry:        time.Now().Add(60 * time.Second),
	})

	resp := pakeInitResponse{
		Message: serverMessage,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleVerify handles the second PAKE message from the client
func (m *AuthManager) HandleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.RemoteAddr
	val, ok := m.pakeSessions.Load(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	session := val.(*pakeSession)

	if time.Now().After(session.Expiry) {
		m.pakeSessions.Delete(sessionID)
		http.Error(w, "Session expired", http.StatusGone)
		return
	}

	var req pakeVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Verify client's confirmation: HMAC(key, ServerMessage)
	if err := crypto.VerifyConfirmation(session.Key, session.ServerMessage, req.Confirmation); err != nil {
		m.pakeSessions.Delete(sessionID)
		clientIP := getClientIP(r)

		if val, ok := m.pakeAttempts.Load(clientIP); ok {
			state := val.(*clientState)
			state.mu.Lock()
			state.attempts++
			state.mu.Unlock()
		}

		http.Error(w, "Invalid confirmation", http.StatusUnauthorized)
		return
	}

	// Generate server's confirmation: HMAC(key, ClientMessage)
	serverConfirmation := crypto.GenerateConfirmation(session.Key, session.ClientMessage)

	// Store the key for the session
	m.tokenKeys.Store(m.Code, session.Key)

	resp := pakeVerifyResponse{
		Confirmation: serverConfirmation,
		Code:         m.Code,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetKey returns the shared key for a given code
func (m *AuthManager) GetKey(code string) ([]byte, bool) {
	val, ok := m.tokenKeys.Load(code)
	if !ok {
		return nil, false
	}
	return val.([]byte), true
}

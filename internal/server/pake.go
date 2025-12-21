package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/zulfikawr/warp/internal/crypto"
)

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
	Token        string `json:"token,omitempty"`
}

func (s *Server) handlePAKEInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := getClientIP(r)
	attempts, _ := s.pakeAttempts.LoadOrStore(clientIP, 0)
	if attempts.(int) >= 5 {
		http.Error(w, "Too many attempts", http.StatusTooManyRequests)
		return
	}

	var req pakeInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	state, err := crypto.InitializePAKE(s.PAKECode, true)
	if err != nil {
		http.Error(w, "Failed to initialize PAKE", http.StatusInternalServerError)
		return
	}

	// Server (Role 1) updates with Client's message (X)
	// This computes Y and the shared key
	key, err := state.ComputeSharedKey(req.Message)
	if err != nil {
		http.Error(w, "Failed to compute shared key", http.StatusBadRequest)
		return
	}

	serverMessage := state.Bytes()

	// Store session
	sessionID := r.RemoteAddr
	s.pakeSessions.Store(sessionID, &pakeSession{
		State:         state,
		Key:           key,
		ClientMessage: req.Message,
		ServerMessage: serverMessage,
		Expiry:        time.Now().Add(60 * time.Second),
	})

	resp := pakeInitResponse{
		Message: serverMessage,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handlePAKEVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.RemoteAddr
	val, ok := s.pakeSessions.Load(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	session := val.(*pakeSession)

	if time.Now().After(session.Expiry) {
		s.pakeSessions.Delete(sessionID)
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
		s.pakeSessions.Delete(sessionID)
		clientIP := getClientIP(r)
		if val, ok := s.pakeAttempts.Load(clientIP); ok {
			s.pakeAttempts.Store(clientIP, val.(int)+1)
		}
		http.Error(w, "Invalid confirmation", http.StatusUnauthorized)
		return
	}

	// Generate server's confirmation: HMAC(key, ClientMessage)
	serverConfirmation := crypto.GenerateConfirmation(session.Key, session.ClientMessage)

	// Store the key for the token
	s.tokenKeys.Store(s.Token, session.Key)

	resp := pakeVerifyResponse{
		Confirmation: serverConfirmation,
		Token:        s.Token,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

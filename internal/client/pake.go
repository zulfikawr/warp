package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/zulfikawr/warp/internal/crypto"
	"github.com/zulfikawr/warp/internal/protocol"
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
	Token        string `json:"token"`
}

// PerformPAKEHandshake performs the PAKE handshake with the server at baseURL.
// Returns the shared key and the download token.
func (d *Downloader) PerformPAKEHandshake(baseURL string, code string) ([]byte, string, error) {
	// 1. Initialize PAKE
	// Client is Role 0
	state, err := crypto.InitializePAKE(code, false)
	if err != nil {
		return nil, "", fmt.Errorf("failed to initialize PAKE: %w", err)
	}
	clientMessage := state.Bytes()

	// 2. POST /pake/init
	initReq := pakeInitRequest{Message: clientMessage}
	initReqBody, _ := json.Marshal(initReq)
	resp, err := d.client.Post(baseURL+protocol.PAKEInitPath, "application/json", bytes.NewBuffer(initReqBody))
	if err != nil {
		return nil, "", fmt.Errorf("PAKE init request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("PAKE init failed with status: %d", resp.StatusCode)
	}

	var initResp pakeInitResponse
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		return nil, "", fmt.Errorf("failed to decode PAKE init response: %w", err)
	}

	// 3. Compute shared key
	// Client (Role 0) updates with Server's message (Y)
	key, err := state.ComputeSharedKey(initResp.Message)
	if err != nil {
		return nil, "", fmt.Errorf("failed to compute shared key: %w", err)
	}

	// 4. POST /pake/verify
	// Client sends HMAC(key, serverMessage)
	clientConfirmation := crypto.GenerateConfirmation(key, initResp.Message)
	verifyReq := pakeVerifyRequest{Confirmation: clientConfirmation}
	verifyReqBody, _ := json.Marshal(verifyReq)
	resp, err = d.client.Post(baseURL+protocol.PAKEVerifyPath, "application/json", bytes.NewBuffer(verifyReqBody))
	if err != nil {
		return nil, "", fmt.Errorf("PAKE verify request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error body
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		return nil, "", fmt.Errorf("PAKE verify failed with status: %d, body: %s", resp.StatusCode, buf.String())
	}

	var verifyResp pakeVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return nil, "", fmt.Errorf("failed to decode PAKE verify response: %w", err)
	}

	// 5. Verify server's confirmation: HMAC(key, clientMessage)
	if err := crypto.VerifyConfirmation(key, clientMessage, verifyResp.Confirmation); err != nil {
		return nil, "", fmt.Errorf("server PAKE confirmation failed: %w", err)
	}

	return key, verifyResp.Token, nil
}

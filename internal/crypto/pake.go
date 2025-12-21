package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"

	"github.com/schollz/pake/v3"
)

// PAKEState wraps the pake.Pake object.
type PAKEState struct {
	p *pake.Pake
}

// InitializePAKE initializes the PAKE protocol with a code.
func InitializePAKE(code string, isServer bool) (*PAKEState, error) {
	role := 0
	if isServer {
		role = 1
	}
	p, err := pake.InitCurve([]byte(code), role, "p256")
	if err != nil {
		return nil, err
	}
	return &PAKEState{p: p}, nil
}

// Bytes returns the public message to be sent to the peer.
func (s *PAKEState) Bytes() []byte {
	return s.p.Bytes()
}

// ComputeSharedKey computes the shared key from the peer's message.
// It returns the 32-byte shared key.
func (s *PAKEState) ComputeSharedKey(peerMessage []byte) ([]byte, error) {
	err := s.p.Update(peerMessage)
	if err != nil {
		return nil, err
	}
	return s.p.SessionKey()
}

// GenerateConfirmation generates an HMAC of the shared key to confirm agreement.
func GenerateConfirmation(key []byte, message []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return h.Sum(nil)
}

// VerifyConfirmation verifies the HMAC of the shared key.
func VerifyConfirmation(key []byte, message []byte, confirmation []byte) error {
	expected := GenerateConfirmation(key, message)
	if !hmac.Equal(expected, confirmation) {
		return errors.New("PAKE key confirmation failed")
	}
	return nil
}

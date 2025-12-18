package crypto

import (
	"bytes"
	"io"
	"testing"
)

func TestDeriveKey(t *testing.T) {
	password := "test-password"
	salt := []byte("test-salt-32-bytes-long-salt")

	key1 := DeriveKey(password, salt)
	key2 := DeriveKey(password, salt)

	if len(key1) != KeySize {
		t.Errorf("expected key size %d, got %d", KeySize, len(key1))
	}

	if !bytes.Equal(key1, key2) {
		t.Error("same password and salt should produce same key")
	}

	// Different password should produce different key
	key3 := DeriveKey("different-password", salt)
	if bytes.Equal(key1, key3) {
		t.Error("different passwords should produce different keys")
	}
}

func TestGenerateSalt(t *testing.T) {
	salt1, err := GenerateSalt()
	if err != nil {
		t.Fatalf("failed to generate salt: %v", err)
	}

	if len(salt1) != SaltSize {
		t.Errorf("expected salt size %d, got %d", SaltSize, len(salt1))
	}

	salt2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("failed to generate salt: %v", err)
	}

	if bytes.Equal(salt1, salt2) {
		t.Error("consecutive salt generations should be different")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, KeySize)
	plaintext := []byte("Hello, World! This is a test message for encryption.")

	// Encrypt
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if len(ciphertext) <= len(plaintext) {
		t.Error("ciphertext should be longer than plaintext (includes nonce and tag)")
	}

	// Decrypt
	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted text doesn't match original.\nExpected: %s\nGot: %s", plaintext, decrypted)
	}
}

func TestEncryptDecryptWithWrongKey(t *testing.T) {
	key1 := make([]byte, KeySize)
	key2 := make([]byte, KeySize)
	key2[0] = 1 // Make it different

	plaintext := []byte("Secret message")

	ciphertext, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Try to decrypt with wrong key
	_, err = Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("decryption with wrong key should fail")
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	key := make([]byte, KeySize)
	
	// Too short ciphertext
	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Error("decrypting short ciphertext should fail")
	}

	// Invalid ciphertext
	invalid := make([]byte, 100)
	_, err = Decrypt(invalid, key)
	if err == nil {
		t.Error("decrypting invalid ciphertext should fail")
	}
}

func TestEncryptReader(t *testing.T) {
	key := make([]byte, KeySize)
	plaintext := []byte("This is a test message for streaming encryption. " +
		"It should be encrypted in chunks and then successfully decrypted back.")

	// Create encrypt reader
	plaintextReader := bytes.NewReader(plaintext)
	encReader, err := NewEncryptReader(plaintextReader, key)
	if err != nil {
		t.Fatalf("failed to create encrypt reader: %v", err)
	}

	// Read encrypted data
	var encrypted bytes.Buffer
	_, err = io.Copy(&encrypted, encReader)
	if err != nil {
		t.Fatalf("failed to read encrypted data: %v", err)
	}

	// Create decrypt reader
	decReader, err := NewDecryptReader(&encrypted, key)
	if err != nil {
		t.Fatalf("failed to create decrypt reader: %v", err)
	}

	// Read decrypted data
	var decrypted bytes.Buffer
	_, err = io.Copy(&decrypted, decReader)
	if err != nil {
		t.Fatalf("failed to read decrypted data: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted.Bytes()) {
		t.Errorf("decrypted data doesn't match original.\nExpected: %s\nGot: %s",
			plaintext, decrypted.Bytes())
	}
}

func TestEncryptReaderLargeData(t *testing.T) {
	key := make([]byte, KeySize)
	
	// Create 1MB of test data
	plaintext := make([]byte, 1024*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	// Encrypt
	plaintextReader := bytes.NewReader(plaintext)
	encReader, err := NewEncryptReader(plaintextReader, key)
	if err != nil {
		t.Fatalf("failed to create encrypt reader: %v", err)
	}

	var encrypted bytes.Buffer
	_, err = io.Copy(&encrypted, encReader)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Decrypt
	decReader, err := NewDecryptReader(&encrypted, key)
	if err != nil {
		t.Fatalf("failed to create decrypt reader: %v", err)
	}

	var decrypted bytes.Buffer
	_, err = io.Copy(&decrypted, decReader)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted.Bytes()) {
		t.Error("large data decryption failed")
	}
}

func TestEncryptReaderSmallReads(t *testing.T) {
	key := make([]byte, KeySize)
	plaintext := []byte("Small test data for small reads")

	// Encrypt
	plaintextReader := bytes.NewReader(plaintext)
	encReader, err := NewEncryptReader(plaintextReader, key)
	if err != nil {
		t.Fatalf("failed to create encrypt reader: %v", err)
	}

	// Read encrypted data in small chunks (1 byte at a time)
	var encrypted bytes.Buffer
	buf := make([]byte, 1)
	for {
		n, err := encReader.Read(buf)
		if n > 0 {
			encrypted.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read error: %v", err)
		}
	}

	// Decrypt
	decReader, err := NewDecryptReader(&encrypted, key)
	if err != nil {
		t.Fatalf("failed to create decrypt reader: %v", err)
	}

	var decrypted bytes.Buffer
	_, err = io.Copy(&decrypted, decReader)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted.Bytes()) {
		t.Errorf("small reads decryption failed.\nExpected: %s\nGot: %s",
			plaintext, decrypted.Bytes())
	}
}

func TestDecryptReaderWithWrongKey(t *testing.T) {
	key1 := make([]byte, KeySize)
	key2 := make([]byte, KeySize)
	key2[0] = 1 // Different key

	plaintext := []byte("Secret message")

	// Encrypt with key1
	encReader, err := NewEncryptReader(bytes.NewReader(plaintext), key1)
	if err != nil {
		t.Fatalf("failed to create encrypt reader: %v", err)
	}

	var encrypted bytes.Buffer
	io.Copy(&encrypted, encReader)

	// Try to decrypt with key2
	decReader, err := NewDecryptReader(&encrypted, key2)
	if err != nil {
		t.Fatalf("failed to create decrypt reader: %v", err)
	}

	var decrypted bytes.Buffer
	_, err = io.Copy(&decrypted, decReader)
	if err == nil {
		t.Error("decryption with wrong key should fail")
	}
}

package protocol

import (
	"bytes"
	"testing"
)

func TestCryptoStateKeyGeneration(t *testing.T) {
	cs, err := NewCryptoState()
	if err != nil {
		t.Fatalf("NewCryptoState() failed: %v", err)
	}

	// Check public key is generated
	pubKey := cs.GetPublicKey()
	if len(pubKey) != DHKeySize {
		t.Errorf("Public key size = %d, want %d", len(pubKey), DHKeySize)
	}

	// Check it's not all zeros (should be random)
	allZero := true
	for _, b := range pubKey {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Public key is all zeros")
	}
}

func TestCryptoStateKeyExchange(t *testing.T) {
	// Create two crypto states (simulating two peers)
	cs1, err := NewCryptoState()
	if err != nil {
		t.Fatalf("NewCryptoState(1) failed: %v", err)
	}

	cs2, err := NewCryptoState()
	if err != nil {
		t.Fatalf("NewCryptoState(2) failed: %v", err)
	}

	// Exchange public keys
	err = cs1.SetRemotePublicKey(cs2.GetPublicKey())
	if err != nil {
		t.Fatalf("cs1.SetRemotePublicKey() failed: %v", err)
	}

	err = cs2.SetRemotePublicKey(cs1.GetPublicKey())
	if err != nil {
		t.Fatalf("cs2.SetRemotePublicKey() failed: %v", err)
	}

	// Both should be established
	if !cs1.IsEstablished() {
		t.Error("cs1 not established")
	}
	if !cs2.IsEstablished() {
		t.Error("cs2 not established")
	}
}

func TestCryptoStateEncryptDecrypt(t *testing.T) {
	// Set up two crypto states
	cs1, _ := NewCryptoState()
	cs2, _ := NewCryptoState()

	cs1.SetRemotePublicKey(cs2.GetPublicKey())
	cs2.SetRemotePublicKey(cs1.GetPublicKey())

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "short message",
			plaintext: []byte("Hello, World!"),
		},
		{
			name:      "empty message",
			plaintext: []byte(""),
		},
		{
			name:      "long message",
			plaintext: bytes.Repeat([]byte("A"), 10000),
		},
		{
			name:      "binary data",
			plaintext: []byte{0x00, 0xFF, 0x55, 0xAA, 0xDE, 0xAD, 0xBE, 0xEF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt with cs1
			ciphertext, err := cs1.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() failed: %v", err)
			}

			// Should be longer (nonce + tag overhead)
			if len(ciphertext) < len(tt.plaintext)+NonceSize+TagSize {
				t.Errorf("Ciphertext too short: got %d bytes", len(ciphertext))
			}

			// Decrypt with cs2
			decrypted, err := cs2.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() failed: %v", err)
			}

			// Should match original
			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Decrypted data mismatch:\ngot  %v\nwant %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestCryptoStateNotEstablished(t *testing.T) {
	cs, _ := NewCryptoState()

	// Try to encrypt before key exchange
	_, err := cs.Encrypt([]byte("test"))
	if err == nil {
		t.Error("Expected error when encrypting before key exchange")
	}

	// Try to decrypt before key exchange
	_, err = cs.Decrypt([]byte("test"))
	if err == nil {
		t.Error("Expected error when decrypting before key exchange")
	}
}

func TestCryptoStateInvalidCiphertext(t *testing.T) {
	cs1, _ := NewCryptoState()
	cs2, _ := NewCryptoState()

	cs1.SetRemotePublicKey(cs2.GetPublicKey())
	cs2.SetRemotePublicKey(cs1.GetPublicKey())

	tests := []struct {
		name       string
		ciphertext []byte
	}{
		{
			name:       "too short",
			ciphertext: []byte("short"),
		},
		{
			name:       "tampered data",
			ciphertext: append([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, bytes.Repeat([]byte{0xFF}, 50)...),
		},
		{
			name:       "empty",
			ciphertext: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cs2.Decrypt(tt.ciphertext)
			if err == nil {
				t.Error("Expected error for invalid ciphertext")
			}
		})
	}
}

func TestCryptoStateInvalidPublicKey(t *testing.T) {
	cs, _ := NewCryptoState()

	tests := []struct {
		name      string
		publicKey []byte
	}{
		{
			name:      "too short",
			publicKey: []byte{1, 2, 3},
		},
		{
			name:      "too long",
			publicKey: make([]byte, DHKeySize+1),
		},
		{
			name:      "empty",
			publicKey: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cs.SetRemotePublicKey(tt.publicKey)
			if err == nil {
				t.Error("Expected error for invalid public key")
			}
		})
	}
}

func TestCryptoStateUniqueKeys(t *testing.T) {
	// Generate multiple crypto states
	states := make([]*CryptoState, 10)
	for i := range states {
		var err error
		states[i], err = NewCryptoState()
		if err != nil {
			t.Fatalf("NewCryptoState() failed: %v", err)
		}
	}

	// Check all public keys are unique
	for i := 0; i < len(states); i++ {
		for j := i + 1; j < len(states); j++ {
			if bytes.Equal(states[i].GetPublicKey(), states[j].GetPublicKey()) {
				t.Errorf("Duplicate public keys at indices %d and %d", i, j)
			}
		}
	}
}

func TestCryptoStateNonceIncrement(t *testing.T) {
	cs1, _ := NewCryptoState()
	cs2, _ := NewCryptoState()

	cs1.SetRemotePublicKey(cs2.GetPublicKey())
	cs2.SetRemotePublicKey(cs1.GetPublicKey())

	// Encrypt multiple messages
	messages := []string{"first", "second", "third", "fourth", "fifth"}
	ciphertexts := make([][]byte, len(messages))

	for i, msg := range messages {
		ct, err := cs1.Encrypt([]byte(msg))
		if err != nil {
			t.Fatalf("Encrypt(%d) failed: %v", i, err)
		}
		ciphertexts[i] = ct
	}

	// Check that nonces are different (first 12 bytes)
	for i := 0; i < len(ciphertexts); i++ {
		for j := i + 1; j < len(ciphertexts); j++ {
			if bytes.Equal(ciphertexts[i][:NonceSize], ciphertexts[j][:NonceSize]) {
				t.Errorf("Duplicate nonces in messages %d and %d", i, j)
			}
		}
	}

	// Decrypt all messages
	for i, ct := range ciphertexts {
		pt, err := cs2.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt(%d) failed: %v", i, err)
		}
		if string(pt) != messages[i] {
			t.Errorf("Message %d: got %q, want %q", i, string(pt), messages[i])
		}
	}
}

func TestCryptoStateAuthenticationTag(t *testing.T) {
	cs1, _ := NewCryptoState()
	cs2, _ := NewCryptoState()

	cs1.SetRemotePublicKey(cs2.GetPublicKey())
	cs2.SetRemotePublicKey(cs1.GetPublicKey())

	plaintext := []byte("authenticated message")
	ciphertext, _ := cs1.Encrypt(plaintext)

	// Tamper with the last byte (authentication tag)
	ciphertext[len(ciphertext)-1] ^= 0xFF

	// Decryption should fail
	_, err := cs2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Expected authentication failure for tampered ciphertext")
	}
}

func BenchmarkCryptoEncrypt(b *testing.B) {
	cs1, _ := NewCryptoState()
	cs2, _ := NewCryptoState()
	cs1.SetRemotePublicKey(cs2.GetPublicKey())
	cs2.SetRemotePublicKey(cs1.GetPublicKey())

	data := make([]byte, 1400) // Typical packet size

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cs1.Encrypt(data)
	}
}

func BenchmarkCryptoDecrypt(b *testing.B) {
	cs1, _ := NewCryptoState()
	cs2, _ := NewCryptoState()
	cs1.SetRemotePublicKey(cs2.GetPublicKey())
	cs2.SetRemotePublicKey(cs1.GetPublicKey())

	data := make([]byte, 1400)
	ciphertext, _ := cs1.Encrypt(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cs2.Decrypt(ciphertext)
	}
}

func BenchmarkKeyExchange(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cs1, _ := NewCryptoState()
		cs2, _ := NewCryptoState()
		cs1.SetRemotePublicKey(cs2.GetPublicKey())
		cs2.SetRemotePublicKey(cs1.GetPublicKey())
	}
}

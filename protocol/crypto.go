package protocol

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	NonceSize = 12
	KeySize   = 32
	TagSize   = 16
	DHKeySize = 32
)

var (
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrInvalidPublicKey  = errors.New("invalid public key")
	ErrHandshakeTimeout  = errors.New("handshake timeout")
)

type CryptoState struct {
	cipher       cipher.AEAD
	localPrivate [32]byte
	localPublic  [32]byte
	remotePublic [32]byte
	sharedSecret [32]byte
	txNonce      uint64
	rxNonce      uint64
	nonceMu      sync.Mutex
	established  bool
}

func NewCryptoState() (*CryptoState, error) {
	cs := &CryptoState{}
	
	// Generate ephemeral key pair
	if _, err := io.ReadFull(rand.Reader, cs.localPrivate[:]); err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	
	// Compute public key
	curve25519.ScalarBaseMult(&cs.localPublic, &cs.localPrivate)
	
	return cs, nil
}

func (cs *CryptoState) GetPublicKey() []byte {
	return cs.localPublic[:]
}

func (cs *CryptoState) SetRemotePublicKey(remotePublic []byte) error {
	if len(remotePublic) != DHKeySize {
		return ErrInvalidPublicKey
	}
	
	copy(cs.remotePublic[:], remotePublic)
	
	// Compute shared secret
	curve25519.ScalarMult(&cs.sharedSecret, &cs.localPrivate, &cs.remotePublic)
	
	// Derive encryption key using HKDF
	salt := make([]byte, 32)
	info := []byte("udp-network-v1")
	
	kdf := hkdf.New(sha256.New, cs.sharedSecret[:], salt, info)
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(kdf, key); err != nil {
		return fmt.Errorf("key derivation failed: %w", err)
	}
	
	// Create cipher
	var err error
	cs.cipher, err = chacha20poly1305.New(key)
	if err != nil {
		return fmt.Errorf("cipher creation failed: %w", err)
	}
	
	cs.established = true
	return nil
}

func (cs *CryptoState) IsEstablished() bool {
	return cs.established
}

func (cs *CryptoState) Encrypt(plaintext []byte) ([]byte, error) {
	if !cs.established {
		return nil, errors.New("crypto not established")
	}
	
	cs.nonceMu.Lock()
	nonce := make([]byte, NonceSize)
	binary.BigEndian.PutUint64(nonce[4:], cs.txNonce)
	cs.txNonce++
	cs.nonceMu.Unlock()
	
	// Allocate output buffer: nonce + ciphertext + tag
	output := make([]byte, NonceSize+len(plaintext)+TagSize)
	copy(output[:NonceSize], nonce)
	
	// Encrypt in-place
	cs.cipher.Seal(output[NonceSize:NonceSize], nonce, plaintext, nil)
	
	return output, nil
}

func (cs *CryptoState) Decrypt(ciphertext []byte) ([]byte, error) {
	if !cs.established {
		return nil, errors.New("crypto not established")
	}
	
	if len(ciphertext) < NonceSize+TagSize {
		return nil, ErrInvalidCiphertext
	}
	
	// Extract nonce
	nonce := ciphertext[:NonceSize]
	
	// Decrypt
	plaintext, err := cs.cipher.Open(nil, nonce, ciphertext[NonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	
	// Update rx nonce for replay protection (optional)
	rxNonce := binary.BigEndian.Uint64(nonce[4:])
	cs.nonceMu.Lock()
	if rxNonce > cs.rxNonce {
		cs.rxNonce = rxNonce
	}
	cs.nonceMu.Unlock()
	
	return plaintext, nil
}

// CryptoTransport wraps Transport with encryption
type CryptoTransport struct {
	*Transport
	cryptoStates map[string]*CryptoState
	cryptoMu     sync.RWMutex
}

func NewCryptoTransport(address string) (*CryptoTransport, error) {
	transport, err := NewTransport(address)
	if err != nil {
		return nil, err
	}
	
	ct := &CryptoTransport{
		Transport:    transport,
		cryptoStates: make(map[string]*CryptoState),
	}
	
	// Override the original handleData to decrypt first
	transport.OnConnect = ct.onConnect
	
	return ct, nil
}

func (ct *CryptoTransport) onConnect(conn *Connection) {
	// Initialize crypto state for new connection
	cs, err := NewCryptoState()
	if err != nil {
		return
	}
	
	ct.cryptoMu.Lock()
	ct.cryptoStates[conn.Addr.String()] = cs
	ct.cryptoMu.Unlock()
	
	// Send public key in handshake
	ct.sendCryptoHandshake(conn, cs)
}

func (ct *CryptoTransport) sendCryptoHandshake(conn *Connection, cs *CryptoState) {
	packet := &Packet{
		Type:    TypeHandshake,
		Flags:   FlagCompressed, // Reuse this flag for crypto handshake
		Payload: cs.GetPublicKey(),
	}
	
	ct.outgoingCh <- &outgoingPacket{
		packet: packet,
		conn:   conn,
	}
}

func (ct *CryptoTransport) SendEncrypted(conn *Connection, data []byte) error {
	ct.cryptoMu.RLock()
	cs, exists := ct.cryptoStates[conn.Addr.String()]
	ct.cryptoMu.RUnlock()
	
	if !exists || !cs.IsEstablished() {
		return errors.New("encryption not established")
	}
	
	// Encrypt the data
	encrypted, err := cs.Encrypt(data)
	if err != nil {
		return err
	}
	
	// Send using underlying transport
	return ct.Send(conn, encrypted)
}

func (ct *CryptoTransport) DecryptPacket(conn *Connection, data []byte) ([]byte, error) {
	ct.cryptoMu.RLock()
	cs, exists := ct.cryptoStates[conn.Addr.String()]
	ct.cryptoMu.RUnlock()
	
	if !exists || !cs.IsEstablished() {
		return nil, errors.New("encryption not established")
	}
	
	return cs.Decrypt(data)
}
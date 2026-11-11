package protocol

import (
	"bytes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"io"
	"math"
	"sync"
)

const (
	NonceSize     = 12
	KeySize       = 32
	TagSize       = 16
	DHKeySize     = 32
	secureVersion = 2
	replayWindow  = 1024
)

var (
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrInvalidPublicKey  = errors.New("invalid public key")
	ErrHandshakeTimeout  = errors.New("handshake timeout")
	ErrPSKRequired       = errors.New("secure v2 requires an explicit random PSK of at least 32 bytes; legacy unauthenticated security is disabled")
	ErrReplay            = errors.New("replayed or expired encrypted packet")
)

// CryptoState implements v2 PSK-authenticated ephemeral X25519. This custom
// protocol requires independent review. Provision unique admission secrets over
// authenticated HTTPS; never embed a game-wide secret in clients. One use/session.
type CryptoState struct {
	mu                      sync.Mutex
	psk                     []byte
	keyID                   string
	initiator               bool
	private                 *ecdh.PrivateKey
	random                  [32]byte
	hello, response, finish []byte
	transcript              [32]byte
	tx, rx                  cipher.AEAD
	confirm                 []byte
	txNonce                 uint64
	exhausted               bool
	rxMax                   uint64
	rxSeen                  bool
	received                [replayWindow / 64]uint64
	established             bool
}

// NewCryptoState fails closed. Use NewCryptoStateWithPSK and the v2 handshake.
func NewCryptoState() (*CryptoState, error) { return nil, ErrPSKRequired }
func NewCryptoStateWithPSK(psk []byte, initiator bool, keyID ...string) (*CryptoState, error) {
	if len(psk) < KeySize {
		return nil, ErrPSKRequired
	}
	id := ""
	if len(keyID) > 0 {
		id = keyID[0]
	}
	if len(id) > 128 {
		return nil, errors.New("key ID too long")
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	cs := &CryptoState{psk: append([]byte(nil), psk...), initiator: initiator, private: key, keyID: id}
	if _, err = io.ReadFull(rand.Reader, cs.random[:]); err != nil {
		return nil, err
	}
	return cs, nil
}
func (cs *CryptoState) GetPublicKey() []byte {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return append([]byte(nil), cs.private.PublicKey().Bytes()...)
}
func (cs *CryptoState) SetRemotePublicKey([]byte) error {
	return errors.New("unauthenticated public-key exchange disabled; use v2 handshake")
}
func mac(key []byte, label string, chunks ...[]byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte("udp-network-v2/" + label + "\x00"))
	for _, b := range chunks {
		h.Write(b)
	}
	return h.Sum(nil)
}
func handshakeKeyID(data []byte) (string, error) {
	if len(data) < 3 || data[0] != secureVersion || data[1] != 1 || int(data[2]) > 128 || len(data) != 3+int(data[2])+64+32 {
		return "", errors.New("invalid v2 client hello")
	}
	return string(data[3 : 3+int(data[2])]), nil
}
func validateHello(psk []byte, id string, data []byte) bool {
	parsed, err := handshakeKeyID(data)
	return err == nil && parsed == id && hmac.Equal(data[len(data)-32:], mac(psk, "client hello", data[:len(data)-32]))
}
func (cs *CryptoState) StartHandshake() ([]byte, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.initiator {
		return nil, errors.New("only initiator sends client hello")
	}
	if cs.hello == nil {
		prefix := []byte{secureVersion, 1, byte(len(cs.keyID))}
		prefix = append(prefix, []byte(cs.keyID)...)
		prefix = append(prefix, cs.random[:]...)
		prefix = append(prefix, cs.private.PublicKey().Bytes()...)
		cs.hello = append(prefix, mac(cs.psk, "client hello", prefix)...)
	}
	return append([]byte(nil), cs.hello...), nil
}

// HandleHandshake is idempotent for retransmission of the same handshake.
func (cs *CryptoState) HandleHandshake(data []byte) ([]byte, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(data) < 2 || data[0] != secureVersion {
		return nil, errors.New("unsupported secure protocol version")
	}
	switch data[1] {
	case 1:
		if cs.initiator || !validateHello(cs.psk, cs.keyID, data) {
			return nil, errors.New("client authentication failed")
		}
		if cs.hello != nil {
			if !bytes.Equal(cs.hello, data) {
				return nil, errors.New("session cannot be rekeyed")
			}
			return append([]byte(nil), cs.response...), nil
		}
		cs.hello = append([]byte(nil), data...)
		prefix := []byte{secureVersion, 2}
		prefix = append(prefix, cs.random[:]...)
		prefix = append(prefix, cs.private.PublicKey().Bytes()...)
		cs.response = append(prefix, mac(cs.psk, "server hello", cs.hello, prefix)...)
		pos := 3 + len(cs.keyID) + 32
		if err := cs.derive(data[pos : pos+32]); err != nil {
			cs.hello, cs.response = nil, nil
			return nil, err
		}
		return append([]byte(nil), cs.response...), nil
	case 2:
		if !cs.initiator || cs.hello == nil || len(data) != 98 || !hmac.Equal(data[66:], mac(cs.psk, "server hello", cs.hello, data[:66])) {
			return nil, errors.New("server authentication failed")
		}
		if cs.response != nil {
			if !bytes.Equal(cs.response, data) {
				return nil, errors.New("session cannot be rekeyed")
			}
			return append([]byte(nil), cs.finish...), nil
		}
		cs.response = append([]byte(nil), data...)
		if err := cs.derive(data[34:66]); err != nil {
			cs.response = nil
			return nil, err
		}
		cs.finish = append([]byte{secureVersion, 3}, mac(cs.confirm, "client finished", cs.transcript[:])...)
		// Receipt of the encrypted server connection handshake confirms server DH.
		cs.established = true
		return append([]byte(nil), cs.finish...), nil
	case 3:
		if cs.initiator || cs.response == nil || cs.tx == nil || cs.rx == nil || len(cs.confirm) != 32 || len(data) != 34 || !hmac.Equal(data[2:], mac(cs.confirm, "client finished", cs.transcript[:])) {
			return nil, errors.New("client key confirmation failed")
		}
		cs.established = true
		return nil, nil
	default:
		return nil, errors.New("unknown handshake stage")
	}
}
func (cs *CryptoState) derive(remote []byte) error {
	pub, err := ecdh.X25519().NewPublicKey(remote)
	if err != nil {
		return ErrInvalidPublicKey
	}
	shared, err := cs.private.ECDH(pub)
	if err != nil {
		return ErrInvalidPublicKey
	}
	cs.transcript = sha256.Sum256(append(append([]byte(nil), cs.hello...), cs.response...))
	input := append(shared, cs.psk...)
	reader := hkdf.New(sha256.New, input, cs.transcript[:], []byte("udp-network-v2/session/client-server/keys"))
	material := make([]byte, 96)
	if _, err = io.ReadFull(reader, material); err != nil {
		return err
	}
	clientKey, serverKey := material[:32], material[32:64]
	if cs.initiator {
		cs.tx, err = chacha20poly1305.New(clientKey)
		cs.rx, _ = chacha20poly1305.New(serverKey)
	} else {
		cs.tx, err = chacha20poly1305.New(serverKey)
		cs.rx, _ = chacha20poly1305.New(clientKey)
	}
	cs.confirm = append([]byte(nil), material[64:]...)
	clear(material)
	clear(input)
	return err
}
func (cs *CryptoState) IsEstablished() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.established
}
func (cs *CryptoState) SessionID() uint64 {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	id := binary.BigEndian.Uint64(cs.transcript[:8])
	if id == 0 {
		id = 1
	}
	return id
}
func (cs *CryptoState) Encrypt(plaintext []byte) ([]byte, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.established || cs.tx == nil {
		return nil, errors.New("crypto not established")
	}
	if cs.exhausted {
		return nil, errors.New("encryption nonce exhausted")
	}
	nonce := make([]byte, NonceSize)
	if cs.initiator {
		nonce[3] = 1
	} else {
		nonce[3] = 2
	}
	binary.BigEndian.PutUint64(nonce[4:], cs.txNonce)
	if cs.txNonce == math.MaxUint64 {
		cs.exhausted = true
	} else {
		cs.txNonce++
	}
	return cs.tx.Seal(nonce, nonce, plaintext, cs.transcript[:]), nil
}
func (cs *CryptoState) Decrypt(ciphertext []byte) ([]byte, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.established || cs.rx == nil {
		return nil, errors.New("crypto not established")
	}
	if len(ciphertext) < NonceSize+TagSize {
		return nil, ErrInvalidCiphertext
	}
	nonce := ciphertext[:NonceSize]
	prefix := byte(1)
	if cs.initiator {
		prefix = 2
	}
	if nonce[0] != 0 || nonce[1] != 0 || nonce[2] != 0 || nonce[3] != prefix {
		return nil, ErrInvalidCiphertext
	}
	n := binary.BigEndian.Uint64(nonce[4:])
	if cs.rxSeen && n <= cs.rxMax && cs.rxMax-n >= replayWindow {
		return nil, ErrReplay
	}
	index, bit := (n%replayWindow)/64, uint(n%64)
	if cs.rxSeen && n <= cs.rxMax && cs.received[index]&(uint64(1)<<bit) != 0 {
		return nil, ErrReplay
	}
	plain, err := cs.rx.Open(nil, nonce, ciphertext[NonceSize:], cs.transcript[:])
	if err != nil {
		return nil, fmt.Errorf("%w: authentication failed", ErrInvalidCiphertext)
	}
	if !cs.rxSeen {
		cs.rxSeen = true
		cs.rxMax = n
	} else if n > cs.rxMax {
		if n-cs.rxMax >= replayWindow {
			clear(cs.received[:])
		} else {
			for x := cs.rxMax + 1; x <= n; x++ {
				cs.received[(x%replayWindow)/64] &^= uint64(1) << uint(x%64)
				if x == math.MaxUint64 {
					break
				}
			}
		}
		cs.rxMax = n
	}
	cs.received[index] |= uint64(1) << bit
	return plain, nil
}

// CryptoTransport was an unauthenticated legacy wrapper and is disabled.
type CryptoTransport struct{ *Transport }

func NewCryptoTransport(string) (*CryptoTransport, error)        { return nil, ErrPSKRequired }
func (*CryptoTransport) SendEncrypted(*Connection, []byte) error { return ErrPSKRequired }
func (*CryptoTransport) DecryptPacket(*Connection, []byte) ([]byte, error) {
	return nil, ErrPSKRequired
}

package protocol

import (
	"bytes"
	"errors"
	"math"
	"sync"
	"testing"
)

var testPSK = bytes.Repeat([]byte{0x42}, 32)

func finishCryptoPair(t testing.TB, a, b *CryptoState) {
	t.Helper()
	hello, err := a.StartHandshake()
	if err != nil {
		t.Fatal(err)
	}
	response, err := b.HandleHandshake(hello)
	if err != nil {
		t.Fatal(err)
	}
	finish, err := a.HandleHandshake(response)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.HandleHandshake(finish); err != nil {
		t.Fatal(err)
	}
}
func cryptoPair(t testing.TB) (*CryptoState, *CryptoState) {
	t.Helper()
	a, err := NewCryptoStateWithPSK(testPSK, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewCryptoStateWithPSK(testPSK, false)
	if err != nil {
		t.Fatal(err)
	}
	finishCryptoPair(t, a, b)
	return a, b
}
func TestLegacyCryptoFailsClosed(t *testing.T) {
	if _, err := NewCryptoState(); !errors.Is(err, ErrPSKRequired) {
		t.Fatal(err)
	}
	if _, err := NewCryptoStateWithPSK([]byte("weak"), true); !errors.Is(err, ErrPSKRequired) {
		t.Fatal(err)
	}
	a, _ := NewCryptoStateWithPSK(testPSK, true)
	if err := a.SetRemotePublicKey(a.GetPublicKey()); err == nil {
		t.Fatal("bare key accepted")
	}
}
func TestCryptoBidirectionalKeysAndNonces(t *testing.T) {
	a, b := cryptoPair(t)
	p := []byte("same payload")
	x, _ := a.Encrypt(p)
	y, _ := b.Encrypt(p)
	if bytes.Equal(x[:NonceSize], y[:NonceSize]) || bytes.Equal(x, y) {
		t.Fatal("bidirectional nonce/key reuse")
	}
	if _, err := a.Decrypt(x); err == nil {
		t.Fatal("reflection accepted")
	}
	for _, tc := range []struct {
		state *CryptoState
		data  []byte
	}{{b, x}, {a, y}} {
		got, err := tc.state.Decrypt(tc.data)
		if err != nil || !bytes.Equal(got, p) {
			t.Fatalf("%x %v", got, err)
		}
	}
	if a.SessionID() != b.SessionID() {
		t.Fatal("session IDs differ")
	}
}
func TestCryptoHandshakeAuthenticationAndVersion(t *testing.T) {
	a, _ := NewCryptoStateWithPSK(testPSK, true, "ticket")
	b, _ := NewCryptoStateWithPSK(testPSK, false, "ticket")
	hello, _ := a.StartHandshake()
	bad := append([]byte(nil), hello...)
	bad[len(bad)-1] ^= 1
	if _, err := b.HandleHandshake(bad); err == nil {
		t.Fatal("modified hello accepted")
	}
	wrong, _ := NewCryptoStateWithPSK(bytes.Repeat([]byte{3}, 32), false, "ticket")
	if _, err := wrong.HandleHandshake(hello); err == nil {
		t.Fatal("wrong PSK accepted")
	}
	legacy := append([]byte(nil), hello...)
	legacy[0] = 1
	if _, err := b.HandleHandshake(legacy); err == nil {
		t.Fatal("legacy accepted")
	}
	response, err := b.HandleHandshake(hello)
	if err != nil {
		t.Fatal(err)
	}
	if b.IsEstablished() {
		t.Fatal("server established without confirmation")
	}
	bad = append([]byte(nil), response...)
	bad[35] ^= 1
	if _, err = a.HandleHandshake(bad); err == nil {
		t.Fatal("modified public key accepted")
	}
	finish, err := a.HandleHandshake(response)
	if err != nil {
		t.Fatal(err)
	}
	bad = append([]byte(nil), finish...)
	bad[3] ^= 1
	if _, err = b.HandleHandshake(bad); err == nil {
		t.Fatal("bad confirmation accepted")
	}
	if _, err = b.HandleHandshake(finish); err != nil {
		t.Fatal(err)
	}
	again, err := a.HandleHandshake(response)
	if err != nil || !bytes.Equal(again, finish) {
		t.Fatal("handshake retry changed state")
	}
	c, _ := NewCryptoStateWithPSK(testPSK, false, "ticket")
	other, err := c.HandleHandshake(hello)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(response, other) {
		t.Fatal("session randomness missing")
	}
	if _, err = a.HandleHandshake(other); err == nil {
		t.Fatal("session rekey accepted")
	}
}
func TestCryptoKeyFirstByteCollision(t *testing.T) {
	a, _ := NewCryptoStateWithPSK(testPSK, true)
	var b *CryptoState
	for i := 0; i < 10000; i++ {
		b, _ = NewCryptoStateWithPSK(testPSK, false)
		if a.GetPublicKey()[0] == b.GetPublicKey()[0] {
			break
		}
		b = nil
	}
	if b == nil {
		t.Fatal("could not generate colliding prefix")
	}
	finishCryptoPair(t, a, b)
	ct, _ := a.Encrypt([]byte("works"))
	if _, err := b.Decrypt(ct); err != nil {
		t.Fatal(err)
	}
}
func TestCryptoInvalidDHAndMalformedHandshakes(t *testing.T) {
	a, _ := NewCryptoStateWithPSK(testPSK, true)
	b, _ := NewCryptoStateWithPSK(testPSK, false)
	for _, data := range [][]byte{nil, {2}, {2, 1}, {2, 2}, {2, 3}, make([]byte, 200)} {
		if _, err := b.HandleHandshake(data); err == nil {
			t.Fatal("malformed handshake accepted")
		}
	}
	hello, _ := a.StartHandshake()
	bad := append([]byte(nil), hello...)
	clear(bad[35:67])
	copy(bad[len(bad)-32:], mac(testPSK, "client hello", bad[:len(bad)-32]))
	if _, err := b.HandleHandshake(bad); err == nil {
		t.Fatal("low-order X25519 key accepted")
	}
	forgedFinish := append([]byte{2, 3}, mac(nil, "client finished", make([]byte, 32))...)
	if _, err := b.HandleHandshake(forgedFinish); err == nil || b.IsEstablished() {
		t.Fatal("failed DH allowed key confirmation")
	}
	finishCryptoPair(t, a, b)
}
func TestCryptoReplayWindowAndAuthentication(t *testing.T) {
	a, b := cryptoPair(t)
	packets := make([][]byte, replayWindow+2)
	for i := range packets {
		packets[i], _ = a.Encrypt([]byte{byte(i)})
	}
	for _, i := range []int{3, 1, 2, 0} {
		if _, err := b.Decrypt(packets[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Decrypt(packets[1]); !errors.Is(err, ErrReplay) {
		t.Fatal("replay accepted", err)
	}
	bad := append([]byte(nil), packets[len(packets)-1]...)
	bad[len(bad)-1] ^= 1
	if _, err := b.Decrypt(bad); err == nil {
		t.Fatal("tamper accepted")
	}
	if _, err := b.Decrypt(packets[4]); err != nil {
		t.Fatal("invalid packet advanced window", err)
	}
	if _, err := b.Decrypt(packets[len(packets)-1]); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Decrypt(packets[0]); !errors.Is(err, ErrReplay) {
		t.Fatal("old packet accepted", err)
	}
	for _, data := range [][]byte{nil, {}, make([]byte, 27)} {
		if _, err := b.Decrypt(data); err == nil {
			t.Fatal("short packet accepted")
		}
	}
}
func TestCryptoConcurrentAndExhaustion(t *testing.T) {
	a, b := cryptoPair(t)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ct, err := a.Encrypt([]byte("parallel"))
			if err != nil {
				t.Error(err)
				return
			}
			if _, err = b.Decrypt(ct); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	a.mu.Lock()
	a.txNonce = math.MaxUint64
	a.mu.Unlock()
	if _, err := a.Encrypt(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Encrypt(nil); err == nil {
		t.Fatal("nonce wrapped")
	}
}
func BenchmarkCryptoEncrypt(b *testing.B) {
	a, _ := cryptoPair(b)
	data := make([]byte, 1400)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := a.Encrypt(data); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCryptoDecrypt(b *testing.B) {
	a, receiver := cryptoPair(b)
	data := make([]byte, 1400)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; {
		b.StopTimer()
		n := min(256, b.N-i)
		packets := make([][]byte, n)
		for j := range packets {
			packets[j], _ = a.Encrypt(data)
		}
		b.StartTimer()
		for _, ct := range packets {
			if _, err := receiver.Decrypt(ct); err != nil {
				b.Fatal(err)
			}
		}
		i += n
	}
}
func BenchmarkKeyExchange(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cryptoPair(b)
	}
}

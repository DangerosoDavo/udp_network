package protocol

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func secureTransportPair(t *testing.T) (*SecureTransport, *SecureTransport, *Connection, *Connection, chan []byte) {
	t.Helper()
	server, err := NewSecureTransportWithPSK("127.0.0.1:0", testPSK)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	client, err := NewSecureTransportWithPSK("127.0.0.1:0", testPSK)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Stop)
	messages := make(chan []byte, 32)
	joined := make(chan *Connection, 1)
	server.OnConnect = func(c *Connection) { c.SetMessageCallback(func(b []byte) { messages <- b }); joined <- c }
	// Embedded lifecycle APIs must retain the secure processors too.
	server.Transport.Start()
	c, err := client.SecureConnect(server.conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	var s *Connection
	select {
	case s = <-joined:
	case <-time.After(time.Second):
		t.Fatal("server callback missing")
	}
	return server, client, s, c, messages
}
func TestSecureTransportLoopbackAndPlaintextRefusal(t *testing.T) {
	server, client, s, c, messages := secureTransportPair(t)
	data := bytes.Repeat([]byte("modular spaceship"), 400)
	if err := client.Transport.Send(c, data); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-messages:
		if !bytes.Equal(got, data) {
			t.Fatal("fragmented payload changed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("secure delivery timed out")
	}
	// The deprecated switch cannot downgrade security, and forged ACK/close/data
	// packets must not reach the underlying reliability or application handlers.
	server.RequireEncryption = false
	s.sendWindowMu.Lock()
	s.sendWindow[333] = &Packet{Type: TypeData, SeqNum: 333}
	s.sendWindowMu.Unlock()
	addr := client.conn.LocalAddr().(*net.UDPAddr)
	for _, p := range []*Packet{{Type: TypeData, SessionID: s.SessionID(), Payload: []byte("plaintext")}, {Type: TypeAck, SessionID: s.SessionID(), AckNum: 333}, {Type: TypeClose, SessionID: s.SessionID()}} {
		server.handlePacketSecure(p, addr)
	}
	s.sendWindowMu.RLock()
	pending := s.sendWindow[333] != nil
	s.sendWindowMu.RUnlock()
	if !pending || s.GetState() != StateConnected {
		t.Fatal("plaintext control accepted")
	}
	s.removeFromSendWindow(333)
	select {
	case got := <-messages:
		t.Fatalf("plaintext delivered: %q", got)
	case <-time.After(40 * time.Millisecond):
	}
}

func TestSecureHandshakeAndDataAcrossLoss(t *testing.T) {
	server, err := NewSecureTransportWithPSK("127.0.0.1:0", testPSK)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	client, err := NewSecureTransportWithPSK("127.0.0.1:0", testPSK)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Stop()
	var connects, plaintext atomic.Int32
	messages := make(chan string, 8)
	server.OnConnect = func(c *Connection) { connects.Add(1); c.SetMessageCallback(func(b []byte) { messages <- string(b) }) }
	server.Start()
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	defer func() { relay.Close(); <-stopped }()
	var dataPhase atomic.Bool
	go func() {
		defer close(stopped)
		buf := make([]byte, 2048)
		var peer *net.UDPAddr
		dropped := make(map[string]bool)
		for {
			n, addr, err := relay.ReadFromUDP(buf)
			if err != nil {
				return
			}
			p, err := Unmarshal(buf[:n])
			if err != nil {
				continue
			}
			fromServer := addr.String() == server.conn.LocalAddr().String()
			dest := server.conn.LocalAddr().(*net.UDPAddr)
			direction := "client"
			if fromServer {
				dest = peer
				direction = "server"
			} else {
				peer = addr
			}
			if dest == nil {
				continue
			}
			key := ""
			if p.Type == TypeCryptoHandshake && len(p.Payload) > 1 {
				key = direction + string([]byte{p.Payload[1]})
			} else if p.Type == TypeEncrypted {
				key = direction + "encrypted-handshake"
				if dataPhase.Load() {
					key = direction + "encrypted-data"
				}
			} else {
				plaintext.Add(1)
			}
			if key != "" && !dropped[key] {
				dropped[key] = true
				continue
			}
			_, _ = relay.WriteToUDP(buf[:n], dest)
			if dataPhase.Load() && p.Type == TypeEncrypted && !fromServer {
				_, _ = relay.WriteToUDP(buf[:n], dest)
			}
		}
	}()
	c, err := client.SecureConnect(relay.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	dataPhase.Store(true)
	if err = client.SendSecure(c, []byte("retry safe")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-messages:
		if got != "retry safe" {
			t.Fatal(got)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("loss recovery failed")
	}
	select {
	case <-messages:
		t.Fatal("duplicate application delivery")
	case <-time.After(150 * time.Millisecond):
	}
	if connects.Load() != 1 {
		t.Fatal("duplicate connection callback", connects.Load())
	}
	if plaintext.Load() != 0 {
		t.Fatal("plaintext data/control on secure wire")
	}
}
func TestSecurePacketTamperingAndReplay(t *testing.T) {
	server, client, s, c, messages := secureTransportPair(t)
	addr := client.conn.LocalAddr().(*net.UDPAddr)
	peer := client.peer(server.conn.LocalAddr().String())
	raw, err := (&Packet{Type: TypeData, SessionID: s.SessionID(), SeqNum: 0, Payload: []byte("authenticated")}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	ct, err := peer.crypto.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	outer := &Packet{Type: TypeEncrypted, SessionID: c.SessionID(), Payload: ct}
	// Outer routing fields have a single canonical representation; all semantic
	// inner fields (sequence, ACK, flags, fragments, session ID) are AEAD-protected.
	modified := *outer
	modified.SeqNum = 9
	server.handlePacketSecure(&modified, addr)
	modified = *outer
	modified.SessionID ^= 1
	server.handlePacketSecure(&modified, addr)
	modified = *outer
	modified.Payload = append([]byte(nil), ct...)
	modified.Payload[NonceSize+4] ^= 1
	server.handlePacketSecure(&modified, addr)
	select {
	case <-messages:
		t.Fatal("tampered packet delivered")
	case <-time.After(20 * time.Millisecond):
	}
	server.handlePacketSecure(outer, addr)
	select {
	case got := <-messages:
		if string(got) != "authenticated" {
			t.Fatal(string(got))
		}
	case <-time.After(time.Second):
		t.Fatal("valid packet rejected after tampering")
	}
	server.handlePacketSecure(outer, addr)
	// A reliability retry reencrypts with a new crypto nonce, while base reliable
	// sequence suppression ensures no repeat application delivery.
	ct2, _ := peer.crypto.Encrypt(raw)
	server.handlePacketSecure(&Packet{Type: TypeEncrypted, SessionID: c.SessionID(), Payload: ct2}, addr)
	select {
	case <-messages:
		t.Fatal("replay/retry delivered twice")
	case <-time.After(30 * time.Millisecond):
	}
}
func TestSecureUnknownPacketsAndAdmissionResolver(t *testing.T) {
	if _, err := NewSecureTransport("127.0.0.1:0"); !errors.Is(err, ErrPSKRequired) {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server, err := NewSecureTransportWithConfig("127.0.0.1:0", SecureConfig{LookupPSK: func(id string) ([]byte, error) {
		calls.Add(1)
		if id != "account-session-ticket" {
			return nil, errors.New("expired/unknown admission")
		}
		return testPSK, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	joined := make(chan *Connection, 4)
	server.OnConnect = func(c *Connection) { joined <- c }
	server.Start()
	unknown := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
	server.handlePacketSecure(&Packet{Type: TypeData, Payload: []byte("old peer")}, unknown)
	server.handlePacketSecure(&Packet{Type: TypeCryptoHandshake, Payload: make([]byte, 32)}, unknown)
	if server.GetConnectionCount() != 0 || len(server.peers) != 0 || calls.Load() != 0 {
		t.Fatal("unauthenticated peer allocated state")
	}
	pendingCrypto, _ := NewCryptoStateWithPSK(testPSK, true, "account-session-ticket")
	pendingHello, _ := pendingCrypto.StartHandshake()
	server.handleCryptoHandshake(unknown, pendingHello)
	pending := server.peer(unknown.String())
	if pending == nil {
		t.Fatal("missing pending authenticated handshake")
	}
	if _, ok := server.PeerKeyID(pending.conn); ok {
		t.Fatal("pending handshake exposed account identity")
	}
	client, err := NewSecureTransportWithConfig("127.0.0.1:0", SecureConfig{PSK: testPSK, KeyID: "account-session-ticket"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Stop()
	if _, err = client.SecureConnect(server.conn.LocalAddr().String()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() == 0 {
		t.Fatal("admission resolver not consulted")
	}
	var bound *Connection
	select {
	case bound = <-joined:
	case <-time.After(time.Second):
		t.Fatal("missing authenticated callback")
	}
	if id, ok := server.PeerKeyID(bound); !ok || id != "account-session-ticket" {
		t.Fatal("authenticated identity missing", id, ok)
	}
	replacement := NewConnection(bound.Addr)
	replacement.setSessionID(bound.SessionID())
	replacement.SetState(StateConnected)
	if _, ok := server.PeerKeyID(replacement); ok {
		t.Fatal("same-address replacement inherited identity")
	}
	// Previously authenticated hellos can be resent, but cannot reset a live
	// session's keys, identity, nonce state, or connection callback.
	sender := client.peer(server.conn.LocalAddr().String())
	hello, _ := sender.crypto.StartHandshake()
	originalPeer := server.peer(bound.Addr.String())
	originalSession := bound.SessionID()
	server.handleCryptoHandshake(bound.Addr, hello)
	other, _ := NewCryptoStateWithPSK(testPSK, true, "account-session-ticket")
	otherHello, _ := other.StartHandshake()
	server.handleCryptoHandshake(bound.Addr, otherHello)
	if server.peer(bound.Addr.String()) != originalPeer || bound.SessionID() != originalSession {
		t.Fatal("replayed hello replaced live session")
	}
	select {
	case <-joined:
		t.Fatal("replayed hello reset callback")
	case <-time.After(20 * time.Millisecond):
	}
	bound.SetState(StateClosed)
	if _, ok := server.PeerKeyID(bound); ok {
		t.Fatal("closed connection retained authenticated identity")
	}
	bad, err := NewSecureTransportWithPSK("127.0.0.1:0", bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err = bad.SecureConnectContext(ctx, server.conn.LocalAddr().String()); err == nil {
		t.Fatal("wrong secret admitted")
	}
	if bad.GetConnectionCount() != 0 {
		t.Fatal("failed connect leaked connection")
	}
}

package protocol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// SecureConfig selects an admission secret. For games, use KeyID + LookupPSK
// backed by short-lived account/session-bound tickets issued over HTTPS. A
// static PSK is only suitable for trusted deployments/prototypes. LookupPSK
// runs on the receive processor and must be bounded, fast and concurrency safe.
type SecureConfig struct {
	PSK       []byte
	KeyID     string
	LookupPSK func(keyID string) ([]byte, error)
}
type securePeer struct {
	crypto  *CryptoState
	conn    *Connection
	created time.Time
	mu      sync.Mutex
	flight  []byte
	bound   sync.Once
}

func (p *securePeer) setFlight(b []byte) {
	p.mu.Lock()
	p.flight = append([]byte(nil), b...)
	p.mu.Unlock()
}
func (p *securePeer) getFlight() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.flight...)
}

// SecureTransport always authenticates/encrypts application and control packets.
// v1 and plaintext packets are rejected, even when RequireEncryption is false.
type SecureTransport struct {
	*Transport
	cryptoMu          sync.RWMutex
	peers             map[string]*securePeer
	config            SecureConfig
	RequireEncryption bool // Deprecated: v2 always requires encryption.
}

func NewSecureTransport(string) (*SecureTransport, error) { return nil, ErrPSKRequired }
func NewSecureTransportWithPSK(address string, psk []byte) (*SecureTransport, error) {
	return NewSecureTransportWithConfig(address, SecureConfig{PSK: psk})
}
func NewSecureTransportWithConfig(address string, config SecureConfig) (*SecureTransport, error) {
	if len(config.KeyID) > 128 {
		return nil, errors.New("key ID too long")
	}
	if len(config.PSK) < KeySize && config.LookupPSK == nil {
		return nil, ErrPSKRequired
	}
	transport, err := NewTransport(address)
	if err != nil {
		return nil, err
	}
	config.PSK = append([]byte(nil), config.PSK...)
	st := &SecureTransport{Transport: transport, peers: make(map[string]*securePeer), config: config, RequireEncryption: true}
	transport.processorIncoming = st.processIncomingSecure
	transport.processorOutgoing = st.processOutgoingSecure
	return st, nil
}
func (st *SecureTransport) Start()              { st.Transport.Start() }
func (st *SecureTransport) LocalAddr() net.Addr { return st.conn.LocalAddr() }
func (st *SecureTransport) Stop() {
	st.Transport.Stop()
	st.cryptoMu.Lock()
	for _, p := range st.peers {
		p.conn.SetState(StateClosed)
	}
	st.peers = make(map[string]*securePeer)
	st.cryptoMu.Unlock()
}
func (st *SecureTransport) peer(addr string) *securePeer {
	st.cryptoMu.RLock()
	defer st.cryptoMu.RUnlock()
	return st.peers[addr]
}
func (st *SecureTransport) processIncomingSecure() {
	defer st.wg.Done()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-st.ctx.Done():
			return
		case <-tick.C:
			st.prunePeers()
		case in := <-st.incomingCh:
			st.handlePacketSecure(in.packet, in.addr)
		}
	}
}
func (st *SecureTransport) prunePeers() {
	st.cryptoMu.Lock()
	defer st.cryptoMu.Unlock()
	for addr, p := range st.peers {
		state := p.conn.GetState()
		if state == StateClosed || (state != StateConnected && time.Since(p.created) > 10*time.Second) {
			p.conn.SetState(StateClosed)
			delete(st.peers, addr)
			continue
		}
		if state == StateConnected {
			st.connMu.RLock()
			active := st.connections[addr] == p.conn
			st.connMu.RUnlock()
			if !active {
				delete(st.peers, addr)
			}
		}
	}
}
func canonicalEnvelope(p *Packet) bool {
	return p.Flags == 0 && p.SeqNum == 0 && p.AckNum == 0 && p.FragmentID == 0 && p.FragOffset == 0 && p.FragTotal == 0
}
func (st *SecureTransport) handlePacketSecure(packet *Packet, addr *net.UDPAddr) {
	if !canonicalEnvelope(packet) {
		return
	}
	if packet.Type == TypeCryptoHandshake {
		if packet.SessionID == 0 {
			st.handleCryptoHandshake(addr, packet.Payload)
		}
		return
	}
	if packet.Type != TypeEncrypted {
		return
	}
	peer := st.peer(addr.String())
	if peer == nil || peer.conn.GetState() == StateClosed || !peer.crypto.IsEstablished() || packet.SessionID != peer.crypto.SessionID() {
		return
	}
	data, err := peer.crypto.Decrypt(packet.Payload)
	if err != nil {
		return
	}
	inner, err := Unmarshal(data)
	if err != nil || inner.Type == TypeEncrypted || inner.Type == TypeCryptoHandshake || inner.SessionID != packet.SessionID {
		return
	}
	st.Transport.handlePacket(inner, addr)
}
func (st *SecureTransport) handleCryptoHandshake(addr *net.UDPAddr, data []byte) {
	peer := st.peer(addr.String())
	if peer == nil {
		id, err := handshakeKeyID(data)
		if err != nil {
			return
		}
		psk := st.config.PSK
		if st.config.LookupPSK != nil {
			psk, err = st.config.LookupPSK(id)
			if err != nil {
				return
			}
		} else if id != st.config.KeyID {
			return
		}
		if len(psk) < KeySize || !validateHello(psk, id, data) {
			return
		}
		// Reject unauthenticated traffic before expensive key generation/allocation.
		st.cryptoMu.Lock()
		if len(st.peers) >= 1024 {
			st.cryptoMu.Unlock()
			return
		}
		st.cryptoMu.Unlock()
		cs, err := NewCryptoStateWithPSK(psk, false, id)
		if err != nil {
			return
		}
		peer = &securePeer{crypto: cs, conn: NewConnection(addr), created: time.Now()}
		st.cryptoMu.Lock()
		if current := st.peers[addr.String()]; current != nil {
			peer = current
		} else {
			st.peers[addr.String()] = peer
		}
		st.cryptoMu.Unlock()
	}
	response, err := peer.crypto.HandleHandshake(data)
	if err != nil {
		return
	}
	if peer.conn.GetState() == StateClosed {
		return
	}
	if len(response) > 0 {
		peer.setFlight(response)
		st.sendHandshake(peer, response)
	}
	if peer.crypto.IsEstablished() {
		peer.bound.Do(func() { st.bindConnection(peer.conn) })
		peer.conn.setSessionID(peer.crypto.SessionID())
		st.connMu.Lock()
		if current := st.connections[addr.String()]; current == nil {
			if st.maxConns > 0 && len(st.connections) >= st.maxConns {
				st.connMu.Unlock()
				peer.conn.SetState(StateClosed)
				return
			}
			st.connections[addr.String()] = peer.conn
		}
		st.connMu.Unlock()
	}
}
func (st *SecureTransport) sendHandshake(peer *securePeer, payload []byte) {
	packet := &Packet{Type: TypeCryptoHandshake, Payload: payload}
	data, err := packet.Marshal()
	if err == nil {
		_, _ = st.conn.WriteToUDP(data, peer.conn.Addr)
	}
}
func (st *SecureTransport) processOutgoingSecure() {
	defer st.wg.Done()
	for {
		select {
		case <-st.ctx.Done():
			return
		case out := <-st.outgoingCh:
			peer := st.peer(out.conn.Addr.String())
			if peer != nil && peer.conn == out.conn {
				if err := st.sendInner(peer, out.packet); err != nil {
					st.removeConnection(out.conn)
				}
			}
		}
	}
}
func (st *SecureTransport) sendInner(peer *securePeer, packet *Packet) error {
	if !peer.crypto.IsEstablished() {
		return errors.New("encryption not established")
	}
	data, err := packet.Marshal()
	if err != nil {
		return err
	}
	encrypted, err := peer.crypto.Encrypt(data)
	if err != nil {
		return err
	}
	outer := &Packet{Type: TypeEncrypted, SessionID: peer.crypto.SessionID(), Payload: encrypted}
	wire, err := outer.Marshal()
	if err != nil {
		return err
	}
	_, err = st.conn.WriteToUDP(wire, peer.conn.Addr)
	return err
}
func (st *SecureTransport) SecureConnect(address string) (*Connection, error) {
	ctx, cancel := context.WithTimeout(st.ctx, 5*time.Second)
	defer cancel()
	return st.SecureConnectContext(ctx, address)
}
func (st *SecureTransport) SecureConnectContext(ctx context.Context, address string) (*Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := st.ctx.Err(); err != nil {
		return nil, err
	}
	if len(st.config.PSK) < KeySize {
		return nil, ErrPSKRequired
	}
	st.Start()
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	if addr.IP == nil || addr.IP.IsUnspecified() {
		return nil, errors.New("connect requires a concrete remote address")
	}
	cs, err := NewCryptoStateWithPSK(st.config.PSK, true, st.config.KeyID)
	if err != nil {
		return nil, err
	}
	peer := &securePeer{crypto: cs, conn: NewConnection(addr), created: time.Now()}
	peer.conn.SetState(StateHandshaking)
	peer.bound.Do(func() { st.bindConnection(peer.conn) })
	hello, err := cs.StartHandshake()
	if err != nil {
		return nil, err
	}
	peer.setFlight(hello)
	st.cryptoMu.Lock()
	if len(st.peers) >= 1024 {
		st.cryptoMu.Unlock()
		peer.conn.SetState(StateClosed)
		return nil, errors.New("secure peer limit")
	}
	if _, exists := st.peers[addr.String()]; exists {
		st.cryptoMu.Unlock()
		peer.conn.SetState(StateClosed)
		return nil, errors.New("secure peer already exists")
	}
	st.peers[addr.String()] = peer
	st.cryptoMu.Unlock()
	success := false
	defer func() {
		if !success {
			st.cryptoMu.Lock()
			if st.peers[addr.String()] == peer {
				delete(st.peers, addr.String())
			}
			st.cryptoMu.Unlock()
			st.removeConnection(peer.conn)
		}
	}()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if peer.conn.GetState() == StateClosed {
			return nil, errors.New("connection rejected or closed")
		}
		st.sendHandshake(peer, peer.getFlight())
		if cs.IsEstablished() {
			peer.conn.setSessionID(cs.SessionID())
			if err = st.sendInner(peer, &Packet{Type: TypeHandshake, SessionID: cs.SessionID()}); err != nil {
				return nil, err
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %v", ErrHandshakeTimeout, ctx.Err())
		case <-st.ctx.Done():
			return nil, errors.New("transport stopped")
		case <-tick.C:
		}
		if peer.conn.GetState() == StateConnected {
			success = true
			return peer.conn, nil
		}
	}
}
func (st *SecureTransport) SendSecure(conn *Connection, data []byte) error {
	return st.SendSecureContext(st.ctx, conn, data)
}
func (st *SecureTransport) SendSecureContext(ctx context.Context, conn *Connection, data []byte) error {
	if conn == nil {
		return errors.New("nil connection")
	}
	peer := st.peer(conn.Addr.String())
	if peer == nil || peer.conn != conn || !peer.crypto.IsEstablished() {
		return errors.New("encryption not established")
	}
	return st.Transport.SendContext(ctx, conn, data)
}

// PeerKeyID returns the authenticated admission identifier for this exact live
// connection. Applications can map the opaque identifier to account/session
// authorization. Pending, closed, and replaced connections have no identity.
func (st *SecureTransport) PeerKeyID(conn *Connection) (string, bool) {
	if conn == nil || conn.Addr == nil {
		return "", false
	}
	st.cryptoMu.RLock()
	defer st.cryptoMu.RUnlock()
	peer := st.peers[conn.Addr.String()]
	if peer == nil || peer.conn != conn {
		return "", false
	}
	st.connMu.RLock()
	defer st.connMu.RUnlock()
	if st.connections[conn.Addr.String()] != conn || conn.GetState() != StateConnected || !peer.crypto.IsEstablished() || conn.SessionID() != peer.crypto.SessionID() {
		return "", false
	}
	return peer.crypto.keyID, true
}

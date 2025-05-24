package protocol

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// SecureTransport provides encrypted UDP communication
type SecureTransport struct {
	*Transport
	cryptoStates map[string]*CryptoState
	cryptoMu     sync.RWMutex

	// Configuration
	RequireEncryption bool // If true, reject unencrypted data packets
}

func NewSecureTransport(address string) (*SecureTransport, error) {
	transport, err := NewTransport(address)
	if err != nil {
		return nil, err
	}

	st := &SecureTransport{
		Transport:         transport,
		cryptoStates:      make(map[string]*CryptoState),
		RequireEncryption: true,
	}

	// Override the processIncoming to handle crypto packets
	return st, nil
}

func (st *SecureTransport) Start() {
	st.wg.Add(4)
	go st.readLoop()
	go st.processIncomingSecure() // Use our secure processor
	go st.processOutgoing()
	go st.maintenanceLoop()
}

func (st *SecureTransport) processIncomingSecure() {
	defer st.wg.Done()

	for {
		select {
		case <-st.ctx.Done():
			return
		case in := <-st.incomingCh:
			st.handlePacketSecure(in.packet, in.addr)
		}
	}
}

func (st *SecureTransport) handlePacketSecure(packet *Packet, addr *net.UDPAddr) {
	// Get or create connection
	st.connMu.Lock()
	conn, exists := st.connections[addr.String()]
	if !exists {
		if st.maxConns > 0 && len(st.connections) >= st.maxConns {
			st.connMu.Unlock()
			return
		}
		conn = NewConnection(addr)
		st.connections[addr.String()] = conn
	}
	st.connMu.Unlock()

	conn.lastActivity = time.Now()

	// Handle different packet types
	switch packet.Type {
	case TypeCryptoHandshake:
		st.handleCryptoHandshake(conn, packet)

	case TypeHandshake:
		// Only handle handshake if crypto is established
		st.cryptoMu.RLock()
		cs, hasCrypto := st.cryptoStates[conn.Addr.String()]
		st.cryptoMu.RUnlock()

		if hasCrypto && cs.IsEstablished() {
			// Use parent's handshake handler
			st.Transport.handleHandshake(conn, packet)
		}

	case TypeEncrypted:
		st.handleEncrypted(conn, packet)

	default:
		// Handle other packet types normally
		st.Transport.handlePacket(packet, addr)
	}
}

func (st *SecureTransport) handleCryptoHandshake(conn *Connection, packet *Packet) {
	// Get or create crypto state
	st.cryptoMu.Lock()
	cs, exists := st.cryptoStates[conn.Addr.String()]
	if !exists {
		var err error
		cs, err = NewCryptoState()
		if err != nil {
			st.cryptoMu.Unlock()
			return
		}
		st.cryptoStates[conn.Addr.String()] = cs
	}
	st.cryptoMu.Unlock()

	// Process the public key
	if len(packet.Payload) == DHKeySize {
		if err := cs.SetRemotePublicKey(packet.Payload); err != nil {
			return
		}

		// If we haven't sent our key yet (server side), send it
		if !exists {
			response := &Packet{
				Type:    TypeCryptoHandshake,
				Payload: cs.GetPublicKey(),
			}
			st.outgoingCh <- &outgoingPacket{
				packet: response,
				conn:   conn,
			}
		}
	}
}

func (st *SecureTransport) handleEncrypted(conn *Connection, packet *Packet) {
	st.cryptoMu.RLock()
	cs, exists := st.cryptoStates[conn.Addr.String()]
	st.cryptoMu.RUnlock()

	if !exists || !cs.IsEstablished() {
		return
	}

	// Decrypt the payload
	decrypted, err := cs.Decrypt(packet.Payload)
	if err != nil {
		return
	}

	// Create a new packet with decrypted data
	decryptedPacket := &Packet{
		Type:       TypeData,
		Flags:      packet.Flags &^ FlagEncrypted,
		SeqNum:     packet.SeqNum,
		AckNum:     packet.AckNum,
		FragmentID: packet.FragmentID,
		FragOffset: packet.FragOffset,
		FragTotal:  packet.FragTotal,
		Payload:    decrypted,
		Timestamp:  packet.Timestamp,
	}

	// Process as normal data packet
	st.Transport.handleData(conn, decryptedPacket)
}

// SecureConnect establishes an encrypted connection
func (st *SecureTransport) SecureConnect(address string) (*Connection, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	// Create connection and crypto state
	conn := NewConnection(addr)
	cs, err := NewCryptoState()
	if err != nil {
		return nil, err
	}

	// Register connection and crypto state
	st.connMu.Lock()
	st.connections[addr.String()] = conn
	st.connMu.Unlock()

	st.cryptoMu.Lock()
	st.cryptoStates[addr.String()] = cs
	st.cryptoMu.Unlock()

	// Phase 1: Exchange crypto keys
	cryptoPacket := &Packet{
		Type:    TypeCryptoHandshake,
		Payload: cs.GetPublicKey(),
	}
	st.outgoingCh <- &outgoingPacket{
		packet: cryptoPacket,
		conn:   conn,
	}

	// Wait for crypto to be established
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cs.IsEstablished() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !cs.IsEstablished() {
		return nil, fmt.Errorf("crypto handshake timeout")
	}

	// Phase 2: Normal connection handshake
	conn.state = StateHandshaking
	handshakePacket := &Packet{
		Type:   TypeHandshake,
		SeqNum: 0,
	}
	st.outgoingCh <- &outgoingPacket{
		packet: handshakePacket,
		conn:   conn,
	}

	// Wait for connection to be established
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if conn.state == StateConnected {
			return conn, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return nil, fmt.Errorf("connection handshake timeout")
}

// SendSecure sends encrypted data
func (st *SecureTransport) SendSecure(conn *Connection, data []byte) error {
	if conn.state != StateConnected {
		return fmt.Errorf("connection not established")
	}

	st.cryptoMu.RLock()
	cs, exists := st.cryptoStates[conn.Addr.String()]
	st.cryptoMu.RUnlock()

	if !exists || !cs.IsEstablished() {
		return fmt.Errorf("encryption not established")
	}

	// Encrypt the data
	encrypted, err := cs.Encrypt(data)
	if err != nil {
		return err
	}

	// Send as encrypted packet
	packet := &Packet{
		Type:      TypeEncrypted,
		Flags:     FlagEncrypted,
		SeqNum:    conn.nextSeqNum,
		Payload:   encrypted,
		Timestamp: time.Now(),
	}

	conn.nextSeqNum++

	st.outgoingCh <- &outgoingPacket{
		packet: packet,
		conn:   conn,
	}

	return nil
}

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
	
	// Store original callbacks
	originalOnConnect := transport.OnConnect
	originalOnDisconnect := transport.OnDisconnect
	
	// Override callbacks to handle crypto
	transport.OnConnect = func(conn *Connection) {
		st.initCrypto(conn)
		if originalOnConnect != nil {
			originalOnConnect(conn)
		}
	}
	
	transport.OnDisconnect = func(conn *Connection) {
		st.cleanupCrypto(conn)
		if originalOnDisconnect != nil {
			originalOnDisconnect(conn)
		}
	}
	
	return st, nil
}

func (st *SecureTransport) initCrypto(conn *Connection) {
	cs, err := NewCryptoState()
	if err != nil {
		return
	}
	
	st.cryptoMu.Lock()
	st.cryptoStates[conn.Addr.String()] = cs
	st.cryptoMu.Unlock()
}

func (st *SecureTransport) cleanupCrypto(conn *Connection) {
	st.cryptoMu.Lock()
	delete(st.cryptoStates, conn.Addr.String())
	st.cryptoMu.Unlock()
}

// Override handlePacket to support crypto handshake
func (st *SecureTransport) handlePacket(packet *Packet, addr *net.UDPAddr) {
	st.connMu.Lock()
	conn, exists := st.connections[addr.String()]
	if !exists {
		if st.maxConns > 0 && len(st.connections) >= st.maxConns {
			st.connMu.Unlock()
			return
		}
		conn = NewConnection(addr)
		st.connections[addr.String()] = conn
		
		// Initialize crypto for new connection
		st.initCrypto(conn)
	}
	st.connMu.Unlock()
	
	conn.lastActivity = time.Now()
	
	switch packet.Type {
	case TypeCryptoHandshake:
		st.handleCryptoHandshake(conn, packet)
	case TypeEncrypted:
		st.handleEncrypted(conn, packet)
	case TypeHandshake:
		st.Transport.handleHandshake(conn, packet)
	case TypeData:
		if st.RequireEncryption {
			// Reject unencrypted data if encryption is required
			return
		}
		st.Transport.handleData(conn, packet)
	case TypeAck:
		st.Transport.handleAck(conn, packet)
	case TypeNack:
		st.Transport.handleNack(conn, packet)
	case TypeKeepAlive:
		// Just update lastActivity (already done above)
	case TypeBitmapAck:
		st.Transport.handleBitmapAck(conn, packet)
	}
}

func (st *SecureTransport) handleCryptoHandshake(conn *Connection, packet *Packet) {
	st.cryptoMu.Lock()
	cs, exists := st.cryptoStates[conn.Addr.String()]
	st.cryptoMu.Unlock()
	
	if !exists {
		// Create new crypto state for incoming connection
		var err error
		cs, err = NewCryptoState()
		if err != nil {
			return
		}
		
		st.cryptoMu.Lock()
		st.cryptoStates[conn.Addr.String()] = cs
		st.cryptoMu.Unlock()
	}
	
	if len(packet.Payload) == DHKeySize {
		// Set remote public key
		if err := cs.SetRemotePublicKey(packet.Payload); err != nil {
			return
		}
		
		// If we're the server (receiving initial handshake), send our key back
		if conn.state == StateNew {
			// Send our public key
			response := &Packet{
				Type:    TypeCryptoHandshake,
				Payload: cs.GetPublicKey(),
			}
			st.outgoingCh <- &outgoingPacket{
				packet: response,
				conn:   conn,
			}
		}
		
		// If crypto is now established, proceed with regular handshake
		if cs.IsEstablished() && conn.state != StateConnected {
			// Send regular handshake
			handshake := &Packet{
				Type: TypeHandshake,
			}
			st.outgoingCh <- &outgoingPacket{
				packet: handshake,
				conn:   conn,
			}
			
			// Update state
			if conn.state == StateNew {
				conn.state = StateHandshaking
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
		Flags:      packet.Flags &^ FlagEncrypted, // Remove encrypted flag
		SeqNum:     packet.SeqNum,
		AckNum:     packet.AckNum,
		FragmentID: packet.FragmentID,
		FragOffset: packet.FragOffset,
		FragTotal:  packet.FragTotal,
		Payload:    decrypted,
		Timestamp:  packet.Timestamp,
	}
	
	// Process as normal data packet
	st.handleData(conn, decryptedPacket)
}

// SecureConnect establishes an encrypted connection
func (st *SecureTransport) SecureConnect(address string) (*Connection, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	
	// Create crypto state
	cs, err := NewCryptoState()
	if err != nil {
		return nil, err
	}
	
	conn := NewConnection(addr)
	
	st.connMu.Lock()
	st.connections[addr.String()] = conn
	st.connMu.Unlock()
	
	st.cryptoMu.Lock()
	st.cryptoStates[addr.String()] = cs
	st.cryptoMu.Unlock()
	
	// Send crypto handshake first
	cryptoHandshake := &Packet{
		Type:    TypeCryptoHandshake,
		Payload: cs.GetPublicKey(),
	}
	
	st.outgoingCh <- &outgoingPacket{
		packet: cryptoHandshake,
		conn:   conn,
	}
	
	// Wait for crypto establishment
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	
	for {
		select {
		case <-timeout.C:
			return nil, fmt.Errorf("crypto handshake timeout")
		case <-time.After(100 * time.Millisecond):
			if cs.IsEstablished() {
				// Now do regular handshake
				handshake := &Packet{
					Type: TypeHandshake,
				}
				conn.state = StateHandshaking
				st.outgoingCh <- &outgoingPacket{
					packet: handshake,
					conn:   conn,
				}
				
				// Wait for connection
				for {
					select {
					case <-timeout.C:
						return nil, fmt.Errorf("connection timeout")
					case <-time.After(100 * time.Millisecond):
						if conn.state == StateConnected {
							return conn, nil
						}
					}
				}
			}
		}
	}
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
	
	// Wait if send window is full
	for !conn.canSend() {
		time.Sleep(10 * time.Millisecond)
	}
	
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

// Override Start to use our handlePacket
func (st *SecureTransport) Start() {
	st.wg.Add(4)
	go st.readLoop()
	go st.processIncoming()
	go st.processOutgoing()
	go st.maintenanceLoop()
}

func (st *SecureTransport) processIncoming() {
	defer st.wg.Done()
	
	for {
		select {
		case <-st.ctx.Done():
			return
		case in := <-st.incomingCh:
			st.handlePacket(in.packet, in.addr)
		}
	}
}
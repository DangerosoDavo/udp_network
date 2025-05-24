package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

type Transport struct {
	conn        *net.UDPConn
	connections map[string]*Connection
	connMu      sync.RWMutex
	maxConns    int // Maximum number of connections (0 = unlimited)

	// Packet processing
	incomingCh chan *incomingPacket
	outgoingCh chan *outgoingPacket

	// Callbacks
	OnConnect    func(*Connection)
	OnDisconnect func(*Connection)

	// Configuration
	ConnTimeout       time.Duration // Connection timeout (default 30s)
	KeepAliveInterval time.Duration // Keep-alive interval (default 10s)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type incomingPacket struct {
	packet *Packet
	addr   *net.UDPAddr
}

type outgoingPacket struct {
	packet *Packet
	conn   *Connection
}

func NewTransport(address string) (*Transport, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	t := &Transport{
		conn:              conn,
		connections:       make(map[string]*Connection),
		incomingCh:        make(chan *incomingPacket, 1000),
		outgoingCh:        make(chan *outgoingPacket, 1000),
		ConnTimeout:       30 * time.Second, // Default 30s timeout
		KeepAliveInterval: 10 * time.Second, // Default 10s keep-alive
		ctx:               ctx,
		cancel:            cancel,
	}

	return t, nil
}

func (t *Transport) Start() {
	t.wg.Add(4)
	go t.readLoop()
	go t.processIncoming()
	go t.processOutgoing()
	go t.maintenanceLoop()
}

func (t *Transport) Stop() {
	t.cancel()
	t.conn.Close()
	t.wg.Wait()
}

func (t *Transport) Connect(address string) (*Connection, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn := NewConnection(addr)

	t.connMu.Lock()
	t.connections[addr.String()] = conn
	t.connMu.Unlock()

	// Send handshake
	handshake := &Packet{
		Type:   TypeHandshake,
		SeqNum: 0,
	}

	conn.state = StateHandshaking

	t.outgoingCh <- &outgoingPacket{
		packet: handshake,
		conn:   conn,
	}

	// Wait for connection establishment
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

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

func (t *Transport) readLoop() {
	defer t.wg.Done()

	buf := make([]byte, 65536)

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			t.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, addr, err := t.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}

			packet, err := Unmarshal(buf[:n])
			if err != nil {
				continue
			}

			t.incomingCh <- &incomingPacket{
				packet: packet,
				addr:   addr,
			}
		}
	}
}

func (t *Transport) processIncoming() {
	defer t.wg.Done()

	for {
		select {
		case <-t.ctx.Done():
			return
		case in := <-t.incomingCh:
			t.handlePacket(in.packet, in.addr)
		}
	}
}

func (t *Transport) processOutgoing() {
	defer t.wg.Done()

	for {
		select {
		case <-t.ctx.Done():
			return
		case out := <-t.outgoingCh:
			data, err := out.packet.Marshal()
			if err != nil {
				continue
			}

			t.conn.WriteToUDP(data, out.conn.Addr)

			// Add to send window if it's a data packet or encrypted packet
			if out.packet.Type == TypeData || out.packet.Type == TypeEncrypted {
				out.conn.addToSendWindow(out.packet)
			}
		}
	}
}

func (t *Transport) maintenanceLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.performMaintenance()
		}
	}
}

func (t *Transport) performMaintenance() {
	now := time.Now()

	// First pass: collect connections to remove
	var toRemove []string
	t.connMu.RLock()
	for addr, conn := range t.connections {
		// Check for timeout or closed state
		if now.Sub(conn.lastActivity) > t.ConnTimeout || conn.state == StateClosed {
			toRemove = append(toRemove, addr)
		}
	}
	t.connMu.RUnlock()

	// Remove dead connections
	if len(toRemove) > 0 {
		t.connMu.Lock()
		for _, addr := range toRemove {
			if conn, exists := t.connections[addr]; exists {
				if t.OnDisconnect != nil && conn.state != StateClosed {
					conn.state = StateClosed
					t.OnDisconnect(conn)
				}
				delete(t.connections, addr)
			}
		}
		t.connMu.Unlock()
	}

	// Second pass: maintenance on active connections
	t.connMu.RLock()
	for _, conn := range t.connections {
		if conn.state != StateConnected {
			continue
		}

		// Clean up old fragments
		conn.CleanupFragments()

		// Retransmissions
		for _, p := range conn.getRetransmissions() {
			p.Timestamp = now
			t.outgoingCh <- &outgoingPacket{
				packet: p,
				conn:   conn,
			}
			conn.onPacketLoss()
		}

		// Keep-alive based on configured interval
		if now.Sub(conn.lastActivity) > t.KeepAliveInterval {
			keepAlive := &Packet{
				Type: TypeKeepAlive,
			}
			t.outgoingCh <- &outgoingPacket{
				packet: keepAlive,
				conn:   conn,
			}
		}
	}
	t.connMu.RUnlock()
}

func (t *Transport) handlePacket(packet *Packet, addr *net.UDPAddr) {
	t.connMu.Lock()
	conn, exists := t.connections[addr.String()]
	if !exists {
		// Check connection limit
		if t.maxConns > 0 && len(t.connections) >= t.maxConns {
			t.connMu.Unlock()
			return // Drop packet, too many connections
		}
		conn = NewConnection(addr)
		t.connections[addr.String()] = conn
	}
	t.connMu.Unlock()

	conn.lastActivity = time.Now()

	switch packet.Type {
	case TypeHandshake:
		t.handleHandshake(conn, packet)
	case TypeData:
		t.handleData(conn, packet)
	case TypeAck:
		t.handleAck(conn, packet)
	case TypeNack:
		t.handleNack(conn, packet)
	case TypeKeepAlive:
		// Just update lastActivity
	case TypeBitmapAck:
		t.handleBitmapAck(conn, packet)
	}
}

func (t *Transport) handleHandshake(conn *Connection, packet *Packet) {
	if conn.state == StateNew {
		// Server side: receiving initial handshake
		conn.state = StateHandshaking

		// Send handshake response
		response := &Packet{
			Type:   TypeHandshake,
			AckNum: packet.SeqNum + 1,
		}
		t.outgoingCh <- &outgoingPacket{
			packet: response,
			conn:   conn,
		}

		// Transition to connected and trigger callback
		conn.state = StateConnected
		if t.OnConnect != nil {
			t.OnConnect(conn)
		}
	} else if conn.state == StateHandshaking {
		// Client side: receiving handshake response
		conn.state = StateConnected
		if t.OnConnect != nil {
			t.OnConnect(conn)
		}
	}
}

func (t *Transport) handleData(conn *Connection, packet *Packet) {
	// Track received packet
	conn.TrackReceived(packet.SeqNum)

	// Send ACK immediately for fast feedback
	ack := &Packet{
		Type:   TypeAck,
		AckNum: packet.SeqNum,
	}
	t.outgoingCh <- &outgoingPacket{
		packet: ack,
		conn:   conn,
	}

	// Also send periodic bitmap ACKs
	if packet.SeqNum%8 == 0 {
		baseSeq, bitmap := conn.GetAckBitmap()
		bitmapAck := &Packet{
			Type:   TypeBitmapAck,
			SeqNum: baseSeq,
			// Store bitmap in first 8 bytes of payload
			Payload: make([]byte, 8),
		}
		binary.BigEndian.PutUint64(bitmapAck.Payload, bitmap)

		t.outgoingCh <- &outgoingPacket{
			packet: bitmapAck,
			conn:   conn,
		}
	}

	// Process data immediately - no ordering required
	if conn.onMessage != nil {
		// For fragments, add to reassembly buffer
		if packet.IsFragment() {
			conn.AddFragment(packet)
		} else {
			// Deliver complete packets immediately
			go conn.onMessage(packet.Payload)
		}
	}
}

func (t *Transport) handleAck(conn *Connection, packet *Packet) {
	conn.removeFromSendWindow(packet.AckNum)
	conn.onAck()

	// Update RTT if we have timing info
	conn.sendWindowMu.RLock()
	if p, exists := conn.sendWindow[packet.AckNum]; exists {
		rtt := time.Since(p.Timestamp)
		conn.updateRTT(rtt)
	}
	conn.sendWindowMu.RUnlock()
}

func (t *Transport) handleNack(conn *Connection, packet *Packet) {
	// Immediate retransmission
	conn.sendWindowMu.RLock()
	if p, exists := conn.sendWindow[packet.AckNum]; exists {
		p.Timestamp = time.Now()
		t.outgoingCh <- &outgoingPacket{
			packet: p,
			conn:   conn,
		}
	}
	conn.sendWindowMu.RUnlock()

	conn.onPacketLoss()
}

func (t *Transport) handleBitmapAck(conn *Connection, packet *Packet) {
	if len(packet.Payload) >= 8 {
		bitmap := binary.BigEndian.Uint64(packet.Payload)
		conn.ProcessBitmapAck(packet.SeqNum, bitmap)
	}
}

func (t *Transport) Send(conn *Connection, data []byte) error {
	if conn.state != StateConnected {
		return fmt.Errorf("connection not established (state: %v)", conn.state)
	}

	// Wait if send window is full
	for !conn.canSend() {
		time.Sleep(10 * time.Millisecond)
	}

	packet := &Packet{
		Type:      TypeData,
		SeqNum:    conn.nextSeqNum,
		Payload:   data,
		Timestamp: time.Now(),
	}

	conn.nextSeqNum++

	t.outgoingCh <- &outgoingPacket{
		packet: packet,
		conn:   conn,
	}

	return nil
}

// SetMaxConnections sets the maximum number of concurrent connections
func (t *Transport) SetMaxConnections(max int) {
	t.maxConns = max
}

// GetConnectionCount returns the current number of active connections
func (t *Transport) GetConnectionCount() int {
	t.connMu.RLock()
	defer t.connMu.RUnlock()
	return len(t.connections)
}

// CloseConnection forcefully closes a specific connection
func (t *Transport) CloseConnection(addr string) {
	t.connMu.Lock()
	if conn, exists := t.connections[addr]; exists {
		conn.state = StateClosed
		delete(t.connections, addr)
		t.connMu.Unlock()

		if t.OnDisconnect != nil {
			t.OnDisconnect(conn)
		}
	} else {
		t.connMu.Unlock()
	}
}

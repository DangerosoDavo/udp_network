package protocol

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Transport callbacks/configuration must be set before Start. Plain UDP is for
// trusted development networks; use authenticated SecureTransport on the Internet.
type Transport struct {
	conn                                 *net.UDPConn
	connections                          map[string]*Connection
	connMu                               sync.RWMutex
	maxConns                             int
	incomingCh                           chan *incomingPacket
	outgoingCh                           chan *outgoingPacket
	OnConnect                            func(*Connection)
	OnDisconnect                         func(*Connection)
	ConnTimeout, KeepAliveInterval       time.Duration
	ctx                                  context.Context
	cancel                               context.CancelFunc
	wg                                   sync.WaitGroup
	startOnce, stopOnce                  sync.Once
	lifecycleMu                          sync.Mutex
	configMu                             sync.RWMutex
	processorIncoming, processorOutgoing func()
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
	return &Transport{conn: conn, connections: make(map[string]*Connection), maxConns: 1024,
		incomingCh: make(chan *incomingPacket, 1000), outgoingCh: make(chan *outgoingPacket, 1000),
		ConnTimeout: 30 * time.Second, KeepAliveInterval: 10 * time.Second, ctx: ctx, cancel: cancel}, nil
}
func (t *Transport) Start() {
	incoming, outgoing := t.processIncoming, t.processOutgoing
	if t.processorIncoming != nil {
		incoming = t.processorIncoming
	}
	if t.processorOutgoing != nil {
		outgoing = t.processorOutgoing
	}
	t.startWithProcessors(incoming, outgoing)
}

func (t *Transport) LocalAddr() net.Addr { return t.conn.LocalAddr() }
func (t *Transport) startWithProcessors(incoming, outgoing func()) {
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()
	if t.ctx.Err() != nil {
		return
	}
	t.startOnce.Do(func() { t.wg.Add(4); go t.readLoop(); go incoming(); go outgoing(); go t.maintenanceLoop() })
}
func (t *Transport) Stop() {
	t.lifecycleMu.Lock()
	t.stopOnce.Do(func() {
		t.cancel()
		t.conn.Close()
		t.connMu.Lock()
		conns := t.connections
		t.connections = make(map[string]*Connection)
		t.connMu.Unlock()
		for _, c := range conns {
			c.SetState(StateClosed)
		}
	})
	t.lifecycleMu.Unlock()
	t.wg.Wait()
}
func (t *Transport) bindConnection(c *Connection) {
	c.mu.Lock()
	c.onClose = func() {
		if t.OnDisconnect != nil {
			t.OnDisconnect(c)
		}
	}
	c.mu.Unlock()
	c.startDispatcher()
}
func (t *Transport) enqueue(ctx context.Context, c *Connection, p *Packet) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.ctx.Err(); err != nil {
		return err
	}
	select {
	case <-c.done:
		return errors.New("connection closed")
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.ctx.Done():
		return t.ctx.Err()
	case <-c.done:
		return errors.New("connection closed")
	case t.outgoingCh <- &outgoingPacket{p, c}:
		return nil
	}
}
func (t *Transport) Connect(address string) (*Connection, error) {
	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancel()
	return t.ConnectContext(ctx, address)
}
func (t *Transport) ConnectContext(ctx context.Context, address string) (*Connection, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	if addr.IP == nil || addr.IP.IsUnspecified() {
		return nil, errors.New("connect requires a concrete remote address")
	}
	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		return nil, err
	}
	id := binary.BigEndian.Uint64(random[:])
	if id == 0 {
		id = 1
	}
	c := NewConnection(addr)
	c.setSessionID(id)
	c.SetState(StateHandshaking)
	t.bindConnection(c)
	t.connMu.Lock()
	if old := t.connections[addr.String()]; old != nil && old.GetState() != StateClosed {
		t.connMu.Unlock()
		c.SetState(StateClosed)
		return nil, errors.New("connection already exists")
	}
	if t.maxConns > 0 && len(t.connections) >= t.maxConns {
		t.connMu.Unlock()
		c.SetState(StateClosed)
		return nil, errors.New("connection limit")
	}
	t.connections[addr.String()] = c
	t.connMu.Unlock()
	success := false
	defer func() {
		if !success {
			t.removeConnection(c)
		}
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	p := &Packet{Type: TypeHandshake, SessionID: id}
	for {
		if c.GetState() == StateConnected {
			success = true
			return c, nil
		}
		if err = t.enqueue(ctx, c, p); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.ctx.Done():
			return nil, t.ctx.Err()
		case <-c.done:
			return nil, errors.New("connection rejected or closed")
		case <-ticker.C:
		}
	}
}
func (t *Transport) readLoop() {
	defer t.wg.Done()
	buf := make([]byte, HeaderSize+MaxWirePayloadSize+1)
	for {
		n, addr, err := t.conn.ReadFromUDP(buf)
		if err != nil {
			if t.ctx.Err() != nil {
				return
			}
			continue
		}
		p, err := Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		select {
		case <-t.ctx.Done():
			return
		case t.incomingCh <- &incomingPacket{p, addr}:
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
			if err == nil {
				_, err = t.conn.WriteToUDP(data, out.conn.Addr)
			}
			if err != nil {
				t.removeConnection(out.conn)
			}
		}
	}
}
func (t *Transport) maintenanceLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(25 * time.Millisecond)
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
func (t *Transport) removeConnection(c *Connection) {
	t.connMu.Lock()
	if t.connections[c.Addr.String()] == c {
		delete(t.connections, c.Addr.String())
	}
	t.connMu.Unlock()
	c.SetState(StateClosed)
}
func (t *Transport) performMaintenance() {
	t.connMu.RLock()
	conns := make([]*Connection, 0, len(t.connections))
	for _, c := range t.connections {
		conns = append(conns, c)
	}
	t.connMu.RUnlock()
	now := time.Now()
	for _, c := range conns {
		c.mu.RLock()
		last, state := c.lastActivity, c.state
		c.mu.RUnlock()
		t.configMu.RLock()
		timeout, interval := t.ConnTimeout, t.KeepAliveInterval
		t.configMu.RUnlock()
		if state == StateClosed || (timeout > 0 && now.Sub(last) > timeout) {
			t.removeConnection(c)
			continue
		}
		if state != StateConnected {
			continue
		}
		c.CleanupFragments()
		packets := c.getRetransmissions()
		if len(packets) > 0 {
			c.onPacketLoss()
		}
		for _, p := range packets {
			if t.enqueue(t.ctx, c, p) != nil {
				break
			}
		}
		if c.GetState() == StateClosed {
			t.removeConnection(c)
			continue
		}
		c.mu.Lock()
		keep := interval > 0 && now.Sub(c.lastKeepAlive) >= interval
		if keep {
			c.lastKeepAlive = now
		}
		c.mu.Unlock()
		if keep {
			_ = t.enqueue(t.ctx, c, &Packet{Type: TypeKeepAlive, SessionID: c.SessionID()})
		}
	}
}
func (t *Transport) handlePacket(p *Packet, addr *net.UDPAddr) {
	if p == nil || p.validate() != nil || p.SessionID == 0 {
		return
	}
	t.connMu.Lock()
	c := t.connections[addr.String()]
	if c == nil {
		if p.Type != TypeHandshake || p.AckNum != 0 || (t.maxConns > 0 && len(t.connections) >= t.maxConns) {
			t.connMu.Unlock()
			return
		}
		c = NewConnection(addr)
		c.setSessionID(p.SessionID)
		t.bindConnection(c)
		t.connections[addr.String()] = c
	}
	t.connMu.Unlock()
	if c.SessionID() != p.SessionID {
		return
	}
	if p.Type == TypeHandshake {
		c.touch()
		t.handleHandshake(c, p)
		return
	}
	if c.GetState() != StateConnected {
		return
	}
	c.touch()
	switch p.Type {
	case TypeData:
		t.handleData(c, p)
	case TypeAck:
		t.handleAck(c, p)
	case TypeNack:
		t.handleNack(c, p)
	case TypeBitmapAck:
		t.handleBitmapAck(c, p)
	case TypeClose:
		t.removeConnection(c)
	}
}
func (t *Transport) handleHandshake(c *Connection, p *Packet) {
	state := c.GetState()
	if p.AckNum == 0 && (state == StateNew || state == StateConnected) {
		if state == StateNew {
			c.SetState(StateConnected)
			if t.OnConnect != nil && !c.queueTask(func() { t.OnConnect(c) }) {
				t.removeConnection(c)
				return
			}
		}
		_ = t.enqueue(t.ctx, c, &Packet{Type: TypeHandshake, AckNum: 1, SessionID: c.SessionID()})
	} else if p.AckNum == 1 && state == StateHandshaking {
		c.SetState(StateConnected)
		if t.OnConnect != nil && !c.queueTask(func() { t.OnConnect(c) }) {
			t.removeConnection(c)
		}
	}
}
func (t *Transport) handleData(c *Connection, p *Packet) {
	if c.GetState() != StateConnected || p.validate() != nil {
		return
	}
	c.receiveMu.Lock()
	defer c.receiveMu.Unlock()
	if p.Flags&FlagUnreliableSequenced != 0 {
		if c.unreliableSeen && !sequenceBefore(c.lastUnreliable, p.SeqNum) {
			return
		}
		c.unreliableSeen = true
		c.lastUnreliable = p.SeqNum
		// Disposable updates may be dropped under application backpressure.
		_ = c.deliver(p.Payload)
		return
	}
	c.receivedMu.RLock()
	duplicate := sequenceBefore(p.SeqNum, c.minUnacked) || c.receivedSeqs[p.SeqNum]
	outside := !sequenceBefore(p.SeqNum, c.minUnacked) && p.SeqNum-c.minUnacked >= receiveWindowSize
	c.receivedMu.RUnlock()
	if outside {
		return
	}
	if !duplicate {
		data, complete := p.Payload, true
		if p.IsFragment() {
			var err error
			data, complete, err = c.assemble(p)
			if err != nil {
				t.removeConnection(c)
				return
			}
		}
		if complete && !c.deliver(data) {
			t.removeConnection(c)
			return
		}
		c.TrackReceived(p.SeqNum)
	}
	_ = t.enqueue(t.ctx, c, &Packet{Type: TypeAck, AckNum: p.SeqNum, SessionID: c.SessionID()})
	// Cumulative+selective ACK recovers lost individual ACKs; duplicates re-ACK.
	if p.SeqNum%8 == 0 || duplicate {
		base, bits := c.GetAckBitmap()
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, bits)
		_ = t.enqueue(t.ctx, c, &Packet{Type: TypeBitmapAck, SeqNum: base, Payload: data, SessionID: c.SessionID()})
	}
}
func (t *Transport) handleAck(c *Connection, p *Packet) { c.acknowledge(p.AckNum) }
func (t *Transport) handleNack(c *Connection, p *Packet) {
	if retry := c.retransmit(p.AckNum); retry != nil {
		c.onPacketLoss()
		_ = t.enqueue(t.ctx, c, retry)
	}
}
func (t *Transport) handleBitmapAck(c *Connection, p *Packet) {
	if len(p.Payload) == 8 {
		c.ProcessBitmapAck(p.SeqNum, binary.BigEndian.Uint64(p.Payload))
	}
}

// Send queues a reliable, duplicate-suppressed UNORDERED message, fragmenting as
// necessary. Success means accepted locally, not acknowledged/application-applied.
// Message callbacks are serialized in arrival/completion order. Use application
// command IDs/revisions for business ordering and retry-safe durable mutations.
func (t *Transport) Send(c *Connection, data []byte) error { return t.SendContext(t.ctx, c, data) }
func (t *Transport) SendContext(ctx context.Context, c *Connection, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return errors.New("nil connection")
	}
	if len(data) > MaxMessageSize {
		return fmt.Errorf("message exceeds %d bytes", MaxMessageSize)
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(t.ctx, cancel)
	defer cancel()
	defer stop()
	// Serialize complete sends, including fragments, with cancellable acquisition.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.ctx.Done():
		return t.ctx.Err()
	case <-c.done:
		return errors.New("connection closed")
	case <-c.sendToken:
	}
	defer func() { c.sendToken <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.GetState() != StateConnected {
		return errors.New("connection not established")
	}
	// Own the payload: the caller may immediately reuse its input buffer.
	owned := append([]byte(nil), data...)
	fragments, id := FragmentMessage(owned, MaxPayloadSize)
	if len(fragments) > 1 {
		id = c.nextFragmentID
		c.nextFragmentID++
	}
	for i, fragment := range fragments {
		p := &Packet{Type: TypeData, Payload: fragment, SessionID: c.SessionID()}
		if len(fragments) > 1 {
			p.Flags = FlagFragment
			p.FragmentID = id
			p.FragOffset = uint8(i)
			p.FragTotal = uint8(len(fragments))
		}
		if err := c.reserve(ctx, p); err != nil {
			// Abandoning a partially sent message creates a reliability/fragment gap.
			if i > 0 {
				t.removeConnection(c)
			}
			return err
		}
		if err := t.enqueue(ctx, c, p); err != nil {
			t.removeConnection(c)
			return err
		}
	}
	return nil
}

// SendUnreliableSequenced queues one disposable state update (no fragments or
// retransmission). Older/duplicate state is discarded. One sequence channel per
// connection: combine related state or use application channel/tick identifiers.
func (t *Transport) SendUnreliableSequenced(c *Connection, data []byte) error {
	if c == nil || c.GetState() != StateConnected {
		return errors.New("connection not established")
	}
	if len(data) > MaxPayloadSize {
		return errors.New("unreliable payload too large")
	}
	c.sendWindowMu.Lock()
	seq := c.nextUnreliable
	c.nextUnreliable++
	c.sendWindowMu.Unlock()
	p := &Packet{Type: TypeData, Flags: FlagUnreliableSequenced, SeqNum: seq, SessionID: c.SessionID(), Payload: append([]byte(nil), data...)}
	if t.ctx.Err() != nil {
		return t.ctx.Err()
	}
	select {
	case <-c.done:
		return errors.New("connection closed")
	default:
	}
	select {
	case t.outgoingCh <- &outgoingPacket{p, c}:
		return nil
	default:
		return errors.New("outgoing queue full")
	}
}
func (t *Transport) SetMaxConnections(max int) { t.connMu.Lock(); t.maxConns = max; t.connMu.Unlock() }

// SetKeepAlive safely updates timeout configuration while running.
func (t *Transport) SetKeepAlive(timeout, interval time.Duration) {
	t.configMu.Lock()
	t.ConnTimeout = timeout
	t.KeepAliveInterval = interval
	t.configMu.Unlock()
}
func (t *Transport) GetConnectionCount() int {
	t.connMu.RLock()
	defer t.connMu.RUnlock()
	return len(t.connections)
}
func (t *Transport) CloseConnection(addr string) {
	t.connMu.RLock()
	c := t.connections[addr]
	t.connMu.RUnlock()
	if c != nil {
		_ = t.enqueue(t.ctx, c, &Packet{Type: TypeClose, SessionID: c.SessionID()})
		t.removeConnection(c)
	}
}

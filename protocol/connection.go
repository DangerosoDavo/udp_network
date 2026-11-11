package protocol

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

type ConnectionState int

const (
	StateNew ConnectionState = iota
	StateHandshaking
	StateConnected
	StateClosed
)
const (
	receiveWindowSize    = 4096
	maxFragmentTransfers = 64
	maxReassemblyBytes   = 4 << 20
	maxDeliveryBytes     = 4 << 20
)

type FragmentBuffer struct {
	fragments map[uint8][]byte
	total     uint8
	received  uint32
	firstSeen time.Time
	complete  bool
	bytes     int
	baseSeq   uint32
}
type delivery struct {
	data []byte
	task func()
}

// Connection owns bounded protocol state. Message callbacks run serially in arrival/
// reassembly completion order, NOT sequence order. Callbacks must return promptly:
// shutdown cannot interrupt user code. A closed connection must not be reused.
type Connection struct {
	mu                                    sync.RWMutex
	Addr                                  *net.UDPAddr
	state                                 ConnectionState
	sessionID                             uint64
	nextSeqNum, nextAckNum, lastAckedSeq  uint32
	sendWindow                            map[uint32]*Packet
	sendWindowMu                          sync.RWMutex
	windowSize                            uint32
	windowCond                            *sync.Cond
	windowChanged, sendToken              chan struct{}
	nextUnreliable                        uint32
	nextFragmentID                        uint16 // guarded by sendToken; never shared across connections
	fragments                             map[uint16]*FragmentBuffer
	fragmentsMu                           sync.RWMutex
	fragmentBytes                         int
	receivedSeqs                          map[uint32]bool
	receivedMu                            sync.RWMutex
	minUnacked                            uint32
	receiveMu                             sync.Mutex
	unreliableSeen                        bool
	lastUnreliable                        uint32
	rtt, rttVar, rto                      time.Duration
	lastActivity, lastKeepAlive           time.Time
	cwnd, ssthresh                        float64
	onMessage                             func([]byte)
	onClose                               func()
	done                                  chan struct{}
	closeOnce, dispatchOnce, callbackOnce sync.Once
	callbackReady                         chan struct{}
	deliveryCh                            chan delivery
	deliveryMu                            sync.Mutex
	deliveryBytes                         int
}

func NewConnection(addr *net.UDPAddr) *Connection {
	c := &Connection{Addr: addr, state: StateNew, sendWindow: make(map[uint32]*Packet),
		fragments: make(map[uint16]*FragmentBuffer), receivedSeqs: make(map[uint32]bool),
		windowSize: 32, rtt: 100 * time.Millisecond, rttVar: 50 * time.Millisecond, rto: 300 * time.Millisecond,
		lastActivity: time.Now(), cwnd: 1, ssthresh: 16, done: make(chan struct{}),
		windowChanged: make(chan struct{}, 1), sendToken: make(chan struct{}, 1), deliveryCh: make(chan delivery, 64), callbackReady: make(chan struct{})}
	c.windowCond = sync.NewCond(&c.sendWindowMu)
	c.sendToken <- struct{}{}
	return c
}
func (c *Connection) updateRTT(sample time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sample <= 0 {
		return
	}
	c.rttVar = time.Duration(.75*float64(c.rttVar) + .25*float64(abs(c.rtt-sample)))
	c.rtt = time.Duration(.875*float64(c.rtt) + .125*float64(sample))
	c.rto = c.rtt + 4*c.rttVar
	if c.rto < 100*time.Millisecond {
		c.rto = 100 * time.Millisecond
	}
	if c.rto > 3*time.Second {
		c.rto = 3 * time.Second
	}
}
func (c *Connection) onPacketLoss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ssthresh = c.cwnd / 2
	if c.ssthresh < 1 {
		c.ssthresh = 1
	}
	c.cwnd = 1
}
func (c *Connection) onAck() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cwnd < c.ssthresh {
		c.cwnd++
	} else {
		c.cwnd += 1 / c.cwnd
	}
	if c.cwnd > float64(c.windowSize) {
		c.cwnd = float64(c.windowSize)
	}
}
func (c *Connection) canSend() bool {
	c.sendWindowMu.RLock()
	defer c.sendWindowMu.RUnlock()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return float64(len(c.sendWindow)) < c.cwnd
}
func (c *Connection) notifyWindow() {
	select {
	case c.windowChanged <- struct{}{}:
	default:
	}
	c.windowCond.Broadcast()
}

// reserve registers immutable packet ownership BEFORE enqueue/write, atomically
// reserving its sequence and congestion window space. Lock order is window -> mu.
func (c *Connection) reserve(ctx context.Context, p *Packet) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-c.done:
			return errors.New("connection closed")
		default:
		}
		c.sendWindowMu.Lock()
		c.mu.RLock()
		limit, state := c.cwnd, c.state
		c.mu.RUnlock()
		if state != StateConnected {
			c.sendWindowMu.Unlock()
			return errors.New("connection not established")
		}
		if float64(len(c.sendWindow)) < limit {
			p.SeqNum = c.nextSeqNum
			c.nextSeqNum++
			p.Timestamp = time.Now()
			c.sendWindow[p.SeqNum] = p
			c.sendWindowMu.Unlock()
			return nil
		}
		c.sendWindowMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return errors.New("connection closed")
		case <-c.windowChanged:
		}
	}
}
func (c *Connection) addToSendWindow(p *Packet) {
	c.sendWindowMu.Lock()
	c.sendWindow[p.SeqNum] = p
	c.sendWindowMu.Unlock()
}
func (c *Connection) removeFromSendWindow(seq uint32) {
	c.sendWindowMu.Lock()
	delete(c.sendWindow, seq)
	c.notifyWindow()
	c.sendWindowMu.Unlock()
}
func (c *Connection) acknowledge(seq uint32) bool {
	c.sendWindowMu.Lock()
	p, ok := c.sendWindow[seq]
	if ok {
		delete(c.sendWindow, seq)
	}
	c.sendWindowMu.Unlock()
	if !ok {
		return false
	}
	c.onAck()
	if p.retries == 0 && !p.Timestamp.IsZero() {
		c.updateRTT(time.Since(p.Timestamp))
	}
	c.sendWindowMu.Lock()
	c.notifyWindow()
	c.sendWindowMu.Unlock()
	return true
}
func (c *Connection) getRetransmissions() []*Packet {
	c.mu.RLock()
	rto := c.rto
	c.mu.RUnlock()
	c.sendWindowMu.Lock()
	defer c.sendWindowMu.Unlock()
	now := time.Now()
	var result []*Packet
	for seq, p := range c.sendWindow {
		wait := rto * time.Duration(1<<min(p.retries, 4))
		if wait > 3*time.Second {
			wait = 3 * time.Second
		}
		if now.Sub(p.Timestamp) < wait {
			continue
		}
		if p.retries >= 10 {
			c.SetState(StateClosed)
			break
		}
		cp := *p
		cp.Timestamp = now
		cp.retries++
		c.sendWindow[seq] = &cp
		result = append(result, &cp)
	}
	return result
}
func (c *Connection) retransmit(seq uint32) *Packet {
	c.sendWindowMu.Lock()
	defer c.sendWindowMu.Unlock()
	p := c.sendWindow[seq]
	if p == nil || time.Since(p.Timestamp) < 100*time.Millisecond {
		return nil
	}
	if p.retries >= 10 {
		c.SetState(StateClosed)
		return nil
	}
	cp := *p
	cp.Timestamp = time.Now()
	cp.retries++
	c.sendWindow[seq] = &cp
	return &cp
}
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
func sequenceBefore(a, b uint32) bool { return int32(a-b) < 0 }
func (c *Connection) assemble(p *Packet) ([]byte, bool, error) {
	if p.FragTotal < 2 || p.FragOffset >= p.FragTotal || len(p.Payload) > MaxPayloadSize {
		return nil, false, errors.New("invalid fragment metadata")
	}
	c.fragmentsMu.Lock()
	defer c.fragmentsMu.Unlock()
	b := c.fragments[p.FragmentID]
	if b == nil {
		if len(c.fragments) >= maxFragmentTransfers {
			return nil, false, errors.New("fragment transfer limit")
		}
		b = &FragmentBuffer{fragments: make(map[uint8][]byte), total: p.FragTotal, firstSeen: time.Now(), baseSeq: p.SeqNum - uint32(p.FragOffset)}
		c.fragments[p.FragmentID] = b
	}
	if b.total != p.FragTotal {
		return nil, false, errors.New("inconsistent fragment total")
	}
	if b.baseSeq != p.SeqNum-uint32(p.FragOffset) {
		return nil, false, errors.New("fragment sequence base mismatch")
	}
	if _, ok := b.fragments[p.FragOffset]; ok {
		return nil, false, nil
	}
	if c.fragmentBytes+len(p.Payload) > maxReassemblyBytes {
		return nil, false, errors.New("fragment byte limit")
	}
	b.fragments[p.FragOffset] = p.Payload
	b.bytes += len(p.Payload)
	c.fragmentBytes += len(p.Payload)
	if len(b.fragments) != int(b.total) {
		return nil, false, nil
	}
	data := make([]byte, 0, b.bytes)
	for i := uint8(0); i < b.total; i++ {
		data = append(data, b.fragments[i]...)
	}
	delete(c.fragments, p.FragmentID)
	c.fragmentBytes -= b.bytes
	return data, true, nil
}
func (c *Connection) AddFragment(p *Packet) {
	data, complete, err := c.assemble(p)
	if err == nil && complete {
		if !c.deliver(data) {
			c.SetState(StateClosed)
		}
	}
}
func (c *Connection) CleanupFragments() {
	c.fragmentsMu.Lock()
	defer c.fragmentsMu.Unlock()
	for id, b := range c.fragments {
		if time.Since(b.firstSeen) > 30*time.Second {
			delete(c.fragments, id)
			c.fragmentBytes -= b.bytes
			if c.GetState() == StateConnected {
				c.SetState(StateClosed)
			}
		}
	}
}

// TrackReceived returns true for a new reliable sequence within the bounded window.
func (c *Connection) TrackReceived(seq uint32) bool {
	c.receivedMu.Lock()
	defer c.receivedMu.Unlock()
	if sequenceBefore(seq, c.minUnacked) || seq-c.minUnacked >= receiveWindowSize || c.receivedSeqs[seq] {
		return false
	}
	c.receivedSeqs[seq] = true
	for c.receivedSeqs[c.minUnacked] {
		delete(c.receivedSeqs, c.minUnacked)
		c.minUnacked++
	}
	return true
}
func (c *Connection) GetAckBitmap() (uint32, uint64) {
	c.receivedMu.RLock()
	defer c.receivedMu.RUnlock()
	base := c.minUnacked
	var bitmap uint64
	for i := uint32(0); i < 64; i++ {
		if c.receivedSeqs[base+i] {
			bitmap |= 1 << i
		}
	}
	return base, bitmap
}
func (c *Connection) ProcessBitmapAck(base uint32, bitmap uint64) {
	c.sendWindowMu.RLock()
	var seqs []uint32
	if sequenceBefore(c.nextSeqNum, base) {
		c.sendWindowMu.RUnlock()
		return
	}
	for seq := range c.sendWindow {
		delta := seq - base
		if sequenceBefore(seq, base) || (delta < 64 && bitmap&(1<<delta) != 0) {
			seqs = append(seqs, seq)
		}
	}
	c.sendWindowMu.RUnlock()
	for _, seq := range seqs {
		c.acknowledge(seq)
	}
}
func (c *Connection) GetState() ConnectionState { c.mu.RLock(); defer c.mu.RUnlock(); return c.state }
func (c *Connection) SetState(state ConnectionState) {
	c.mu.Lock()
	if c.state == StateClosed {
		c.mu.Unlock()
		return
	}
	c.state = state
	c.mu.Unlock()
	if state == StateClosed {
		c.closeOnce.Do(func() { close(c.done) })
	}
}
func (c *Connection) SessionID() uint64      { c.mu.RLock(); defer c.mu.RUnlock(); return c.sessionID }
func (c *Connection) setSessionID(id uint64) { c.mu.Lock(); c.sessionID = id; c.mu.Unlock() }
func (c *Connection) touch()                 { c.mu.Lock(); c.lastActivity = time.Now(); c.mu.Unlock() }
func (c *Connection) SetMessageCallback(fn func([]byte)) {
	c.mu.Lock()
	c.onMessage = fn
	c.mu.Unlock()
	if fn != nil {
		c.callbackOnce.Do(func() { close(c.callbackReady) })
		c.startDispatcher()
	}
}
func (c *Connection) startDispatcher() { c.dispatchOnce.Do(func() { go c.dispatch() }) }
func (c *Connection) dispatch() {
	defer func() {
		c.mu.RLock()
		fn := c.onClose
		c.mu.RUnlock()
		if fn != nil {
			fn()
		}
	}()
	for {
		select {
		case <-c.done:
			return
		case d := <-c.deliveryCh:
			c.deliveryMu.Lock()
			c.deliveryBytes -= len(d.data)
			c.deliveryMu.Unlock()
			select {
			case <-c.done:
				return
			default:
			}
			if d.task != nil {
				d.task()
				continue
			}
			select {
			case <-c.done:
				return
			case <-c.callbackReady:
			}
			c.mu.RLock()
			fn := c.onMessage
			c.mu.RUnlock()
			if fn != nil {
				fn(d.data)
			}
		}
	}
}
func (c *Connection) queueTask(fn func()) bool {
	c.startDispatcher()
	select {
	case <-c.done:
		return false
	case c.deliveryCh <- delivery{task: fn}:
		return true
	default:
		return false
	}
}
func (c *Connection) deliver(data []byte) bool {
	c.deliveryMu.Lock()
	defer c.deliveryMu.Unlock()
	if c.deliveryBytes+len(data) > maxDeliveryBytes {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.deliveryCh <- delivery{data: data}:
		c.deliveryBytes += len(data)
		return true
	default:
		return false
	}
}

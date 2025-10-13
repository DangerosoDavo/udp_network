package protocol

import (
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

type FragmentBuffer struct {
	fragments map[uint8][]byte
	total     uint8
	received  uint32 // Bitmap of received fragments
	firstSeen time.Time
	complete  bool
}

type Connection struct {
	mu    sync.RWMutex
	Addr  *net.UDPAddr
	state ConnectionState

	// Sequence numbers
	nextSeqNum   uint32
	nextAckNum   uint32
	lastAckedSeq uint32

	// Send window
	sendWindow   map[uint32]*Packet
	sendWindowMu sync.RWMutex
	windowSize   uint32
	windowCond   *sync.Cond // Condition variable for window availability

	// Fragment reassembly
	fragments   map[uint16]*FragmentBuffer
	fragmentsMu sync.RWMutex

	// Received packets tracking for bitmap ACK
	receivedSeqs map[uint32]bool
	receivedMu   sync.RWMutex
	minUnacked   uint32

	// Timing
	rtt          time.Duration
	rttVar       time.Duration
	rto          time.Duration // Retransmission timeout
	lastActivity time.Time

	// Congestion control
	cwnd     float64 // Congestion window
	ssthresh float64 // Slow start threshold

	// Callbacks
	onMessage func([]byte)
	onClose   func()
}

func NewConnection(addr *net.UDPAddr) *Connection {
	c := &Connection{
		Addr:         addr,
		state:        StateNew,
		sendWindow:   make(map[uint32]*Packet),
		fragments:    make(map[uint16]*FragmentBuffer),
		receivedSeqs: make(map[uint32]bool),
		windowSize:   32,
		rtt:          100 * time.Millisecond,
		rttVar:       50 * time.Millisecond,
		rto:          300 * time.Millisecond,
		lastActivity: time.Now(),
		cwnd:         1.0,
		ssthresh:     16.0,
	}
	// Initialize condition variable with sendWindowMu
	c.windowCond = sync.NewCond(&c.sendWindowMu)
	return c
}

func (c *Connection) updateRTT(sampleRTT time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rtt == 0 {
		c.rtt = sampleRTT
		c.rttVar = sampleRTT / 2
	} else {
		alpha := 0.125
		beta := 0.25

		c.rttVar = time.Duration((1-beta)*float64(c.rttVar) + beta*float64(abs(c.rtt-sampleRTT)))
		c.rtt = time.Duration((1-alpha)*float64(c.rtt) + alpha*float64(sampleRTT))
	}

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

	// Congestion control: multiplicative decrease
	c.ssthresh = c.cwnd / 2
	if c.ssthresh < 1 {
		c.ssthresh = 1
	}
	c.cwnd = 1
}

func (c *Connection) onAck() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Congestion control: additive increase
	if c.cwnd < c.ssthresh {
		// Slow start
		c.cwnd += 1
	} else {
		// Congestion avoidance
		c.cwnd += 1 / c.cwnd
	}

	if c.cwnd > float64(c.windowSize) {
		c.cwnd = float64(c.windowSize)
	}
}

func (c *Connection) canSend() bool {
	c.sendWindowMu.RLock()
	defer c.sendWindowMu.RUnlock()

	return float64(len(c.sendWindow)) < c.cwnd
}

func (c *Connection) addToSendWindow(p *Packet) {
	c.sendWindowMu.Lock()
	defer c.sendWindowMu.Unlock()

	c.sendWindow[p.SeqNum] = p
}

func (c *Connection) removeFromSendWindow(seqNum uint32) {
	c.sendWindowMu.Lock()
	defer c.sendWindowMu.Unlock()

	delete(c.sendWindow, seqNum)
	// Signal that window space is available
	c.windowCond.Signal()
}

func (c *Connection) getRetransmissions() []*Packet {
	c.sendWindowMu.RLock()
	defer c.sendWindowMu.RUnlock()

	var packets []*Packet
	now := time.Now()

	for _, p := range c.sendWindow {
		if now.Sub(p.Timestamp) > c.rto {
			packets = append(packets, p)
		}
	}

	return packets
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func (c *Connection) AddFragment(packet *Packet) {
	c.fragmentsMu.Lock()
	defer c.fragmentsMu.Unlock()

	fragID := packet.FragmentID

	// Get or create fragment buffer
	fragBuf, exists := c.fragments[fragID]
	if !exists {
		fragBuf = &FragmentBuffer{
			fragments: make(map[uint8][]byte),
			total:     packet.FragTotal,
			firstSeen: time.Now(),
		}
		c.fragments[fragID] = fragBuf
	}

	// Add fragment
	fragBuf.fragments[packet.FragOffset] = packet.Payload
	fragBuf.received |= (1 << packet.FragOffset)

	// Check if complete
	if len(fragBuf.fragments) == int(fragBuf.total) {
		// Reassemble in order
		var data []byte
		for i := uint8(0); i < fragBuf.total; i++ {
			data = append(data, fragBuf.fragments[i]...)
		}

		// Deliver the complete message
		if c.onMessage != nil {
			go c.onMessage(data)
		}

		// Clean up
		delete(c.fragments, fragID)
	}
}

func (c *Connection) CleanupFragments() {
	c.fragmentsMu.Lock()
	defer c.fragmentsMu.Unlock()

	now := time.Now()
	for fragID, fragBuf := range c.fragments {
		if now.Sub(fragBuf.firstSeen) > 30*time.Second {
			delete(c.fragments, fragID)
		}
	}
}

// Track received packet for bitmap ACK
func (c *Connection) TrackReceived(seqNum uint32) {
	c.receivedMu.Lock()
	defer c.receivedMu.Unlock()

	c.receivedSeqs[seqNum] = true

	// Update minimum unacked
	for c.receivedSeqs[c.minUnacked] {
		delete(c.receivedSeqs, c.minUnacked)
		c.minUnacked++
	}
}

// Generate bitmap for ACK
func (c *Connection) GetAckBitmap() (baseSeq uint32, bitmap uint64) {
	c.receivedMu.RLock()
	defer c.receivedMu.RUnlock()

	baseSeq = c.minUnacked

	// Build bitmap for next 64 sequence numbers
	for i := uint32(0); i < 64; i++ {
		if c.receivedSeqs[baseSeq+i] {
			bitmap |= (1 << i)
		}
	}

	return baseSeq, bitmap
}

// ProcessBitmapAck handles bitmap acknowledgments
func (c *Connection) ProcessBitmapAck(baseSeq uint32, bitmap uint64) {
	c.sendWindowMu.Lock()
	defer c.sendWindowMu.Unlock()

	// Remove acknowledged packets
	removed := 0
	for i := uint32(0); i < 64; i++ {
		if bitmap&(1<<i) != 0 {
			seqNum := baseSeq + i
			if _, exists := c.sendWindow[seqNum]; exists {
				delete(c.sendWindow, seqNum)
				removed++
				c.onAck()
			}
		}
	}

	// Signal if any packets were removed
	if removed > 0 {
		c.windowCond.Broadcast()
	}
}

// GetState returns the current connection state
func (c *Connection) GetState() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// SetState sets the connection state (for WebSocket and other transports)
func (c *Connection) SetState(state ConnectionState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = state
}

// SetMessageCallback sets the callback for received messages
func (c *Connection) SetMessageCallback(callback func([]byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMessage = callback
}

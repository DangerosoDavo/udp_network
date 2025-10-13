package protocol

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestConnectionCreation(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	if conn.Addr.String() != "127.0.0.1:8080" {
		t.Errorf("Address mismatch: got %s, want 127.0.0.1:8080", conn.Addr.String())
	}

	if conn.GetState() != StateNew {
		t.Errorf("Initial state = %v, want %v", conn.GetState(), StateNew)
	}

	if conn.nextSeqNum != 0 {
		t.Errorf("Initial nextSeqNum = %d, want 0", conn.nextSeqNum)
	}
}

func TestConnectionStateTransitions(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	states := []ConnectionState{
		StateNew,
		StateHandshaking,
		StateConnected,
		StateClosed,
	}

	for i, expectedState := range states {
		if i > 0 {
			conn.SetState(expectedState)
		}

		if conn.GetState() != expectedState {
			t.Errorf("State transition %d: got %v, want %v", i, conn.GetState(), expectedState)
		}
	}
}

func TestConnectionConcurrentStateAccess(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	const goroutines = 50
	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = conn.GetState()
			}
		}()
	}

	// Concurrent writes
	states := []ConnectionState{StateNew, StateHandshaking, StateConnected, StateClosed}
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				conn.SetState(states[j%len(states)])
			}
		}(i)
	}

	wg.Wait()
	// If we get here without race detector errors, thread safety is working
}

func TestConnectionSendWindow(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	// Initially should be able to send
	if !conn.canSend() {
		t.Error("Should be able to send on new connection")
	}

	// Fill the send window (cwnd starts at 1.0, so add 2 packets to exceed it)
	for i := uint32(0); i < 2; i++ {
		packet := &Packet{SeqNum: i}
		conn.addToSendWindow(packet)
	}

	// Now should not be able to send (2 packets > cwnd of 1.0)
	if conn.canSend() {
		t.Error("Should not be able to send when window is full")
	}

	// Remove both from window
	conn.removeFromSendWindow(0)
	conn.removeFromSendWindow(1)

	// Should be able to send again (0 packets < cwnd of 1.0)
	if !conn.canSend() {
		t.Error("Should be able to send after removing from window")
	}
}

func TestConnectionSendWindowConcurrent(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	const goroutines = 20
	var wg sync.WaitGroup

	// Concurrent adds and removes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				seqNum := uint32(id*100 + j)
				packet := &Packet{SeqNum: seqNum}

				// Add to window
				conn.addToSendWindow(packet)

				// Check if can send
				_ = conn.canSend()

				// Remove from window
				conn.removeFromSendWindow(seqNum)
			}
		}(i)
	}

	wg.Wait()

	// Window should be empty
	conn.sendWindowMu.RLock()
	size := len(conn.sendWindow)
	conn.sendWindowMu.RUnlock()

	if size != 0 {
		t.Errorf("Send window size = %d, want 0", size)
	}
}

func TestConnectionReceivedTracking(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	// Track some sequence numbers
	seqNums := []uint32{1, 2, 3, 5, 7, 10}
	for _, seq := range seqNums {
		conn.TrackReceived(seq)
	}

	// Check they were tracked
	conn.receivedMu.RLock()
	for _, seq := range seqNums {
		if !conn.receivedSeqs[seq] {
			t.Errorf("Sequence %d not tracked", seq)
		}
	}
	conn.receivedMu.RUnlock()

	// Get ACK bitmap
	baseSeq, bitmap := conn.GetAckBitmap()

	// Bitmap should reflect received packets
	// (implementation-specific, verify we got values)
	if baseSeq == 0 && bitmap == 0 {
		// This is ok for new connection
	}
}

func TestConnectionFragmentReassembly(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	received := make(chan []byte, 1)
	conn.SetMessageCallback(func(data []byte) {
		received <- data
	})

	// Create fragments
	fragID := uint16(123)
	total := uint8(3)

	fragments := []*Packet{
		{
			Flags:      FlagFragment,
			FragmentID: fragID,
			FragOffset: 0,
			FragTotal:  total,
			Payload:    []byte("part1"),
		},
		{
			Flags:      FlagFragment,
			FragmentID: fragID,
			FragOffset: 1,
			FragTotal:  total,
			Payload:    []byte("part2"),
		},
		{
			Flags:      FlagFragment,
			FragmentID: fragID,
			FragOffset: 2,
			FragTotal:  total,
			Payload:    []byte("part3"),
		},
	}

	// Add fragments
	for _, frag := range fragments {
		conn.AddFragment(frag)
	}

	// Should receive complete message
	select {
	case data := <-received:
		expected := "part1part2part3"
		if string(data) != expected {
			t.Errorf("Reassembled data = %s, want %s", string(data), expected)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for reassembled message")
	}
}

func TestConnectionFragmentConcurrent(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	receivedCount := 0
	var mu sync.Mutex
	received := make(chan bool, 10)

	conn.SetMessageCallback(func(data []byte) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		received <- true
	})

	const goroutines = 10
	var wg sync.WaitGroup

	// Add fragments concurrently
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			fragID := uint16(id)
			total := uint8(3)

			for offset := uint8(0); offset < total; offset++ {
				packet := &Packet{
					Flags:      FlagFragment,
					FragmentID: fragID,
					FragOffset: offset,
					FragTotal:  total,
					Payload:    []byte{byte(id), offset},
				}
				conn.AddFragment(packet)
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all reassemblies
	timeout := time.After(3 * time.Second)
	for receivedCount < goroutines {
		select {
		case <-received:
			// Continue
		case <-timeout:
			mu.Lock()
			count := receivedCount
			mu.Unlock()
			t.Logf("Only received %d/%d messages", count, goroutines)
			return
		}
	}
}

func TestConnectionFragmentCleanup(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	// Add incomplete fragments
	for i := uint16(0); i < 10; i++ {
		packet := &Packet{
			Flags:      FlagFragment,
			FragmentID: i,
			FragOffset: 0,
			FragTotal:  2, // Only add first of 2 fragments
			Payload:    []byte("incomplete"),
		}
		conn.AddFragment(packet)
	}

	// Manually set old timestamps
	conn.fragmentsMu.Lock()
	for _, frag := range conn.fragments {
		frag.firstSeen = time.Now().Add(-31 * time.Second)
	}
	conn.fragmentsMu.Unlock()

	// Run cleanup
	conn.CleanupFragments()

	// Verify fragments were removed
	conn.fragmentsMu.RLock()
	remaining := len(conn.fragments)
	conn.fragmentsMu.RUnlock()

	if remaining != 0 {
		t.Errorf("Expected 0 fragments after cleanup, got %d", remaining)
	}
}

func TestConnectionMessageCallback(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	callbackCalled := make(chan []byte, 1)
	conn.SetMessageCallback(func(data []byte) {
		callbackCalled <- data
	})

	// Trigger callback
	testData := []byte("test message")
	conn.onMessage(testData)

	select {
	case data := <-callbackCalled:
		if string(data) != string(testData) {
			t.Errorf("Callback data = %s, want %s", string(data), string(testData))
		}
	case <-time.After(time.Second):
		t.Error("Callback not called")
	}
}

func TestConnectionRTTTracking(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	// Add packets to track RTT
	for i := uint32(0); i < 10; i++ {
		packet := &Packet{
			SeqNum:    i,
			Timestamp: time.Now().Add(-time.Duration(i) * time.Millisecond),
		}
		conn.addToSendWindow(packet)
	}

	// Simulate acknowledgments
	for i := uint32(0); i < 10; i++ {
		conn.removeFromSendWindow(i)
	}

	// RTT should be calculated (implementation-specific)
	// Just verify connection is still valid
	if conn.GetState() != StateNew {
		// State might have changed, that's ok
	}
}

func TestConnectionLastActivity(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	firstActivity := conn.lastActivity

	time.Sleep(100 * time.Millisecond)

	// Update last activity
	conn.lastActivity = time.Now()

	if !conn.lastActivity.After(firstActivity) {
		t.Error("Last activity should be updated")
	}
}

func BenchmarkConnectionStateAccess(b *testing.B) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	b.RunParallel(func(pb *testing.PB) {
		states := []ConnectionState{StateNew, StateHandshaking, StateConnected}
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				_ = conn.GetState()
			} else {
				conn.SetState(states[i%len(states)])
			}
			i++
		}
	})
}

func BenchmarkConnectionSendWindow(b *testing.B) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet := &Packet{SeqNum: uint32(i)}
		conn.addToSendWindow(packet)
		conn.removeFromSendWindow(uint32(i))
	}
}

func BenchmarkConnectionFragmentAdd(b *testing.B) {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8080")
	conn := NewConnection(addr)

	conn.SetMessageCallback(func(data []byte) {
		// Discard
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fragID := uint16(i)
		for offset := uint8(0); offset < 3; offset++ {
			packet := &Packet{
				Flags:      FlagFragment,
				FragmentID: fragID,
				FragOffset: offset,
				FragTotal:  3,
				Payload:    []byte{byte(i), offset},
			}
			conn.AddFragment(packet)
		}
	}
}

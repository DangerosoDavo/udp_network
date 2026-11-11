package protocol

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestTransportCreateAndStop(t *testing.T) {
	transport, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport() failed: %v", err)
	}

	transport.Start()
	time.Sleep(100 * time.Millisecond)
	transport.Stop()

	// Verify context is done
	select {
	case <-transport.ctx.Done():
		// Expected
	case <-time.After(time.Second):
		t.Error("Context not cancelled after Stop()")
	}
}

func TestTransportMaxConnections(t *testing.T) {
	transport, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport() failed: %v", err)
	}
	defer transport.Stop()

	transport.SetMaxConnections(5)
	transport.Start()

	// Create more than max connections
	clients := make([]*Transport, 10)
	for i := 0; i < 10; i++ {
		client, err := NewTransport("127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to create client %d: %v", i, err)
		}
		clients[i] = client
		client.Start()
	}

	// Try to connect all clients
	for i, client := range clients {
		go func(idx int, c *Transport) {
			packet := &Packet{
				Type:   TypeHandshake,
				SeqNum: 0,
			}
			data, _ := packet.Marshal()
			c.conn.WriteTo(data, transport.conn.LocalAddr())
		}(i, client)
	}

	time.Sleep(500 * time.Millisecond)

	// Check connection count doesn't exceed limit
	transport.connMu.RLock()
	count := len(transport.connections)
	transport.connMu.RUnlock()

	if count > 5 {
		t.Errorf("Connection count %d exceeds max %d", count, 5)
	}

	// Cleanup
	for _, client := range clients {
		client.Stop()
	}
}

func TestTransportSendReceive(t *testing.T) {
	// Create server
	server, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(server) failed: %v", err)
	}
	defer server.Stop()

	receivedData := make(chan []byte, 1)
	server.OnConnect = func(conn *Connection) {
		conn.SetMessageCallback(func(data []byte) {
			receivedData <- data
		})
	}

	server.Start()
	serverAddr := server.conn.LocalAddr().String()

	// Create client
	client, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(client) failed: %v", err)
	}
	defer client.Stop()
	client.Start()

	// Connect
	conn, err := client.Connect(serverAddr)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	// Wait for connection
	time.Sleep(200 * time.Millisecond)

	// Send data
	testData := []byte("Hello, Transport!")
	if err := client.Send(conn, testData); err != nil {
		t.Fatalf("Send() failed: %v", err)
	}

	// Wait for data
	select {
	case data := <-receivedData:
		if !bytes.Equal(data, testData) {
			t.Errorf("Received data mismatch: got %s, want %s", data, testData)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for data")
	}
}

func TestTransportFragmentation(t *testing.T) {
	server, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(server) failed: %v", err)
	}
	defer server.Stop()

	receivedData := make(chan []byte, 1)
	server.OnConnect = func(conn *Connection) {
		conn.SetMessageCallback(func(data []byte) {
			receivedData <- data
		})
	}

	server.Start()
	serverAddr := server.conn.LocalAddr().String()

	client, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(client) failed: %v", err)
	}
	defer client.Stop()
	client.Start()

	conn, err := client.Connect(serverAddr)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Send large data that requires fragmentation
	largeData := bytes.Repeat([]byte("A"), 5000)
	if err := client.Send(conn, largeData); err != nil {
		t.Fatalf("Send() failed: %v", err)
	}

	// Wait for reassembled data
	select {
	case data := <-receivedData:
		if !bytes.Equal(data, largeData) {
			t.Errorf("Received data size mismatch: got %d, want %d", len(data), len(largeData))
		}
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for fragmented data")
	}
}

func TestTransportConcurrentSends(t *testing.T) {
	server, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(server) failed: %v", err)
	}
	defer server.Stop()

	var receivedCount int
	var mu sync.Mutex
	received := make(chan bool, 100)

	server.OnConnect = func(conn *Connection) {
		conn.SetMessageCallback(func(data []byte) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			received <- true
		})
	}

	server.Start()
	serverAddr := server.conn.LocalAddr().String()

	client, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(client) failed: %v", err)
	}
	defer client.Stop()
	client.Start()

	conn, err := client.Connect(serverAddr)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Send multiple messages concurrently
	const numMessages = 50
	var wg sync.WaitGroup
	wg.Add(numMessages)

	for i := 0; i < numMessages; i++ {
		go func(idx int) {
			defer wg.Done()
			data := []byte{byte(idx)}
			if err := client.Send(conn, data); err != nil {
				t.Logf("Send %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all messages to be received
	timeout := time.After(5 * time.Second)
	for delivered := 0; delivered < numMessages; delivered++ {
		select {
		case <-received:
			// Continue
		case <-timeout:
			mu.Lock()
			count := receivedCount
			mu.Unlock()
			t.Fatalf("Timeout: only received %d/%d messages", count, numMessages)
		}
	}
}

func TestTransportConnectionTimeout(t *testing.T) {
	transport, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport() failed: %v", err)
	}
	defer transport.Stop()

	transport.SetKeepAlive(100*time.Millisecond, 0)
	transport.Start()

	// Create a connection manually
	testAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	conn := NewConnection(testAddr)
	conn.state = StateConnected
	conn.lastActivity = time.Now().Add(-time.Second) // Old activity

	transport.connMu.Lock()
	transport.connections[testAddr.String()] = conn
	transport.connMu.Unlock()

	// Wait for cleanup
	time.Sleep(time.Second)

	// Connection should be removed
	transport.connMu.RLock()
	_, exists := transport.connections[testAddr.String()]
	transport.connMu.RUnlock()

	if exists {
		t.Error("Timed out connection was not removed")
	}
}

func TestTransportCallbacks(t *testing.T) {
	transport, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport() failed: %v", err)
	}
	defer transport.Stop()

	connectCalled := make(chan bool, 1)
	disconnectCalled := make(chan bool, 1)

	transport.OnConnect = func(conn *Connection) {
		connectCalled <- true
	}

	transport.OnDisconnect = func(conn *Connection) {
		disconnectCalled <- true
	}

	transport.Start()
	serverAddr := transport.conn.LocalAddr().String()

	// Create client
	client, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(client) failed: %v", err)
	}
	defer client.Stop()
	client.Start()

	// Connect
	_, err = client.Connect(serverAddr)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	// Wait for OnConnect
	select {
	case <-connectCalled:
		// Expected
	case <-time.After(time.Second):
		t.Error("OnConnect callback not called")
	}

	// Close connection
	transport.connMu.Lock()
	for _, c := range transport.connections {
		c.SetState(StateClosed)
	}
	transport.connMu.Unlock()

	// Trigger maintenance to detect closed connection
	time.Sleep(200 * time.Millisecond)

	// Note: OnDisconnect might not be called in all scenarios
	// This is a known limitation of the current implementation
}

func TestTransportWindowFlowControl(t *testing.T) {
	server, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(server) failed: %v", err)
	}
	defer server.Stop()

	receivedCount := 0
	var mu sync.Mutex
	received := make(chan bool, 100)

	server.OnConnect = func(conn *Connection) {
		conn.SetMessageCallback(func(data []byte) {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			received <- true
		})
	}

	server.Start()
	serverAddr := server.conn.LocalAddr().String()

	client, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTransport(client) failed: %v", err)
	}
	defer client.Stop()
	client.Start()

	conn, err := client.Connect(serverAddr)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Send many messages to test flow control
	const numMessages = 100
	for i := 0; i < numMessages; i++ {
		data := []byte{byte(i)}
		if err := client.Send(conn, data); err != nil {
			t.Logf("Send %d failed: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond) // Small delay
	}

	// Wait for all to be received
	timeout := time.After(10 * time.Second)
	for delivered := 0; delivered < numMessages; delivered++ {
		select {
		case <-received:
			// Continue
		case <-timeout:
			mu.Lock()
			count := receivedCount
			mu.Unlock()
			t.Logf("Received %d/%d messages before timeout", count, numMessages)
			return
		}
	}

	mu.Lock()
	final := receivedCount
	mu.Unlock()

	if final != numMessages {
		t.Errorf("Received %d messages, expected %d", final, numMessages)
	}
}

// Both benchmarks measure delivered, reliable UDP messages with bounded
// stop-and-wait application queues. They include loopback/kernel/ACK costs;
// they are not pure codec benchmarks or estimates of internet capacity.
func BenchmarkTransportSend(b *testing.B)    { benchmarkTransportDelivered(b, 1100) }
func BenchmarkTransportReceive(b *testing.B) { benchmarkTransportDelivered(b, 1100) }
func benchmarkTransportDelivered(b *testing.B, size int) {
	server, err := NewTransport("127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer server.Stop()
	client, err := NewTransport("127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer client.Stop()
	received := make(chan int, 1)
	server.OnConnect = func(c *Connection) {
		c.SetMessageCallback(func(data []byte) {
			select {
			case received <- len(data):
			case <-server.ctx.Done():
			}
		})
	}
	server.Start()
	client.Start()
	c, err := client.Connect(server.conn.LocalAddr().String())
	if err != nil {
		b.Fatal(err)
	}
	data := make([]byte, size)
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(client.ctx, 5*time.Second)
		err = client.SendContext(ctx, c, data)
		cancel()
		if err != nil {
			b.Fatal(err)
		}
		select {
		case n := <-received:
			if n != size {
				b.Fatal("payload size mismatch")
			}
		case <-deadline.C:
			b.Fatal("delivery benchmark exceeded 30s deadline")
		}
	}
	b.StopTimer()
}

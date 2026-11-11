package udp_network

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

func TestWebSocketServerCreateAndStop(t *testing.T) {
	server, err := NewWebSocketServer("localhost:0", &ServerOptions{
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)
	server.Stop()

	time.Sleep(100 * time.Millisecond)
	// If we get here without panic, server started and stopped cleanly
}

func TestWebSocketClientConnectDisconnect(t *testing.T) {
	// Start server
	server, err := NewWebSocketServer("localhost:18082", &ServerOptions{
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	// Create client
	client, err := NewWebSocketClient(&ClientOptions{})
	if err != nil {
		t.Fatalf("NewWebSocketClient() failed: %v", err)
	}
	defer client.Stop()

	// Connect
	if err := client.Connect("ws://localhost:18082"); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	// Disconnect
	client.Stop()
	time.Sleep(100 * time.Millisecond)

	if client.IsConnected() {
		t.Error("Client should be disconnected")
	}
}

func TestWebSocketSendReceive(t *testing.T) {
	// Start server
	server, err := NewWebSocketServer("localhost:18083", &ServerOptions{
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	receivedOnServer := make(chan *protocol.Message, 1)
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		receivedOnServer <- msg
	})

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	// Create client
	client, err := NewWebSocketClient(&ClientOptions{})
	if err != nil {
		t.Fatalf("NewWebSocketClient() failed: %v", err)
	}
	defer client.Stop()

	receivedOnClient := make(chan *protocol.Message, 1)
	client.SetMessageHandler(func(msg *protocol.Message) {
		receivedOnClient <- msg
	})

	// Connect
	if err := client.Connect("ws://localhost:18083"); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Drain welcome message from server
	select {
	case <-receivedOnClient:
		// Welcome message received, discard it
	case <-time.After(time.Second):
		t.Error("Timeout waiting for welcome message")
	}

	// Client -> Server (using SendText)
	if err := client.SendText("Hello Server"); err != nil {
		t.Fatalf("SendText() failed: %v", err)
	}

	select {
	case msg := <-receivedOnServer:
		if msg.Data != "Hello Server" {
			t.Errorf("Server received wrong data: got %v, want 'Hello Server'", msg.Data)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for message on server")
	}

	// Server -> Client (echo back)
	server.clientsMu.RLock()
	var serverConn *protocol.Connection
	for _, wsConn := range server.clients {
		serverConn = wsConn.Connection
		break
	}
	server.clientsMu.RUnlock()

	if serverConn != nil {
		echoMsg := &protocol.Message{
			Type:      protocol.MessageTypeText,
			ID:        "test-2",
			Timestamp: time.Now().Unix(),
			Data:      "Hello Client",
		}
		if err := server.SendMessage(serverConn, echoMsg); err != nil {
			t.Fatalf("Server SendMessage() failed: %v", err)
		}

		select {
		case msg := <-receivedOnClient:
			if msg.Data != "Hello Client" {
				t.Errorf("Client received wrong data: got %v, want 'Hello Client'", msg.Data)
			}
		case <-time.After(2 * time.Second):
			t.Error("Timeout waiting for message on client")
		}
	}
}

func TestWebSocketMultipleClients(t *testing.T) {
	server, err := NewWebSocketServer("localhost:18084", &ServerOptions{
		MaxClients: 5,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	// Create multiple clients
	const numClients = 3
	clients := make([]*WebSocketClient, numClients)

	for i := 0; i < numClients; i++ {
		client, err := NewWebSocketClient(&ClientOptions{})
		if err != nil {
			t.Fatalf("NewWebSocketClient(%d) failed: %v", i, err)
		}
		clients[i] = client

		// Set handler to suppress output
		client.SetMessageHandler(func(msg *protocol.Message) {
			// Discard messages
		})

		if err := client.Connect("ws://localhost:18084"); err != nil {
			t.Fatalf("Client %d Connect() failed: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond) // Longer wait for all connections

	// Check server has correct client count
	count := server.GetClientCount()
	if count != numClients {
		t.Errorf("Server has %d clients, expected %d", count, numClients)
	}

	// Cleanup
	for _, client := range clients {
		if client != nil {
			client.Stop()
		}
	}

	time.Sleep(200 * time.Millisecond)

	// Check clients disconnected
	count = server.GetClientCount()
	if count != 0 {
		t.Errorf("Server has %d clients after disconnect, expected 0", count)
	}
}

func TestWebSocketBroadcast(t *testing.T) {
	server, err := NewWebSocketServer("localhost:18085", &ServerOptions{
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	// Create multiple clients
	const numClients = 3
	clients := make([]*WebSocketClient, numClients)
	receivedChans := make([]chan *protocol.Message, numClients)

	for i := 0; i < numClients; i++ {
		client, err := NewWebSocketClient(&ClientOptions{})
		if err != nil {
			t.Fatalf("NewWebSocketClient(%d) failed: %v", i, err)
		}
		clients[i] = client

		receivedChans[i] = make(chan *protocol.Message, 2) // Buffer for welcome + broadcast
		ch := receivedChans[i]                             // Capture for closure
		client.SetMessageHandler(func(msg *protocol.Message) {
			ch <- msg
		})

		if err := client.Connect("ws://localhost:18085"); err != nil {
			t.Fatalf("Client %d Connect() failed: %v", i, err)
		}
	}

	time.Sleep(300 * time.Millisecond)

	// Drain welcome messages from all clients
	for i := 0; i < numClients; i++ {
		select {
		case <-receivedChans[i]:
			// Welcome message, discard
		case <-time.After(time.Second):
			t.Errorf("Client %d: timeout waiting for welcome message", i)
		}
	}

	// Broadcast message
	broadcastMsg := &protocol.Message{
		Type:      protocol.MessageTypeText,
		ID:        "broadcast-1",
		Timestamp: time.Now().Unix(),
		Data:      "Hello Everyone",
	}

	server.Broadcast(broadcastMsg)

	// Verify all clients received it
	for i := 0; i < numClients; i++ {
		select {
		case msg := <-receivedChans[i]:
			if msg.Data != "Hello Everyone" {
				t.Errorf("Client %d received wrong data: %v", i, msg.Data)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("Client %d timeout waiting for broadcast", i)
		}
	}

	// Cleanup
	for _, client := range clients {
		client.Stop()
	}
}

func TestWebSocketConcurrentMessages(t *testing.T) {
	server, err := NewWebSocketServer("localhost:18087", &ServerOptions{
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	receivedCount := 0
	var mu sync.Mutex
	received := make(chan bool, 100)

	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		received <- true
	})

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	client, err := NewWebSocketClient(&ClientOptions{})
	if err != nil {
		t.Fatalf("NewWebSocketClient() failed: %v", err)
	}
	defer client.Stop()

	if err := client.Connect("ws://localhost:18087"); err != nil {
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
			text := fmt.Sprintf("Message %d", idx)
			if err := client.SendText(text); err != nil {
				t.Errorf("SendText %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all messages
	timeout := time.After(5 * time.Second)
	for i := 0; i < numMessages; i++ {
		select {
		case <-received:
			// Continue
		case <-timeout:
			mu.Lock()
			count := receivedCount
			mu.Unlock()
			t.Fatalf("Only received %d/%d messages before timeout", count, numMessages)
		}
	}
}

func TestWebSocketMaxConnections(t *testing.T) {
	const maxClients = 3
	server, err := NewWebSocketServer("localhost:18088", &ServerOptions{
		MaxClients: maxClients,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	// Try to connect more than max
	clients := make([]*WebSocketClient, maxClients+2)

	for i := 0; i < maxClients+2; i++ {
		client, err := NewWebSocketClient(&ClientOptions{})
		if err != nil {
			continue
		}
		clients[i] = client

		_ = client.Connect("ws://localhost:18088")
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(300 * time.Millisecond)

	// Check server enforced limit
	count := server.GetClientCount()
	if count > maxClients {
		t.Errorf("Server has %d clients, max is %d", count, maxClients)
	}

	// Cleanup
	for _, client := range clients {
		if client != nil {
			client.Stop()
		}
	}
}

func TestWebSocketReconnect(t *testing.T) {
	server, err := NewWebSocketServer("localhost:18089", &ServerOptions{
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	client, err := NewWebSocketClient(&ClientOptions{})
	if err != nil {
		t.Fatalf("NewWebSocketClient() failed: %v", err)
	}
	defer client.Stop()

	// Initial connect
	if err := client.Connect("ws://localhost:18089"); err != nil {
		t.Fatalf("Initial Connect() failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	// Disconnect
	client.Stop()
	time.Sleep(200 * time.Millisecond)

	// Create new client for reconnect
	client2, err := NewWebSocketClient(&ClientOptions{})
	if err != nil {
		t.Fatalf("NewWebSocketClient() failed: %v", err)
	}
	defer client2.Stop()

	// Reconnect
	if err := client2.Connect("ws://localhost:18089"); err != nil {
		t.Fatalf("Reconnect() failed: %v", err)
	}

	if !client2.IsConnected() {
		t.Error("Client should be reconnected")
	}
}

func TestWebSocketConnectionCallbacks(t *testing.T) {
	connectCalled := make(chan bool, 1)
	disconnectCalled := make(chan bool, 1)

	server, err := NewWebSocketServer("localhost:18090", &ServerOptions{
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("NewWebSocketServer() failed: %v", err)
	}

	server.SetConnectionCallbacks(
		func(conn *protocol.Connection) {
			connectCalled <- true
		},
		func(conn *protocol.Connection) {
			disconnectCalled <- true
		},
	)

	go server.Start()
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	client, err := NewWebSocketClient(&ClientOptions{})
	if err != nil {
		t.Fatalf("NewWebSocketClient() failed: %v", err)
	}

	// Connect
	if err := client.Connect("ws://localhost:18090"); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	// Wait for connect callback
	select {
	case <-connectCalled:
		// Expected
	case <-time.After(2 * time.Second):
		t.Error("Connect callback not called")
	}

	// Disconnect
	client.Stop()

	// Wait for disconnect callback
	select {
	case <-disconnectCalled:
		// Expected
	case <-time.After(2 * time.Second):
		t.Error("Disconnect callback not called")
	}
}

// Both names now measure delivered roundtrips, with a bounded acknowledgement
// channel and observable errors rather than enqueue-only or ignored-failure rates.
func benchmarkWebSocketRoundTrip(b *testing.B) {
	server, err := NewWebSocketServer("127.0.0.1:0", nil)
	if err != nil {
		b.Fatal(err)
	}
	sendErrors := make(chan error, 1)
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if err := server.SendMessage(conn, msg); err != nil {
			select {
			case sendErrors <- err:
			default:
			}
		}
	})
	if err := server.StartWithError(); err != nil {
		b.Fatal(err)
	}
	defer server.Stop()
	client, err := NewWebSocketClient(nil)
	if err != nil {
		b.Fatal(err)
	}
	defer client.Stop()
	received := make(chan struct{}, 1)
	client.SetMessageHandler(func(msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeText {
			received <- struct{}{}
		}
	})
	if err := client.Connect("ws://" + server.Addr().String()); err != nil {
		b.Fatal(err)
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	const payload = "benchmark message"
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.SendText(payload); err != nil {
			b.Fatal(err)
		}
		select {
		case <-received:
		case err := <-sendErrors:
			b.Fatal(err)
		case <-timer.C:
			b.Fatal("roundtrip timeout")
		}
	}
}
func BenchmarkWebSocketSendMessage(b *testing.B)    { benchmarkWebSocketRoundTrip(b) }
func BenchmarkWebSocketReceiveMessage(b *testing.B) { benchmarkWebSocketRoundTrip(b) }

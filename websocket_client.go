package udp_network

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
	"github.com/gorilla/websocket"
)

// WebSocketClient implements UnifiedClient using WebSockets
type WebSocketClient struct {
	ws              *websocket.Conn
	connection      *protocol.Connection
	serverAddr      string
	customHandler   func(*protocol.Message)
	codec           *protocol.MessageCodec
	mu              sync.Mutex
	connected       bool
	heartbeatStop   chan bool
	readLoopStop    chan bool
	timeout         time.Duration
	keepAlive       time.Duration
	enableLogging   bool
}

// NewWebSocketClient creates a new WebSocket client
func NewWebSocketClient(options *ClientOptions) (*WebSocketClient, error) {
	if options == nil {
		options = &ClientOptions{}
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	keepAlive := options.KeepAlive
	if keepAlive == 0 {
		keepAlive = 10 * time.Second
	}

	return &WebSocketClient{
		codec:         protocol.NewMessageCodec(),
		timeout:       timeout,
		keepAlive:     keepAlive,
		enableLogging: options.EnableLogging,
	}, nil
}

func (c *WebSocketClient) Connect(serverAddr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected to %s", c.serverAddr)
	}

	// Add ws:// protocol if not present
	if len(serverAddr) > 0 && serverAddr[0] != 'w' {
		serverAddr = "ws://" + serverAddr
	}

	c.serverAddr = serverAddr

	// Create WebSocket connection
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	ws, _, err := dialer.Dial(serverAddr, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.ws = ws
	c.connected = true

	// Create protocol connection wrapper
	remoteAddr := &net.UDPAddr{
		IP:   net.ParseIP("websocket"),
		Port: 0,
	}
	c.connection = protocol.NewConnection(remoteAddr)
	c.connection.SetState(protocol.StateConnected)

	if c.enableLogging {
		log.Printf("Connected to WebSocket server: %s", serverAddr)
	}

	// Start read loop
	c.readLoopStop = make(chan bool)
	go c.readLoop()

	// Start heartbeat
	c.startHeartbeat()

	return nil
}

func (c *WebSocketClient) Start() {
	// WebSocket client doesn't need explicit start
}

func (c *WebSocketClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop heartbeat
	if c.heartbeatStop != nil {
		close(c.heartbeatStop)
		c.heartbeatStop = nil
	}

	// Stop read loop
	if c.readLoopStop != nil {
		close(c.readLoopStop)
		c.readLoopStop = nil
	}

	// Close WebSocket
	if c.ws != nil {
		c.ws.Close()
		c.ws = nil
	}

	c.connected = false

	if c.enableLogging {
		log.Println("WebSocket client stopped")
	}
}

func (c *WebSocketClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.readLoopStop:
			return
		default:
		}

		c.ws.SetReadDeadline(time.Now().Add(c.timeout))
		messageType, data, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if c.enableLogging {
					log.Printf("WebSocket read error: %v", err)
				}
			}
			return
		}

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			// Decode message
			msg, err := c.codec.Decode(data)
			if err != nil {
				if c.enableLogging {
					log.Printf("Message decode error: %v", err)
				}
				continue
			}

			// Handle message
			if c.customHandler != nil {
				c.customHandler(msg)
			} else {
				c.handleMessage(msg)
			}
		}
	}
}

func (c *WebSocketClient) handleMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypeText:
		fmt.Printf("\n[Server Text]: %s\n> ", msg.Data)
	case protocol.MessageTypeJSON:
		if data, ok := msg.Data.(map[string]interface{}); ok {
			fmt.Printf("\n[Server JSON]: %v\n> ", data)
		}
	}
}

func (c *WebSocketClient) SendText(text string) error {
	// Check if text is large enough to require chunking
	const largeStringThreshold = 250 * 1024

	if len(text) > largeStringThreshold {
		fmt.Printf("📦 Text size (%d bytes) exceeds threshold, using chunked transfer\n", len(text))
		return c.SendLargeString(text, &LargeStringOptions{
			Metadata: map[string]interface{}{
				"type":          "auto-chunked-text",
				"original_size": len(text),
			},
		})
	}

	msg := &protocol.Message{
		Type:      protocol.MessageTypeText,
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().Unix(),
		Data:      text,
	}

	return c.SendMessage(c.connection, msg)
}

func (c *WebSocketClient) SendCommand(cmd string, params map[string]interface{}) error {
	data := map[string]interface{}{
		"command": cmd,
	}

	for k, v := range params {
		data[k] = v
	}

	msg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	return c.SendMessage(c.connection, msg)
}

func (c *WebSocketClient) SendJSON(data interface{}) error {
	msg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	return c.SendMessage(c.connection, msg)
}

func (c *WebSocketClient) SendMessage(conn *protocol.Connection, msg *protocol.Message) error {
	c.mu.Lock()
	if !c.connected || c.ws == nil {
		c.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	ws := c.ws
	c.mu.Unlock()

	// Encode message
	data, err := c.codec.Encode(msg)
	if err != nil {
		return fmt.Errorf("encode error: %w", err)
	}

	// Send via WebSocket
	ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return ws.WriteMessage(websocket.TextMessage, data)
}

func (c *WebSocketClient) SetMessageHandler(handler func(*protocol.Message)) {
	c.customHandler = handler
}

func (c *WebSocketClient) SetKeepAlive(timeout, interval time.Duration) {
	c.timeout = timeout
	c.keepAlive = interval
}

func (c *WebSocketClient) GetTransportType() TransportType {
	return TransportWebSocket
}

func (c *WebSocketClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

func (c *WebSocketClient) startHeartbeat() {
	c.heartbeatStop = make(chan bool)

	go func() {
		ticker := time.NewTicker(c.keepAlive)
		defer ticker.Stop()

		for {
			select {
			case <-c.heartbeatStop:
				return
			case <-ticker.C:
				c.mu.Lock()
				ws := c.ws
				connected := c.connected
				c.mu.Unlock()

				if ws != nil && connected {
					// Send ping
					ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
					if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
						if c.enableLogging {
							log.Printf("Ping failed: %v", err)
						}
					}
				}
			}
		}
	}()
}

// SendLargeString sends a large string using chunked transfer
func (c *WebSocketClient) SendLargeString(largeString string, options *LargeStringOptions) error {
	if c.connection == nil {
		return fmt.Errorf("not connected")
	}

	// Default options
	if options == nil {
		options = &LargeStringOptions{}
	}
	if options.RetryAttempts == 0 {
		options.RetryAttempts = 3
	}
	if options.ChunkTimeout == 0 {
		options.ChunkTimeout = 5 * time.Second
	}

	// Generate chunks
	chunks, err := protocol.ChunkString(largeString, options.TransferID, options.Metadata)
	if err != nil {
		return fmt.Errorf("failed to chunk string: %v", err)
	}

	fmt.Printf("📦 Sending large string in %d chunks (%.2f MB total)\n",
		len(chunks), float64(len(largeString))/(1024*1024))

	// Send chunks with retry logic
	for i, chunk := range chunks {
		sent := false

		for attempt := 0; attempt < options.RetryAttempts; attempt++ {
			msg := &protocol.Message{
				Type:      protocol.MessageTypeChunked,
				ID:        fmt.Sprintf("%s_%d", chunk.Metadata.TransferID, i),
				Timestamp: time.Now().Unix(),
				Data:      chunk,
			}

			if err := c.SendMessage(c.connection, msg); err != nil {
				if attempt < options.RetryAttempts-1 {
					fmt.Printf("⚠️  Chunk %d/%d failed (attempt %d), retrying...\n",
						i+1, len(chunks), attempt+1)
					time.Sleep(time.Second)
					continue
				}
				return fmt.Errorf("failed to send chunk %d after %d attempts: %v",
					i, options.RetryAttempts, err)
			}

			sent = true
			break
		}

		if !sent {
			return fmt.Errorf("failed to send chunk %d", i)
		}

		// Progress callback
		if options.OnProgress != nil {
			options.OnProgress(i+1, len(chunks))
		}

		// Small delay between chunks
		if i < len(chunks)-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	fmt.Printf("✅ Large string sent successfully (%d chunks)\n", len(chunks))
	return nil
}

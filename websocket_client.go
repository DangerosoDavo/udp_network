package udp_network

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
	"github.com/gorilla/websocket"
)

type webSocketClientSession struct {
	ws        *websocket.Conn
	conn      *protocol.Connection
	done      chan struct{}
	once      sync.Once
	writeGate chan struct{}
}

func (s *webSocketClientSession) close() {
	s.once.Do(func() { close(s.done); _ = s.ws.Close(); s.conn.SetState(protocol.StateClosed) })
}

type WebSocketClient struct {
	mu                 sync.RWMutex
	connectMu          sync.Mutex
	session            *webSocketClientSession
	connection         *protocol.Connection
	dialCancel         context.CancelFunc
	epoch              uint64
	customHandler      func(*protocol.Message)
	rawHandler         func([]byte)
	borrowedRawHandler func([]byte)
	codec              *protocol.MessageCodec
	timeout, keepAlive time.Duration
	options            ClientOptions
}

func NewWebSocketClient(options *ClientOptions) (*WebSocketClient, error) {
	var opts ClientOptions
	if options != nil {
		opts = *options
	}
	if opts.Timeout < 0 || opts.KeepAlive < 0 || opts.MaxMessageBytes < 0 {
		return nil, fmt.Errorf("websocket limits must be nonnegative")
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.KeepAlive == 0 {
		opts.KeepAlive = 10 * time.Second
	}
	if opts.MaxMessageBytes == 0 {
		opts.MaxMessageBytes = 1 << 20
	}
	if opts.KeepAlive >= opts.Timeout {
		return nil, fmt.Errorf("keepalive must be shorter than timeout")
	}
	return &WebSocketClient{codec: protocol.NewMessageCodec(), timeout: opts.Timeout, keepAlive: opts.KeepAlive, options: opts}, nil
}

func (c *WebSocketClient) Connect(address string) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	if !strings.Contains(address, "://") {
		address = "ws://" + address
	}
	c.mu.Lock()
	if c.session != nil {
		select {
		case <-c.session.done:
		default:
			c.mu.Unlock()
			return fmt.Errorf("already connected")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	c.dialCancel = cancel
	c.epoch++
	epoch := c.epoch
	c.mu.Unlock()
	defer cancel()
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, TLSClientConfig: c.options.TLSConfig}
	ws, response, err := dialer.DialContext(ctx, address, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		c.mu.Lock()
		if c.epoch == epoch {
			c.dialCancel = nil
		}
		c.mu.Unlock()
		return fmt.Errorf("websocket dial: %w", err)
	}
	addr, _ := net.ResolveUDPAddr("udp", ws.RemoteAddr().String())
	if addr == nil {
		addr = &net.UDPAddr{}
	}
	conn := protocol.NewConnection(addr)
	conn.SetState(protocol.StateConnected)
	session := &webSocketClientSession{ws: ws, conn: conn, done: make(chan struct{}), writeGate: make(chan struct{}, 1)}
	c.mu.Lock()
	if c.epoch != epoch {
		c.mu.Unlock()
		session.close()
		return context.Canceled
	}
	c.session = session
	c.connection = conn
	c.dialCancel = nil
	timeout, interval := c.timeout, c.keepAlive
	c.mu.Unlock()
	ws.SetReadLimit(c.options.MaxMessageBytes)
	_ = ws.SetReadDeadline(time.Now().Add(timeout))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(timeout)) })
	go c.readLoop(session, timeout)
	go c.heartbeat(session, interval)
	return nil
}

func (c *WebSocketClient) Start() {}
func (c *WebSocketClient) Stop() {
	c.mu.Lock()
	c.epoch++
	if c.dialCancel != nil {
		c.dialCancel()
		c.dialCancel = nil
	}
	session := c.session
	c.session = nil
	c.connection = nil
	c.mu.Unlock()
	if session != nil {
		session.close()
	}
}

func (c *WebSocketClient) readLoop(session *webSocketClientSession, timeout time.Duration) {
	defer session.close()
	var buffer []byte
	var probe [1]byte
	for {
		kind, reader, err := session.ws.NextReader()
		if err != nil {
			return
		}
		buffer, err = readWebSocketFrame(reader, buffer, probe[:], c.options.MaxMessageBytes)
		if err != nil {
			return
		}
		_ = session.ws.SetReadDeadline(time.Now().Add(timeout))
		if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
			continue
		}
		c.mu.RLock()
		handler := c.customHandler
		rawHandler := c.rawHandler
		borrowedRawHandler := c.borrowedRawHandler
		current := c.session == session
		c.mu.RUnlock()
		if !current {
			return
		}
		if borrowedRawHandler != nil {
			borrowedRawHandler(buffer)
			continue
		}
		if rawHandler != nil {
			// Owned raw delivery remains independent of the next read's buffer.
			rawHandler(append([]byte(nil), buffer...))
			continue
		}
		msg, err := c.codec.Decode(buffer)
		if err != nil {
			return
		}
		if handler != nil {
			handler(msg)
		}
	}
}

func (c *WebSocketClient) heartbeat(session *webSocketClientSession, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer session.close()
	for {
		select {
		case <-session.done:
			return
		case <-ticker.C:
			// WriteControl is documented safe concurrently with the single data writer.
			if err := session.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

func (c *WebSocketClient) SendMessage(_ *protocol.Connection, msg *protocol.Message) error {
	return c.SendMessageContext(context.Background(), msg)
}
func (c *WebSocketClient) SendMessageContext(ctx context.Context, msg *protocol.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("not connected")
	}
	data, err := c.codec.Encode(msg)
	if err != nil {
		return err
	}
	if int64(len(data)) > c.options.MaxMessageBytes {
		return fmt.Errorf("message exceeds websocket limit")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-session.done:
		return fmt.Errorf("connection closed")
	case session.writeGate <- struct{}{}:
	}
	defer func() { <-session.writeGate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-session.done:
		return fmt.Errorf("connection closed")
	default:
	}
	deadline := time.Now().Add(5 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	stopCancel := context.AfterFunc(ctx, session.close)
	defer stopCancel()
	_ = session.ws.SetWriteDeadline(deadline)
	if err := session.ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
		session.close()
		return err
	}
	return nil
}

func (c *WebSocketClient) SendText(text string) error {
	if len(text) > 250*1024 {
		return c.SendLargeString(text, nil)
	}
	return c.SendMessage(nil, &protocol.Message{Type: protocol.MessageTypeText, Timestamp: time.Now().Unix(), Data: text})
}
func (c *WebSocketClient) SendCommand(cmd string, params map[string]interface{}) error {
	data := make(map[string]interface{}, len(params)+1)
	for key, value := range params {
		data[key] = value
	}
	data["command"] = cmd
	return c.SendJSON(data)
}
func (c *WebSocketClient) SendJSON(data interface{}) error {
	return c.SendMessage(nil, &protocol.Message{Type: protocol.MessageTypeJSON, Timestamp: time.Now().Unix(), Data: data})
}
func (c *WebSocketClient) SetMessageHandler(handler func(*protocol.Message)) {
	c.mu.Lock()
	c.customHandler = handler
	c.mu.Unlock()
}

// SetRawMessageHandler selects raw frame delivery instead of MessageCodec
// decoding. A non-nil raw handler takes precedence over SetMessageHandler;
// setting it to nil restores decoded delivery when no borrowed handler is set.
// Text and binary messages share
// this path. The caller must validate the complete envelope and its payload.
//
// Each buffer belongs to that received frame and is not reused by the client,
// so the handler may retain it. Delivery remains synchronous and ordered on the
// reader goroutine, with the same frame-size limit and read deadline. Handlers
// must return promptly: processing time consumes the next read's timeout and
// prevents that reader from processing ping/pong control frames.
func (c *WebSocketClient) SetRawMessageHandler(handler func([]byte)) {
	c.mu.Lock()
	c.rawHandler = handler
	c.mu.Unlock()
}

// SetBorrowedRawMessageHandler selects allocation-efficient raw frame delivery.
// It takes precedence over owned raw and decoded handlers. Setting it to nil
// restores whichever of those handlers is configured.
//
// The buffer is valid only until the callback returns and must not be retained
// or used by another goroutine. Copy it before retaining data. The client may
// overwrite it immediately after return. Delivery is synchronous, ordered and
// subject to the same size limits and read deadline as the other handlers. The
// caller must validate the complete envelope and payload before using them.
// The reader retains its grown buffer, bounded by MaxMessageBytes, until that
// connection's read loop ends.
func (c *WebSocketClient) SetBorrowedRawMessageHandler(handler func([]byte)) {
	c.mu.Lock()
	c.borrowedRawHandler = handler
	c.mu.Unlock()
}
func (c *WebSocketClient) SetKeepAlive(timeout, interval time.Duration) {
	if timeout <= 0 || interval <= 0 || interval >= timeout {
		return
	}
	c.mu.Lock()
	c.timeout = timeout
	c.keepAlive = interval
	c.mu.Unlock()
}
func (c *WebSocketClient) GetTransportType() TransportType { return TransportWebSocket }
func (c *WebSocketClient) IsConnected() bool {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return false
	}
	select {
	case <-session.done:
		return false
	default:
		return true
	}
}
func (c *WebSocketClient) SendLargeString(text string, options *LargeStringOptions) error {
	return sendLargeString(text, options, c.SendMessageContext)
}

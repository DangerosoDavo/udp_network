package udp_network

import (
	"context"
	"fmt"
	"github.com/DangerosoDavo/udp_network/protocol"
	"sync"
	"time"
)

type SecureClient struct {
	transport               *protocol.SecureTransport
	mu                      sync.RWMutex
	conn                    *protocol.Connection
	message                 func([]byte)
	connected, disconnected func()
}

// NewSecureClient fails closed: callers must provision explicit admission keys.
func NewSecureClient() (*SecureClient, error) { return nil, protocol.ErrPSKRequired }
func NewSecureClientWithPSK(psk []byte) (*SecureClient, error) {
	return NewSecureClientWithConfig(protocol.SecureConfig{PSK: psk})
}
func NewSecureClientWithConfig(config protocol.SecureConfig) (*SecureClient, error) {
	transport, err := protocol.NewSecureTransportWithConfig(":0", config)
	if err != nil {
		return nil, err
	}
	c := &SecureClient{transport: transport}
	transport.OnConnect = func(conn *protocol.Connection) {
		c.mu.Lock()
		c.conn = conn
		cb := c.connected
		c.mu.Unlock()
		conn.SetMessageCallback(func(data []byte) {
			c.mu.RLock()
			f := c.message
			c.mu.RUnlock()
			if f != nil {
				f(data)
			}
		})
		if cb != nil {
			cb()
		}
	}
	transport.OnDisconnect = func(conn *protocol.Connection) {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		cb := c.disconnected
		c.mu.Unlock()
		if cb != nil {
			cb()
		}
	}
	return c, nil
}
func (c *SecureClient) Connect(address string) error {
	c.transport.Start()
	conn, err := c.transport.SecureConnect(address)
	if err == nil {
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()
	}
	return err
}
func (c *SecureClient) connection() *protocol.Connection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}
func (c *SecureClient) Send(data []byte) error {
	conn := c.connection()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.transport.SendSecure(conn, data)
}
func (c *SecureClient) SendMessage(msg *protocol.Message) error {
	data, err := protocol.NewMessageCodec().Encode(msg)
	if err != nil {
		return err
	}
	return c.Send(data)
}
func (c *SecureClient) SendMessageContext(ctx context.Context, msg *protocol.Message) error {
	data, err := protocol.NewMessageCodec().Encode(msg)
	if err != nil {
		return err
	}
	return c.transport.SendSecureContext(ctx, c.connection(), data)
}
func (c *SecureClient) SendText(text string) error {
	return c.SendMessage(&protocol.Message{Type: protocol.MessageTypeText, ID: fmt.Sprintf("%d", time.Now().UnixNano()), Timestamp: time.Now().Unix(), Data: text})
}
func (c *SecureClient) OnMessage(callback func([]byte)) {
	c.mu.Lock()
	c.message = callback
	c.mu.Unlock()
}
func (c *SecureClient) OnConnect(callback func()) { c.mu.Lock(); c.connected = callback; c.mu.Unlock() }
func (c *SecureClient) OnDisconnect(callback func()) {
	c.mu.Lock()
	c.disconnected = callback
	c.mu.Unlock()
}
func (c *SecureClient) Close() {
	if c.transport != nil {
		c.transport.Stop()
	}
}
func (c *SecureClient) IsConnected() bool {
	conn := c.connection()
	return conn != nil && conn.GetState() == protocol.StateConnected
}
func (c *SecureClient) GetConnectionInfo() string {
	conn := c.connection()
	if conn == nil {
		return "Not connected"
	}
	return fmt.Sprintf("Connected to %s (v2 PSK-authenticated X25519/ChaCha20-Poly1305)", conn.Addr.String())
}

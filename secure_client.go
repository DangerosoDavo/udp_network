package udp_network

import (
	"fmt"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

type SecureClient struct {
	transport *protocol.SecureTransport
	conn      *protocol.Connection
}

func NewSecureClient() (*SecureClient, error) {
	transport, err := protocol.NewSecureTransport(":0")
	if err != nil {
		return nil, err
	}

	return &SecureClient{
		transport: transport,
	}, nil
}

func (c *SecureClient) Connect(address string) error {
	c.transport.Start()

	conn, err := c.transport.SecureConnect(address)
	if err != nil {
		return err
	}

	c.conn = conn
	return nil
}

func (c *SecureClient) Send(data []byte) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	return c.transport.SendSecure(c.conn, data)
}

func (c *SecureClient) SendMessage(msg *protocol.Message) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	handler := protocol.NewMessageHandler(c.transport.Transport)
	return handler.SendMessage(c.conn, msg)
}

func (c *SecureClient) SendText(text string) error {
	msg := &protocol.Message{
		Type:      protocol.MessageTypeText,
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().Unix(),
		Data:      text,
	}
	return c.SendMessage(msg)
}

func (c *SecureClient) OnMessage(callback func([]byte)) {
	if c.conn != nil {
		c.conn.SetMessageCallback(callback)
	}
}

func (c *SecureClient) OnConnect(callback func()) {
	c.transport.OnConnect = func(conn *protocol.Connection) {
		if callback != nil {
			callback()
		}
	}
}

func (c *SecureClient) OnDisconnect(callback func()) {
	c.transport.OnDisconnect = func(conn *protocol.Connection) {
		if callback != nil {
			callback()
		}
	}
}

func (c *SecureClient) Close() {
	if c.transport != nil {
		c.transport.Stop()
	}
}

func (c *SecureClient) IsConnected() bool {
	return c.conn != nil && c.conn.GetState() == protocol.StateConnected
}

func (c *SecureClient) GetConnectionInfo() string {
	if c.conn == nil {
		return "Not connected"
	}

	return fmt.Sprintf("Connected to %s (Encrypted: ChaCha20-Poly1305)", c.conn.Addr.String())
}

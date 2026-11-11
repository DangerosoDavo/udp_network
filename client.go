package udp_network

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

type Client struct {
	transport     *protocol.Transport
	handler       *protocol.MessageHandler
	Connection    *protocol.Connection // Exported for access
	serverAddr    string
	customHandler func(*protocol.Message)
	mu            sync.Mutex // Protects connection state
	connectMu     sync.Mutex
	stopped       bool
	connected     bool // Tracks connection state
}

func NewClient() (*Client, error) {
	transport, err := protocol.NewTransport(":0") // Random port
	if err != nil {
		return nil, err
	}

	c := &Client{
		transport: transport,
		handler:   protocol.NewMessageHandler(transport),
	}
	transport.OnConnect = func(conn *protocol.Connection) {
		c.handler.SetMessageCallback(conn, func(msg *protocol.Message) {
			c.mu.Lock()
			handler := c.customHandler
			c.mu.Unlock()
			if handler != nil {
				handler(msg)
			} else {
				c.handleMessage(msg)
			}
		})
	}
	transport.OnDisconnect = func(conn *protocol.Connection) {
		c.mu.Lock()
		if c.Connection == conn {
			c.connected = false
		}
		c.mu.Unlock()
	}

	return c, nil
}

func (c *Client) Connect(serverAddr string) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	c.mu.Lock()

	// Prevent double-connect
	if c.connected {
		c.mu.Unlock()
		return fmt.Errorf("already connected to %s", c.serverAddr)
	}
	if c.stopped {
		c.mu.Unlock()
		return fmt.Errorf("client stopped; construct a new client")
	}

	c.serverAddr = serverAddr
	c.mu.Unlock()
	c.transport.Start()

	conn, err := c.transport.Connect(serverAddr)
	if err != nil {
		return err
	}

	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return fmt.Errorf("client stopped")
	}
	c.Connection = conn
	c.connected = true
	c.mu.Unlock()

	log.Printf("Connected to server: %s", serverAddr)

	// Start heartbeat to keep connection alive
	// The transport owns keepalives; no additional application heartbeat workers.

	return nil
}

func (c *Client) Start() {
	c.transport.Start()
}

func (c *Client) Stop() {
	c.mu.Lock()

	c.connected = false
	c.stopped = true
	c.mu.Unlock()
	c.transport.Stop()
}

func (c *Client) handleMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypeText:
		fmt.Printf("\n[Server Text]: %s\n> ", msg.Data)

	case protocol.MessageTypeJSON:
		if data, ok := msg.Data.(map[string]interface{}); ok {
			c.handleJSONMessage(data)
		}
	}
}

func (c *Client) handleJSONMessage(data map[string]interface{}) {
	// Pretty print JSON responses
	formatted, _ := json.MarshalIndent(data, "", "  ")
	fmt.Printf("\n[Server JSON]:\n%s\n> ", string(formatted))
}

func (c *Client) SendText(text string) error {
	// Check if text is large enough to require chunking
	// Use 250KB as threshold (leaving room for protocol overhead)
	const largeStringThreshold = 250 * 1024

	if len(text) > largeStringThreshold {
		// Use large string method for big strings
		fmt.Printf("📦 Text size (%d bytes) exceeds threshold, using chunked transfer\n", len(text))
		return c.SendLargeString(text, &LargeStringOptions{
			Metadata: map[string]interface{}{
				"type":          "auto-chunked-text",
				"original_size": len(text),
			},
		})
	}

	// Normal path for small strings
	msg := &protocol.Message{
		Type:      protocol.MessageTypeText,
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().Unix(),
		Data:      text,
	}

	return c.SendMessage(c.CurrentConnection(), msg)
}

func (c *Client) SendCommand(cmd string, params map[string]interface{}) error {
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

	return c.SendMessage(c.CurrentConnection(), msg)
}

func (c *Client) SendJSON(data interface{}) error {
	msg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	return c.SendMessage(c.CurrentConnection(), msg)
}

func (c *Client) RunInteractive() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("\nUDP Network Client")
	fmt.Println("Commands:")
	fmt.Println("  /stats         - Get server statistics")
	fmt.Println("  /clients       - List connected clients")
	fmt.Println("  /broadcast <msg> - Broadcast message to all clients")
	fmt.Println("  /ping          - Ping the server")
	fmt.Println("  /stress <n>    - Send n messages rapidly")
	fmt.Println("  /quit          - Exit client")
	fmt.Println("\nType messages to send to server:")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if input == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			cmd := parts[0]

			switch cmd {
			case "/quit":
				return

			case "/stats":
				c.SendCommand("stats", nil)

			case "/clients":
				c.SendCommand("list_clients", nil)

			case "/broadcast":
				if len(parts) > 1 {
					message := strings.Join(parts[1:], " ")
					c.SendCommand("broadcast", map[string]interface{}{
						"message": message,
					})
				}

			case "/ping":
				start := time.Now()
				msg := &protocol.Message{
					Type:      protocol.MessageTypeCommand,
					Timestamp: time.Now().Unix(),
					Data:      "ping",
				}
				c.SendMessage(c.CurrentConnection(), msg)
				// Note: RTT calculation would be done in response handler
				fmt.Printf("Ping sent at %v\n", start)

			case "/stress":
				if len(parts) > 1 {
					var count int
					fmt.Sscanf(parts[1], "%d", &count)
					c.stressTest(count)
				}

			default:
				fmt.Printf("Unknown command: %s\n", cmd)
			}
		} else {
			// Send as text message
			if err := c.SendText(input); err != nil {
				fmt.Printf("Error sending message: %v\n", err)
			} else {
				fmt.Printf("[Sent]: %s\n", input)
			}
		}
	}
}

func (c *Client) stressTest(count int) {
	fmt.Printf("Sending %d messages...\n", count)
	start := time.Now()

	for i := 0; i < count; i++ {
		msg := fmt.Sprintf("Stress test message %d/%d", i+1, count)
		if err := c.SendText(msg); err != nil {
			fmt.Printf("Error on message %d: %v\n", i+1, err)
			break
		}

		// Small delay to avoid overwhelming
		if i%100 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	elapsed := time.Since(start)
	rate := float64(count) / elapsed.Seconds()
	fmt.Printf("Sent %d messages in %v (%.2f msg/sec)\n", count, elapsed, rate)
}

// SetMessageHandler allows overriding the message handler
func (c *Client) SetMessageHandler(handler func(*protocol.Message)) {
	c.mu.Lock()
	c.customHandler = handler
	c.mu.Unlock()
}

// CurrentConnection safely reads the connection pointer. The legacy exported
// Connection field should not be read concurrently with Connect or Stop.
func (c *Client) CurrentConnection() *protocol.Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Connection
}

// SendMessage sends a message to the server
func (c *Client) SendMessage(conn *protocol.Connection, msg *protocol.Message) error {
	return c.handler.SendMessage(conn, msg)
}

// SetKeepAlive configures the connection timeout and keep-alive settings
func (c *Client) SetKeepAlive(timeout, interval time.Duration) {
	if c.transport != nil {
		c.transport.SetKeepAlive(timeout, interval)
	}
}

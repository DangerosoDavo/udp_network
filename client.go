package udp_network

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

type Client struct {
	transport     *protocol.Transport
	handler       *protocol.MessageHandler
	Connection    *protocol.Connection // Exported for access
	serverAddr    string
	customHandler func(*protocol.Message)
	heartbeatStop chan bool
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

	return c, nil
}

func (c *Client) Connect(serverAddr string) error {
	c.serverAddr = serverAddr

	conn, err := c.transport.Connect(serverAddr)
	if err != nil {
		return err
	}

	c.Connection = conn

	// Set up message handler
	c.handler.SetMessageCallback(conn, func(msg *protocol.Message) {
		if c.customHandler != nil {
			c.customHandler(msg)
		} else {
			c.handleMessage(msg)
		}
	})

	log.Printf("Connected to server: %s", serverAddr)

	// Start heartbeat to keep connection alive
	c.startHeartbeat()

	return nil
}

func (c *Client) Start() {
	c.transport.Start()
}

func (c *Client) Stop() {
	// Stop heartbeat
	if c.heartbeatStop != nil {
		close(c.heartbeatStop)
	}
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
	msg := &protocol.Message{
		Type:      protocol.MessageTypeText,
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().Unix(),
		Data:      text,
	}

	return c.handler.SendMessage(c.Connection, msg)
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

	return c.handler.SendMessage(c.Connection, msg)
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
				c.handler.SendMessage(c.Connection, msg)
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
	c.customHandler = handler
	// If already connected, update the callback
	if c.Connection != nil {
		c.handler.SetMessageCallback(c.Connection, handler)
	}
}

// SendMessage sends a message to the server
func (c *Client) SendMessage(conn *protocol.Connection, msg *protocol.Message) error {
	return c.handler.SendMessage(conn, msg)
}

// startHeartbeat sends periodic heartbeats to keep connection alive
func (c *Client) startHeartbeat() {
	c.heartbeatStop = make(chan bool)

	go func() {
		ticker := time.NewTicker(5 * time.Second) // Send heartbeat every 5 seconds
		defer ticker.Stop()

		for {
			select {
			case <-c.heartbeatStop:
				return
			case <-ticker.C:
				// Send a lightweight heartbeat message
				if c.Connection != nil && c.Connection.GetState() == protocol.StateConnected {
					heartbeat := &protocol.Message{
						Type:      protocol.MessageTypeCommand,
						Timestamp: time.Now().Unix(),
						Data:      "heartbeat",
					}
					c.SendMessage(c.Connection, heartbeat)
				}
			}
		}
	}()
}

// SetKeepAlive configures the connection timeout and keep-alive settings
func (c *Client) SetKeepAlive(timeout, interval time.Duration) {
	if c.transport != nil {
		c.transport.ConnTimeout = timeout
		c.transport.KeepAliveInterval = interval
	}
}

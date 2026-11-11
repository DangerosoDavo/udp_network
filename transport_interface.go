package udp_network

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

// TransportType indicates the underlying transport protocol
type TransportType string

const (
	TransportUDP       TransportType = "udp"
	TransportWebSocket TransportType = "websocket"
)

// UnifiedServer provides a common interface for both UDP and WebSocket servers
type UnifiedServer interface {
	// Start starts the server
	Start()

	// Stop stops the server
	Stop()

	// SetMessageHandler sets the message handler for incoming messages
	SetMessageHandler(handler func(*protocol.Connection, *protocol.Message))

	// SendMessage sends a message to a specific client
	SendMessage(conn *protocol.Connection, msg *protocol.Message) error

	// Broadcast sends a message to all connected clients
	Broadcast(msg *protocol.Message)

	// Clients returns all connected clients
	Clients() map[string]*protocol.Connection

	// SetMaxClients sets the maximum number of concurrent connections
	SetMaxClients(max int)

	// GetClientCount returns the current number of connected clients
	GetClientCount() int

	// SetKeepAlive configures connection timeout and keep-alive settings
	SetKeepAlive(timeout, interval time.Duration)

	// SetConnectionCallbacks sets callbacks for connection events
	SetConnectionCallbacks(onConnect, onDisconnect func(*protocol.Connection))

	// GetTransportType returns the underlying transport type
	GetTransportType() TransportType
}

// UnifiedClient provides a common interface for both UDP and WebSocket clients
type UnifiedClient interface {
	// Connect connects to a server
	Connect(serverAddr string) error

	// Start starts the client transport
	Start()

	// Stop stops the client and closes connections
	Stop()

	// SendText sends a text message
	SendText(text string) error

	// SendCommand sends a command with parameters
	SendCommand(cmd string, params map[string]interface{}) error

	// SendJSON sends arbitrary JSON data
	SendJSON(data interface{}) error

	// SendMessage sends a custom protocol message
	SendMessage(conn *protocol.Connection, msg *protocol.Message) error

	// SetMessageHandler sets the message handler
	SetMessageHandler(handler func(*protocol.Message))

	// SetKeepAlive configures connection settings
	SetKeepAlive(timeout, interval time.Duration)

	// GetTransportType returns the underlying transport type
	GetTransportType() TransportType

	// IsConnected returns true if connected
	IsConnected() bool
}

// ServerOptions allows configuring server creation
type ServerOptions struct {
	Transport     TransportType
	MaxClients    int
	Timeout       time.Duration
	KeepAlive     time.Duration
	EnableLogging bool
	// WebSocket limits; zero selects bounded defaults.
	MaxPendingMessages int
	MaxMessageBytes    int64
	CheckOrigin        func(*http.Request) bool
	TLSConfig          *tls.Config
}

// ClientOptions allows configuring client creation
type ClientOptions struct {
	Transport       TransportType
	Timeout         time.Duration
	KeepAlive       time.Duration
	EnableLogging   bool
	MaxMessageBytes int64
	TLSConfig       *tls.Config
}

// NewUnifiedServer creates a server with the specified transport type
func NewUnifiedServer(address string, options *ServerOptions) (UnifiedServer, error) {
	if options == nil {
		options = &ServerOptions{
			Transport: TransportUDP,
		}
	}

	switch options.Transport {
	case TransportWebSocket:
		return NewWebSocketServer(address, options)
	case TransportUDP, "":
		server, err := NewServer(address)
		if err != nil {
			return nil, err
		}

		// Apply options
		if options.MaxClients > 0 {
			server.SetMaxClients(options.MaxClients)
		}
		if options.Timeout > 0 || options.KeepAlive > 0 {
			timeout := options.Timeout
			if timeout == 0 {
				timeout = 30 * time.Second
			}
			keepAlive := options.KeepAlive
			if keepAlive == 0 {
				keepAlive = 10 * time.Second
			}
			server.SetKeepAlive(timeout, keepAlive)
		}

		return &udpServerAdapter{Server: server}, nil
	default:
		return nil, fmt.Errorf("unsupported transport %q", options.Transport)
	}
}

// NewUnifiedClient creates a client with the specified transport type
func NewUnifiedClient(options *ClientOptions) (UnifiedClient, error) {
	if options == nil {
		options = &ClientOptions{
			Transport: TransportUDP,
		}
	}

	switch options.Transport {
	case TransportWebSocket:
		return NewWebSocketClient(options)
	case TransportUDP, "":
		client, err := NewClient()
		if err != nil {
			return nil, err
		}

		// Apply options
		if options.Timeout > 0 || options.KeepAlive > 0 {
			timeout := options.Timeout
			if timeout == 0 {
				timeout = 30 * time.Second
			}
			keepAlive := options.KeepAlive
			if keepAlive == 0 {
				keepAlive = 10 * time.Second
			}
			client.SetKeepAlive(timeout, keepAlive)
		}

		return &udpClientAdapter{Client: client}, nil
	default:
		return nil, fmt.Errorf("unsupported transport %q", options.Transport)
	}
}

// udpServerAdapter wraps Server to implement UnifiedServer
type udpServerAdapter struct {
	*Server
}

func (a *udpServerAdapter) GetTransportType() TransportType {
	return TransportUDP
}

// udpClientAdapter wraps Client to implement UnifiedClient
type udpClientAdapter struct {
	*Client
}

func (a *udpClientAdapter) GetTransportType() TransportType {
	return TransportUDP
}

func (a *udpClientAdapter) IsConnected() bool {
	return a.Connection != nil && a.Connection.GetState() == protocol.StateConnected
}

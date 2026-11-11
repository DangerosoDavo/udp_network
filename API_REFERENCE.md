> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](README.md) and [migration notes](MAINTENANCE.md) for supported contracts.

# UDP Network Package - API Reference

Complete API documentation for the UDP Network package - a reliable UDP protocol implementation with encryption support.

## Table of Contents

1. [Installation](#installation)
2. [Quick Start](#quick-start)
3. [Core Packages](#core-packages)
4. [Server API](#server-api)
5. [Client API](#client-api)
6. [Secure Server API](#secure-server-api)
7. [Secure Client API](#secure-client-api)
8. [Protocol Package](#protocol-package)
9. [Message Types](#message-types)
10. [Large Transfer API](#large-transfer-api)
11. [Advanced Configuration](#advanced-configuration)
12. [Error Handling](#error-handling)
13. [Examples](#examples)

---

## Installation

```bash
go get github.com/DangerosoDavo/udp_network
```

### Import

```go
import (
    udpnet "github.com/DangerosoDavo/udp_network"
    "github.com/DangerosoDavo/udp_network/protocol"
)
```

---

## Quick Start

### Basic Server/Client

```go
// Server
server, _ := udpnet.NewServer(":8080")
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    fmt.Printf("Received: %v\n", msg.Data)
})
server.Start()
defer server.Stop()

// Client
client, _ := udpnet.NewClient()
client.Connect("localhost:8080")
client.SendText("Hello, World!")
```

### Secure Communication

```go
// Secure Server
server, _ := udpnet.NewSecureServer(":8080")
server.OnMessage(func(conn *protocol.Connection, data []byte) {
    fmt.Printf("Encrypted message: %s\n", string(data))
})
server.Start()

// Secure Client
client, _ := udpnet.NewSecureClient()
client.Connect("localhost:8080")
client.Send([]byte("Encrypted message"))
```

---

## Core Packages

### Main Package: `udp_network`

High-level server and client implementations.

### Protocol Package: `protocol`

Low-level protocol implementation including packets, connections, and transport.

---

## Server API

### Type: `Server`

Unencrypted UDP server with reliability features.

#### Constructor

```go
func NewServer(address string) (*Server, error)
```

Creates a new UDP server listening on the specified address.

**Parameters:**
- `address`: Bind address (e.g., `:8080`, `0.0.0.0:8080`)

**Returns:**
- `*Server`: Server instance
- `error`: Error if binding fails

**Example:**
```go
server, err := udpnet.NewServer(":8080")
if err != nil {
    log.Fatal(err)
}
```

#### Methods

##### `Start()`

```go
func (s *Server) Start()
```

Starts the server and begins accepting connections.

**Example:**
```go
server.Start()
```

---

##### `Stop()`

```go
func (s *Server) Stop()
```

Gracefully shuts down the server.

**Example:**
```go
defer server.Stop()
```

---

##### `SetMessageHandler()`

```go
func (s *Server) SetMessageHandler(handler func(*protocol.Connection, *protocol.Message))
```

Sets a custom message handler for incoming messages.

**Parameters:**
- `handler`: Function called when a message is received

**Example:**
```go
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    switch msg.Type {
    case protocol.MessageTypeText:
        fmt.Printf("Text: %s\n", msg.Data)
    case protocol.MessageTypeJSON:
        fmt.Printf("JSON: %v\n", msg.Data)
    }
})
```

---

##### `SendMessage()`

```go
func (s *Server) SendMessage(conn *protocol.Connection, msg *protocol.Message) error
```

Sends a message to a specific client.

**Parameters:**
- `conn`: Client connection
- `msg`: Message to send

**Returns:**
- `error`: Error if send fails

**Example:**
```go
response := &protocol.Message{
    Type: protocol.MessageTypeText,
    Data: "Hello, Client!",
    Timestamp: time.Now().Unix(),
}
server.SendMessage(conn, response)
```

---

##### `Broadcast()`

```go
func (s *Server) Broadcast(msg *protocol.Message)
```

Sends a message to all connected clients.

**Parameters:**
- `msg`: Message to broadcast

**Example:**
```go
announcement := &protocol.Message{
    Type: protocol.MessageTypeJSON,
    Data: map[string]interface{}{
        "event": "server_announcement",
        "message": "System maintenance in 5 minutes",
    },
    Timestamp: time.Now().Unix(),
}
server.Broadcast(announcement)
```

---

##### `Clients()`

```go
func (s *Server) Clients() map[string]*protocol.Connection
```

Returns a copy of all connected clients.

**Returns:**
- `map[string]*protocol.Connection`: Map of client address to connection

**Example:**
```go
clients := server.Clients()
fmt.Printf("Connected clients: %d\n", len(clients))
for addr, conn := range clients {
    fmt.Printf("  - %s (state: %v)\n", addr, conn.GetState())
}
```

---

##### `SetMaxClients()`

```go
func (s *Server) SetMaxClients(max int)
```

Sets the maximum number of concurrent connections.

**Parameters:**
- `max`: Maximum connections (0 = unlimited)

**Example:**
```go
server.SetMaxClients(1000)
```

---

##### `GetClientCount()`

```go
func (s *Server) GetClientCount() int
```

Returns the current number of connected clients.

**Returns:**
- `int`: Number of active connections

**Example:**
```go
count := server.GetClientCount()
fmt.Printf("Active connections: %d\n", count)
```

---

##### `SetKeepAlive()`

```go
func (s *Server) SetKeepAlive(timeout, interval time.Duration)
```

Configures connection timeout and keep-alive settings.

**Parameters:**
- `timeout`: Connection timeout duration
- `interval`: Keep-alive packet interval

**Example:**
```go
// 5 minute timeout, 30 second keep-alive
server.SetKeepAlive(5*time.Minute, 30*time.Second)
```

---

##### `SetConnectionCallbacks()`

```go
func (s *Server) SetConnectionCallbacks(
    onConnect func(*protocol.Connection),
    onDisconnect func(*protocol.Connection),
)
```

Sets callbacks for connection events.

**Parameters:**
- `onConnect`: Called when a client connects
- `onDisconnect`: Called when a client disconnects

**Example:**
```go
server.SetConnectionCallbacks(
    func(conn *protocol.Connection) {
        log.Printf("Client connected: %s", conn.Addr.String())
    },
    func(conn *protocol.Connection) {
        log.Printf("Client disconnected: %s", conn.Addr.String())
    },
)
```

---

##### `Transport()`

```go
func (s *Server) Transport() *protocol.Transport
```

Returns the underlying transport for advanced configuration.

**Returns:**
- `*protocol.Transport`: Transport instance

**Example:**
```go
transport := server.Transport()
transport.SetMaxConnections(5000)
```

---

## Client API

### Type: `Client`

Unencrypted UDP client with automatic reconnection and heartbeat.

#### Constructor

```go
func NewClient() (*Client, error)
```

Creates a new UDP client.

**Returns:**
- `*Client`: Client instance
- `error`: Error if initialization fails

**Example:**
```go
client, err := udpnet.NewClient()
if err != nil {
    log.Fatal(err)
}
```

#### Methods

##### `Connect()`

```go
func (c *Client) Connect(serverAddr string) error
```

Connects to a server and starts automatic heartbeat.

**Parameters:**
- `serverAddr`: Server address (e.g., `localhost:8080`)

**Returns:**
- `error`: Error if connection fails or already connected

**Example:**
```go
err := client.Connect("localhost:8080")
if err != nil {
    log.Fatal(err)
}
```

---

##### `Start()`

```go
func (c *Client) Start()
```

Starts the client transport layer.

**Example:**
```go
client.Start()
```

---

##### `Stop()`

```go
func (c *Client) Stop()
```

Stops the client and closes connections.

**Example:**
```go
defer client.Stop()
```

---

##### `SendText()`

```go
func (c *Client) SendText(text string) error
```

Sends a text message to the server. Automatically uses chunked transfer for messages >250KB.

**Parameters:**
- `text`: Text message to send

**Returns:**
- `error`: Error if send fails

**Example:**
```go
err := client.SendText("Hello, Server!")
if err != nil {
    log.Printf("Send failed: %v", err)
}
```

---

##### `SendCommand()`

```go
func (c *Client) SendCommand(cmd string, params map[string]interface{}) error
```

Sends a command with parameters as JSON.

**Parameters:**
- `cmd`: Command name
- `params`: Command parameters (can be nil)

**Returns:**
- `error`: Error if send fails

**Example:**
```go
client.SendCommand("get_stats", nil)

client.SendCommand("broadcast", map[string]interface{}{
    "message": "Hello everyone!",
})
```

---

##### `SendJSON()`

```go
func (c *Client) SendJSON(data interface{}) error
```

Sends arbitrary JSON data.

**Parameters:**
- `data`: Data to encode as JSON

**Returns:**
- `error`: Error if send fails

**Example:**
```go
client.SendJSON(map[string]interface{}{
    "action": "update_profile",
    "username": "alice",
    "status": "online",
})
```

---

##### `SendMessage()`

```go
func (c *Client) SendMessage(conn *protocol.Connection, msg *protocol.Message) error
```

Sends a custom protocol message.

**Parameters:**
- `conn`: Connection to use
- `msg`: Message to send

**Returns:**
- `error`: Error if send fails

**Example:**
```go
msg := &protocol.Message{
    Type: protocol.MessageTypeCommand,
    ID: "cmd-123",
    Timestamp: time.Now().Unix(),
    Data: "ping",
}
client.SendMessage(client.Connection, msg)
```

---

##### `SetMessageHandler()`

```go
func (c *Client) SetMessageHandler(handler func(*protocol.Message))
```

Sets a custom message handler for incoming messages.

**Parameters:**
- `handler`: Function called when a message is received

**Example:**
```go
client.SetMessageHandler(func(msg *protocol.Message) {
    fmt.Printf("Received: %v\n", msg.Data)
})
```

---

##### `SetKeepAlive()`

```go
func (c *Client) SetKeepAlive(timeout, interval time.Duration)
```

Configures connection timeout and keep-alive settings.

**Parameters:**
- `timeout`: Connection timeout duration
- `interval`: Keep-alive packet interval

**Example:**
```go
client.SetKeepAlive(2*time.Minute, 10*time.Second)
```

---

##### `RunInteractive()`

```go
func (c *Client) RunInteractive()
```

Runs an interactive CLI for testing.

**Commands:**
- `/quit` - Exit
- `/stats` - Get server stats
- `/clients` - List clients
- `/broadcast <msg>` - Broadcast message
- `/ping` - Ping server
- `/stress <n>` - Send n messages

**Example:**
```go
client.RunInteractive()
```

---

## Secure Server API

### Type: `SecureServer`

Encrypted UDP server using ChaCha20-Poly1305.

#### Constructor

```go
func NewSecureServer(address string) (*SecureServer, error)
```

Creates a new secure UDP server with end-to-end encryption.

**Parameters:**
- `address`: Bind address

**Returns:**
- `*SecureServer`: Secure server instance
- `error`: Error if initialization fails

**Example:**
```go
server, err := udpnet.NewSecureServer(":8080")
if err != nil {
    log.Fatal(err)
}
```

#### Methods

##### `Start()`

```go
func (s *SecureServer) Start()
```

Starts the secure server.

**Example:**
```go
server.Start()
```

---

##### `Stop()`

```go
func (s *SecureServer) Stop()
```

Stops the secure server.

**Example:**
```go
defer server.Stop()
```

---

##### `OnMessage()`

```go
func (s *SecureServer) OnMessage(handler func(*protocol.Connection, []byte))
```

Sets message handler for decrypted messages.

**Parameters:**
- `handler`: Function receiving decrypted data

**Example:**
```go
server.OnMessage(func(conn *protocol.Connection, data []byte) {
    fmt.Printf("From %s: %s\n", conn.Addr.String(), string(data))
})
```

---

##### `SendTo()`

```go
func (s *SecureServer) SendTo(conn *protocol.Connection, data []byte) error
```

Sends encrypted data to a specific client.

**Parameters:**
- `conn`: Client connection
- `data`: Data to encrypt and send

**Returns:**
- `error`: Error if send fails

**Example:**
```go
server.SendTo(conn, []byte("Encrypted response"))
```

---

##### `Broadcast()`

```go
func (s *SecureServer) Broadcast(data []byte) error
```

Broadcasts encrypted data to all clients.

**Parameters:**
- `data`: Data to encrypt and broadcast

**Returns:**
- `error`: Error if broadcast fails for any client

**Example:**
```go
server.Broadcast([]byte("Server announcement"))
```

---

##### `GetClientCount()`

```go
func (s *SecureServer) GetClientCount() int
```

Returns the number of connected clients.

**Returns:**
- `int`: Number of connections

---

##### `SetMaxConnections()`

```go
func (s *SecureServer) SetMaxConnections(max int)
```

Sets the maximum number of connections.

**Parameters:**
- `max`: Maximum connections (0 = unlimited)

**Example:**
```go
server.SetMaxConnections(500)
```

---

##### `GetTransport()`

```go
func (s *SecureServer) GetTransport() *protocol.SecureTransport
```

Returns the underlying secure transport.

**Returns:**
- `*protocol.SecureTransport`: Secure transport instance

---

## Secure Client API

### Type: `SecureClient`

Encrypted UDP client using ChaCha20-Poly1305.

#### Constructor

```go
func NewSecureClient() (*SecureClient, error)
```

Creates a new secure UDP client.

**Returns:**
- `*SecureClient`: Secure client instance
- `error`: Error if initialization fails

**Example:**
```go
client, err := udpnet.NewSecureClient()
if err != nil {
    log.Fatal(err)
}
```

#### Methods

##### `Connect()`

```go
func (c *SecureClient) Connect(address string) error
```

Connects to a secure server with encryption handshake.

**Parameters:**
- `address`: Server address

**Returns:**
- `error`: Error if connection or crypto handshake fails

**Example:**
```go
err := client.Connect("localhost:8080")
if err != nil {
    log.Fatal(err)
}
```

---

##### `Send()`

```go
func (c *SecureClient) Send(data []byte) error
```

Sends encrypted data to the server.

**Parameters:**
- `data`: Data to encrypt and send

**Returns:**
- `error`: Error if send fails

**Example:**
```go
client.Send([]byte("Encrypted message"))
```

---

##### `SendText()`

```go
func (c *SecureClient) SendText(text string) error
```

Sends encrypted text message. Auto-chunks if >250KB.

**Parameters:**
- `text`: Text to send

**Returns:**
- `error`: Error if send fails

**Example:**
```go
client.SendText("Hello, secure world!")
```

---

##### `SendMessage()`

```go
func (c *SecureClient) SendMessage(msg *protocol.Message) error
```

Sends a protocol message with encryption.

**Parameters:**
- `msg`: Message to send

**Returns:**
- `error`: Error if send fails

---

##### `OnMessage()`

```go
func (c *SecureClient) OnMessage(callback func([]byte))
```

Sets callback for receiving decrypted messages.

**Parameters:**
- `callback`: Function to handle decrypted data

**Example:**
```go
client.OnMessage(func(data []byte) {
    fmt.Printf("Received: %s\n", string(data))
})
```

---

##### `OnConnect()`

```go
func (c *SecureClient) OnConnect(callback func())
```

Sets callback for connection establishment.

**Parameters:**
- `callback`: Function called when connected

**Example:**
```go
client.OnConnect(func() {
    log.Println("Connected securely!")
})
```

---

##### `OnDisconnect()`

```go
func (c *SecureClient) OnDisconnect(callback func())
```

Sets callback for disconnection.

**Parameters:**
- `callback`: Function called when disconnected

---

##### `Close()`

```go
func (c *SecureClient) Close()
```

Closes the connection.

**Example:**
```go
defer client.Close()
```

---

##### `IsConnected()`

```go
func (c *SecureClient) IsConnected() bool
```

Checks if client is connected.

**Returns:**
- `bool`: True if connected

**Example:**
```go
if client.IsConnected() {
    client.Send([]byte("data"))
}
```

---

##### `GetConnectionInfo()`

```go
func (c *SecureClient) GetConnectionInfo() string
```

Returns connection information string.

**Returns:**
- `string`: Connection details

**Example:**
```go
info := client.GetConnectionInfo()
fmt.Println(info)
// Output: Connected to 127.0.0.1:8080 (Encrypted: ChaCha20-Poly1305)
```

---

## Protocol Package

### Type: `Connection`

Represents a UDP connection with reliability features.

#### Methods

##### `GetState()`

```go
func (c *Connection) GetState() ConnectionState
```

Returns the current connection state.

**Returns:**
- `ConnectionState`: Current state

**States:**
- `StateNew` - Just created
- `StateHandshaking` - Performing handshake
- `StateConnected` - Fully connected
- `StateClosed` - Connection closed

**Example:**
```go
if conn.GetState() == protocol.StateConnected {
    // Send data
}
```

---

##### `SetMessageCallback()`

```go
func (c *Connection) SetMessageCallback(callback func([]byte))
```

Sets callback for received messages.

**Parameters:**
- `callback`: Function to handle received data

---

### Type: `Transport`

Low-level transport layer handling packet transmission.

#### Methods

##### `SetMaxConnections()`

```go
func (t *Transport) SetMaxConnections(max int)
```

Sets maximum concurrent connections.

**Parameters:**
- `max`: Maximum connections (0 = unlimited)

---

##### `GetConnectionCount()`

```go
func (t *Transport) GetConnectionCount() int
```

Returns current connection count.

**Returns:**
- `int`: Number of connections

---

##### `CloseConnection()`

```go
func (t *Transport) CloseConnection(addr string)
```

Forcefully closes a specific connection.

**Parameters:**
- `addr`: Address of connection to close

---

## Message Types

### Type: `Message`

```go
type Message struct {
    Type      MessageType            `json:"type"`
    ID        string                 `json:"id,omitempty"`
    Timestamp int64                  `json:"ts"`
    Data      interface{}            `json:"data"`
    Metadata  map[string]interface{} `json:"meta,omitempty"`
}
```

### Message Type Constants

```go
const (
    MessageTypeText    MessageType = "text"
    MessageTypeJSON    MessageType = "json"
    MessageTypeCommand MessageType = "command"
    MessageTypeBinary  MessageType = "binary"
    MessageTypeChunked MessageType = "chunked"
)
```

### Creating Messages

```go
// Text message
msg := &protocol.Message{
    Type: protocol.MessageTypeText,
    Data: "Hello, World!",
    Timestamp: time.Now().Unix(),
}

// JSON message
msg := &protocol.Message{
    Type: protocol.MessageTypeJSON,
    Data: map[string]interface{}{
        "action": "login",
        "username": "alice",
    },
    Timestamp: time.Now().Unix(),
}

// Command message
msg := &protocol.Message{
    Type: protocol.MessageTypeCommand,
    Data: "ping",
    Timestamp: time.Now().Unix(),
}
```

---

## Large Transfer API

For sending messages >250KB.

### Type: `LargeStringOptions`

```go
type LargeStringOptions struct {
    TransferID    string
    Metadata      map[string]interface{}
    OnProgress    func(sent, total int)
    RetryAttempts int
    ChunkTimeout  time.Duration
}
```

### Client Methods

##### `SendLargeString()`

```go
func (c *Client) SendLargeString(largeString string, options *LargeStringOptions) error
```

Sends a large string using chunked transfer.

**Parameters:**
- `largeString`: String to send
- `options`: Transfer options (can be nil for defaults)

**Returns:**
- `error`: Error if transfer fails

**Example:**
```go
largeData := string(make([]byte, 5*1024*1024)) // 5MB

options := &udpnet.LargeStringOptions{
    OnProgress: func(sent, total int) {
        fmt.Printf("Progress: %d/%d chunks\n", sent, total)
    },
    RetryAttempts: 3,
}

err := client.SendLargeString(largeData, options)
if err != nil {
    log.Fatal(err)
}
```

### Server Methods

##### `EnableLargeStringReceiver()`

```go
func EnableLargeStringReceiver(s *Server) *LargeStringReceiver
```

Enables large string receiving on a server.

**Parameters:**
- `s`: Server instance

**Returns:**
- `*LargeStringReceiver`: Receiver for handling large transfers

**Example:**
```go
receiver := udpnet.EnableLargeStringReceiver(server)
receiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
    fmt.Printf("Received large string: %d bytes\n", len(data))
})
```

### Type: `LargeStringReceiver`

##### `SetDefaultHandler()`

```go
func (lsr *LargeStringReceiver) SetDefaultHandler(handler func(string, map[string]interface{}))
```

Sets default handler for all received large strings.

**Parameters:**
- `handler`: Function called when transfer completes

---

##### `GetTransferStatus()`

```go
func (lsr *LargeStringReceiver) GetTransferStatus(transferID string) (received, total int, exists bool)
```

Gets status of an in-progress transfer.

**Parameters:**
- `transferID`: Transfer ID

**Returns:**
- `received`: Chunks received
- `total`: Total chunks
- `exists`: True if transfer exists

---

##### `GetStats()`

```go
func (lsr *LargeStringReceiver) GetStats() []protocol.ChunkedTransferStats
```

Returns statistics for all active transfers.

**Returns:**
- `[]protocol.ChunkedTransferStats`: Transfer statistics

---

## Advanced Configuration

### Connection Timeouts

```go
// Server
server.SetKeepAlive(5*time.Minute, 30*time.Second)

// Client
client.SetKeepAlive(2*time.Minute, 10*time.Second)
```

**Recommendations:**
- Game servers: 5-10 minute timeout, 1 minute keep-alive
- Chat apps: 30-60 second timeout, 10 second keep-alive
- IoT devices: 10 minute timeout, 2 minute keep-alive

### Connection Limits

```go
// Limit to 1000 concurrent connections
server.SetMaxClients(1000)

// Or use transport directly
transport := server.Transport()
transport.SetMaxConnections(5000)
```

### Encryption Configuration

```go
// Get secure transport for advanced config
secureTransport := secureServer.GetTransport()

// Disable encryption requirement (not recommended)
secureTransport.RequireEncryption = false
```

---

## Error Handling

### Common Errors

```go
// Connection already established
err := client.Connect("localhost:8080")
if err != nil {
    if err.Error() == "already connected" {
        // Handle double connect
    }
}

// Connection not established
err := client.SendText("test")
if err != nil {
    if strings.Contains(err.Error(), "not connected") {
        // Reconnect
    }
}

// Connection limit reached
// Server returns NACK packet when limit reached
```

### Best Practices

```go
// Always check connection state
if client.Connection != nil && client.Connection.GetState() == protocol.StateConnected {
    client.SendText("data")
}

// Use defer for cleanup
defer server.Stop()
defer client.Stop()

// Handle crypto handshake failures
client, err := udpnet.NewSecureClient()
if err != nil {
    log.Fatal("Failed to initialize crypto:", err)
}

err = client.Connect("server:8080")
if err != nil {
    log.Fatal("Crypto handshake failed:", err)
}
```

---

## Examples

### Simple Echo Server

```go
package main

import (
    "log"
    udpnet "github.com/DangerosoDavo/udp_network"
    "github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
    server, err := udpnet.NewServer(":8080")
    if err != nil {
        log.Fatal(err)
    }

    server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
        log.Printf("Echo: %v", msg.Data)
        server.SendMessage(conn, msg)
    })

    server.Start()
    defer server.Stop()

    select {} // Block forever
}
```

### Game Server

```go
type Player struct {
    ID   string
    Conn *protocol.Connection
    X, Y float64
}

func main() {
    server, _ := udpnet.NewServer(":8080")
    server.SetKeepAlive(10*time.Minute, 1*time.Minute)
    server.SetMaxClients(1000)

    players := make(map[string]*Player)
    var mu sync.RWMutex

    server.SetConnectionCallbacks(
        func(conn *protocol.Connection) {
            mu.Lock()
            players[conn.Addr.String()] = &Player{
                ID:   generateID(),
                Conn: conn,
            }
            mu.Unlock()
        },
        func(conn *protocol.Connection) {
            mu.Lock()
            delete(players, conn.Addr.String())
            mu.Unlock()
        },
    )

    server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
        if data, ok := msg.Data.(map[string]interface{}); ok {
            handleGameAction(data, players, server)
        }
    })

    server.Start()
    defer server.Stop()
    select {}
}
```

### Secure Chat Client

```go
func main() {
    client, _ := udpnet.NewSecureClient()

    client.OnMessage(func(data []byte) {
        fmt.Printf("[Server]: %s\n", string(data))
    })

    err := client.Connect("localhost:9999")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        msg := scanner.Text()
        client.Send([]byte(msg))
    }
}
```

### Large File Transfer

```go
func main() {
    client, _ := udpnet.NewClient()
    client.Connect("localhost:8080")
    defer client.Stop()

    // Read large file
    data, _ := os.ReadFile("large_file.txt")

    // Send with progress tracking
    options := &udpnet.LargeStringOptions{
        OnProgress: func(sent, total int) {
            progress := float64(sent) / float64(total) * 100
            fmt.Printf("\rProgress: %.2f%%", progress)
        },
        RetryAttempts: 5,
    }

    err := client.SendLargeString(string(data), options)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("\nTransfer complete!")
}
```

---

## Performance Tips

1. **Batch Operations**: Send multiple small messages as one JSON message
2. **Connection Pooling**: Reuse connections instead of creating new ones
3. **Tune Keep-Alive**: Set appropriate intervals for your use case
4. **Use Chunked Transfer**: For data >250KB, automatic chunking is optimal
5. **Monitor Connection Count**: Set appropriate limits based on server capacity

---

## Security Considerations

1. **Always use SecureServer/SecureClient for sensitive data**
2. **Key exchange happens automatically** - no manual key management
3. **Each connection has unique encryption keys** (perfect forward secrecy)
4. **Replay protection** through nonce sequencing
5. **Set connection limits** to prevent DoS attacks

---

## License

Non-commercial use only. See LICENSE file for details.

---

## Support

- **GitHub**: https://github.com/DangerosoDavo/udp_network
- **Issues**: https://github.com/DangerosoDavo/udp_network/issues
- **Documentation**: [README.md](README.md), [API.md](API.md)

---

*Generated for UDP Network Package v1.0*

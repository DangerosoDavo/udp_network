# UDP Network Protocol - API Reference

## Server API

### Creating a Server

```go
server, err := udp_network.NewServer(address string) (*Server, error)
```

Creates a new UDP server listening on the specified address.

### Server Methods

#### Start/Stop

```go
server.Start()
server.Stop()
```

#### Connection Management

```go
// Set maximum concurrent connections (0 = unlimited)
server.SetMaxClients(max int)

// Get current connection count
count := server.GetClientCount() int

// Get all connected clients
clients := server.Clients() map[string]*protocol.Connection

// Configure connection timeouts
server.SetKeepAlive(timeout, keepAliveInterval time.Duration)
```

#### Message Handling

```go
// Set custom message handler
server.SetMessageHandler(handler func(*protocol.Connection, *protocol.Message))

// Send message to specific client
err := server.SendMessage(conn *protocol.Connection, msg *protocol.Message) error

// Broadcast to all clients
server.Broadcast(msg *protocol.Message)
```

#### Connection Callbacks

```go
// Set connection event callbacks
server.SetConnectionCallbacks(
    onConnect func(*protocol.Connection),
    onDisconnect func(*protocol.Connection),
)
```

## Client API

### Creating a Client

```go
client, err := udp_network.NewClient() (*Client, error)
```

Creates a new UDP client with automatic heartbeat support.

### Client Methods

#### Connection

```go
// Start the client
client.Start()

// Connect to server (starts automatic heartbeat)
err := client.Connect(address string) error

// Stop the client
client.Stop()

// Configure connection timeouts
client.SetKeepAlive(timeout, keepAliveInterval time.Duration)
```

#### Sending Messages

```go
// Send text message
err := client.SendText(text string) error

// Send command with parameters
err := client.SendCommand(cmd string, params map[string]interface{}) error

// Send custom message
err := client.SendMessage(conn *protocol.Connection, msg *protocol.Message) error
```

#### Message Handling

```go
// Set custom message handler
client.SetMessageHandler(handler func(*protocol.Message))
```

#### Interactive Mode

```go
// Run interactive CLI mode
client.RunInteractive()
```

## Message Types

### Message Structure

```go
type Message struct {
    Type      MessageType            `json:"type"`
    ID        string                 `json:"id,omitempty"`
    Timestamp int64                  `json:"ts"`
    Data      interface{}            `json:"data"`
    Metadata  map[string]interface{} `json:"meta,omitempty"`
}
```

### Message Types

```go
const (
    MessageTypeText    MessageType = "text"    // Plain text messages
    MessageTypeJSON    MessageType = "json"    // JSON data
    MessageTypeCommand MessageType = "command" // Commands
    MessageTypeBinary  MessageType = "binary"  // Binary data
)
```

## Connection API

### Connection Properties

```go
// Get connection state
state := conn.GetState() ConnectionState

// Connection states
const (
    StateNew         ConnectionState = 0
    StateHandshaking ConnectionState = 1
    StateConnected   ConnectionState = 2
    StateClosed      ConnectionState = 3
)

// Get remote address
addr := conn.Addr *net.UDPAddr
```

## Transport Configuration

### Advanced Settings

```go
// Get underlying transport
transport := server.Transport()

// Set connection limits
transport.SetMaxConnections(max int)

// Get connection count
count := transport.GetConnectionCount() int

// Force close a connection
transport.CloseConnection(addr string)
```

## Example: Game Server

```go
// Create game server
server, err := udp_network.NewServer(":8080")
if err != nil {
    log.Fatal(err)
}

// Configure for persistent gaming sessions
server.SetKeepAlive(10*time.Minute, 1*time.Minute)
server.SetMaxClients(5000)

// Player management
type Player struct {
    ID   string
    Conn *protocol.Connection
    // ... game state
}

players := make(map[string]*Player)
var playersMu sync.RWMutex

// Set up connection handlers
server.SetConnectionCallbacks(
    func(conn *protocol.Connection) {
        // Player joined
        playersMu.Lock()
        players[conn.Addr.String()] = &Player{
            ID:   GeneratePlayerID(),
            Conn: conn,
        }
        playersMu.Unlock()
    },
    func(conn *protocol.Connection) {
        // Player left
        playersMu.Lock()
        delete(players, conn.Addr.String())
        playersMu.Unlock()
    },
)

// Handle game messages
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    playersMu.RLock()
    player := players[conn.Addr.String()]
    playersMu.RUnlock()
    
    if player == nil {
        return
    }
    
    // Process game actions
    switch msg.Type {
    case protocol.MessageTypeJSON:
        if data, ok := msg.Data.(map[string]interface{}); ok {
            HandleGameAction(player, data)
        }
    }
})

server.Start()
```

## Best Practices

1. **Connection Management**
   - Use appropriate timeouts for your use case
   - For game servers: 5-10 minute timeouts
   - For chat apps: 30-60 second timeouts
   - For monitoring: 10-30 second timeouts

2. **Message Handling**
   - Use JSON messages for structured data
   - Use text messages for simple strings
   - Implement proper error handling

3. **Scalability**
   - Set connection limits based on server capacity
   - Monitor connection count and message rates
   - Use broadcasts efficiently

4. **Reliability**
   - Messages are automatically retransmitted
   - Connections are maintained with heartbeats
   - Handle disconnect events for cleanup

5. **Performance**
   - Messages are delivered out-of-order for speed
   - Use tokenization for text-heavy applications
   - Keep messages under 1400 bytes to avoid fragmentation
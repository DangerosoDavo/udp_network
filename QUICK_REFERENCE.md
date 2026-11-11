> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](README.md) and [migration notes](MAINTENANCE.md) for supported contracts.

# UDP Network - Quick Reference

One-page cheat sheet for the UDP Network package.

## Installation

```bash
go get github.com/DangerosoDavo/udp_network
```

```go
import (
    udpnet "github.com/DangerosoDavo/udp_network"
    "github.com/DangerosoDavo/udp_network/protocol"
)
```

---

## Basic Server

```go
server, _ := udpnet.NewServer(":8080")
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    // Handle message
})
server.Start()
defer server.Stop()
```

### Server Methods

| Method | Description |
|--------|-------------|
| `Start()` | Start server |
| `Stop()` | Stop server |
| `SetMessageHandler(func)` | Set message handler |
| `SendMessage(conn, msg)` | Send to client |
| `Broadcast(msg)` | Broadcast to all |
| `SetMaxClients(n)` | Set connection limit |
| `GetClientCount()` | Get client count |
| `SetKeepAlive(timeout, interval)` | Configure timeouts |
| `SetConnectionCallbacks(onConnect, onDisconnect)` | Set callbacks |
| `Clients()` | Get all connections |

---

## Basic Client

```go
client, _ := udpnet.NewClient()
client.Connect("localhost:8080")
defer client.Stop()
```

### Client Methods

| Method | Description |
|--------|-------------|
| `Connect(addr)` | Connect to server |
| `Stop()` | Disconnect |
| `SendText(text)` | Send text message |
| `SendJSON(data)` | Send JSON |
| `SendCommand(cmd, params)` | Send command |
| `SetMessageHandler(func)` | Set message handler |
| `SetKeepAlive(timeout, interval)` | Configure timeouts |
| `RunInteractive()` | Start CLI mode |

---

## Secure Server

```go
server, _ := udpnet.NewSecureServer(":8080")
server.OnMessage(func(conn *protocol.Connection, data []byte) {
    // Handle encrypted message
})
server.Start()
defer server.Stop()
```

### Secure Server Methods

| Method | Description |
|--------|-------------|
| `Start()` | Start server |
| `Stop()` | Stop server |
| `OnMessage(func)` | Set message handler |
| `SendTo(conn, data)` | Send encrypted to client |
| `Broadcast(data)` | Broadcast encrypted |
| `SetMaxConnections(n)` | Set connection limit |
| `GetClientCount()` | Get client count |

---

## Secure Client

```go
client, _ := udpnet.NewSecureClient()
client.Connect("localhost:8080")
defer client.Close()
```

### Secure Client Methods

| Method | Description |
|--------|-------------|
| `Connect(addr)` | Connect with encryption |
| `Close()` | Close connection |
| `Send(data)` | Send encrypted data |
| `SendText(text)` | Send encrypted text |
| `OnMessage(func)` | Set message handler |
| `IsConnected()` | Check connection status |
| `GetConnectionInfo()` | Get connection details |

---

## Message Types

```go
// Text
msg := &protocol.Message{
    Type: protocol.MessageTypeText,
    Data: "Hello",
    Timestamp: time.Now().Unix(),
}

// JSON
msg := &protocol.Message{
    Type: protocol.MessageTypeJSON,
    Data: map[string]interface{}{"key": "value"},
    Timestamp: time.Now().Unix(),
}

// Command
msg := &protocol.Message{
    Type: protocol.MessageTypeCommand,
    Data: "ping",
    Timestamp: time.Now().Unix(),
}
```

### Message Type Constants

| Constant | Description |
|----------|-------------|
| `MessageTypeText` | Plain text |
| `MessageTypeJSON` | JSON data |
| `MessageTypeCommand` | Command |
| `MessageTypeBinary` | Binary data |
| `MessageTypeChunked` | Chunked transfer |

---

## Large Transfers

### Sending (Client)

```go
options := &udpnet.LargeStringOptions{
    OnProgress: func(sent, total int) {
        fmt.Printf("%d/%d\n", sent, total)
    },
}
client.SendLargeString(largeData, options)
```

### Receiving (Server)

```go
receiver := udpnet.EnableLargeStringReceiver(server)
receiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
    fmt.Printf("Received: %d bytes\n", len(data))
})
```

---

## Connection States

| State | Description |
|-------|-------------|
| `StateNew` | Just created |
| `StateHandshaking` | Connecting |
| `StateConnected` | Connected |
| `StateClosed` | Closed |

```go
if conn.GetState() == protocol.StateConnected {
    // Send data
}
```

---

## Configuration

### Timeouts

```go
// Game server
server.SetKeepAlive(10*time.Minute, 1*time.Minute)

// Chat app
server.SetKeepAlive(1*time.Minute, 10*time.Second)

// IoT device
client.SetKeepAlive(10*time.Minute, 2*time.Minute)
```

### Connection Limits

```go
server.SetMaxClients(1000)

// Or via transport
transport := server.Transport()
transport.SetMaxConnections(5000)
```

---

## Common Patterns

### Echo Server

```go
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    server.SendMessage(conn, msg) // Echo back
})
```

### Broadcast Server

```go
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    server.Broadcast(msg) // Send to all
})
```

### Request/Response

```go
// Client
msg := &protocol.Message{
    Type: protocol.MessageTypeCommand,
    ID: "req-123",
    Data: "get_data",
}
client.SendMessage(client.Connection, msg)

// Server
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    response := &protocol.Message{
        Type: protocol.MessageTypeJSON,
        ID: msg.ID, // Same ID
        Data: map[string]interface{}{"result": "data"},
    }
    server.SendMessage(conn, response)
})
```

### Connection Events

```go
server.SetConnectionCallbacks(
    func(conn *protocol.Connection) {
        log.Printf("Connected: %s", conn.Addr.String())
    },
    func(conn *protocol.Connection) {
        log.Printf("Disconnected: %s", conn.Addr.String())
    },
)
```

---

## Interactive Client Commands

```bash
./client localhost:8080

> /stats              # Get server stats
> /clients            # List clients
> /broadcast <msg>    # Broadcast message
> /ping               # Ping server
> /stress 100         # Send 100 messages
> /quit               # Exit
> Hello               # Send text message
```

---

## Error Handling

```go
// Check connection before sending
if client.Connection == nil ||
   client.Connection.GetState() != protocol.StateConnected {
    return fmt.Errorf("not connected")
}

// Handle send errors
if err := client.SendText("test"); err != nil {
    log.Printf("Send failed: %v", err)
}

// Always cleanup
defer server.Stop()
defer client.Stop()
defer secureClient.Close()
```

---

## Protocol Features

### Reliability
- ✅ Automatic ACK/NACK
- ✅ Packet retransmission
- ✅ Out-of-order delivery
- ✅ Congestion control
- ✅ Flow control

### Security (Secure variants)
- ✅ ChaCha20-Poly1305 encryption
- ✅ Curve25519 key exchange
- ✅ Perfect forward secrecy
- ✅ Replay protection
- ✅ Authenticated encryption

### Performance
- ✅ 10,000+ msg/sec
- ✅ Sub-ms latency (local)
- ✅ Automatic fragmentation
- ✅ Text tokenization (2-4x compression)
- ✅ Bitmap ACKs (bulk acknowledgment)

---

## Performance Tips

1. **Batch small messages** into single JSON message
2. **Reuse connections** - don't reconnect frequently
3. **Use appropriate timeouts** for your use case
4. **Auto-chunking** happens at 250KB threshold
5. **Set connection limits** based on server capacity
6. **Monitor client count** regularly

---

## Security Best Practices

1. **Use Secure variants** for sensitive data
2. **Set connection limits** to prevent DoS
3. **Monitor connection rate** for abuse detection
4. **Validate all input** in message handlers
5. **Use timeouts** to prevent resource exhaustion

---

## Debugging

```go
// Get transport for debugging
transport := server.Transport()
count := transport.GetConnectionCount()
log.Printf("Active connections: %d", count)

// Check connection state
log.Printf("State: %v", conn.GetState())

// Get all clients
clients := server.Clients()
for addr, conn := range clients {
    log.Printf("%s: %v", addr, conn.GetState())
}

// Get connection info (secure client)
info := secureClient.GetConnectionInfo()
log.Println(info)
```

---

## Building Examples

```bash
# Basic server/client
go build -o server ./cmd/server
go build -o client ./cmd/client

# Secure server
go build -o secure-server ./cmd/secure-server

# Examples
go build ./examples/chat
go build ./examples/game-server
go build ./examples/secure-chat

# Tests
go run tests/simple/main.go
go run tests/performance/main.go
```

---

## Common Use Cases

### Game Server
```go
server.SetKeepAlive(10*time.Minute, 1*time.Minute)
server.SetMaxClients(5000)
// Use JSON messages for game events
```

### Chat Application
```go
server.SetKeepAlive(1*time.Minute, 10*time.Second)
// Use Broadcast() for messages
```

### IoT Data Collection
```go
server.SetKeepAlive(15*time.Minute, 3*time.Minute)
server.SetMaxClients(10000)
// Use MessageTypeJSON for sensor data
```

### File Transfer
```go
// Use SendLargeString() for files >250KB
client.SendLargeString(fileData, options)
```

---

## Links

- **Full API Reference**: [API_REFERENCE.md](API_REFERENCE.md)
- **Security Fixes**: [SECURITY_FIXES.md](SECURITY_FIXES.md)
- **Detailed Documentation**: [API.md](API.md)
- **Usage Guide**: [README-USAGE.md](README-USAGE.md)
- **Troubleshooting**: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

---

*UDP Network Package - Quick Reference v1.0*

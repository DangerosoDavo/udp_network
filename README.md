# UDP Network Protocol

A high-performance UDP networking protocol implementation in Go with reliability features, JSON message support, and LLM-style string tokenization for efficient text transmission. Optimized for real-time applications like game servers and chat systems.

## Features

- **Reliable UDP Transport**: ACK/NACK system with automatic retransmission and out-of-order delivery
- **String Tokenization**: BPE-style tokenizer for efficient text encoding (2-4x compression)
- **JSON Messaging**: Structured message format with automatic serialization
- **Packet Fragmentation**: Automatic handling of large messages
- **Persistent Connections**: Configurable timeouts and automatic heartbeat for long-lived connections
- **Connection Management**: Lightweight session tracking with connection limits
- **Congestion Control**: Adaptive windowing with RTT-based timeouts
- **Game Server Ready**: Connection callbacks and state management for game logic

## Architecture

### Protocol Stack
```
Application Layer (JSON Messages)
     ↓
Tokenization Layer (Text Compression)
     ↓
Message Layer (Fragmentation/Reassembly)
     ↓
Transport Layer (Reliability/Flow Control)
     ↓
UDP Socket Layer
```

### Packet Format
```
[Header: 16 bytes]
- Magic: 2 bytes (0x5544)
- Type: 1 byte (DATA/ACK/NACK/KEEPALIVE/HANDSHAKE)
- Flags: 1 byte (FRAGMENT/COMPRESSED/TOKENIZED/LASTFRAG)
- SeqNum: 4 bytes
- AckNum: 4 bytes
- FragmentInfo: 4 bytes (ID:16, Offset:8, Total:8)

[Payload: Variable up to 1400 bytes]
```

## Building

```bash
# Install Go 1.21+ if not already installed
# Extract the provided go1.24.3.linux-amd64.tar.gz if needed

# Build server
go build -o udp-server ./cmd/server

# Build client
go build -o udp-client ./cmd/client

# Build chat example
go build -o chat ./examples/chat
```

## Usage

### Basic Server/Client

```bash
# Start server
./udp-server

# In another terminal, start client
./udp-client localhost:8080
```

### Client Commands
- Type any text to send as a message
- `/stats` - Get server statistics
- `/clients` - List connected clients
- `/broadcast <msg>` - Broadcast to all clients
- `/ping` - Ping the server
- `/stress <n>` - Send n messages rapidly
- `/quit` - Exit

### Chat Example

```bash
# Start chat server
./chat -server -addr :9000

# Connect clients
./chat -addr localhost:9000
```

## API Examples

### Basic Usage

```go
// Create server
server, err := udp_network.NewServer(":8080")
server.Start()

// Create client  
client, err := udp_network.NewClient()
client.Start()
err = client.Connect("localhost:8080")

// Send message
msg := &protocol.Message{
    Type: protocol.MessageTypeText,
    Data: "Hello, World!",
}
client.SendMessage(client.Connection, msg)
```

### Game Server Configuration

```go
// Create game server with persistent connections
server, err := udp_network.NewServer(":8080")

// Configure for gaming (5 min timeout, 30s keep-alive)
server.SetKeepAlive(5*time.Minute, 30*time.Second)
server.SetMaxClients(1000)

// Set up game callbacks
server.SetConnectionCallbacks(
    func(conn *protocol.Connection) {
        // Player joined
        playerID := conn.Addr.String()
        log.Printf("Player connected: %s", playerID)
        // Initialize player state...
    },
    func(conn *protocol.Connection) {
        // Player disconnected
        playerID := conn.Addr.String()
        log.Printf("Player disconnected: %s", playerID)
        // Cleanup player state...
    },
)

// Custom message handler
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    // Handle game messages
    switch msg.Type {
    case protocol.MessageTypeJSON:
        // Process game actions...
    }
})

server.Start()
```

### Client with Auto-Heartbeat

```go
client, err := udp_network.NewClient()
client.Start()

// Connect - heartbeat starts automatically
err = client.Connect("localhost:8080")

// Configure keep-alive if needed (optional)
client.SetKeepAlive(5*time.Minute, 30*time.Second)

// Send game actions
action := &protocol.Message{
    Type: protocol.MessageTypeJSON,
    Data: map[string]interface{}{
        "action": "move",
        "x": 100,
        "y": 200,
    },
}
client.SendMessage(client.Connection, action)
```

## Performance

- **Tokenization**: ~2-4x compression for typical English text
- **Throughput**: Handles 10,000+ messages/sec on modern hardware
- **Latency**: Sub-millisecond RTT on local network
- **Reliability**: Automatic retransmission with exponential backoff

## Protocol Features

### Reliability
- Sequence numbers for packet ordering
- Selective ACK/NACK for efficient retransmission  
- Bitmap ACKs for bulk acknowledgment (up to 64 packets)
- Sliding window for flow control
- Adaptive timeout based on RTT measurement
- Out-of-order packet delivery for minimal latency

### Tokenization
- Pre-loaded common English bigrams and words
- Dynamic vocabulary learning with BPE algorithm
- Binary encoding reduces token size to 2 bytes
- Fallback to raw encoding if compression is inefficient

### Connection Management
- Lightweight handshake protocol
- Configurable connection timeouts (default 30s, can be set to hours)
- Automatic client-side heartbeat (every 5 seconds)
- Server-side keep-alive messages (configurable interval)
- Connection state callbacks for application integration
- Connection limits for scalability
- Graceful connection cleanup

### Game Server Features
- Persistent connections for long gaming sessions
- Connection event hooks for player join/leave
- Efficient broadcast to all players
- Per-connection state management
- Minimal overhead heartbeat system

## Recent Improvements

### Connection Persistence (Latest)
- Added automatic client-side heartbeat (5-second interval)
- Configurable connection timeout (default 30s, can extend to hours)
- Proper connection cleanup to prevent memory leaks
- Single disconnect event instead of spam

### Performance Optimizations
- Out-of-order packet delivery for minimal latency
- Bitmap ACKs reduce acknowledgment traffic by up to 64x
- Reduced keep-alive frequency from 5s to 10s
- Connection limit support for scalability

### Bug Fixes
- Fixed server OnConnect callback not triggering
- Fixed connection cleanup causing disconnect spam
- Fixed message handler setup race conditions
- Fixed fragmentation for large messages

## Documentation

- [API Reference](API.md) - Complete API documentation
- [Troubleshooting Guide](TROUBLESHOOTING.md) - Common issues and solutions
- [Usage Examples](README-USAGE.md) - How to run server and client

## Quick Start

```bash
# Build everything
./build.sh

# Terminal 1 - Start server
./bin/server

# Terminal 2 - Start client
./bin/client localhost:8080

# Type messages in client, see them logged in server terminal
```

## Testing

```bash
# Run all tests
./test.sh all

# Run specific tests
./test.sh basic    # Basic connectivity
./test.sh perf     # Performance benchmark
./test.sh stress   # Stress test with packet loss
```

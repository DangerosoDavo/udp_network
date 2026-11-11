> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](README.md) and [migration notes](MAINTENANCE.md) for supported contracts.

# WebSocket Transport - Unified API Guide

Complete guide to using WebSocket transport with the same API as UDP networking.

## Overview

The UDP Network package now supports **WebSocket transport** as a fallback/alternative to UDP, allowing web-based clients to use the same API. This enables:

- ✅ **Browser compatibility** - Web apps can connect directly
- ✅ **Unified API** - Same code works for UDP and WebSocket
- ✅ **Easy switching** - Change transport with one parameter
- ✅ **Feature parity** - All features work on both transports
- ✅ **Automatic adaptation** - Protocol-specific optimizations

---

## Quick Start

### Basic WebSocket Server

```go
import udpnet "github.com/DangerosoDavo/udp_network"

// Create WebSocket server
server, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
    Transport: udpnet.TransportWebSocket,
    MaxClients: 100,
})

server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    // Handle message - same as UDP!
})

server.Start()
```

### Basic WebSocket Client

```go
// Create WebSocket client
client, _ := udpnet.NewUnifiedClient(&udpnet.ClientOptions{
    Transport: udpnet.TransportWebSocket,
})

client.Connect("ws://localhost:8080")
client.SendText("Hello, WebSocket!")
```

---

## Unified Transport Interface

### Creating Servers

```go
// UDP Server
udpServer, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
    Transport: udpnet.TransportUDP,  // UDP transport
})

// WebSocket Server
wsServer, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
    Transport: udpnet.TransportWebSocket,  // WebSocket transport
})

// Both implement UnifiedServer interface - same methods!
```

### Creating Clients

```go
// UDP Client
udpClient, _ := udpnet.NewUnifiedClient(&udpnet.ClientOptions{
    Transport: udpnet.TransportUDP,
})

// WebSocket Client
wsClient, _ := udpnet.NewUnifiedClient(&udpnet.ClientOptions{
    Transport: udpnet.TransportWebSocket,
})

// Both implement UnifiedClient interface - same methods!
```

---

## UnifiedServer Interface

All methods work identically across UDP and WebSocket:

```go
type UnifiedServer interface {
    Start()
    Stop()
    SetMessageHandler(func(*protocol.Connection, *protocol.Message))
    SendMessage(*protocol.Connection, *protocol.Message) error
    Broadcast(*protocol.Message)
    Clients() map[string]*protocol.Connection
    SetMaxClients(int)
    GetClientCount() int
    SetKeepAlive(timeout, interval time.Duration)
    SetConnectionCallbacks(onConnect, onDisconnect func(*protocol.Connection))
    GetTransportType() TransportType  // Returns "udp" or "websocket"
}
```

### Example: Transport-Agnostic Server

```go
func runServer(transport udpnet.TransportType) {
    server, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
        Transport: transport,  // UDP or WebSocket
    })

    // This code works for BOTH transports!
    server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
        log.Printf("[%s] Received: %v", server.GetTransportType(), msg.Data)
        server.SendMessage(conn, msg) // Echo
    })

    server.SetConnectionCallbacks(
        func(conn *protocol.Connection) {
            log.Printf("Client connected via %s", server.GetTransportType())
        },
        func(conn *protocol.Connection) {
            log.Printf("Client disconnected")
        },
    )

    server.Start()
    defer server.Stop()
}

// Use with UDP
runServer(udpnet.TransportUDP)

// Use with WebSocket
runServer(udpnet.TransportWebSocket)
```

---

## UnifiedClient Interface

```go
type UnifiedClient interface {
    Connect(serverAddr string) error
    Start()
    Stop()
    SendText(text string) error
    SendCommand(cmd string, params map[string]interface{}) error
    SendJSON(data interface{}) error
    SendMessage(*protocol.Connection, *protocol.Message) error
    SetMessageHandler(func(*protocol.Message))
    SetKeepAlive(timeout, interval time.Duration)
    GetTransportType() TransportType
    IsConnected() bool
}
```

### Example: Transport-Agnostic Client

```go
func runClient(transport udpnet.TransportType, addr string) {
    // Adjust address format for WebSocket
    if transport == udpnet.TransportWebSocket {
        addr = "ws://" + addr
    }

    client, _ := udpnet.NewUnifiedClient(&udpnet.ClientOptions{
        Transport: transport,
    })

    client.Connect(addr)
    defer client.Stop()

    // This code works for BOTH transports!
    client.SetMessageHandler(func(msg *protocol.Message) {
        fmt.Printf("Received: %v\n", msg.Data)
    })

    client.SendText("Hello from " + string(transport))
}

// Use with UDP
runClient(udpnet.TransportUDP, "localhost:8080")

// Use with WebSocket
runClient(udpnet.TransportWebSocket, "localhost:8080")
```

---

## Configuration Options

### ServerOptions

```go
type ServerOptions struct {
    Transport     TransportType  // udp or websocket
    MaxClients    int           // Max concurrent connections
    Timeout       time.Duration // Connection timeout
    KeepAlive     time.Duration // Keep-alive interval
    EnableLogging bool          // Enable debug logging
}

// Example
server, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
    Transport:     udpnet.TransportWebSocket,
    MaxClients:    500,
    Timeout:       time.Minute,
    KeepAlive:     10 * time.Second,
    EnableLogging: true,
})
```

### ClientOptions

```go
type ClientOptions struct {
    Transport     TransportType  // udp or websocket
    Timeout       time.Duration // Connection timeout
    KeepAlive     time.Duration // Keep-alive interval
    EnableLogging bool          // Enable debug logging
}

// Example
client, _ := udpnet.NewUnifiedClient(&udpnet.ClientOptions{
    Transport:     udpnet.TransportWebSocket,
    Timeout:       30 * time.Second,
    KeepAlive:     5 * time.Second,
    EnableLogging: true,
})
```

---

## Feature Parity

All features work on both transports:

| Feature | UDP | WebSocket |
|---------|-----|-----------|
| Message Types (Text, JSON, Command) | ✅ | ✅ |
| Broadcast | ✅ | ✅ |
| Connection Callbacks | ✅ | ✅ |
| Keep-Alive/Heartbeat | ✅ | ✅ |
| Connection Limits | ✅ | ✅ |
| Timeouts | ✅ | ✅ |
| Large String Transfer | ✅ | ✅ |
| Message Handlers | ✅ | ✅ |
| Connection State | ✅ | ✅ |

---

## Transport-Specific Details

### WebSocket Transport

**Advantages:**
- ✅ Works in browsers
- ✅ Firewall-friendly (uses HTTP ports)
- ✅ Better NAT traversal
- ✅ Built-in framing (no fragmentation issues)

**Implementation:**
- Uses gorilla/websocket library
- HTTP/1.1 upgrade handshake
- Automatic ping/pong for keep-alive
- Text and binary message support
- CORS-enabled by default (configure as needed)

**URL Format:**
```go
// WebSocket URLs
client.Connect("ws://localhost:8080")
client.Connect("ws://example.com:9000")
client.Connect("wss://secure.example.com")  // Secure WebSocket
```

### UDP Transport

**Advantages:**
- ✅ Lower latency
- ✅ Better for high-throughput
- ✅ Less protocol overhead
- ✅ Custom reliability layer

**Implementation:**
- Custom UDP reliability protocol
- ACK/NACK system
- Packet fragmentation/reassembly
- Congestion control
- Out-of-order delivery

---

## Examples

### Example 1: Chat Server (Both Transports)

```go
package main

import (
    udpnet "github.com/DangerosoDavo/udp_network"
    "github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
    // Try: websocket or udp
    transport := udpnet.TransportWebSocket

    server, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
        Transport: transport,
    })

    server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
        if msg.Type == protocol.MessageTypeText {
            // Broadcast chat message
            broadcast := &protocol.Message{
                Type: protocol.MessageTypeJSON,
                Data: map[string]interface{}{
                    "from":    conn.Addr.String(),
                    "message": msg.Data,
                },
            }
            server.Broadcast(broadcast)
        }
    })

    server.Start()
    defer server.Stop()
    select {}
}
```

### Example 2: Runtime Transport Selection

```go
func createServer(useWebSocket bool) udpnet.UnifiedServer {
    transport := udpnet.TransportUDP
    if useWebSocket {
        transport = udpnet.TransportWebSocket
    }

    server, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
        Transport: transport,
    })

    return server
}

// Select transport based on environment or flag
server := createServer(os.Getenv("USE_WEBSOCKET") == "true")
```

### Example 3: Multi-Transport Server

```go
// Run both transports simultaneously
func main() {
    // UDP server on :8080
    udpServer, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
        Transport: udpnet.TransportUDP,
    })

    // WebSocket server on :8081
    wsServer, _ := udpnet.NewUnifiedServer(":8081", &udpnet.ServerOptions{
        Transport: udpnet.TransportWebSocket,
    })

    // Shared message handler
    handler := func(conn *protocol.Connection, msg *protocol.Message) {
        // Handle message from either transport
    }

    udpServer.SetMessageHandler(handler)
    wsServer.SetMessageHandler(handler)

    udpServer.Start()
    wsServer.Start()

    defer udpServer.Stop()
    defer wsServer.Stop()

    select {}
}
```

---

## Browser Integration

### JavaScript WebSocket Client

```html
<!DOCTYPE html>
<html>
<head>
    <title>UDP Network WebSocket Client</title>
</head>
<body>
    <h1>WebSocket Chat</h1>
    <div id="messages"></div>
    <input type="text" id="input" placeholder="Type message...">
    <button onclick="sendMessage()">Send</button>

    <script>
        const ws = new WebSocket('ws://localhost:8080');

        ws.onopen = () => {
            console.log('Connected to server');
        };

        ws.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            displayMessage(msg);
        };

        function sendMessage() {
            const input = document.getElementById('input');
            const msg = {
                type: 'text',
                ts: Math.floor(Date.now() / 1000),
                data: input.value
            };
            ws.send(JSON.stringify(msg));
            input.value = '';
        }

        function displayMessage(msg) {
            const div = document.getElementById('messages');
            div.innerHTML += `<p>${JSON.stringify(msg.data)}</p>`;
        }
    </script>
</body>
</html>
```

### Message Format

Both transports use JSON-encoded protocol messages:

```javascript
// Text message
{
    "type": "text",
    "ts": 1234567890,
    "data": "Hello, World!"
}

// JSON message
{
    "type": "json",
    "ts": 1234567890,
    "data": {
        "command": "stats"
    }
}

// Command message
{
    "type": "command",
    "ts": 1234567890,
    "data": "ping"
}
```

---

## Migration Guide

### From UDP-Only to Unified

**Before (UDP only):**
```go
server, _ := udpnet.NewServer(":8080")
```

**After (Unified):**
```go
// Still works!
server, _ := udpnet.NewServer(":8080")

// Or use unified interface with explicit transport
server, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
    Transport: udpnet.TransportUDP,  // Explicit UDP
})

// Easy to switch to WebSocket
server, _ := udpnet.NewUnifiedServer(":8080", &udpnet.ServerOptions{
    Transport: udpnet.TransportWebSocket,  // Just change this!
})
```

**No Breaking Changes:** Existing code continues to work unchanged.

---

## Performance Considerations

### UDP Transport
- **Latency**: Sub-millisecond on local network
- **Throughput**: 10,000+ messages/sec
- **Overhead**: Minimal (~16 bytes header)
- **Best for**: Real-time games, IoT, high-frequency trading

### WebSocket Transport
- **Latency**: 1-5ms (HTTP overhead)
- **Throughput**: 5,000-10,000 messages/sec
- **Overhead**: ~2-6 bytes per frame
- **Best for**: Web apps, browser clients, firewall-friendly scenarios

### Recommendations

| Use Case | Recommended Transport |
|----------|----------------------|
| Browser/Web App | WebSocket |
| Native Desktop App | UDP |
| Mobile App | Either (prefer WebSocket for better NAT traversal) |
| IoT Devices | UDP (lower overhead) |
| Real-time Games | UDP (lower latency) |
| Chat Application | Either (WebSocket for web, UDP for native) |
| Behind Firewall | WebSocket (easier to configure) |

---

## Testing

### Running Examples

```bash
# WebSocket Chat
go build -o websocket-chat ./examples/websocket-chat

# Terminal 1: Server
./websocket-chat -server -addr :8080

# Terminal 2: Client
./websocket-chat -addr localhost:8080
```

### Unified Transport Example

```bash
# Build
go build -o unified ./examples/unified-transport

# UDP Server
./unified -server -transport udp -addr :8080

# UDP Client
./unified -transport udp -addr localhost:8080

# WebSocket Server
./unified -server -transport websocket -addr :8081

# WebSocket Client
./unified -transport websocket -addr localhost:8081
```

---

## Security

### WebSocket Security

**CORS Configuration:**
```go
// In websocket_server.go, the upgrader allows all origins by default:
upgrader := websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true  // Configure based on your security needs
    },
}

// Production: Restrict origins
CheckOrigin: func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    return origin == "https://yourdomain.com"
}
```

**TLS/WSS Support:**
```go
// Use wss:// for secure WebSocket
client.Connect("wss://secure.example.com:8080")

// Server-side: Use http.ServeTLS instead of http.ListenAndServe
s.server.ListenAndServeTLS("cert.pem", "key.pem")
```

---

## Troubleshooting

### WebSocket-Specific Issues

**Problem:** Connection refused
```
Error: websocket dial failed: dial tcp: connection refused
```
**Solution:** Ensure server is running and address format is correct (`ws://host:port`)

---

**Problem:** CORS error in browser
```
Access to WebSocket at 'ws://localhost:8080' from origin 'http://example.com' has been blocked
```
**Solution:** Configure `CheckOrigin` in server to allow your origin

---

**Problem:** Frequent disconnections
```
WebSocket read error: websocket: close 1006 (abnormal closure)
```
**Solution:** Increase timeout or decrease keep-alive interval:
```go
server.SetKeepAlive(2*time.Minute, 10*time.Second)
```

---

## API Reference

See [API_REFERENCE.md](API_REFERENCE.md) for complete API documentation.

See [QUICK_REFERENCE.md](QUICK_REFERENCE.md) for quick lookup.

---

## Conclusion

The WebSocket transport provides seamless integration with web-based clients while maintaining the same API as UDP transport. This allows developers to:

1. **Write once, deploy anywhere** - Same code works for UDP and WebSocket
2. **Easy migration** - Switch transports with one parameter
3. **Browser support** - Connect directly from web applications
4. **No code duplication** - Unified interface for all transport types

The unified transport interface makes it easy to support both native and web clients with minimal code changes.

---

*WebSocket Transport Guide - UDP Network Package v1.0*

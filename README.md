# UDP Network Protocol

A high-performance UDP networking protocol implementation in Go with reliability features, end-to-end encryption, JSON message support, and LLM-style string tokenization for efficient text transmission. Optimized for real-time applications like game servers and chat systems.

## 📜 License

**This software is licensed for NON-COMMERCIAL USE ONLY.** 

Commercial use requires explicit written permission. See [LICENSE](LICENSE) for full terms.

For commercial licensing inquiries, contact:
- GitHub: [@DangerosoDavo](https://github.com/DangerosoDavo)
- Repository: [https://github.com/DangerosoDavo/udp_network](https://github.com/DangerosoDavo/udp_network)

## ✨ Features

- **Reliable UDP Transport**: ACK/NACK system with automatic retransmission and out-of-order delivery
- **End-to-End Encryption**: ChaCha20-Poly1305 with Curve25519 key exchange for fast, secure communication
- **String Tokenization**: BPE-style tokenizer for efficient text encoding (2-4x compression)
- **JSON Messaging**: Structured message format with automatic serialization
- **Packet Fragmentation**: Automatic handling of large messages
- **Persistent Connections**: Configurable timeouts and automatic heartbeat for long-lived connections
- **Connection Management**: Lightweight session tracking with connection limits
- **Congestion Control**: Adaptive windowing with RTT-based timeouts
- **Game Server Ready**: Connection callbacks and state management for game logic

## 🏗️ Architecture

### Protocol Stack

```
Application Layer (JSON Messages)
     ↓
Encryption Layer (ChaCha20-Poly1305) [Optional]
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
- Type: 1 byte (DATA/ACK/NACK/KEEPALIVE/HANDSHAKE/CRYPTO/ENCRYPTED)
- Flags: 1 byte (FRAGMENT/COMPRESSED/TOKENIZED/LASTFRAG/ENCRYPTED)
- SeqNum: 4 bytes
- AckNum: 4 bytes
- FragmentInfo: 4 bytes (ID:16, Offset:8, Total:8)

[Payload: Variable up to 1400 bytes]
```

## 🚀 Quick Start

### Prerequisites

- Go 1.21+ (included: `go1.24.3.linux-amd64.tar.gz`)

### Installation

```bash
# Clone the repository
git clone https://github.com/DangerosoDavo/udp_network.git
cd udp_network

# Build everything
./build.sh

# Or manually
go build -o udp-server ./cmd/server
go build -o udp-client ./cmd/client
```

### Basic Usage

```bash
# Terminal 1 - Start server
./udp-server

# Terminal 2 - Start client
./udp-client localhost:8080

# Type messages in client
```

### Secure Communication

```bash
# Terminal 1 - Start secure server
go run examples/secure-chat/main.go -server -addr :9999

# Terminal 2 - Start secure client
go run examples/secure-chat/main.go -addr localhost:9999
```

## 📚 API Examples

### Basic Server/Client

```go
import udpnet "github.com/DangerosoDavo/udp_network"

// Create server
server, err := udpnet.NewServer(":8080")
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    fmt.Printf("Received: %v\n", msg.Data)
})
server.Start()

// Create client  
client, err := udpnet.NewClient()
client.Connect("localhost:8080")
client.SendText("Hello, World!")
```

### Secure Communication

```go
// Secure Server - All traffic encrypted
server, err := udpnet.NewSecureServer(":8080")
server.OnMessage(func(conn *protocol.Connection, data []byte) {
    fmt.Printf("Decrypted message: %s\n", string(data))
})
server.Start()

// Secure Client
client, err := udpnet.NewSecureClient()
client.Connect("localhost:8080")
client.Send([]byte("This is encrypted!"))
```

### Game Server Configuration

```go
// Create game server with persistent connections
server, err := udpnet.NewServer(":8080")

// Configure for gaming (5 min timeout, 30s keep-alive)
server.SetKeepAlive(5*time.Minute, 30*time.Second)
server.SetMaxClients(1000)

// Set up game callbacks
server.SetConnectionCallbacks(
    func(conn *protocol.Connection) {
        // Player joined
        log.Printf("Player connected: %s", conn.Addr.String())
    },
    func(conn *protocol.Connection) {
        // Player disconnected
        log.Printf("Player disconnected: %s", conn.Addr.String())
    },
)

server.Start()
```

## 🔐 Encryption Details

The secure transport provides military-grade encryption with minimal overhead:

- **Algorithm**: ChaCha20-Poly1305 (faster than AES-GCM on most CPUs)
- **Key Exchange**: Curve25519 Diffie-Hellman
- **Perfect Forward Secrecy**: New keys for each connection
- **Overhead**: ~28 bytes per packet (12-byte nonce + 16-byte auth tag)
- **Performance**: Typically 5-15% overhead vs unencrypted

## 📊 Performance

- **Throughput**: 10,000+ messages/sec on modern hardware
- **Latency**: Sub-millisecond RTT on local network
- **Compression**: 2-4x for typical English text via tokenization
- **Encryption Overhead**: 5-15% with ChaCha20-Poly1305

### Benchmark Results

```bash
# Run performance tests
go run tests/performance/main.go

# Run encryption benchmark
go run tests/crypto-benchmark/main.go
```

## 🎮 Examples

### Chat Server
```bash
go run examples/chat/main.go -server
go run examples/chat/main.go
```

### Secure Chat
```bash
go run examples/secure-chat/main.go -server
go run examples/secure-chat/main.go
```

### Game Server
```bash
go run examples/game-server/main.go
```

## 🧪 Testing

```bash
# Run all tests
./test.sh all

# Run specific tests
./test.sh basic    # Basic connectivity
./test.sh perf     # Performance benchmark
./test.sh stress   # Stress test with packet loss
./test.sh crypto   # Encryption tests

# Manual testing
./manual-test.sh   # Interactive test
./quick-test.sh    # Quick verification
```

## 📦 Using as a Module

### From GitHub (Recommended)

```bash
go get github.com/DangerosoDavo/udp_network@latest
```

### Local Development

```go
// In your go.mod
replace github.com/DangerosoDavo/udp_network => /path/to/udp_network
```

### Import in Your Code

```go
import (
    udpnet "github.com/DangerosoDavo/udp_network"
    "github.com/DangerosoDavo/udp_network/protocol"
)
```

## 📖 Documentation

**Quick Links:**
- 🚀 [Quick Reference](QUICK_REFERENCE.md) - One-page cheat sheet for all APIs
- 📚 [Complete API Reference](API_REFERENCE.md) - Full API documentation with examples
- 📑 [Documentation Index](DOCUMENTATION_INDEX.md) - Guide to all documentation

**Detailed Guides:**
- [API Overview](API.md) - High-level API documentation
- [Installation Guide](INSTALLATION.md) - Detailed setup instructions
- [Large String Transfer](LARGE_STRINGS.md) - Sending files >250KB
- [Security Fixes](SECURITY_FIXES.md) - Recent security improvements
- [Troubleshooting Guide](TROUBLESHOOTING.md) - Common issues and solutions
- [Usage Examples](README-USAGE.md) - Running server and client

## 🛠️ Protocol Features

### Reliability
- Sequence numbers for packet ordering
- Selective ACK/NACK for efficient retransmission
- Bitmap ACKs for bulk acknowledgment (up to 64 packets)
- Sliding window flow control
- Adaptive timeout based on RTT measurement
- Out-of-order packet delivery

### Security
- End-to-end encryption with ChaCha20-Poly1305
- Curve25519 key exchange
- Perfect forward secrecy
- Authenticated encryption (AEAD)
- Replay attack protection

### Efficiency
- BPE tokenization for text compression
- Dynamic vocabulary learning
- Binary token encoding (2 bytes per token)
- Automatic fragmentation for large messages
- Minimal protocol overhead

### Connection Management
- Lightweight handshake protocol
- Configurable timeouts
- Automatic heartbeat
- Connection state callbacks
- Graceful shutdown

## 🤝 Contributing

This project is licensed for non-commercial use only. Contributions are welcome under the same license terms. Please ensure any contributions maintain backward compatibility and include appropriate tests.

## ⚖️ Legal Notice

**This software is provided for NON-COMMERCIAL USE ONLY.**

Any commercial use, including but not limited to:
- Use in commercial products or services
- Use by for-profit organizations
- Selling or licensing the software
- Monetizing applications built with this software

**REQUIRES explicit written permission from the copyright holder.**

See [LICENSE](LICENSE) for complete terms and conditions.

## 📧 Contact

For commercial licensing or other inquiries:
- GitHub: [@DangerosoDavo](https://github.com/DangerosoDavo)
- Repository: [https://github.com/DangerosoDavo/udp_network](https://github.com/DangerosoDavo/udp_network)
> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](../README.md) and [migration notes](../MAINTENANCE.md) for supported contracts.

# UDP Network Protocol - Examples

This directory contains example applications demonstrating various features of the UDP Network Protocol.

## Available Examples

### 1. Chat Application (`chat/`)

A simple chat application demonstrating basic UDP communication with reliability.

**Features:**
- Real-time messaging
- User nicknames
- Server broadcast messages
- Connection status notifications

**Usage:**
```bash
# Start server
go run chat/main.go -server -addr :9000

# Start client(s)
go run chat/main.go -addr localhost:9000
```

### 2. Secure Chat (`secure-chat/`)

An encrypted chat application using ChaCha20-Poly1305 encryption.

**Features:**
- End-to-end encryption
- Secure key exchange (Curve25519)
- All features of basic chat
- Perfect forward secrecy

**Usage:**
```bash
# Start secure server
go run secure-chat/main.go -server -addr :9999

# Start secure client(s)
go run secure-chat/main.go -addr localhost:9999
```

### 3. Game Server (`game-server/`)

A demonstration of using the protocol for game server applications.

**Features:**
- Player management
- Game state synchronization
- Position updates
- Action handling
- Persistent connections
- Broadcast to all players

**Usage:**
```bash
# Start game server
go run game-server/main.go

# The server will:
# - Listen on port 9999
# - Accept player connections
# - Handle player movements
# - Broadcast state updates
```

## Building Examples

You can build any example as a standalone binary:

```bash
# Build chat server/client
go build -o chat-app examples/chat/main.go

# Build secure chat
go build -o secure-chat examples/secure-chat/main.go

# Build game server
go build -o game-server examples/game-server/main.go
```

## Example Features Demonstrated

### Basic Communication
- Establishing UDP connections
- Sending and receiving messages
- Connection management
- Graceful shutdown

### Reliability
- Automatic retransmission
- Out-of-order delivery
- Connection heartbeat
- Timeout handling

### Security (secure-chat)
- Encryption key exchange
- Message encryption/decryption
- Secure connection establishment
- Zero configuration security

### Performance (game-server)
- High-frequency updates
- Efficient broadcasting
- Player state management
- Minimal latency design

## License

These examples are provided under the same non-commercial license as the main project. See the [LICENSE](../LICENSE) file for details.
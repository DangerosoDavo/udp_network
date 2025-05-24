# Secure Chat Example

This example demonstrates end-to-end encrypted communication using the UDP Network Protocol with ChaCha20-Poly1305 encryption.

## Features

- **Military-grade encryption**: ChaCha20-Poly1305 authenticated encryption
- **Secure key exchange**: Curve25519 Diffie-Hellman
- **Perfect Forward Secrecy**: New keys for each session
- **Zero configuration**: Encryption is automatic and transparent
- **Minimal overhead**: Only ~28 bytes per packet

## How It Works

1. **Connection Establishment**:
   - Client initiates connection to server
   - Both parties generate ephemeral Curve25519 key pairs
   - Public keys are exchanged during handshake
   - Shared secret is computed using ECDH

2. **Message Encryption**:
   - Each message is encrypted with ChaCha20-Poly1305
   - Unique nonce for each message prevents replay attacks
   - Authentication tag ensures message integrity

3. **Communication**:
   - All messages are automatically encrypted before sending
   - Decryption happens transparently on receive
   - Failed authentication results in dropped packets

## Usage

### Starting the Server

```bash
go run main.go -server -addr :9999
```

The server will:
- Listen on the specified port
- Accept encrypted connections
- Echo messages back to clients

### Starting a Client

```bash
go run main.go -addr localhost:9999
```

The client will:
- Connect to the server with encryption
- Establish secure session
- Allow you to type and send encrypted messages

## Security Details

### Encryption
- **Algorithm**: ChaCha20-Poly1305 (IETF variant)
- **Key Size**: 256 bits
- **Nonce Size**: 96 bits (12 bytes)
- **Tag Size**: 128 bits (16 bytes)

### Key Exchange
- **Algorithm**: X25519 (Curve25519)
- **Key Size**: 256 bits
- **Security Level**: 128 bits

### Performance
- **Overhead**: ~28 bytes per packet
- **CPU Impact**: 5-15% vs unencrypted
- **Latency**: <1ms additional

## Code Structure

```go
// Server setup
server, err := udpnet.NewSecureServer(":9999")
server.OnMessage(func(conn *protocol.Connection, data []byte) {
    // Handle decrypted messages
})

// Client setup
client, err := udpnet.NewSecureClient()
client.Connect("localhost:9999")
client.Send([]byte("Encrypted message"))
```

## Testing

You can verify encryption is working by:
1. Running packet capture (e.g., Wireshark)
2. Observing that packet payloads are encrypted
3. Checking connection info shows "Encrypted: ChaCha20-Poly1305"

## License

This example is provided under the same non-commercial license as the main project.
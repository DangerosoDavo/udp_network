> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](README.md) and [migration notes](MAINTENANCE.md) for supported contracts.

# UDP Network Package - Documentation Index

Complete documentation structure for developers using the UDP Network package.

## 📚 Documentation Overview

This package provides several documentation files tailored for different use cases and audiences.

---

## 🚀 Getting Started

### For First-Time Users

1. **[README.md](README.md)** - Start here!
   - Overview of features
   - Quick start guide
   - Architecture diagram
   - Performance benchmarks
   - Basic examples

2. **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)** - One-page cheat sheet
   - All APIs at a glance
   - Common patterns
   - Quick code snippets
   - Best for developers who want a quick lookup

---

## 📖 API Documentation

### Comprehensive Reference

**[API_REFERENCE.md](API_REFERENCE.md)** - Complete API documentation
- Full method signatures
- Parameter descriptions
- Return types and errors
- Usage examples for every method
- Covers:
  - Server API (unencrypted)
  - Client API (unencrypted)
  - SecureServer API (encrypted)
  - SecureClient API (encrypted)
  - Protocol package
  - Message types
  - Large transfer system
  - Advanced configuration

**Best for:** Developers integrating the package into their applications

---

### Original API Guide

**[API.md](API.md)** - Original API documentation
- High-level overview
- Basic examples
- Message types
- Connection management
- Encryption details
- Best practices

**Best for:** Understanding the overall design and capabilities

---

## 🔧 Usage & Setup

### Installation & Configuration

**[INSTALLATION.md](INSTALLATION.md)** - Detailed setup instructions
- Go installation
- Package installation
- Build instructions
- Environment setup
- Dependency management

**[README-USAGE.md](README-USAGE.md)** - Running examples
- Server startup
- Client usage
- Command-line options
- Example programs

---

## 🎯 Feature-Specific Guides

### Large String/File Transfer

**[LARGE_STRINGS.md](LARGE_STRINGS.md)** - Large data transfer guide
- Chunking mechanism
- Progress tracking
- Error handling
- Performance optimization
- Use cases (>250KB files)

### Security & Encryption

**[SECURITY_FIXES.md](SECURITY_FIXES.md)** - Recent security improvements
- Cryptographic enhancements
- Race condition fixes
- DoS vulnerability mitigation
- Resource management improvements
- Before/after comparisons

**Best for:** Security-conscious developers and auditors

---

## 🐛 Troubleshooting

**[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - Common issues and solutions
- Connection problems
- Performance issues
- Firewall configuration
- Platform-specific issues
- Debug techniques

---

## 📝 Quick Lookup Guide

### Use This Matrix to Find What You Need

| What You Want | Document to Read |
|---------------|------------------|
| Get started quickly | [README.md](README.md) |
| One-page API reference | [QUICK_REFERENCE.md](QUICK_REFERENCE.md) |
| Complete API details | [API_REFERENCE.md](API_REFERENCE.md) |
| Install the package | [INSTALLATION.md](INSTALLATION.md) |
| Run examples | [README-USAGE.md](README-USAGE.md) |
| Send large files | [LARGE_STRINGS.md](LARGE_STRINGS.md) |
| Understand security | [SECURITY_FIXES.md](SECURITY_FIXES.md) |
| Fix connection issues | [TROUBLESHOOTING.md](TROUBLESHOOTING.md) |
| Understand design | [API.md](API.md) |

---

## 🎓 Learning Path

### Beginner Track

1. Read [README.md](README.md) - Understand what the package does
2. Follow Quick Start in [README.md](README.md) - Run your first example
3. Review [QUICK_REFERENCE.md](QUICK_REFERENCE.md) - Learn basic patterns
4. Build simple echo server using [API_REFERENCE.md](API_REFERENCE.md)

### Intermediate Track

5. Review [API.md](API.md) - Understand architecture
6. Implement secure communication using SecureServer/SecureClient
7. Add large file transfer using [LARGE_STRINGS.md](LARGE_STRINGS.md)
8. Configure timeouts and connection limits

### Advanced Track

9. Study [SECURITY_FIXES.md](SECURITY_FIXES.md) - Understand security model
10. Review protocol package in [API_REFERENCE.md](API_REFERENCE.md)
11. Implement custom transport configurations
12. Optimize performance for your use case

---

## 📊 API Coverage by Document

### Server APIs

| Feature | Quick Ref | API Ref | API.md |
|---------|-----------|---------|--------|
| NewServer | ✅ | ✅ | ✅ |
| Start/Stop | ✅ | ✅ | ✅ |
| SetMessageHandler | ✅ | ✅ | ✅ |
| Broadcast | ✅ | ✅ | ✅ |
| SetMaxClients | ✅ | ✅ | ✅ |
| SetKeepAlive | ✅ | ✅ | ✅ |
| SetConnectionCallbacks | ✅ | ✅ | ✅ |

### Client APIs

| Feature | Quick Ref | API Ref | API.md |
|---------|-----------|---------|--------|
| NewClient | ✅ | ✅ | ✅ |
| Connect | ✅ | ✅ | ✅ |
| SendText | ✅ | ✅ | ✅ |
| SendJSON | ✅ | ✅ | ✅ |
| SendCommand | ✅ | ✅ | ✅ |
| SetMessageHandler | ✅ | ✅ | ✅ |

### Secure APIs

| Feature | Quick Ref | API Ref | API.md |
|---------|-----------|---------|--------|
| NewSecureServer | ✅ | ✅ | ✅ |
| NewSecureClient | ✅ | ✅ | ✅ |
| Encryption details | ❌ | ✅ | ✅ |
| Key exchange | ❌ | ✅ | ✅ |

### Large Transfer APIs

| Feature | Quick Ref | API Ref | Large Strings |
|---------|-----------|---------|---------------|
| SendLargeString | ✅ | ✅ | ✅ |
| EnableReceiver | ✅ | ✅ | ✅ |
| Progress tracking | ✅ | ✅ | ✅ |
| Chunking details | ❌ | ✅ | ✅ |

---

## 🔍 Common Scenarios

### "I want to build a chat application"

1. Read: [README.md](README.md) Quick Start
2. Use: `examples/chat/main.go` as template
3. Reference: [QUICK_REFERENCE.md](QUICK_REFERENCE.md) for Broadcast pattern
4. Add encryption: Review SecureServer in [API_REFERENCE.md](API_REFERENCE.md)

### "I need to transfer large files"

1. Read: [LARGE_STRINGS.md](LARGE_STRINGS.md)
2. Reference: Large Transfer API in [API_REFERENCE.md](API_REFERENCE.md)
3. Use: `SendLargeString()` with progress tracking
4. Test: `examples/large-string/main.go`

### "I'm building a game server"

1. Read: Game Server Configuration in [API_REFERENCE.md](API_REFERENCE.md)
2. Configure: Appropriate timeouts (10 min, 1 min keep-alive)
3. Implement: Connection callbacks for player join/leave
4. Reference: Game Server Example in [API_REFERENCE.md](API_REFERENCE.md)

### "Connection issues / debugging"

1. Read: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. Check: Firewall settings
3. Debug: Connection state using methods in [API_REFERENCE.md](API_REFERENCE.md)
4. Test: Basic connectivity with `tests/simple/main.go`

### "Security review / audit"

1. Read: [SECURITY_FIXES.md](SECURITY_FIXES.md) - Recent improvements
2. Review: Encryption API in [API_REFERENCE.md](API_REFERENCE.md)
3. Check: Encryption Details in [API.md](API.md)
4. Test: `tests/crypto-functional/main.go`

---

## 💡 Code Examples by Location

### In Documentation

- [README.md](README.md) - Basic server/client, secure communication
- [API_REFERENCE.md](API_REFERENCE.md) - Complete examples for every API
- [QUICK_REFERENCE.md](QUICK_REFERENCE.md) - Quick snippets
- [API.md](API.md) - Game server, secure chat examples
- [LARGE_STRINGS.md](LARGE_STRINGS.md) - Large transfer examples

### In Examples Directory

```
examples/
├── chat/              # Chat application
├── game-server/       # Game server template
├── large-string/      # Large file transfer
├── large-string-simple/ # Simplified large transfer
└── secure-chat/       # Encrypted chat
```

### In Tests Directory

```
tests/
├── simple/            # Basic connectivity test
├── performance/       # Performance benchmark
├── crypto-functional/ # Encryption test
├── large-string-test/ # Large transfer test
└── stress/           # Stress test
```

---

## 🏗️ Architecture References

### Protocol Stack

See [README.md](README.md) for visual diagram:
```
Application Layer (JSON Messages)
     ↓
Encryption Layer (ChaCha20-Poly1305) [Optional]
     ↓
Message Layer (Fragmentation/Reassembly)
     ↓
Transport Layer (Reliability/Flow Control)
     ↓
UDP Socket Layer
```

### Packet Format

Detailed in [README.md](README.md) and [API.md](API.md):
- Header: 16 bytes
- Payload: Up to 1400 bytes
- Magic number, type, flags, sequence numbers
- Fragment information

---

## 📦 Package Structure

```
udp_network/
├── *.go              # Main package (Server, Client)
├── protocol/         # Protocol implementation
│   ├── transport.go
│   ├── connection.go
│   ├── packet.go
│   ├── message.go
│   ├── crypto.go
│   ├── secure_transport.go
│   └── chunked_transfer.go
├── tokenizer/        # Text tokenization
├── cmd/             # Command-line tools
├── examples/        # Example applications
├── tests/           # Test programs
└── docs/            # Documentation (this file)
```

---

## 🔖 Quick Links

### Essential Reading
- **New Users**: [README.md](README.md) → [QUICK_REFERENCE.md](QUICK_REFERENCE.md)
- **API Integration**: [API_REFERENCE.md](API_REFERENCE.md)
- **Security Review**: [SECURITY_FIXES.md](SECURITY_FIXES.md)

### External Resources
- **GitHub**: https://github.com/DangerosoDavo/udp_network
- **Issues**: https://github.com/DangerosoDavo/udp_network/issues
- **Go Docs**: Run `godoc -http=:6060` and visit http://localhost:6060/pkg/

---

## 📝 Documentation Maintenance

### For Contributors

When adding new features:
1. Update [API_REFERENCE.md](API_REFERENCE.md) with complete method documentation
2. Add to [QUICK_REFERENCE.md](QUICK_REFERENCE.md) if it's commonly used
3. Update [README.md](README.md) if it's a major feature
4. Add examples to `examples/` directory
5. Update this index

### For Bug Fixes

1. Document fix in [SECURITY_FIXES.md](SECURITY_FIXES.md) if security-related
2. Add to [TROUBLESHOOTING.md](TROUBLESHOOTING.md) if it's a common issue
3. Update examples if behavior changes

---

## 📄 License

This package is licensed for **non-commercial use only**. See [LICENSE](LICENSE) for full terms.

Commercial licensing inquiries: See [README.md](README.md) contact section.

---

## 🤝 Community

- Report bugs: GitHub Issues
- Request features: GitHub Issues
- Security issues: See [SECURITY_FIXES.md](SECURITY_FIXES.md) for contact

---

*Documentation Index - Last Updated: 2025*

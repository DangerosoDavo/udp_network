> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](README.md) and [migration notes](MAINTENANCE.md) for supported contracts.

# Installing UDP Network Protocol as a Module

This guide shows how to use the UDP Network Protocol in your own Go projects.

## Option 1: Using GitHub (Recommended)

### 1. Push to GitHub

First, push this code to your GitHub repository:

```bash
cd /mnt/g/udp_network
git init
git add .
git commit -m "Initial commit"
git remote add origin https://github.com/yourusername/udp_network.git
git push -u origin main
```

### 2. Tag a Version

```bash
git tag v1.0.0
git push origin v1.0.0
```

### 3. Import in Your Project

In your Go project:

```bash
go get github.com/yourusername/udp_network@latest
```

## Option 2: Using Local Module (Development)

### 1. Update go.mod

Change the module path to match your intended import path:
```
module github.com/yourusername/udp_network
```

### 2. In Your Project

Use `replace` directive for local development:

```go
// go.mod in your project
module myproject

go 1.21

require github.com/yourusername/udp_network v0.0.0

replace github.com/yourusername/udp_network => /path/to/udp_network
```

## Option 3: Using go.work (Go 1.18+)

Create a workspace for multiple modules:

```bash
# In parent directory
go work init
go work use ./udp_network
go work use ./yourproject
```

## Usage Example

Once installed, use it in your project:

```go
package main

import (
    "log"
    "time"
    
    udpnet "github.com/yourusername/udp_network"
    "github.com/yourusername/udp_network/protocol"
)

func main() {
    // Create server
    server, err := udpnet.NewServer(":8080")
    if err != nil {
        log.Fatal(err)
    }
    
    // Configure for your needs
    server.SetKeepAlive(5*time.Minute, 30*time.Second)
    server.SetMaxClients(1000)
    
    // Set up handlers
    server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
        log.Printf("Received: %v", msg.Data)
    })
    
    server.Start()
    defer server.Stop()
    
    // Keep running
    select {}
}
```

## Module Structure

The module exports the following packages:

```
github.com/yourusername/udp_network          # Main server/client
github.com/yourusername/udp_network/protocol # Protocol types and transport
github.com/yourusername/udp_network/tokenizer # Text tokenization
```

## Best Practices

1. **Version Control**: Tag releases for stability
   ```bash
   git tag v1.0.1
   git push origin v1.0.1
   ```

2. **Semantic Versioning**: Use semantic versioning
   - v1.0.0 - Initial release
   - v1.0.1 - Bug fixes
   - v1.1.0 - New features
   - v2.0.0 - Breaking changes

3. **Documentation**: Keep your module documented
   ```go
   // Package udp_network provides a high-performance UDP networking protocol
   // with reliability features, JSON message support, and string tokenization.
   package udp_network
   ```

4. **Minimal Dependencies**: This module has zero external dependencies!

## Private Repositories

For private GitHub repos:

```bash
# Configure git to use SSH
git config --global url."git@github.com:".insteadOf "https://github.com/"

# Or use personal access token
export GOPRIVATE=github.com/yourusername
go get github.com/yourusername/udp_network@latest
```

## Vendoring

To vendor the dependency:

```bash
go mod vendor
```

This creates a `vendor/` directory with all dependencies.
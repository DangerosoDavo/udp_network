> Historical documentation: API, security, size and performance claims below may describe pre-v2 behavior. Follow the [current README](README.md) and [migration notes](MAINTENANCE.md) for supported contracts.

# UDP Network Protocol - Troubleshooting Guide

## Common Issues and Solutions

### Connection Issues

#### "Connection timeout" error
**Problem**: Client can't connect to server
**Solutions**:
- Verify server is running: `netstat -an | grep 8080`
- Check firewall settings
- Ensure correct address format: `localhost:8080` or `127.0.0.1:8080`
- Try increasing connection timeout in client

#### Connections dropping unexpectedly
**Problem**: Clients disconnect after being idle
**Solution**: Configure longer timeouts
```go
// For game servers
server.SetKeepAlive(5*time.Minute, 30*time.Second)
client.SetKeepAlive(5*time.Minute, 30*time.Second)
```

#### "Too many connections" errors
**Problem**: Server rejecting new connections
**Solution**: Increase connection limit
```go
server.SetMaxClients(10000) // or 0 for unlimited
```

### Message Issues

#### Messages not being received
**Problem**: Server logs show connection but no messages
**Solutions**:
1. Check message handler is set before Start():
```go
server.SetMessageHandler(handler)
server.Start() // Must be after SetMessageHandler
```

2. Verify message format:
```go
msg := &protocol.Message{
    Type: protocol.MessageTypeText,
    Data: "test",
}
```

3. Look at the correct terminal:
- Server terminal shows received messages
- Client terminal shows server responses

#### Large messages failing
**Problem**: Messages over 1400 bytes not delivered
**Solution**: Protocol automatically fragments, but check:
- Network MTU settings
- Firewall packet size limits
- Consider breaking into smaller messages

### Performance Issues

#### High CPU usage
**Problem**: Server using too much CPU
**Solutions**:
- Reduce keep-alive frequency
- Limit concurrent connections
- Check for message processing loops

#### Memory usage growing
**Problem**: Server memory keeps increasing
**Solutions**:
- Connections are now auto-cleaned
- Check for goroutine leaks in message handlers
- Monitor with: `go tool pprof`

#### Message delays
**Problem**: Messages taking time to arrive
**Solutions**:
- Protocol delivers out-of-order for speed
- Check network latency: `ping server`
- Reduce message size for faster transmission

### Development Issues

#### IDE showing false errors
**Problem**: "undefined method" errors in IDE
**Solutions**:
```bash
go mod tidy
go build ./...
# Restart language server
```

#### Build failures
**Problem**: Package not found errors
**Solution**: Ensure go.mod is correct:
```
module udp_network
go 1.21
```

### Debugging Tips

#### Enable verbose logging
Add debug prints to handlers:
```go
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    log.Printf("Received from %s: %+v", conn.Addr, msg)
    // ... handle message
})
```

#### Monitor connections
```go
go func() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        log.Printf("Active connections: %d", server.GetClientCount())
    }
}()
```

#### Test with simple echo
```go
// Simple echo server test
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    server.SendMessage(conn, msg) // Echo back
})
```

### Platform-Specific Issues

#### Linux
- Check ulimit for max file descriptors: `ulimit -n`
- Increase if needed: `ulimit -n 65536`

#### Windows
- Windows Firewall may block UDP
- Run as Administrator if port < 1024

#### Docker
- Use host network mode for UDP: `--network host`
- Or expose UDP ports: `-p 8080:8080/udp`

### Getting Help

1. Check server logs for error messages
2. Run the echo test (`tests/echo/main.go`)
3. Try the simple test (`tests/simple/main.go`)
4. Enable debug logging in your handlers

### Quick Fixes

**Connection drops after /stress command**
- Normal behavior - connection now properly cleaned up
- Single disconnect event fired for game cleanup

**"undefined: protocol.StateConnected" errors**
- Import the protocol package:
```go
import "udp_network/protocol"
```

**Heartbeat not working**
- Heartbeat starts automatically on Connect()
- Check client is still running
- Verify no network issues

**Broadcast not working**
- Ensure all clients are connected
- Check server has clients: `server.GetClientCount()`
- Verify message format is correct
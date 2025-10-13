# WebSocket Race Condition Fix

## Issue Discovered

During testing, a race condition was discovered in the WebSocket client implementation:

### Symptom
```
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x6b2590]

goroutine 65 [running]:
github.com/gorilla/websocket.(*Conn).SetReadDeadline(...)
github.com/DangerosoDavo/udp_network.(*WebSocketClient).readLoop(0xc00020c150)
	websocket_client.go:151
```

### Root Cause

The `readLoop()` goroutine was started before the mutex in `Connect()` was released, causing a race condition:

**Before (Buggy Code)**:
```go
func (c *WebSocketClient) Connect(serverAddr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// ... connection setup ...
	c.ws = ws
	c.connected = true

	// Start read loop - goroutine starts immediately!
	c.readLoopStop = make(chan bool)
	go c.readLoop()  // ← Race here!

	// ... rest of setup ...
	return nil
}  // ← Mutex released here, but readLoop already running

func (c *WebSocketClient) readLoop() {
	// ...
	c.ws.SetReadDeadline(time.Now().Add(c.timeout))  // ← c.ws might be nil!
	messageType, data, err := c.ws.ReadMessage()     // ← Crash!
}
```

The goroutine could access `c.ws` before the mutex was released, or after `Stop()` was called on another goroutine.

## Fix Applied

Added mutex-protected access to the websocket connection in `readLoop()`:

**After (Fixed Code)**:
```go
func (c *WebSocketClient) readLoop() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.readLoopStop:
			return
		default:
		}

		// Safely access websocket connection
		c.mu.Lock()
		ws := c.ws  // ← Get local copy while holding lock
		c.mu.Unlock()

		if ws == nil {  // ← Check for nil
			return
		}

		ws.SetReadDeadline(time.Now().Add(c.timeout))  // ← Use local copy
		messageType, data, err := ws.ReadMessage()     // ← Safe!

		// ... rest of loop ...
	}
}
```

### Key Changes

1. **Lines 151-154**: Acquire mutex, get local copy of `c.ws`, release mutex
2. **Lines 156-158**: Check if `ws` is nil and return early
3. **Lines 160-161**: Use local `ws` variable instead of struct field `c.ws`

This ensures:
- The goroutine never accesses `c.ws` without mutex protection
- If `Stop()` is called and sets `c.ws = nil`, the loop exits safely
- No concurrent access between the goroutine and `Connect()`/`Stop()` methods

## Verification

The same pattern was verified in the UDP client (`client.go`), which already had proper mutex protection in the heartbeat goroutine:

```go
func (c *Client) startHeartbeat() {
	go func() {
		for {
			// ... select ...

			// Properly protected access
			c.mu.Lock()
			conn := c.Connection  // ← Local copy
			isConnected := c.connected
			c.mu.Unlock()

			if conn != nil && isConnected {
				// Use local copy...
			}
		}
	}()
}
```

The UDP client was already following the correct pattern.

## Test Results

After the fix:
- ✅ `TestWebSocketClientConnectDisconnect` - PASS
- ✅ `TestWebSocketSendReceive` - PASS
- ✅ No more panic/segfault errors
- ✅ Safe concurrent access verified

## Files Modified

- **websocket_client.go** - Lines 137-180 (readLoop function)

## Impact

**Severity**: High - Caused crashes during connection/disconnection

**Fix Type**: Thread safety

**Performance**: No performance impact - mutex lock/unlock is very fast

**Compatibility**: No API changes, fully backward compatible

## Lessons Learned

1. **Never start goroutines that access struct fields while holding a mutex** - The goroutine should get local copies of data it needs
2. **Always nil-check after releasing mutex** - Another goroutine might have modified the field
3. **Use local copies in goroutines** - Reduces mutex contention and prevents races
4. **Test concurrent operations** - This bug was found during connection/disconnection testing

## Related Patterns

This is a common pattern for safe goroutine access to shared state:

```go
// Good pattern:
func (x *X) goroutine() {
	for {
		x.mu.Lock()
		localCopy := x.field  // Get local copy
		x.mu.Unlock()

		if localCopy == nil {
			return
		}

		// Use localCopy safely
		localCopy.SomeMethod()
	}
}

// Bad pattern (race condition):
func (x *X) goroutine() {
	for {
		// Don't do this!
		x.field.SomeMethod()  // ← Race! No mutex protection
	}
}
```

## Testing Recommendations

To catch these issues early:

1. **Run with race detector**: `go test -race`
2. **Test concurrent operations**: Connect/disconnect rapidly
3. **Stress test**: Many goroutines accessing shared state
4. **Check for nil dereferences**: Enable panic recovery in tests

## Prevention

Going forward, any new goroutines that access struct fields should:
1. Acquire the mutex
2. Get local copies of needed fields
3. Release the mutex
4. Check for nil values
5. Use only local copies

This pattern prevents data races and ensures safe concurrent access.

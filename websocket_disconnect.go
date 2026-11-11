package udp_network

import "fmt"

const (
	WebSocketOpReadNext       = "read_next"
	WebSocketOpReadFrame      = "read_frame"
	WebSocketOpHeartbeat      = "heartbeat"
	WebSocketOpWrite          = "write"
	WebSocketOpContextCancel  = "context_cancel"
	WebSocketOpManualStop     = "manual_stop"
	WebSocketOpProtocolDecode = "protocol_decode"
)

// WebSocketDisconnectError retains the first terminal cause for a client
// session. Err is the original transport/context/codec error. Consumers may
// classify it with errors.Is/As without logging potentially sensitive error text.
type WebSocketDisconnectError struct {
	Operation string
	Err       error
}

func (e *WebSocketDisconnectError) Error() string {
	return fmt.Sprintf("websocket %s: %v", e.Operation, e.Err)
}
func (e *WebSocketDisconnectError) Unwrap() error { return e.Err }

// LastDisconnectError is nil until a session terminates. The cause is recorded
// before IsConnected becomes false and survives Stop. Starting a new Connect
// attempt clears it; a previous session cannot overwrite that new generation.
// This returns a wrapper copy. The underlying original error is preserved.
func (c *WebSocketClient) LastDisconnectError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastDisconnect == nil {
		return nil
	}
	copy := *c.lastDisconnect
	return &copy
}

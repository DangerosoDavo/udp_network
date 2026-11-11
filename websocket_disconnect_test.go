package udp_network

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func waitWebSocketDisconnect(t *testing.T, c *WebSocketClient) *WebSocketDisconnectError {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for c.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.IsConnected() {
		t.Fatal("session remained connected")
	}
	var cause *WebSocketDisconnectError
	if !errors.As(c.LastDisconnectError(), &cause) || cause.Err == nil {
		t.Fatal("disconnected session has no terminal cause")
	}
	return cause
}

func TestWebSocketDisconnectRemoteCloseSurvivesStopAndReconnect(t *testing.T) {
	address := rawTestServer(t, func(ws *websocket.Conn) {
		_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "remote reason"), time.Now().Add(time.Second))
	})
	c, _ := NewWebSocketClient(nil)
	t.Cleanup(c.Stop)
	if c.LastDisconnectError() != nil {
		t.Fatal("new client has error")
	}
	if err := c.Connect(address); err != nil {
		t.Fatal(err)
	}
	cause := waitWebSocketDisconnect(t, c)
	var remote *websocket.CloseError
	if cause.Operation != WebSocketOpReadNext || !errors.As(cause, &remote) || remote.Code != websocket.CloseGoingAway || remote.Text != "remote reason" {
		t.Fatalf("remote error lost: %T/%s", cause.Err, cause.Operation)
	}
	c.Stop()
	if kept := waitWebSocketDisconnect(t, c); kept.Operation != WebSocketOpReadNext {
		t.Fatal("manual Stop overwrote transport cause")
	}
	cause.Operation = "caller edit"
	if got := c.LastDisconnectError().(*WebSocketDisconnectError); got.Operation != WebSocketOpReadNext {
		t.Fatal("caller mutated stored wrapper")
	}
	stop := make(chan struct{})
	defer close(stop)
	quiet := rawTestServer(t, func(ws *websocket.Conn) { <-stop })
	if err := c.Connect(quiet); err != nil {
		t.Fatal(err)
	}
	if c.LastDisconnectError() != nil {
		t.Fatal("new connection inherited old failure")
	}
	c.Stop()
	if got := waitWebSocketDisconnect(t, c); got.Operation != WebSocketOpManualStop || !errors.Is(got, net.ErrClosed) {
		t.Fatal("manual cause missing")
	}
}

func TestWebSocketDisconnectReadTimeoutAndFrameFailure(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	quiet := rawTestServer(t, func(ws *websocket.Conn) { <-stop })
	c, _ := NewWebSocketClient(&ClientOptions{Timeout: 80 * time.Millisecond, KeepAlive: 20 * time.Millisecond})
	t.Cleanup(c.Stop)
	if err := c.Connect(quiet); err != nil {
		t.Fatal(err)
	}
	cause := waitWebSocketDisconnect(t, c)
	var timeout net.Error
	if cause.Operation != WebSocketOpReadNext || !errors.As(cause, &timeout) || !timeout.Timeout() {
		t.Fatalf("timeout lost: %s", cause.Operation)
	}
	broken := rawTestServer(t, func(ws *websocket.Conn) {
		w, err := ws.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return
		}
		_, _ = w.Write(bytes.Repeat([]byte{'x'}, 8192))
		_ = ws.UnderlyingConn().Close()
	})
	if err := c.Connect(broken); err != nil {
		t.Fatal(err)
	}
	cause = waitWebSocketDisconnect(t, c)
	if cause.Operation != WebSocketOpReadFrame {
		t.Fatalf("frame read failure stage lost: %s", cause.Operation)
	}
}

func TestWebSocketDisconnectCodecAndHeartbeat(t *testing.T) {
	broken := rawTestServer(t, func(ws *websocket.Conn) {
		_ = ws.WriteMessage(websocket.BinaryMessage, []byte("invalid JSON"))
		_, _, _ = ws.ReadMessage()
	})
	c, _ := NewWebSocketClient(nil)
	t.Cleanup(c.Stop)
	if err := c.Connect(broken); err != nil {
		t.Fatal(err)
	}
	if got := waitWebSocketDisconnect(t, c); got.Operation != WebSocketOpProtocolDecode {
		t.Fatal("codec failure stage lost")
	}
	stop := make(chan struct{})
	defer close(stop)
	quiet := rawTestServer(t, func(ws *websocket.Conn) { <-stop })
	ws, _, err := websocket.DefaultDialer.Dial(quiet, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.Close()
	isolated := &WebSocketClient{diagnosticEpoch: 1}
	session := &webSocketClientSession{owner: isolated, epoch: 1, done: make(chan struct{}), ws: ws}
	isolated.session = session
	go isolated.heartbeat(session, time.Millisecond)
	if got := waitWebSocketDisconnect(t, isolated); got.Operation != WebSocketOpHeartbeat || !errors.Is(got, net.ErrClosed) {
		t.Fatal("heartbeat failure stage/cause lost")
	}
}

func TestWebSocketWriteContextCauseFirstWinsAndOldGenerationIgnored(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		c := &WebSocketClient{diagnosticEpoch: 1}
		session := &webSocketClientSession{owner: c, epoch: 1, done: make(chan struct{})}
		c.session = session
		ctx, cancel := context.WithCancel(context.Background())
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}
		stop := session.watchWriteContext(ctx)
		cancel()
		defer stop()
		select {
		case <-session.done:
		case <-time.After(time.Second):
			t.Fatal("write cancellation did not close session")
		}
		got := waitWebSocketDisconnect(t, c)
		want := error(context.Canceled)
		if deadline {
			want = context.DeadlineExceeded
		}
		if got.Operation != WebSocketOpContextCancel || !errors.Is(got, want) {
			t.Fatal("write context original cause lost")
		}
		session.closeWith(WebSocketOpReadNext, net.ErrClosed)
		if got := c.LastDisconnectError().(*WebSocketDisconnectError); got.Operation != WebSocketOpContextCancel {
			t.Fatal("later reader close overwrote first cause")
		}
	}
	c := &WebSocketClient{diagnosticEpoch: 2}
	old := &webSocketClientSession{owner: c, epoch: 1, done: make(chan struct{})}
	old.closeWith(WebSocketOpWrite, net.ErrClosed)
	if c.LastDisconnectError() != nil {
		t.Fatal("old session overwrote new generation")
	}
	current := &webSocketClientSession{owner: c, epoch: 2, done: make(chan struct{})}
	c.session = current
	current.closeWith(WebSocketOpWrite, net.ErrClosed)
	if got := waitWebSocketDisconnect(t, c); got.Operation != WebSocketOpWrite || !errors.Is(got, net.ErrClosed) {
		t.Fatal("write cause not wrapped")
	}
}

func TestWebSocketDisconnectGetterConcurrentWithStop(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	address := rawTestServer(t, func(ws *websocket.Conn) { <-stop })
	c, _ := NewWebSocketClient(nil)
	if err := c.Connect(address); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if !c.IsConnected() && c.LastDisconnectError() == nil {
					t.Error("terminal cause published after disconnected status")
				}
				_ = c.LastDisconnectError()
			}
		}()
	}
	c.Stop()
	wg.Wait()
	if got := waitWebSocketDisconnect(t, c); got.Operation != WebSocketOpManualStop {
		t.Fatal("manual stop did not win first")
	}
}

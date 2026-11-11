package udp_network

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
	"github.com/gorilla/websocket"
)

func rawTestServer(t *testing.T, serve func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		serve(c)
	}))
	t.Cleanup(s.Close)
	return "ws" + strings.TrimPrefix(s.URL, "http")
}

func TestWebSocketRawDeliveryOwnsFramesAndRestoresDecodedHandler(t *testing.T) {
	advance := make(chan struct{})
	defer close(advance)
	address := rawTestServer(t, func(c *websocket.Conn) {
		_ = c.WriteMessage(websocket.BinaryMessage, []byte("not-json"))
		<-advance
		_ = c.WriteMessage(websocket.TextMessage, []byte("second-raw-frame"))
		<-advance
		_ = c.WriteMessage(websocket.BinaryMessage, []byte(`{"type":"text","data":"decoded"}`))
		_, _, _ = c.ReadMessage()
	})
	c, err := NewWebSocketClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	raw := make(chan []byte, 2)
	decoded := make(chan *protocol.Message, 1)
	c.SetMessageHandler(func(m *protocol.Message) { decoded <- m })
	c.SetRawMessageHandler(func(p []byte) { raw <- p })
	if err := c.Connect(address); err != nil {
		t.Fatal(err)
	}
	get := func() []byte {
		t.Helper()
		select {
		case p := <-raw:
			return p
		case <-time.After(time.Second):
			t.Fatal("missing raw frame")
			return nil
		}
	}
	first := get()
	advance <- struct{}{}
	second := get()
	if !bytes.Equal(first, []byte("not-json")) || !bytes.Equal(second, []byte("second-raw-frame")) {
		t.Fatal("raw frame changed or decoded")
	}
	select {
	case <-decoded:
		t.Fatal("raw delivery also invoked decoded handler")
	default:
	}
	c.SetRawMessageHandler(nil)
	advance <- struct{}{}
	select {
	case m := <-decoded:
		if m.Type != protocol.MessageTypeText || m.Data != "decoded" {
			t.Fatal("decoded handler did not resume")
		}
	case <-time.After(time.Second):
		t.Fatal("missing decoded delivery")
	}
}

func TestWebSocketRawReadLimitRemainsEnforced(t *testing.T) {
	address := rawTestServer(t, func(c *websocket.Conn) {
		_ = c.WriteMessage(websocket.BinaryMessage, bytes.Repeat([]byte("x"), 65))
		_, _, _ = c.ReadMessage()
	})
	c, err := NewWebSocketClient(&ClientOptions{MaxMessageBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	called := make(chan struct{}, 1)
	c.SetRawMessageHandler(func([]byte) { called <- struct{}{} })
	if err := c.Connect(address); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	for c.IsConnected() && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if c.IsConnected() {
		t.Fatal("oversized frame did not close connection")
	}
	select {
	case <-called:
		t.Fatal("oversized raw frame delivered")
	default:
	}
}

func TestWebSocketBorrowedRawMutationCopyAndHandlerSwitch(t *testing.T) {
	advance := make(chan struct{})
	defer close(advance)
	address := rawTestServer(t, func(c *websocket.Conn) {
		for _, payload := range []string{"first", "second", "owned"} {
			_ = c.WriteMessage(websocket.BinaryMessage, []byte(payload))
			<-advance
		}
	})
	c, err := NewWebSocketClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	received := make(chan []byte, 3)
	c.SetRawMessageHandler(func(p []byte) { received <- p })
	c.SetBorrowedRawMessageHandler(func(p []byte) {
		received <- append([]byte(nil), p...) // Retention requires a copy.
		for i := range p {
			p[i] = '!'
		} // Must not corrupt a subsequent frame.
	})
	if err := c.Connect(address); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"first", "second", "owned"} {
		select {
		case p := <-received:
			if string(p) != want {
				t.Fatal("borrowed mutation corrupted retained copy or later frame")
			}
		case <-time.After(time.Second):
			t.Fatal("missing borrowed/owned delivery")
		}
		if i == 1 {
			c.SetBorrowedRawMessageHandler(nil)
		}
		advance <- struct{}{}
	}
}

func TestWebSocketRawCallbackDoesNotExtendReadDeadline(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	address := rawTestServer(t, func(c *websocket.Conn) {
		_ = c.WriteMessage(websocket.BinaryMessage, []byte("raw"))
		<-stop // Deliberately neither read pings nor send further data/pongs.
	})
	c, err := NewWebSocketClient(&ClientOptions{Timeout: 80 * time.Millisecond, KeepAlive: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	returned := make(chan struct{})
	c.SetRawMessageHandler(func([]byte) { time.Sleep(150 * time.Millisecond); close(returned) })
	if err := c.Connect(address); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("callback did not return")
	}
	until := time.Now().Add(time.Second)
	for c.IsConnected() && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if c.IsConnected() {
		t.Fatal("callback processing incorrectly refreshed read deadline")
	}
}

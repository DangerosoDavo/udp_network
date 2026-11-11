package udp_network

import (
	"bytes"
	"context"
	"errors"
	"github.com/DangerosoDavo/udp_network/protocol"
	"testing"
	"time"
)

func TestSecureWrappersRequireAdmissionSecrets(t *testing.T) {
	if _, err := NewSecureClient(); !errors.Is(err, protocol.ErrPSKRequired) {
		t.Fatal(err)
	}
	if _, err := NewSecureServer("127.0.0.1:0"); !errors.Is(err, protocol.ErrPSKRequired) {
		t.Fatal(err)
	}
}
func TestSecureHighLevelMessagesAndEarlyCallbacks(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	server, err := NewSecureServerWithPSK("127.0.0.1:0", key)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	received := make(chan string, 4)
	server.OnMessage(func(c *protocol.Connection, data []byte) {
		msg, err := protocol.NewMessageCodec().Decode(data)
		if err != nil {
			received <- "decode failed"
			return
		}
		text, _ := msg.Data.(string)
		received <- text
		if err := server.SendTo(c, []byte("secure response")); err != nil {
			received <- err.Error()
		}
	})
	server.Start()
	client, err := NewSecureClientWithPSK(key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	replies := make(chan string, 4)
	client.OnMessage(func(data []byte) { replies <- string(data) })
	if err = client.Connect(server.GetTransport().LocalAddr().String()); err != nil {
		t.Fatal(err)
	}
	if err = client.SendText("spaceship control"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-received:
		if got != "spaceship control" {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendText failed")
	}
	select {
	case got := <-replies:
		if got != "secure response" {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnMessage registered before Connect was lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = client.SendMessageContext(ctx, &protocol.Message{Type: protocol.MessageTypeText, Data: "cancelled"}); !errors.Is(err, context.Canceled) {
		t.Fatal("context not honored", err)
	}
}

package udp_network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
	"github.com/gorilla/websocket"
)

func startWSTest(t *testing.T, opts *ServerOptions) *WebSocketServer {
	t.Helper()
	server, err := NewWebSocketServer("127.0.0.1:0", opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.StartWithError(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	return server
}

func TestUnknownTransportFailsExplicitly(t *testing.T) {
	if _, err := NewUnifiedClient(&ClientOptions{Transport: "misspelled"}); err == nil {
		t.Fatal("unknown client transport silently fell back to plaintext UDP")
	}
	if _, err := NewUnifiedServer("127.0.0.1:0", &ServerOptions{Transport: "misspelled"}); err == nil {
		t.Fatal("unknown server transport silently fell back to plaintext UDP")
	}
}

func TestWebSocketSameIPAndSameObjectReconnect(t *testing.T) {
	server := startWSTest(t, nil)
	var clients []*WebSocketClient
	for i := 0; i < 4; i++ {
		client, err := NewWebSocketClient(nil)
		if err != nil {
			t.Fatal(err)
		}
		client.SetMessageHandler(func(*protocol.Message) {})
		if err := client.Connect("ws://" + server.Addr().String()); err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
		t.Cleanup(client.Stop)
	}
	if got := server.GetClientCount(); got != 4 {
		t.Fatalf("same-IP clients collide: %d", got)
	}
	client := clients[0]
	for i := 0; i < 4; i++ {
		client.Stop()
		if err := client.Connect("ws://" + server.Addr().String()); err != nil {
			t.Fatal(err)
		}
		if err := client.SendText("after reconnect"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWebSocketConcurrentCapacity(t *testing.T) {
	server := startWSTest(t, &ServerOptions{MaxClients: 3})
	var workers sync.WaitGroup
	var mu sync.Mutex
	var clients []*WebSocketClient
	for i := 0; i < 15; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			client, _ := NewWebSocketClient(nil)
			if client.Connect("ws://"+server.Addr().String()) == nil {
				mu.Lock()
				clients = append(clients, client)
				mu.Unlock()
			} else {
				client.Stop()
			}
		}()
	}
	workers.Wait()
	defer func() {
		for _, client := range clients {
			client.Stop()
		}
	}()
	if server.GetClientCount() != 3 || len(clients) != 3 {
		t.Fatalf("capacity not enforced: server=%d admitted=%d", server.GetClientCount(), len(clients))
	}
}

func TestWebSocketOriginAndReadBounds(t *testing.T) {
	server := startWSTest(t, &ServerOptions{MaxMessageBytes: 128})
	header := http.Header{"Origin": []string{"https://untrusted.invalid"}}
	if ws, _, err := websocket.DefaultDialer.Dial("ws://"+server.Addr().String(), header); err == nil {
		ws.Close()
		t.Fatal("cross-origin upgrade accepted")
	}
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+server.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := ws.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, []byte(strings.Repeat("x", 1024))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("oversized peer was not disconnected")
	}
}

func TestWebSocketTLSAndCanceledWrite(t *testing.T) {
	certificateServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificates := certificateServer.TLS.Certificates
	roots := x509.NewCertPool()
	roots.AddCert(certificateServer.Certificate())
	certificateServer.Close()
	server := startWSTest(t, &ServerOptions{TLSConfig: &tls.Config{Certificates: certificates, MinVersion: tls.VersionTLS12}})
	client, err := NewWebSocketClient(&ClientOptions{TLSConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Stop()
	if err := client.Connect("wss://" + server.Addr().String()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.SendMessageContext(ctx, &protocol.Message{Type: protocol.MessageTypeText, Data: "canceled"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled send: %v", err)
	}
	if err := client.SendText("encrypted websocket"); err != nil {
		t.Fatal(err)
	}
}

func TestLargeStringJSONRoundTripAndPeerScope(t *testing.T) {
	receiver := NewLargeStringReceiver()
	defer receiver.Close()
	completed := make(chan string, 2)
	receiver.SetDefaultHandler(func(text string, _ map[string]interface{}) { completed <- text })
	codec := protocol.NewMessageCodec()
	first := strings.Repeat("A", protocol.MaxChunkSize+100)
	second := strings.Repeat("B", protocol.MaxChunkSize+100)
	a, err := protocol.ChunkString(first, "same-id", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := protocol.ChunkString(second, "same-id", nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := range a {
		for peer, chunks := range map[string][]*protocol.ChunkedMessage{"peerA": a, "peerB": b} {
			encoded, err := codec.Encode(&protocol.Message{Type: protocol.MessageTypeChunked, Data: chunks[index]})
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := codec.Decode(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if err := receiver.ProcessChunkedMessageFrom(peer, decoded); err != nil {
				t.Fatal(err)
			}
		}
	}
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case text := <-completed:
			seen[text] = true
		case <-time.After(time.Second):
			t.Fatal("missing completed transfer")
		}
	}
	if !seen[first] || !seen[second] {
		t.Fatal("peer transfers mixed")
	}
}

func TestPlainWrappersAndChunkTransfer(t *testing.T) {
	server, err := NewServer("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	receiver := NewLargeStringReceiver()
	defer receiver.Close()
	completed := make(chan string, 1)
	receiver.SetDefaultHandler(func(text string, _ map[string]interface{}) { completed <- text })
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeChunked {
			if err := receiver.ProcessChunkedMessageFrom(conn.Addr.String(), msg); err != nil {
				t.Error(err)
			}
			return
		}
		if err := server.SendMessage(conn, msg); err != nil {
			t.Error(err)
		}
	})
	server.Start()
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Stop()
	echo := make(chan string, 1)
	client.SetMessageHandler(func(msg *protocol.Message) {
		if text, ok := msg.Data.(string); ok {
			echo <- text
		}
	})
	// Connect must start its own transport and install the callback safely.
	if err := client.Connect(server.Transport().LocalAddr().String()); err != nil {
		t.Fatal(err)
	}
	if err := client.SendText("ordinary UDP message"); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-echo:
		if text != "ordinary UDP message" {
			t.Fatal("bad echo")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing echo")
	}
	text := strings.Repeat("space🚀", 12000)
	if err := client.SendLargeString(text, &LargeStringOptions{TransferID: "wrapper-transfer"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-completed:
		if got != text {
			t.Fatal("chunked transfer corrupted")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("missing chunked transfer")
	}
}

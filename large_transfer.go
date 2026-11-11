package udp_network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

// LargeStringOptions configures application chunking. Successful progress means
// locally queued chunks; it is not a remote completion acknowledgement.
type LargeStringOptions struct {
	TransferID string
	Metadata   map[string]interface{}
	OnProgress func(sent, total int)
	// Deprecated: retransmission belongs to the transport. Retained for source
	// compatibility; does not add application retries.
	RetryAttempts int
	// Maximum local queue time per chunk (default 5 seconds).
	ChunkTimeout time.Duration
}

func sendLargeString(text string, options *LargeStringOptions, send func(context.Context, *protocol.Message) error) error {
	var opts LargeStringOptions
	if options != nil {
		opts = *options
	}
	if opts.ChunkTimeout < 0 {
		return fmt.Errorf("negative chunk timeout")
	}
	if opts.ChunkTimeout == 0 {
		opts.ChunkTimeout = 5 * time.Second
	}
	chunks, err := protocol.ChunkString(text, opts.TransferID, opts.Metadata)
	if err != nil {
		return err
	}
	for i, chunk := range chunks {
		msg := &protocol.Message{Type: protocol.MessageTypeChunked, ID: fmt.Sprintf("%s_%d", chunk.Metadata.TransferID, i), Timestamp: time.Now().Unix(), Data: chunk}
		ctx, cancel := context.WithTimeout(context.Background(), opts.ChunkTimeout)
		err := send(ctx, msg)
		cancel()
		if err != nil {
			return fmt.Errorf("queue chunk %d: %w", i, err)
		}
		if opts.OnProgress != nil {
			opts.OnProgress(i+1, len(chunks))
		}
	}
	return nil
}

func (c *Client) SendLargeString(text string, options *LargeStringOptions) error {
	c.mu.Lock()
	conn := c.Connection
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	return sendLargeString(text, options, func(ctx context.Context, msg *protocol.Message) error {
		return c.handler.SendMessageContext(ctx, conn, msg)
	})
}

func (c *SecureClient) SendLargeString(text string, options *LargeStringOptions) error {
	return sendLargeString(text, options, c.SendMessageContext)
}

type LargeStringReceiver struct {
	reassembler *protocol.ChunkReassembler
	handlers    map[string]func(string, map[string]interface{})
	mu          sync.RWMutex
}

func NewLargeStringReceiver() *LargeStringReceiver {
	receiver := &LargeStringReceiver{reassembler: protocol.NewChunkReassembler(), handlers: make(map[string]func(string, map[string]interface{}))}
	receiver.reassembler.SetCompleteCallback(func(id string, data []byte, metadata map[string]interface{}) {
		receiver.handleComplete(id, string(data), metadata)
	})
	return receiver
}
func (r *LargeStringReceiver) SetHandler(id string, handler func(string, map[string]interface{})) {
	r.mu.Lock()
	r.handlers[id] = handler
	r.mu.Unlock()
}
func (r *LargeStringReceiver) SetDefaultHandler(handler func(string, map[string]interface{})) {
	r.SetHandler("*", handler)
}

func decodeChunk(msg *protocol.Message) (*protocol.ChunkedMessage, error) {
	if msg == nil || msg.Type != protocol.MessageTypeChunked {
		return nil, fmt.Errorf("expected chunked message")
	}
	if chunk, ok := msg.Data.(*protocol.ChunkedMessage); ok && chunk != nil {
		copy := *chunk
		return &copy, nil
	}
	if chunk, ok := msg.Data.(protocol.ChunkedMessage); ok {
		return &chunk, nil
	}
	// JSON interface values have map shape. Decode through the real schema,
	// preserving metadata and numeric range validation performed by encoding/json.
	data, err := json.Marshal(msg.Data)
	if err != nil {
		return nil, err
	}
	var chunk protocol.ChunkedMessage
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, fmt.Errorf("decode chunk: %w", err)
	}
	return &chunk, nil
}

func (r *LargeStringReceiver) ProcessChunkedMessage(msg *protocol.Message) error {
	return r.ProcessChunkedMessageFrom("", msg)
}

// ProcessChunkedMessageFrom isolates reassembly by authenticated caller-provided
// peer/session scope. The scope must not be taken from untrusted message data.
func (r *LargeStringReceiver) ProcessChunkedMessageFrom(scope string, msg *protocol.Message) error {
	chunk, err := decodeChunk(msg)
	if err != nil {
		return err
	}
	if strings.ContainsRune(scope, '\x00') || strings.ContainsRune(chunk.Metadata.TransferID, '\x00') {
		return fmt.Errorf("invalid transfer scope")
	}
	if scope != "" {
		chunk.Metadata.TransferID = scope + "\x00" + chunk.Metadata.TransferID
	}
	return r.reassembler.AddChunk(chunk)
}
func (r *LargeStringReceiver) GetTransferStatus(id string) (int, int, bool) {
	return r.reassembler.GetTransferStatus(id)
}
func (r *LargeStringReceiver) GetStats() []protocol.ChunkedTransferStats {
	return r.reassembler.GetStats()
}

func (r *LargeStringReceiver) Close() { r.reassembler.Close() }
func (r *LargeStringReceiver) handleComplete(id, data string, metadata map[string]interface{}) {
	if index := strings.IndexByte(id, 0); index >= 0 {
		id = id[index+1:]
	}
	r.mu.Lock()
	handler := r.handlers[id]
	if handler == nil {
		handler = r.handlers["*"]
	}
	delete(r.handlers, id)
	r.mu.Unlock()
	if handler != nil {
		handler(data, metadata)
	}
}
func EnableLargeStringReceiver(s *Server) *LargeStringReceiver {
	receiver := NewLargeStringReceiver()
	s.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeChunked {
			_ = receiver.ProcessChunkedMessageFrom(conn.Addr.String(), msg)
		} else {
			s.handleMessage(conn, msg)
		}
	})
	return receiver
}
func EnableLargeStringReceiverSecure(s *SecureServer) *LargeStringReceiver {
	receiver := NewLargeStringReceiver()
	codec := protocol.NewMessageCodec()
	s.OnMessage(func(conn *protocol.Connection, data []byte) {
		msg, err := codec.Decode(data)
		if err == nil && msg.Type == protocol.MessageTypeChunked {
			_ = receiver.ProcessChunkedMessageFrom(conn.Addr.String(), msg)
		}
	})
	return receiver
}

package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DangerosoDavo/udp_network/tokenizer"
	"sync"
	"sync/atomic"
	"time"
)

var globalFragmentID uint32

type MessageType string

const (
	MessageTypeText    MessageType = "text"
	MessageTypeJSON    MessageType = "json"
	MessageTypeCommand MessageType = "command"
	MessageTypeBinary  MessageType = "binary"
	MessageTypeChunked MessageType = "chunked"
)

type Message struct {
	Type      MessageType            `json:"type"`
	ID        string                 `json:"id,omitempty"`
	Timestamp int64                  `json:"ts"`
	Data      interface{}            `json:"data"`
	Metadata  map[string]interface{} `json:"meta,omitempty"`
}
type MessageCodec struct {
	tokenizer *tokenizer.Tokenizer
}

func NewMessageCodec() *MessageCodec {
	return &MessageCodec{tokenizer: tokenizer.NewTokenizer()}
}

type tokenEnvelope struct {
	Type      MessageType            `json:"t"`
	Data      []byte                 `json:"d"`
	Tokenized bool                   `json:"tk"`
	ID        string                 `json:"id,omitempty"`
	Timestamp int64                  `json:"ts,omitempty"`
	Metadata  map[string]interface{} `json:"meta,omitempty"`
}

func (mc *MessageCodec) Encode(msg *Message) ([]byte, error) {
	if msg == nil || msg.Type == "" {
		return nil, errors.New("message type required")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if msg.Type == MessageTypeText {
		if text, ok := msg.Data.(string); ok {
			if len(text) > MaxMessageSize {
				return nil, errors.New("text message too large")
			}
			tokens := mc.tokenizer.EncodeTokensToBinary(mc.tokenizer.Encode(text))
			compressed, err := json.Marshal(tokenEnvelope{msg.Type, tokens, true, msg.ID, msg.Timestamp, msg.Metadata})
			if err != nil {
				return nil, err
			}
			if len(compressed) < len(data) {
				data = compressed
			}
		}
	}
	if len(data) > MaxMessageSize {
		return nil, errors.New("encoded message too large")
	}
	return data, nil
}

// Decode uses RawMessage for the payload so normal messages avoid the previous
// full map parse followed by a second full parse. JSON numeric Data values retain
// encoding/json's float64 convention; use typed schemas/string IDs for uint64 IDs.
func (mc *MessageCodec) Decode(data []byte) (*Message, error) {
	if len(data) > MaxMessageSize {
		return nil, errors.New("encoded message too large")
	}
	var wire struct {
		Type      MessageType            `json:"type"`
		ID        string                 `json:"id"`
		Timestamp int64                  `json:"ts"`
		Data      json.RawMessage        `json:"data"`
		Metadata  map[string]interface{} `json:"meta"`
		Tokenized bool                   `json:"tk"`
		TokenType MessageType            `json:"t"`
		TokenData []byte                 `json:"d"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	msg := &Message{Type: wire.Type, ID: wire.ID, Timestamp: wire.Timestamp, Metadata: wire.Metadata}
	if wire.Tokenized {
		if wire.Type != "" || wire.TokenType != MessageTypeText || wire.TokenData == nil || len(wire.TokenData)%2 != 0 {
			return nil, errors.New("invalid tokenized envelope")
		}
		tokens := mc.tokenizer.DecodeTokensFromBinary(wire.TokenData)
		text, err := mc.tokenizer.DecodeChecked(tokens)
		if err != nil {
			return nil, fmt.Errorf("invalid tokenized text: %w", err)
		}
		if len(text) > MaxMessageSize {
			return nil, errors.New("decoded text too large")
		}
		msg.Type = wire.TokenType
		msg.Data = text
		return msg, nil
	}
	if msg.Type == "" {
		return nil, errors.New("message type required")
	}
	if len(wire.Data) > 0 {
		if err := json.Unmarshal(wire.Data, &msg.Data); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

// FragmentMessage returns no fragments for invalid size or a message exceeding
// the wire's 255-fragment limit. Fragments alias data; Transport.Send takes a copy.
func FragmentMessage(data []byte, maxSize int) ([][]byte, uint16) {
	if maxSize <= 0 {
		return nil, 0
	}
	count := (len(data) + maxSize - 1) / maxSize
	if count > 255 {
		return nil, 0
	}
	if len(data) <= maxSize {
		return [][]byte{data}, 0
	}
	id := uint16(atomic.AddUint32(&globalFragmentID, 1))
	fragments := make([][]byte, 0, count)
	for i := 0; i < len(data); i += maxSize {
		end := min(i+maxSize, len(data))
		fragments = append(fragments, data[i:end])
	}
	return fragments, id
}

type FragmentAssembler struct {
	mu        sync.Mutex
	fragments map[uint16]map[uint8][]byte
	totals    map[uint16]uint8
	firstSeen map[uint16]time.Time
	bytes     int
}

func NewFragmentAssembler() *FragmentAssembler {
	return &FragmentAssembler{fragments: make(map[uint16]map[uint8][]byte), totals: make(map[uint16]uint8), firstSeen: make(map[uint16]time.Time)}
}
func (fa *FragmentAssembler) AddFragment(id uint16, offset, total uint8, data []byte) ([]byte, bool) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	for key, first := range fa.firstSeen {
		if time.Since(first) > 30*time.Second {
			for _, b := range fa.fragments[key] {
				fa.bytes -= len(b)
			}
			delete(fa.fragments, key)
			delete(fa.totals, key)
			delete(fa.firstSeen, key)
		}
	}
	if total == 0 || offset >= total || len(data) > MaxPayloadSize {
		return nil, false
	}
	if fa.fragments[id] == nil {
		if len(fa.fragments) >= maxFragmentTransfers {
			return nil, false
		}
		fa.fragments[id] = make(map[uint8][]byte)
		fa.totals[id] = total
		fa.firstSeen[id] = time.Now()
	}
	if fa.totals[id] != total {
		return nil, false
	}
	if _, exists := fa.fragments[id][offset]; exists {
		return nil, false
	}
	if fa.bytes+len(data) > maxReassemblyBytes {
		return nil, false
	}
	fa.fragments[id][offset] = append([]byte(nil), data...)
	fa.bytes += len(data)
	if len(fa.fragments[id]) != int(total) {
		return nil, false
	}
	var result []byte
	for i := uint8(0); i < total; i++ {
		part := fa.fragments[id][i]
		result = append(result, part...)
		fa.bytes -= len(part)
	}
	delete(fa.fragments, id)
	delete(fa.totals, id)
	delete(fa.firstSeen, id)
	return result, true
}

type MessageHandler struct {
	codec     *MessageCodec
	transport *Transport
	assembler *FragmentAssembler
}

func NewMessageHandler(t *Transport) *MessageHandler {
	return &MessageHandler{NewMessageCodec(), t, NewFragmentAssembler()}
}
func (mh *MessageHandler) SendMessage(c *Connection, msg *Message) error {
	return mh.SendMessageContext(mh.transport.ctx, c, msg)
}
func (mh *MessageHandler) SendMessageContext(ctx context.Context, c *Connection, msg *Message) error {
	data, err := mh.codec.Encode(msg)
	if err != nil {
		return fmt.Errorf("encode error: %w", err)
	}
	return mh.transport.SendContext(ctx, c, data)
}
func (mh *MessageHandler) SetMessageCallback(c *Connection, fn func(*Message)) {
	c.SetMessageCallback(func(data []byte) {
		msg, err := mh.codec.Decode(data)
		if err == nil && fn != nil {
			fn(msg)
		}
	})
}
func (mh *MessageHandler) ParseMessage(data []byte) (*Message, error) { return mh.codec.Decode(data) }

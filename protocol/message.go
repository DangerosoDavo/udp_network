package protocol

import (
	"encoding/json"
	"fmt"
	"github.com/DangerosoDavo/udp_network/tokenizer"
)

type MessageType string

const (
	MessageTypeText    MessageType = "text"
	MessageTypeJSON    MessageType = "json"
	MessageTypeCommand MessageType = "command"
	MessageTypeBinary  MessageType = "binary"
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
	return &MessageCodec{
		tokenizer: tokenizer.NewTokenizer(),
	}
}

func (mc *MessageCodec) Encode(msg *Message) ([]byte, error) {
	// For text messages, try tokenization first
	if msg.Type == MessageTypeText {
		if text, ok := msg.Data.(string); ok {
			// Check if tokenization provides good compression
			tokens := mc.tokenizer.Encode(text)
			tokenData := mc.tokenizer.EncodeTokensToBinary(tokens)

			// Create a lightweight wrapper
			wrapper := map[string]interface{}{
				"t":  string(msg.Type),
				"d":  tokenData,
				"tk": true, // tokenized flag
			}

			if msg.ID != "" {
				wrapper["id"] = msg.ID
			}
			if msg.Timestamp > 0 {
				wrapper["ts"] = msg.Timestamp
			}

			// Compare sizes
			jsonData, _ := json.Marshal(msg)
			wrapperData, _ := json.Marshal(wrapper)

			if len(wrapperData) < len(jsonData) {
				return wrapperData, nil
			}
		}
	}

	// Fall back to regular JSON encoding
	return json.Marshal(msg)
}

func (mc *MessageCodec) Decode(data []byte) (*Message, error) {
	// Try to decode as wrapper first
	var wrapper map[string]interface{}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	// Check if it's tokenized
	if tk, ok := wrapper["tk"].(bool); ok && tk {
		if tokenData, ok := wrapper["d"].([]byte); ok {
			tokens := mc.tokenizer.DecodeTokensFromBinary(tokenData)
			text := mc.tokenizer.Decode(tokens)

			msg := &Message{
				Type: MessageType(wrapper["t"].(string)),
				Data: text,
			}

			if id, ok := wrapper["id"].(string); ok {
				msg.ID = id
			}
			if ts, ok := wrapper["ts"].(float64); ok {
				msg.Timestamp = int64(ts)
			}

			return msg, nil
		}
	}

	// Fall back to regular message decoding
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// FragmentMessage splits large messages into fragments
func FragmentMessage(data []byte, maxSize int) ([][]byte, uint16) {
	if len(data) <= maxSize {
		return [][]byte{data}, 0
	}

	// Generate fragment ID
	fragID := uint16(data[0])<<8 | uint16(data[1])

	var fragments [][]byte
	for i := 0; i < len(data); i += maxSize {
		end := i + maxSize
		if end > len(data) {
			end = len(data)
		}
		fragments = append(fragments, data[i:end])
	}

	return fragments, fragID
}

// ReassembleMessage combines fragments back into original message
type FragmentAssembler struct {
	fragments map[uint16]map[uint8][]byte
	totals    map[uint16]uint8
}

func NewFragmentAssembler() *FragmentAssembler {
	return &FragmentAssembler{
		fragments: make(map[uint16]map[uint8][]byte),
		totals:    make(map[uint16]uint8),
	}
}

func (fa *FragmentAssembler) AddFragment(fragID uint16, offset uint8, total uint8, data []byte) ([]byte, bool) {
	if _, exists := fa.fragments[fragID]; !exists {
		fa.fragments[fragID] = make(map[uint8][]byte)
		fa.totals[fragID] = total
	}

	fa.fragments[fragID][offset] = data

	// Check if we have all fragments
	if len(fa.fragments[fragID]) == int(total) {
		// Reassemble
		var result []byte
		for i := uint8(0); i < total; i++ {
			result = append(result, fa.fragments[fragID][i]...)
		}

		// Clean up
		delete(fa.fragments, fragID)
		delete(fa.totals, fragID)

		return result, true
	}

	return nil, false
}

// High-level message operations
type MessageHandler struct {
	codec     *MessageCodec
	transport *Transport
	assembler *FragmentAssembler
}

func NewMessageHandler(transport *Transport) *MessageHandler {
	return &MessageHandler{
		codec:     NewMessageCodec(),
		transport: transport,
		assembler: NewFragmentAssembler(),
	}
}

func (mh *MessageHandler) SendMessage(conn *Connection, msg *Message) error {
	// Encode message
	data, err := mh.codec.Encode(msg)
	if err != nil {
		return fmt.Errorf("encode error: %w", err)
	}

	// Fragment if necessary
	fragments, _ := FragmentMessage(data, MaxPayloadSize)

	// Send fragments
	for _, fragment := range fragments {
		// Send the fragment - transport.Send will create the packet
		if err := mh.transport.Send(conn, fragment); err != nil {
			return err
		}
	}

	return nil
}

func (mh *MessageHandler) SetMessageCallback(conn *Connection, callback func(*Message)) {
	conn.onMessage = func(data []byte) {
		// Check if this is a fragment
		// This would normally come from packet metadata
		// For now, try to decode directly
		msg, err := mh.codec.Decode(data)
		if err == nil && callback != nil {
			callback(msg)
		}
	}
}

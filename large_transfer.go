package udp_network

import (
	"fmt"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

// LargeStringOptions contains options for sending large strings
type LargeStringOptions struct {
	TransferID    string                 // Optional custom transfer ID
	Metadata      map[string]interface{} // Optional metadata
	OnProgress    func(sent, total int)  // Progress callback
	RetryAttempts int                    // Number of retry attempts per chunk (default: 3)
	ChunkTimeout  time.Duration          // Timeout per chunk (default: 5s)
}

// SendLargeString sends a large string using chunked transfer
func (c *Client) SendLargeString(largeString string, options *LargeStringOptions) error {
	if c.Connection == nil {
		return fmt.Errorf("not connected")
	}

	// Default options
	if options == nil {
		options = &LargeStringOptions{}
	}
	if options.RetryAttempts == 0 {
		options.RetryAttempts = 3
	}
	if options.ChunkTimeout == 0 {
		options.ChunkTimeout = 5 * time.Second
	}

	// Generate chunks
	chunks, err := protocol.ChunkString(largeString, options.TransferID, options.Metadata)
	if err != nil {
		return fmt.Errorf("failed to chunk string: %v", err)
	}

	fmt.Printf("📦 Sending large string in %d chunks (%.2f MB total)\n",
		len(chunks), float64(len(largeString))/(1024*1024))

	// Send chunks with retry logic
	for i, chunk := range chunks {
		sent := false

		for attempt := 0; attempt < options.RetryAttempts; attempt++ {
			msg := &protocol.Message{
				Type:      protocol.MessageTypeChunked,
				ID:        fmt.Sprintf("%s_%d", chunk.Metadata.TransferID, i),
				Timestamp: time.Now().Unix(),
				Data:      chunk,
			}

			if err := c.handler.SendMessage(c.Connection, msg); err != nil {
				if attempt < options.RetryAttempts-1 {
					fmt.Printf("⚠️  Chunk %d/%d failed (attempt %d), retrying...\n",
						i+1, len(chunks), attempt+1)
					time.Sleep(time.Second)
					continue
				}
				return fmt.Errorf("failed to send chunk %d after %d attempts: %v",
					i, options.RetryAttempts, err)
			}

			sent = true
			break
		}

		if !sent {
			return fmt.Errorf("failed to send chunk %d", i)
		}

		// Progress callback
		if options.OnProgress != nil {
			options.OnProgress(i+1, len(chunks))
		}

		// Small delay between chunks to avoid overwhelming the network
		if i < len(chunks)-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	fmt.Printf("✅ Large string sent successfully (%d chunks)\n", len(chunks))
	return nil
}

// SendLargeString sends a large string using chunked transfer (secure version)
func (c *SecureClient) SendLargeString(largeString string, options *LargeStringOptions) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Default options
	if options == nil {
		options = &LargeStringOptions{}
	}
	if options.RetryAttempts == 0 {
		options.RetryAttempts = 3
	}
	if options.ChunkTimeout == 0 {
		options.ChunkTimeout = 5 * time.Second
	}

	// Generate chunks
	chunks, err := protocol.ChunkString(largeString, options.TransferID, options.Metadata)
	if err != nil {
		return fmt.Errorf("failed to chunk string: %v", err)
	}

	fmt.Printf("🔐 Sending large string securely in %d chunks (%.2f MB total)\n",
		len(chunks), float64(len(largeString))/(1024*1024))

	// Send chunks with retry logic
	for i, chunk := range chunks {
		sent := false

		for attempt := 0; attempt < options.RetryAttempts; attempt++ {
			msg := &protocol.Message{
				Type:      protocol.MessageTypeChunked,
				ID:        fmt.Sprintf("%s_%d", chunk.Metadata.TransferID, i),
				Timestamp: time.Now().Unix(),
				Data:      chunk,
			}

			if err := c.SendMessage(msg); err != nil {
				if attempt < options.RetryAttempts-1 {
					fmt.Printf("⚠️  Chunk %d/%d failed (attempt %d), retrying...\n",
						i+1, len(chunks), attempt+1)
					time.Sleep(time.Second)
					continue
				}
				return fmt.Errorf("failed to send chunk %d after %d attempts: %v",
					i, options.RetryAttempts, err)
			}

			sent = true
			break
		}

		if !sent {
			return fmt.Errorf("failed to send chunk %d", i)
		}

		// Progress callback
		if options.OnProgress != nil {
			options.OnProgress(i+1, len(chunks))
		}

		// Small delay between chunks
		if i < len(chunks)-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	fmt.Printf("✅ Large string sent securely (%d chunks)\n", len(chunks))
	return nil
}

// LargeStringReceiver handles receiving large strings
type LargeStringReceiver struct {
	reassembler *protocol.ChunkReassembler
	handlers    map[string]func(string, map[string]interface{})
	mu          sync.RWMutex
}

// NewLargeStringReceiver creates a new large string receiver
func NewLargeStringReceiver() *LargeStringReceiver {
	lsr := &LargeStringReceiver{
		reassembler: protocol.NewChunkReassembler(),
		handlers:    make(map[string]func(string, map[string]interface{})),
	}

	// Set up completion callback
	lsr.reassembler.SetCompleteCallback(func(transferID string, data []byte, metadata map[string]interface{}) {
		lsr.handleComplete(transferID, string(data), metadata)
	})

	return lsr
}

// SetHandler sets a handler for completed large string transfers
func (lsr *LargeStringReceiver) SetHandler(transferID string, handler func(string, map[string]interface{})) {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	lsr.handlers[transferID] = handler
}

// SetDefaultHandler sets a default handler for all transfers
func (lsr *LargeStringReceiver) SetDefaultHandler(handler func(string, map[string]interface{})) {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	lsr.handlers["*"] = handler
}

// ProcessChunkedMessage processes a chunked message
func (lsr *LargeStringReceiver) ProcessChunkedMessage(msg *protocol.Message) error {
	chunk, ok := msg.Data.(*protocol.ChunkedMessage)
	if !ok {
		// Try to convert from map if it was deserialized
		if _, ok := msg.Data.(map[string]interface{}); ok {
			// Convert map back to ChunkedMessage
			// This would need proper type conversion logic
			return fmt.Errorf("chunk data conversion not implemented")
		}
		return fmt.Errorf("invalid chunk data type")
	}

	return lsr.reassembler.AddChunk(chunk)
}

// GetTransferStatus returns the status of a transfer
func (lsr *LargeStringReceiver) GetTransferStatus(transferID string) (received, total int, exists bool) {
	return lsr.reassembler.GetTransferStatus(transferID)
}

// GetStats returns statistics for all active transfers
func (lsr *LargeStringReceiver) GetStats() []protocol.ChunkedTransferStats {
	return lsr.reassembler.GetStats()
}

// handleComplete is called when a transfer completes
func (lsr *LargeStringReceiver) handleComplete(transferID string, data string, metadata map[string]interface{}) {
	lsr.mu.RLock()
	handler, exists := lsr.handlers[transferID]
	if !exists {
		handler = lsr.handlers["*"] // Try default handler
	}
	lsr.mu.RUnlock()

	if handler != nil {
		handler(data, metadata)
	}

	// Clean up specific handler
	lsr.mu.Lock()
	delete(lsr.handlers, transferID)
	lsr.mu.Unlock()
}

// EnableLargeStringReceiver enables large string receiving for a server
func EnableLargeStringReceiver(s *Server) *LargeStringReceiver {
	receiver := NewLargeStringReceiver()

	// Override or extend the message handler to process chunks
	s.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeChunked {
			if err := receiver.ProcessChunkedMessage(msg); err != nil {
				fmt.Printf("Error processing chunk: %v\n", err)
			}

			// Get status
			if chunk, ok := msg.Data.(*protocol.ChunkedMessage); ok {
				received, total, _ := receiver.GetTransferStatus(chunk.Metadata.TransferID)
				fmt.Printf("📦 Transfer %s: %d/%d chunks received\n",
					chunk.Metadata.TransferID[:8], received, total)
			}
		} else {
			// Handle other message types normally
			s.handleMessage(conn, msg)
		}
	})

	return receiver
}

// EnableLargeStringReceiverSecure enables large string receiving for a secure server
func EnableLargeStringReceiverSecure(s *SecureServer) *LargeStringReceiver {
	receiver := NewLargeStringReceiver()

	// Set up message handler to process chunks
	s.OnMessage(func(conn *protocol.Connection, data []byte) {
		// Parse the message
		handler := protocol.NewMessageHandler(s.GetTransport().Transport)
		msg, err := handler.ParseMessage(data)
		if err != nil {
			fmt.Printf("Error parsing message: %v\n", err)
			return
		}

		if msg.Type == protocol.MessageTypeChunked {
			if err := receiver.ProcessChunkedMessage(msg); err != nil {
				fmt.Printf("Error processing chunk: %v\n", err)
			}

			// Get status
			if chunk, ok := msg.Data.(*protocol.ChunkedMessage); ok {
				received, total, _ := receiver.GetTransferStatus(chunk.Metadata.TransferID)
				fmt.Printf("🔐📦 Transfer %s: %d/%d chunks received\n",
					chunk.Metadata.TransferID[:8], received, total)
			}
		}
		// Note: Other message types can be handled by additional logic if needed
	})

	return receiver
}

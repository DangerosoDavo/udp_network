package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	// Maximum chunk size (leaving room for metadata)
	MaxChunkSize = 300 * 1024 // 300KB per chunk

	// Chunk reassembly timeout
	ChunkTimeout = 5 * time.Minute
)

// ChunkMetadata contains information about a chunk
type ChunkMetadata struct {
	TransferID  string    `json:"transfer_id"`
	ChunkIndex  int       `json:"chunk_index"`
	TotalChunks int       `json:"total_chunks"`
	ChunkSize   int       `json:"chunk_size"`
	Checksum    string    `json:"checksum"`
	Timestamp   time.Time `json:"timestamp"`
}

// ChunkedMessage represents a message that can be sent in chunks
type ChunkedMessage struct {
	Type      MessageType            `json:"type"`
	Metadata  ChunkMetadata          `json:"metadata"`
	Data      string                 `json:"data"`
	ExtraData map[string]interface{} `json:"extra,omitempty"`
}

// ChunkReassembler handles reassembly of chunked messages
type ChunkReassembler struct {
	transfers  map[string]*TransferState
	mu         sync.RWMutex
	onComplete func(transferID string, data []byte, metadata map[string]interface{})
}

// TransferState tracks the state of a chunked transfer
type TransferState struct {
	TransferID     string
	TotalChunks    int
	ReceivedChunks map[int][]byte
	Metadata       map[string]interface{}
	StartTime      time.Time
	LastActivity   time.Time
	Complete       bool
	mu             sync.Mutex
}

// NewChunkReassembler creates a new chunk reassembler
func NewChunkReassembler() *ChunkReassembler {
	cr := &ChunkReassembler{
		transfers: make(map[string]*TransferState),
	}

	// Start cleanup goroutine
	go cr.cleanupLoop()

	return cr
}

// SetCompleteCallback sets the callback for completed transfers
func (cr *ChunkReassembler) SetCompleteCallback(callback func(transferID string, data []byte, metadata map[string]interface{})) {
	cr.onComplete = callback
}

// AddChunk adds a chunk to the reassembler
func (cr *ChunkReassembler) AddChunk(chunk *ChunkedMessage) error {
	cr.mu.Lock()
	transfer, exists := cr.transfers[chunk.Metadata.TransferID]
	if !exists {
		transfer = &TransferState{
			TransferID:     chunk.Metadata.TransferID,
			TotalChunks:    chunk.Metadata.TotalChunks,
			ReceivedChunks: make(map[int][]byte),
			Metadata:       chunk.ExtraData,
			StartTime:      time.Now(),
			LastActivity:   time.Now(),
		}
		cr.transfers[chunk.Metadata.TransferID] = transfer
	}
	cr.mu.Unlock()

	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	// Verify chunk
	if chunk.Metadata.ChunkIndex >= chunk.Metadata.TotalChunks {
		return fmt.Errorf("invalid chunk index %d (total chunks: %d)",
			chunk.Metadata.ChunkIndex, chunk.Metadata.TotalChunks)
	}

	// Verify checksum
	hash := sha256.Sum256([]byte(chunk.Data))
	checksum := hex.EncodeToString(hash[:])
	if checksum != chunk.Metadata.Checksum {
		return fmt.Errorf("chunk checksum mismatch")
	}

	// Store chunk
	transfer.ReceivedChunks[chunk.Metadata.ChunkIndex] = []byte(chunk.Data)
	transfer.LastActivity = time.Now()

	// Check if transfer is complete
	if len(transfer.ReceivedChunks) == transfer.TotalChunks {
		transfer.Complete = true

		// Reassemble data
		totalSize := 0
		for i := 0; i < transfer.TotalChunks; i++ {
			totalSize += len(transfer.ReceivedChunks[i])
		}

		completeData := make([]byte, 0, totalSize)
		for i := 0; i < transfer.TotalChunks; i++ {
			completeData = append(completeData, transfer.ReceivedChunks[i]...)
		}

		// Trigger callback
		if cr.onComplete != nil {
			go cr.onComplete(transfer.TransferID, completeData, transfer.Metadata)
		}

		// Clean up
		cr.mu.Lock()
		delete(cr.transfers, chunk.Metadata.TransferID)
		cr.mu.Unlock()
	}

	return nil
}

// GetTransferStatus returns the status of a transfer
func (cr *ChunkReassembler) GetTransferStatus(transferID string) (received, total int, exists bool) {
	cr.mu.RLock()
	transfer, exists := cr.transfers[transferID]
	cr.mu.RUnlock()

	if !exists {
		return 0, 0, false
	}

	transfer.mu.Lock()
	received = len(transfer.ReceivedChunks)
	total = transfer.TotalChunks
	transfer.mu.Unlock()

	return received, total, true
}

// cleanupLoop removes timed-out transfers
func (cr *ChunkReassembler) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cr.mu.Lock()
		now := time.Now()

		for id, transfer := range cr.transfers {
			if now.Sub(transfer.LastActivity) > ChunkTimeout {
				delete(cr.transfers, id)
			}
		}

		cr.mu.Unlock()
	}
}

// ChunkString splits a large string into chunks for transmission
func ChunkString(data string, transferID string, metadata map[string]interface{}) ([]*ChunkedMessage, error) {
	if transferID == "" {
		// Generate transfer ID if not provided
		hash := sha256.Sum256([]byte(data + time.Now().String()))
		transferID = hex.EncodeToString(hash[:16])
	}

	dataBytes := []byte(data)
	totalSize := len(dataBytes)

	// Calculate number of chunks
	numChunks := (totalSize + MaxChunkSize - 1) / MaxChunkSize

	chunks := make([]*ChunkedMessage, 0, numChunks)

	for i := 0; i < numChunks; i++ {
		start := i * MaxChunkSize
		end := start + MaxChunkSize
		if end > totalSize {
			end = totalSize
		}

		chunkData := dataBytes[start:end]

		// Calculate checksum
		hash := sha256.Sum256(chunkData)
		checksum := hex.EncodeToString(hash[:])

		chunk := &ChunkedMessage{
			Type: MessageTypeChunked,
			Metadata: ChunkMetadata{
				TransferID:  transferID,
				ChunkIndex:  i,
				TotalChunks: numChunks,
				ChunkSize:   len(chunkData),
				Checksum:    checksum,
				Timestamp:   time.Now(),
			},
			Data:      string(chunkData),
			ExtraData: metadata,
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// ChunkedTransferStats provides statistics about a chunked transfer
type ChunkedTransferStats struct {
	TransferID     string
	TotalChunks    int
	ReceivedChunks int
	TotalBytes     int
	StartTime      time.Time
	Duration       time.Duration
	Complete       bool
}

// GetStats returns statistics for all active transfers
func (cr *ChunkReassembler) GetStats() []ChunkedTransferStats {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	stats := make([]ChunkedTransferStats, 0, len(cr.transfers))

	for _, transfer := range cr.transfers {
		transfer.mu.Lock()

		totalBytes := 0
		for _, chunk := range transfer.ReceivedChunks {
			totalBytes += len(chunk)
		}

		stat := ChunkedTransferStats{
			TransferID:     transfer.TransferID,
			TotalChunks:    transfer.TotalChunks,
			ReceivedChunks: len(transfer.ReceivedChunks),
			TotalBytes:     totalBytes,
			StartTime:      transfer.StartTime,
			Duration:       time.Since(transfer.StartTime),
			Complete:       transfer.Complete,
		}

		transfer.mu.Unlock()
		stats = append(stats, stat)
	}

	return stats
}

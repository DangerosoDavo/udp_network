package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// 32KiB stays below MaxMessageSize even with worst-case JSON string escaping.
	MaxChunkSize        = 32 * 1024
	ChunkTimeout        = 5 * time.Minute
	maxChunkCount       = 128
	maxChunkTransfers   = 64
	maxChunkBufferBytes = 4 << 20
)

type ChunkMetadata struct {
	TransferID  string    `json:"transfer_id"`
	ChunkIndex  int       `json:"chunk_index"`
	TotalChunks int       `json:"total_chunks"`
	ChunkSize   int       `json:"chunk_size"`
	Checksum    string    `json:"checksum"`
	Timestamp   time.Time `json:"timestamp"`
}
type ChunkedMessage struct {
	Type      MessageType            `json:"type"`
	Metadata  ChunkMetadata          `json:"metadata"`
	Data      string                 `json:"data"`
	ExtraData map[string]interface{} `json:"extra,omitempty"`
}

// ChunkReassembler is bounded by transfer count, bytes, chunk count and metadata
// size. Expiration runs on access (no permanently leaked cleanup goroutine).
// Completion callbacks run synchronously OUTSIDE the mutex and may reenter.
type ChunkReassembler struct {
	transfers  map[string]*TransferState
	completed  map[string]time.Time
	mu         sync.RWMutex
	onComplete func(string, []byte, map[string]interface{})
	bytes      int
	closed     bool
}
type TransferState struct {
	TransferID              string
	TotalChunks             int
	ReceivedChunks          map[int][]byte
	Metadata                map[string]interface{}
	StartTime, LastActivity time.Time
	Complete                bool
	mu                      sync.Mutex // retained for source compatibility; state is guarded by owner cr.mu
}

func NewChunkReassembler() *ChunkReassembler {
	return &ChunkReassembler{transfers: make(map[string]*TransferState), completed: make(map[string]time.Time)}
}
func (cr *ChunkReassembler) SetCompleteCallback(fn func(string, []byte, map[string]interface{})) {
	cr.mu.Lock()
	cr.onComplete = fn
	cr.mu.Unlock()
}
func (cr *ChunkReassembler) cleanupLocked(now time.Time) {
	for id, t := range cr.transfers {
		if now.Sub(t.LastActivity) > ChunkTimeout {
			for _, part := range t.ReceivedChunks {
				cr.bytes -= len(part)
			}
			delete(cr.transfers, id)
		}
	}
	for id, done := range cr.completed {
		if now.Sub(done) > ChunkTimeout {
			delete(cr.completed, id)
		}
	}
}
func (cr *ChunkReassembler) AddChunk(chunk *ChunkedMessage) error {
	if chunk == nil {
		return errors.New("nil chunk")
	}
	m := chunk.Metadata
	if m.TransferID == "" || len(m.TransferID) > 128 || m.TotalChunks < 1 || m.TotalChunks > maxChunkCount || m.ChunkIndex < 0 || m.ChunkIndex >= m.TotalChunks ||
		len(chunk.Data) > MaxChunkSize || m.ChunkSize != len(chunk.Data) {
		return errors.New("invalid chunk metadata")
	}
	hash := sha256.Sum256([]byte(chunk.Data))
	if hex.EncodeToString(hash[:]) != m.Checksum {
		return errors.New("chunk checksum mismatch")
	}
	metadataBytes, err := json.Marshal(chunk.ExtraData)
	if err != nil || len(metadataBytes) > 16*1024 {
		return errors.New("chunk metadata too large or invalid")
	}
	var metadata map[string]interface{}
	if err = json.Unmarshal(metadataBytes, &metadata); err != nil {
		return err
	}
	cr.mu.Lock()
	if cr.closed {
		cr.mu.Unlock()
		return errors.New("reassembler closed")
	}
	cr.cleanupLocked(time.Now())
	if _, done := cr.completed[m.TransferID]; done {
		cr.mu.Unlock()
		return nil
	}
	t := cr.transfers[m.TransferID]
	if t == nil {
		if len(cr.transfers) >= maxChunkTransfers {
			cr.mu.Unlock()
			return errors.New("chunk transfer limit")
		}
		t = &TransferState{TransferID: m.TransferID, TotalChunks: m.TotalChunks, ReceivedChunks: make(map[int][]byte), Metadata: metadata, StartTime: time.Now(), LastActivity: time.Now()}
		cr.transfers[m.TransferID] = t
	}
	if t.TotalChunks != m.TotalChunks {
		cr.mu.Unlock()
		return errors.New("inconsistent chunk total")
	}
	if old, ok := t.ReceivedChunks[m.ChunkIndex]; ok {
		cr.mu.Unlock()
		if !bytes.Equal(old, []byte(chunk.Data)) {
			return errors.New("conflicting duplicate chunk")
		}
		return nil
	}
	if cr.bytes+len(chunk.Data) > maxChunkBufferBytes {
		cr.mu.Unlock()
		return errors.New("chunk buffer byte limit")
	}
	t.ReceivedChunks[m.ChunkIndex] = []byte(chunk.Data)
	cr.bytes += len(chunk.Data)
	t.LastActivity = time.Now()
	if len(t.ReceivedChunks) != t.TotalChunks {
		cr.mu.Unlock()
		return nil
	}
	total := 0
	for _, b := range t.ReceivedChunks {
		total += len(b)
	}
	data := make([]byte, 0, total)
	for i := 0; i < t.TotalChunks; i++ {
		data = append(data, t.ReceivedChunks[i]...)
	}
	cr.bytes -= total
	delete(cr.transfers, m.TransferID)
	// A bounded completed-ID cache suppresses replayed completed transfers.
	if len(cr.completed) >= 1024 {
		var oldest string
		var when time.Time
		for id, at := range cr.completed {
			if oldest == "" || at.Before(when) {
				oldest = id
				when = at
			}
		}
		delete(cr.completed, oldest)
	}
	cr.completed[m.TransferID] = time.Now()
	fn := cr.onComplete
	extra := t.Metadata
	cr.mu.Unlock()
	if fn != nil {
		fn(m.TransferID, data, extra)
	}
	return nil
}
func (cr *ChunkReassembler) GetTransferStatus(id string) (int, int, bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.cleanupLocked(time.Now())
	t, ok := cr.transfers[id]
	if !ok {
		return 0, 0, false
	}
	return len(t.ReceivedChunks), t.TotalChunks, true
}
func (cr *ChunkReassembler) Close() {
	cr.mu.Lock()
	cr.closed = true
	cr.transfers = make(map[string]*TransferState)
	cr.completed = make(map[string]time.Time)
	cr.bytes = 0
	cr.mu.Unlock()
}
func ChunkString(data, id string, metadata map[string]interface{}) ([]*ChunkedMessage, error) {
	if !utf8.ValidString(data) {
		return nil, errors.New("chunked string must be valid UTF-8")
	}
	if len(data) > MaxChunkSize*maxChunkCount {
		return nil, errors.New("chunked message too large")
	}
	if len(id) > 128 {
		return nil, errors.New("transfer ID too long")
	}
	if id == "" {
		hash := sha256.Sum256([]byte(data + time.Now().String()))
		id = hex.EncodeToString(hash[:16])
	}
	parts := make([]string, 0, (len(data)+MaxChunkSize-1)/MaxChunkSize)
	for start := 0; start < len(data); {
		end := min(start+MaxChunkSize, len(data))
		for end < len(data) && !utf8.RuneStart(data[end]) {
			end--
		}
		parts = append(parts, data[start:end])
		start = end
	}
	if len(parts) == 0 {
		parts = append(parts, "")
	}
	if len(parts) > maxChunkCount {
		return nil, errors.New("chunked string has too many UTF-8 chunks")
	}
	count := len(parts)
	chunks := make([]*ChunkedMessage, 0, count)
	for i, part := range parts {
		hash := sha256.Sum256([]byte(part))
		chunks = append(chunks, &ChunkedMessage{Type: MessageTypeChunked, Metadata: ChunkMetadata{TransferID: id, ChunkIndex: i, TotalChunks: count, ChunkSize: len(part), Checksum: hex.EncodeToString(hash[:]), Timestamp: time.Now()}, Data: part, ExtraData: metadata})
	}
	return chunks, nil
}

type ChunkedTransferStats struct {
	TransferID                              string
	TotalChunks, ReceivedChunks, TotalBytes int
	StartTime                               time.Time
	Duration                                time.Duration
	Complete                                bool
}

func (cr *ChunkReassembler) GetStats() []ChunkedTransferStats {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.cleanupLocked(time.Now())
	result := make([]ChunkedTransferStats, 0, len(cr.transfers))
	for _, t := range cr.transfers {
		size := 0
		for _, part := range t.ReceivedChunks {
			size += len(part)
		}
		result = append(result, ChunkedTransferStats{t.TransferID, t.TotalChunks, len(t.ReceivedChunks), size, t.StartTime, time.Since(t.StartTime), false})
	}
	return result
}

// ValidateChunkSize provides callers a stable bound before allocating a transfer.
func ValidateChunkSize(size int) error {
	if size < 0 || size > MaxChunkSize*maxChunkCount {
		return fmt.Errorf("chunked message exceeds %d bytes", MaxChunkSize*maxChunkCount)
	}
	return nil
}

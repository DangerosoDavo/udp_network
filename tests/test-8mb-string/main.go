package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

var (
	mode = flag.String("mode", "both", "server, client, or both")
	size = flag.Float64("size", 8.0, "Size in MB")
	host = flag.String("host", "localhost:8093", "Server address")
)

// Simple chunk structure
type ChunkData struct {
	TransferID  string                 `json:"transfer_id"`
	ChunkIndex  int                    `json:"chunk_index"`
	TotalChunks int                    `json:"total_chunks"`
	Data        string                 `json:"data"`
	Checksum    string                 `json:"checksum"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Transfer manager
type TransferManager struct {
	transfers  map[string]*TransferState
	mu         sync.Mutex
	onComplete func(string, string, map[string]interface{})
}

type TransferState struct {
	TotalChunks int
	Chunks      map[int]string
	Metadata    map[string]interface{}
	StartTime   time.Time
}

func NewTransferManager() *TransferManager {
	return &TransferManager{
		transfers: make(map[string]*TransferState),
	}
}

func (tm *TransferManager) ProcessChunk(chunk ChunkData) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	transfer, exists := tm.transfers[chunk.TransferID]
	if !exists {
		transfer = &TransferState{
			TotalChunks: chunk.TotalChunks,
			Chunks:      make(map[int]string),
			Metadata:    chunk.Metadata,
			StartTime:   time.Now(),
		}
		tm.transfers[chunk.TransferID] = transfer
	}

	transfer.Chunks[chunk.ChunkIndex] = chunk.Data

	fmt.Printf("📦 Received chunk %d/%d for transfer %s (%.2f KB)\n",
		chunk.ChunkIndex+1, chunk.TotalChunks, chunk.TransferID[:8], float64(len(chunk.Data))/1024)

	// Check if complete
	if len(transfer.Chunks) == transfer.TotalChunks {
		// Reassemble
		var result strings.Builder
		totalSize := 0
		for i := 0; i < transfer.TotalChunks; i++ {
			result.WriteString(transfer.Chunks[i])
			totalSize += len(transfer.Chunks[i])
		}

		duration := time.Since(transfer.StartTime)
		fullData := result.String()

		fmt.Printf("\n✅ Transfer %s complete!\n", chunk.TransferID[:8])
		fmt.Printf("   Total size: %.2f MB (%d bytes)\n", float64(totalSize)/(1024*1024), totalSize)
		fmt.Printf("   Transfer time: %v\n", duration)
		fmt.Printf("   Throughput: %.2f MB/s\n", float64(totalSize)/(1024*1024)/duration.Seconds())

		if tm.onComplete != nil {
			tm.onComplete(chunk.TransferID, fullData, transfer.Metadata)
		}

		// Clean up
		delete(tm.transfers, chunk.TransferID)
	}
}

func runServer() {
	server, err := udp_network.NewServer(*host)
	if err != nil {
		log.Fatal(err)
	}

	tm := NewTransferManager()
	tm.onComplete = func(id, data string, metadata map[string]interface{}) {
		fmt.Printf("\n🎉 Large string received successfully!\n")
		fmt.Printf("   First 100 chars: %s\n", data[:min(100, len(data))])
		fmt.Printf("   Last 100 chars: %s\n", data[max(0, len(data)-100):])
		if metadata != nil {
			fmt.Printf("   Metadata: %+v\n", metadata)
		}
	}

	// Handle chunks
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == "chunk" {
			// Parse chunk data
			if chunkMap, ok := msg.Data.(map[string]interface{}); ok {
				chunk := ChunkData{
					TransferID:  fmt.Sprintf("%v", chunkMap["transfer_id"]),
					ChunkIndex:  int(chunkMap["chunk_index"].(float64)),
					TotalChunks: int(chunkMap["total_chunks"].(float64)),
					Data:        fmt.Sprintf("%v", chunkMap["data"]),
					Checksum:    fmt.Sprintf("%v", chunkMap["checksum"]),
				}

				if meta, ok := chunkMap["metadata"].(map[string]interface{}); ok {
					chunk.Metadata = meta
				}

				// Verify checksum
				hash := sha256.Sum256([]byte(chunk.Data))
				checksum := hex.EncodeToString(hash[:])
				if checksum != chunk.Checksum {
					fmt.Printf("❌ Checksum mismatch for chunk %d\n", chunk.ChunkIndex)
					return
				}

				tm.ProcessChunk(chunk)
			}
		} else {
			// Handle other messages normally
			fmt.Printf("Received %s message\n", msg.Type)
		}
	})

	server.Start()
	defer server.Stop()

	fmt.Printf("✅ Server ready on %s to receive large strings\n", *host)

	select {}
}

func sendLargeString(client *udp_network.Client, data string) error {
	// Generate transfer ID
	hash := sha256.Sum256([]byte(data + time.Now().String()))
	transferID := hex.EncodeToString(hash[:16])

	// Calculate chunks
	chunkSize := 300 * 1024 // 300KB per chunk
	totalChunks := (len(data) + chunkSize - 1) / chunkSize

	fmt.Printf("\n📤 Sending %.2f MB in %d chunks...\n", float64(len(data))/(1024*1024), totalChunks)
	startTime := time.Now()

	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(data) {
			end = len(data)
		}

		chunkData := data[start:end]

		// Calculate checksum
		hash := sha256.Sum256([]byte(chunkData))
		checksum := hex.EncodeToString(hash[:])

		chunk := ChunkData{
			TransferID:  transferID,
			ChunkIndex:  i,
			TotalChunks: totalChunks,
			Data:        chunkData,
			Checksum:    checksum,
			Metadata: map[string]interface{}{
				"size_mb": *size,
				"sender":  "test-client",
			},
		}

		msg := &protocol.Message{
			Type:      "chunk",
			ID:        fmt.Sprintf("%s_%d", transferID, i),
			Timestamp: time.Now().Unix(),
			Data:      chunk,
		}

		for attempt := 0; attempt < 3; attempt++ {
			if err := client.SendJSON(msg.Data); err != nil {
				if attempt < 2 {
					fmt.Printf("⚠️  Retry chunk %d (attempt %d)\n", i+1, attempt+2)
					time.Sleep(100 * time.Millisecond)
					continue
				}
				return fmt.Errorf("failed to send chunk %d: %v", i, err)
			}
			break
		}

		fmt.Printf("📊 Sent chunk %d/%d (%.1f%%)\n", i+1, totalChunks, float64(i+1)/float64(totalChunks)*100)

		// Small delay to avoid overwhelming the network
		time.Sleep(5 * time.Millisecond)
	}

	duration := time.Since(startTime)
	throughput := float64(len(data)) / (1024 * 1024) / duration.Seconds()

	fmt.Printf("\n✅ Sent %.2f MB in %v (%.2f MB/s)\n",
		float64(len(data))/(1024*1024), duration, throughput)

	return nil
}

func runClient() {
	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	client.Start()
	defer client.Stop()

	fmt.Printf("🔗 Connecting to %s...\n", *host)
	if err := client.Connect(*host); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	fmt.Println("✅ Connected!")

	// Generate large string
	fmt.Printf("\n📝 Generating %.1f MB test string...\n", *size)

	// Create realistic content
	pattern := "The quick brown fox jumps over the lazy dog. This is test data for large string transfer. "
	sizeBytes := int(*size * 1024 * 1024)
	repetitions := sizeBytes / len(pattern)

	var builder strings.Builder
	builder.Grow(sizeBytes)
	for i := 0; i < repetitions; i++ {
		builder.WriteString(pattern)
		if i%10000 == 0 {
			fmt.Printf("\r📝 Generating... %.1f%%", float64(i*len(pattern))/float64(sizeBytes)*100)
		}
	}

	largeString := builder.String()
	fmt.Printf("\r✅ Generated %d bytes (%.2f MB)     \n", len(largeString), float64(len(largeString))/(1024*1024))

	// Send it
	if err := sendLargeString(client, largeString); err != nil {
		log.Fatal(err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	flag.Parse()

	fmt.Println("UDP Network 8MB String Test")
	fmt.Println("===========================")
	fmt.Printf("Mode: %s\n", *mode)
	fmt.Printf("Size: %.1f MB\n", *size)
	fmt.Printf("Host: %s\n", *host)

	switch *mode {
	case "server":
		runServer()

	case "client":
		runClient()
		time.Sleep(2 * time.Second) // Wait for final chunks

	case "both":
		go runServer()
		time.Sleep(2 * time.Second)
		runClient()
		time.Sleep(3 * time.Second) // Wait for completion

	default:
		fmt.Println("Invalid mode. Use: server, client, or both")
	}
}

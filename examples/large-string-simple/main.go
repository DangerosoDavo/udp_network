package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

type TransferManager struct {
	transfers map[string]*Transfer
	mu        sync.Mutex
}

type Transfer struct {
	ID          string
	TotalChunks int
	Chunks      map[int]string
	Metadata    map[string]interface{}
}

func NewTransferManager() *TransferManager {
	return &TransferManager{
		transfers: make(map[string]*Transfer),
	}
}

func (tm *TransferManager) ProcessChunk(transferID string, chunkIndex, totalChunks int, data string, metadata map[string]interface{}) (complete bool, fullData string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	transfer, exists := tm.transfers[transferID]
	if !exists {
		transfer = &Transfer{
			ID:          transferID,
			TotalChunks: totalChunks,
			Chunks:      make(map[int]string),
			Metadata:    metadata,
		}
		tm.transfers[transferID] = transfer
	}

	transfer.Chunks[chunkIndex] = data

	// Check if complete
	if len(transfer.Chunks) == transfer.TotalChunks {
		// Reassemble
		var builder strings.Builder
		for i := 0; i < transfer.TotalChunks; i++ {
			builder.WriteString(transfer.Chunks[i])
		}

		// Clean up
		delete(tm.transfers, transferID)

		return true, builder.String()
	}

	return false, ""
}

func startServer() {
	server, err := udp_network.NewServer(":8092")
	if err != nil {
		log.Fatal(err)
	}

	transferManager := NewTransferManager()

	// Custom message handler
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == "chunk" {
			// Extract chunk data
			if chunkData, ok := msg.Data.(map[string]interface{}); ok {
				transferID := fmt.Sprintf("%v", chunkData["transfer_id"])
				chunkIndex := int(chunkData["chunk_index"].(float64))
				totalChunks := int(chunkData["total_chunks"].(float64))
				data := fmt.Sprintf("%v", chunkData["data"])

				var metadata map[string]interface{}
				if meta, ok := chunkData["metadata"].(map[string]interface{}); ok {
					metadata = meta
				}

				fmt.Printf("📦 Received chunk %d/%d for transfer %s\n", chunkIndex+1, totalChunks, transferID[:8])

				complete, fullData := transferManager.ProcessChunk(transferID, chunkIndex, totalChunks, data, metadata)
				if complete {
					fmt.Printf("\n✅ Transfer %s complete!\n", transferID[:8])
					fmt.Printf("   Size: %.2f MB (%d bytes)\n", float64(len(fullData))/(1024*1024), len(fullData))
					fmt.Printf("   First 50 chars: %s\n", fullData[:min(50, len(fullData))])
					if metadata != nil {
						fmt.Printf("   Metadata: %+v\n", metadata)
					}
				}
			}
		}
	})

	server.Start()
	defer server.Stop()

	fmt.Println("✅ Server ready on :8092")

	select {}
}

func sendLargeStringSimple(client *udp_network.Client, data string, metadata map[string]interface{}) error {
	// Generate transfer ID
	hash := sha256.Sum256([]byte(data + time.Now().String()))
	transferID := hex.EncodeToString(hash[:16])

	// Chunk the data
	chunkSize := 300 * 1024 // 300KB chunks
	totalChunks := (len(data) + chunkSize - 1) / chunkSize

	fmt.Printf("📤 Sending %d bytes in %d chunks\n", len(data), totalChunks)

	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(data) {
			end = len(data)
		}

		chunkData := data[start:end]

		msg := &protocol.Message{
			Type:      "chunk",
			ID:        fmt.Sprintf("%s_%d", transferID, i),
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"transfer_id":  transferID,
				"chunk_index":  i,
				"total_chunks": totalChunks,
				"data":         chunkData,
				"metadata":     metadata,
			},
		}

		if err := client.SendJSON(msg.Data); err != nil {
			return fmt.Errorf("failed to send chunk %d: %v", i, err)
		}

		fmt.Printf("📊 Sent chunk %d/%d\n", i+1, totalChunks)
		time.Sleep(10 * time.Millisecond) // Small delay between chunks
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	fmt.Println("Simple Large String Transfer Example")
	fmt.Println("===================================")

	// Start server
	go startServer()
	time.Sleep(2 * time.Second)

	// Connect client
	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	client.Start()
	defer client.Stop()

	if err := client.Connect("localhost:8092"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✅ Connected to server")

	// Create large string
	largeString := strings.Repeat("Hello, this is a test of large string transfer! ", 21845) // ~1MB
	fmt.Printf("\n📝 Created test string: %.2f MB\n", float64(len(largeString))/(1024*1024))

	// Send it
	err = sendLargeStringSimple(client, largeString, map[string]interface{}{
		"filename": "test.txt",
		"type":     "text",
		"sender":   "example-client",
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n✅ Transfer complete!")

	// Wait to see results
	time.Sleep(2 * time.Second)
}

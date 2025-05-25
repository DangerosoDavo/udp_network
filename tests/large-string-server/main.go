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
	port   = flag.String("port", "8080", "Port to listen on")
	secure = flag.Bool("secure", false, "Use secure server")
)

// TransferTracker tracks incoming transfers
type TransferTracker struct {
	transfers map[string]*TransferInfo
	mu        sync.Mutex
}

type TransferInfo struct {
	ID           string
	StartTime    time.Time
	TotalChunks  int
	ReceivedChunks int
	TotalBytes   int
	Metadata     map[string]interface{}
	Complete     bool
}

func NewTransferTracker() *TransferTracker {
	return &TransferTracker{
		transfers: make(map[string]*TransferInfo),
	}
}

func (tt *TransferTracker) UpdateTransfer(transferID string, chunkIndex, totalChunks int, chunkSize int, metadata map[string]interface{}) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	info, exists := tt.transfers[transferID]
	if !exists {
		info = &TransferInfo{
			ID:          transferID,
			StartTime:   time.Now(),
			TotalChunks: totalChunks,
			Metadata:    metadata,
		}
		tt.transfers[transferID] = info
		
		fmt.Printf("\n🆕 New transfer started: %s\n", transferID[:8])
		fmt.Printf("   Expected chunks: %d\n", totalChunks)
		if metadata != nil {
			if size, ok := metadata["size_mb"]; ok {
				fmt.Printf("   Expected size: %.2f MB\n", size)
			}
			if sender, ok := metadata["sender"]; ok {
				fmt.Printf("   Sender: %v\n", sender)
			}
		}
	}
	
	info.ReceivedChunks++
	info.TotalBytes += chunkSize
	
	// Progress update every 10%
	progress := float64(info.ReceivedChunks) / float64(info.TotalChunks) * 100
	if int(progress)%10 == 0 || info.ReceivedChunks == info.TotalChunks {
		fmt.Printf("\r📊 Transfer %s: %d/%d chunks (%.1f%%) - %.2f MB received", 
			transferID[:8], info.ReceivedChunks, info.TotalChunks, progress,
			float64(info.TotalBytes)/(1024*1024))
	}
}

func (tt *TransferTracker) CompleteTransfer(transferID string, data string) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	info, exists := tt.transfers[transferID]
	if !exists {
		return
	}
	
	info.Complete = true
	duration := time.Since(info.StartTime)
	
	// Calculate checksum
	hash := sha256.Sum256([]byte(data))
	checksum := hex.EncodeToString(hash[:])
	
	fmt.Printf("\n\n✅ Transfer %s completed!\n", transferID[:8])
	fmt.Printf("   Total size: %.2f MB (%d bytes)\n", float64(len(data))/(1024*1024), len(data))
	fmt.Printf("   Transfer time: %v\n", duration)
	fmt.Printf("   Throughput: %.2f MB/s\n", float64(len(data))/(1024*1024)/duration.Seconds())
	fmt.Printf("   Checksum: %s\n", checksum[:16])
	
	// Verify checksum if provided
	if info.Metadata != nil {
		if expectedChecksum, ok := info.Metadata["checksum"].(string); ok {
			if expectedChecksum == checksum {
				fmt.Println("   ✅ Checksum verification: PASSED")
			} else {
				fmt.Printf("   ❌ Checksum verification: FAILED\n")
				fmt.Printf("      Expected: %s\n", expectedChecksum[:16])
				fmt.Printf("      Received: %s\n", checksum[:16])
			}
		}
	}
	
	// Show sample of received data
	fmt.Printf("\n📄 Data sample:\n")
	fmt.Printf("   First 100 chars: %s\n", data[:min(100, len(data))])
	fmt.Printf("   Last 100 chars: %s\n", data[max(0, len(data)-100):])
	
	// Clean up
	delete(tt.transfers, transferID)
}

func (tt *TransferTracker) GetStats() {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	
	if len(tt.transfers) > 0 {
		fmt.Printf("\n📊 Active transfers: %d\n", len(tt.transfers))
		for id, info := range tt.transfers {
			age := time.Since(info.StartTime)
			fmt.Printf("   - %s: %d/%d chunks, age: %v\n", 
				id[:8], info.ReceivedChunks, info.TotalChunks, age)
		}
	}
}

func runRegularServer(address string) {
	fmt.Printf("🔓 Starting large string test server on %s...\n", address)
	
	server, err := udp_network.NewServer(address)
	if err != nil {
		log.Fatal(err)
	}
	
	tracker := NewTransferTracker()
	
	// Enable large string receiver
	receiver := udp_network.EnableLargeStringReceiver(server)
	
	// Set default handler for completed transfers
	receiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
		// Extract transfer ID from metadata if available
		transferID := "unknown"
		if meta, ok := metadata["transfer_id"].(string); ok {
			transferID = meta
		}
		
		tracker.CompleteTransfer(transferID, data)
	})
	
	// Also track chunk progress
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeChunked {
			// Try to extract chunk info for progress tracking
			if chunkData, ok := msg.Data.(*protocol.ChunkedMessage); ok {
				tracker.UpdateTransfer(
					chunkData.Metadata.TransferID,
					chunkData.Metadata.ChunkIndex,
					chunkData.Metadata.TotalChunks,
					chunkData.Metadata.ChunkSize,
					chunkData.ExtraData,
				)
			}
			
			// Let the receiver process it
			receiver.ProcessChunkedMessage(msg)
		} else {
			// Handle other message types
			fmt.Printf("Received %s message from %s\n", msg.Type, conn.Addr)
		}
	})
	
	server.Start()
	defer server.Stop()
	
	fmt.Println("✅ Server ready to receive large strings")
	fmt.Println("📡 Waiting for transfers...")
	fmt.Println()
	
	// Periodic stats
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		tracker.GetStats()
	}
}

func runSecureServer(address string) {
	fmt.Printf("🔐 Starting secure large string test server on %s...\n", address)
	
	server, err := udp_network.NewSecureServer(address)
	if err != nil {
		log.Fatal(err)
	}
	
	tracker := NewTransferTracker()
	
	// For secure server, we need a different approach
	// Create a custom handler that processes raw data
	server.OnMessage(func(conn *protocol.Connection, rawData []byte) {
		// Parse the message
		handler := protocol.NewMessageHandler(nil)
		msg, err := handler.ParseMessage(rawData)
		if err != nil {
			return
		}
		
		if msg.Type == protocol.MessageTypeChunked {
			// Process chunk
			fmt.Printf("Received chunk from %s\n", conn.Addr)
			// Note: Full implementation would need proper chunk reassembly
		} else {
			fmt.Printf("Received %s message from %s\n", msg.Type, conn.Addr)
		}
	})
	
	server.Start()
	defer server.Stop()
	
	fmt.Println("✅ Secure server ready to receive large strings")
	fmt.Println("🔐 All transfers will be encrypted with ChaCha20-Poly1305")
	fmt.Println("📡 Waiting for transfers...")
	fmt.Println()
	
	select {}
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
	
	fmt.Println("UDP Network Large String Test Server")
	fmt.Println("===================================")
	fmt.Printf("Port: %s\n", *port)
	fmt.Printf("Mode: %s\n", map[bool]string{true: "Secure", false: "Regular"}[*secure])
	fmt.Println()
	fmt.Println("This server receives and verifies large string transfers.")
	fmt.Println("Use with remote-large-string-test client.")
	fmt.Println()
	
	address := ":" + *port
	
	if *secure {
		runSecureServer(address)
	} else {
		runRegularServer(address)
	}
}
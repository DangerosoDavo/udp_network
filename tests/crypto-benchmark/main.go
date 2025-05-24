package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
	
	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== UDP Network Encryption Performance Test ===")
	fmt.Println()
	
	// Test parameters
	messageSize := 1000 // bytes
	numMessages := 10000
	
	fmt.Printf("Test parameters:\n")
	fmt.Printf("- Message size: %d bytes\n", messageSize)
	fmt.Printf("- Number of messages: %d\n", numMessages)
	fmt.Printf("- Encryption: ChaCha20-Poly1305\n")
	fmt.Println()
	
	// Run unencrypted test first
	fmt.Println("Running UNENCRYPTED test...")
	unencryptedDuration := runUnencryptedTest(messageSize, numMessages)
	unencryptedThroughput := calculateThroughput(messageSize, numMessages, unencryptedDuration)
	
	// Run encrypted test
	fmt.Println("\nRunning ENCRYPTED test...")
	encryptedDuration := runEncryptedTest(messageSize, numMessages)
	encryptedThroughput := calculateThroughput(messageSize, numMessages, encryptedDuration)
	
	// Calculate overhead
	overhead := (float64(encryptedDuration) / float64(unencryptedDuration) - 1) * 100
	throughputLoss := (1 - encryptedThroughput/unencryptedThroughput) * 100
	
	// Print results
	fmt.Println("\n=== RESULTS ===")
	fmt.Printf("Unencrypted:\n")
	fmt.Printf("  Duration: %v\n", unencryptedDuration)
	fmt.Printf("  Throughput: %.2f MB/s\n", unencryptedThroughput)
	fmt.Printf("  Messages/sec: %.0f\n", float64(numMessages)/unencryptedDuration.Seconds())
	
	fmt.Printf("\nEncrypted:\n")
	fmt.Printf("  Duration: %v\n", encryptedDuration)
	fmt.Printf("  Throughput: %.2f MB/s\n", encryptedThroughput)
	fmt.Printf("  Messages/sec: %.0f\n", float64(numMessages)/encryptedDuration.Seconds())
	
	fmt.Printf("\nOverhead:\n")
	fmt.Printf("  Time overhead: %.1f%%\n", overhead)
	fmt.Printf("  Throughput reduction: %.1f%%\n", throughputLoss)
	
	// Additional info
	fmt.Printf("\nEncryption details:\n")
	fmt.Printf("  Algorithm: ChaCha20-Poly1305\n")
	fmt.Printf("  Key exchange: Curve25519\n")
	fmt.Printf("  Additional bytes per message: %d (nonce + tag)\n", 12+16)
}

func runUnencryptedTest(messageSize, numMessages int) time.Duration {
	serverAddr := "localhost:9997"
	
	// Create server
	server, err := udpnet.NewServer(serverAddr)
	if err != nil {
		log.Fatal("Failed to create server:", err)
	}
	
	var receivedCount atomic.Int32
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		receivedCount.Add(1)
	})
	
	server.Start()
	defer server.Stop()
	
	// Small delay for server startup
	time.Sleep(200 * time.Millisecond)
	
	// Create client
	client, err := udpnet.NewClient()
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}
	
	// Start client first
	client.Start()
	defer client.Stop()
	
	// Now connect
	if err := client.Connect(serverAddr); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	
	// Wait for connection
	time.Sleep(200 * time.Millisecond)
	
	// Prepare test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	// Create text message
	msg := &protocol.Message{
		Type: protocol.MessageTypeText,
		Data: string(testData),
		Timestamp: time.Now().Unix(),
	}
	
	// Run test
	start := time.Now()
	
	// Send messages
	var errors atomic.Int32
	for i := 0; i < numMessages; i++ {
		if client.Connection != nil {
			if err := client.SendMessage(client.Connection, msg); err != nil {
				errors.Add(1)
			}
		}
	}
	
	duration := time.Since(start)
	
	if errors.Load() > 0 {
		fmt.Printf("  Warning: %d send errors\n", errors.Load())
	}
	
	return duration
}

func runEncryptedTest(messageSize, numMessages int) time.Duration {
	serverAddr := "localhost:9998"
	
	// Create secure server
	server, err := udpnet.NewSecureServer(serverAddr)
	if err != nil {
		log.Fatal("Failed to create secure server:", err)
	}
	
	var receivedCount atomic.Int32
	server.OnMessage(func(conn *protocol.Connection, data []byte) {
		receivedCount.Add(1)
	})
	
	server.Start()
	defer server.Stop()
	
	// Small delay for server startup
	time.Sleep(200 * time.Millisecond)
	
	// Create secure client
	client, err := udpnet.NewSecureClient()
	if err != nil {
		log.Fatal("Failed to create secure client:", err)
	}
	
	if err := client.Connect(serverAddr); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer client.Close()
	
	// Wait for connection and key exchange
	time.Sleep(500 * time.Millisecond)
	
	if !client.IsConnected() {
		log.Fatal("Failed to establish secure connection")
	}
	
	// Prepare test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	// Run test
	start := time.Now()
	
	// Send messages
	var wg sync.WaitGroup
	var errors atomic.Int32
	batchSize := 100
	
	for i := 0; i < numMessages; i += batchSize {
		wg.Add(1)
		go func(startIdx int) {
			defer wg.Done()
			
			end := startIdx + batchSize
			if end > numMessages {
				end = numMessages
			}
			
			for j := startIdx; j < end; j++ {
				if err := client.Send(testData); err != nil {
					errors.Add(1)
				}
			}
		}(i)
	}
	
	wg.Wait()
	duration := time.Since(start)
	
	if errors.Load() > 0 {
		fmt.Printf("  Warning: %d send errors\n", errors.Load())
	}
	
	return duration
}

func calculateThroughput(messageSize, numMessages int, duration time.Duration) float64 {
	totalBytes := float64(messageSize * numMessages)
	totalMB := totalBytes / (1024 * 1024)
	seconds := duration.Seconds()
	return totalMB / seconds
}
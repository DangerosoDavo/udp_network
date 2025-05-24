package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	
	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== Accurate Crypto Performance Test ===\n")
	
	messageSize := 1000
	numMessages := 1000
	
	// Test 1: Unencrypted
	fmt.Println("Testing UNENCRYPTED communication...")
	unencryptedTime := testUnencrypted(messageSize, numMessages)
	
	// Test 2: Encrypted
	fmt.Println("\nTesting ENCRYPTED communication...")
	encryptedTime := testEncrypted(messageSize, numMessages)
	
	// Results
	fmt.Println("\n=== RESULTS ===")
	fmt.Printf("Message size: %d bytes\n", messageSize)
	fmt.Printf("Messages sent: %d\n\n", numMessages)
	
	fmt.Printf("Unencrypted:\n")
	fmt.Printf("  Total time: %v\n", unencryptedTime)
	fmt.Printf("  Per message: %.2f µs\n", float64(unencryptedTime.Microseconds())/float64(numMessages))
	fmt.Printf("  Throughput: %.2f MB/s\n\n", float64(messageSize*numMessages)/(1024*1024)/unencryptedTime.Seconds())
	
	fmt.Printf("Encrypted:\n")
	fmt.Printf("  Total time: %v\n", encryptedTime)
	fmt.Printf("  Per message: %.2f µs\n", float64(encryptedTime.Microseconds())/float64(numMessages))
	fmt.Printf("  Throughput: %.2f MB/s\n\n", float64(messageSize*numMessages)/(1024*1024)/encryptedTime.Seconds())
	
	overhead := (float64(encryptedTime) / float64(unencryptedTime) - 1) * 100
	fmt.Printf("Encryption overhead: %.1f%%\n", overhead)
}

func testUnencrypted(messageSize, numMessages int) time.Duration {
	// Setup server
	server, _ := udpnet.NewServer(":9991")
	var received atomic.Int32
	done := make(chan bool)
	
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if received.Add(1) == int32(numMessages) {
			close(done)
		}
	})
	
	server.Start()
	defer server.Stop()
	
	time.Sleep(200 * time.Millisecond)
	
	// Setup client
	client, _ := udpnet.NewClient()
	client.Start()
	defer client.Stop()
	
	client.Connect("localhost:9991")
	time.Sleep(200 * time.Millisecond)
	
	// Prepare test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	msg := &protocol.Message{
		Type: protocol.MessageTypeText,
		Data: string(testData),
		Timestamp: time.Now().Unix(),
	}
	
	// Measure sending time
	start := time.Now()
	
	for i := 0; i < numMessages; i++ {
		client.SendMessage(client.Connection, msg)
	}
	
	// Wait for all messages to be received
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		fmt.Printf("Warning: Only received %d/%d messages\n", received.Load(), numMessages)
	}
	
	return time.Since(start)
}

func testEncrypted(messageSize, numMessages int) time.Duration {
	// Setup secure server
	server, _ := udpnet.NewSecureServer(":9990")
	var received atomic.Int32
	done := make(chan bool)
	
	server.OnMessage(func(conn *protocol.Connection, data []byte) {
		if received.Add(1) == int32(numMessages) {
			close(done)
		}
	})
	
	server.Start()
	defer server.Stop()
	
	time.Sleep(200 * time.Millisecond)
	
	// Setup secure client
	client, _ := udpnet.NewSecureClient()
	client.Connect("localhost:9990")
	defer client.Close()
	
	time.Sleep(300 * time.Millisecond) // Extra time for key exchange
	
	// Prepare test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	// Measure sending time
	start := time.Now()
	
	var wg sync.WaitGroup
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.Send(testData)
		}()
		
		// Small delay to prevent overwhelming
		if i%100 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	
	wg.Wait()
	
	// Wait for all messages to be received
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		fmt.Printf("Warning: Only received %d/%d messages\n", received.Load(), numMessages)
	}
	
	return time.Since(start)
}
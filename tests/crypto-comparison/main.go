package main

import (
	"fmt"
	"github.com/DangerosoDavo/udp_network/internal/exampleconfig"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

const (
	numMessages = 1000
	messageSize = 1024
)

func testUnencrypted() (delivered int64, duration time.Duration, err error) {
	// Start server transport
	serverTransport, err := protocol.NewTransport(":8080")
	if err != nil {
		return 0, 0, err
	}

	var received int64
	var wg sync.WaitGroup
	wg.Add(1)

	// Set up server message handler
	serverTransport.OnConnect = func(conn *protocol.Connection) {
		conn.SetMessageCallback(func(data []byte) {
			if atomic.AddInt64(&received, 1) == numMessages {
				wg.Done()
			}
		})
	}

	serverTransport.Start()
	defer serverTransport.Stop()

	// Start client transport
	clientTransport, err := protocol.NewTransport(":0")
	if err != nil {
		return 0, 0, err
	}
	clientTransport.Start()
	defer clientTransport.Stop()

	// Connect to server
	conn, err := clientTransport.Connect("localhost:8080")
	if err != nil {
		return 0, 0, err
	}

	// Give connection time to establish
	time.Sleep(100 * time.Millisecond)

	// Generate test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Measure send time
	start := time.Now()

	// Send messages
	for i := 0; i < numMessages; i++ {
		if err := clientTransport.Send(conn, testData); err != nil {
			return 0, 0, err
		}
	}

	// Wait for all messages to be received
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		duration = time.Since(start)
		delivered = atomic.LoadInt64(&received)
	case <-time.After(10 * time.Second):
		duration = 10 * time.Second
		delivered = atomic.LoadInt64(&received)
		err = fmt.Errorf("timeout: only received %d/%d messages", delivered, numMessages)
	}

	return
}

func testEncrypted() (delivered int64, duration time.Duration, err error) {
	// Start server transport
	serverTransport, err := protocol.NewSecureTransportWithPSK(":8081", exampleconfig.PSK())
	if err != nil {
		return 0, 0, err
	}

	var received int64
	var wg sync.WaitGroup
	wg.Add(1)

	// Set up server message handler
	serverTransport.OnConnect = func(conn *protocol.Connection) {
		conn.SetMessageCallback(func(data []byte) {
			if atomic.AddInt64(&received, 1) == numMessages {
				wg.Done()
			}
		})
	}

	serverTransport.Start()
	defer serverTransport.Stop()

	// Start client transport
	clientTransport, err := protocol.NewSecureTransportWithPSK(":0", exampleconfig.PSK())
	if err != nil {
		return 0, 0, err
	}
	clientTransport.Start()
	defer clientTransport.Stop()

	// Connect to server
	conn, err := clientTransport.SecureConnect("localhost:8081")
	if err != nil {
		return 0, 0, err
	}

	// Give connection time to establish
	time.Sleep(100 * time.Millisecond)

	// Generate test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Measure send time
	start := time.Now()

	// Send messages
	for i := 0; i < numMessages; i++ {
		if err := clientTransport.SendSecure(conn, testData); err != nil {
			return 0, 0, err
		}
	}

	// Wait for all messages to be received
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		duration = time.Since(start)
		delivered = atomic.LoadInt64(&received)
	case <-time.After(10 * time.Second):
		duration = 10 * time.Second
		delivered = atomic.LoadInt64(&received)
		err = fmt.Errorf("timeout: only received %d/%d messages", delivered, numMessages)
	}

	return
}

func main() {
	fmt.Println("UDP Network: Secure vs Non-Secure Communication Test")
	fmt.Println("===================================================")
	fmt.Printf("Testing %d messages of %d bytes each\n\n", numMessages, messageSize)

	// Test unencrypted communication
	fmt.Println("Testing unencrypted communication...")
	unencDelivered, unencDuration, err := testUnencrypted()
	if err != nil {
		log.Printf("Unencrypted test error: %v", err)
	}

	time.Sleep(1 * time.Second) // Let the system settle

	// Test encrypted communication
	fmt.Println("Testing encrypted communication...")
	encDelivered, encDuration, err := testEncrypted()
	if err != nil {
		log.Printf("Encrypted test error: %v", err)
	}

	// Results
	fmt.Println("\nResults:")
	fmt.Println("--------")
	fmt.Printf("Unencrypted: %d/%d messages delivered in %v\n",
		unencDelivered, numMessages, unencDuration)
	fmt.Printf("Encrypted:   %d/%d messages delivered in %v\n",
		encDelivered, numMessages, encDuration)

	if unencDelivered > 0 && encDelivered > 0 {
		fmt.Printf("\nThroughput:\n")
		fmt.Printf("Unencrypted: %.2f msg/sec\n",
			float64(unencDelivered)/unencDuration.Seconds())
		fmt.Printf("Encrypted:   %.2f msg/sec\n",
			float64(encDelivered)/encDuration.Seconds())

		fmt.Printf("\nPer-message latency:\n")
		fmt.Printf("Unencrypted: %.2f µs/msg\n",
			float64(unencDuration.Microseconds())/float64(unencDelivered))
		fmt.Printf("Encrypted:   %.2f µs/msg\n",
			float64(encDuration.Microseconds())/float64(encDelivered))
	}

	// Calculate reliability
	fmt.Printf("\nReliability:\n")
	fmt.Printf("Unencrypted: %.1f%% delivery rate\n",
		float64(unencDelivered)/float64(numMessages)*100)
	fmt.Printf("Encrypted:   %.1f%% delivery rate\n",
		float64(encDelivered)/float64(numMessages)*100)

	// Summary
	fmt.Printf("\nSummary:\n")
	if unencDelivered == numMessages && encDelivered == numMessages {
		fmt.Println("✓ Both unencrypted and encrypted communications are reliable")
		if encDuration > unencDuration {
			overhead := (float64(encDuration) - float64(unencDuration)) / float64(unencDuration) * 100
			fmt.Printf("✓ Encryption adds %.1f%% overhead (expected for security)\n", overhead)
		}
	} else {
		fmt.Println("⚠ Message delivery issues detected")
	}
}

package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

var (
	host     = flag.String("host", "localhost", "Server hostname or IP address")
	port     = flag.String("port", "8080", "Server port")
	secure   = flag.Bool("secure", false, "Use secure (encrypted) connection")
	messages = flag.Int("messages", 10, "Number of test messages to send")
	size     = flag.Int("size", 256, "Size of each test message in bytes")
	timeout  = flag.Duration("timeout", 30*time.Second, "Connection timeout")
)

func testRegularConnection(address string, messageCount int, messageSize int) error {
	fmt.Printf("Testing regular UDP connection to %s...\n", address)

	// Start client transport
	clientTransport, err := protocol.NewTransport(":0")
	if err != nil {
		return fmt.Errorf("failed to create transport: %v", err)
	}
	clientTransport.Start()
	defer clientTransport.Stop()

	// Connect to server
	fmt.Printf("Connecting to %s...\n", address)
	conn, err := clientTransport.Connect(address)
	if err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}

	// Set up message reception
	var received int64
	var wg sync.WaitGroup
	wg.Add(1)

	conn.SetMessageCallback(func(data []byte) {
		count := atomic.AddInt64(&received, 1)
		fmt.Printf("Received message %d/%d (%d bytes)\n", count, messageCount, len(data))
		if count == int64(messageCount) {
			wg.Done()
		}
	})

	// Generate test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Send test messages
	fmt.Printf("Sending %d messages of %d bytes each...\n", messageCount, messageSize)
	start := time.Now()

	for i := 0; i < messageCount; i++ {
		if err := clientTransport.Send(conn, testData); err != nil {
			return fmt.Errorf("send failed on message %d: %v", i+1, err)
		}
		fmt.Printf("Sent message %d/%d\n", i+1, messageCount)
		time.Sleep(100 * time.Millisecond) // Small delay between messages
	}

	// Wait for all responses with timeout
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		duration := time.Since(start)
		fmt.Printf("✓ All messages received in %v\n", duration)
		fmt.Printf("✓ Throughput: %.2f msg/sec\n", float64(received)/duration.Seconds())
		return nil
	case <-time.After(*timeout):
		return fmt.Errorf("timeout: only received %d/%d messages", received, messageCount)
	}
}

func testSecureConnection(address string, messageCount int, messageSize int) error {
	fmt.Printf("Testing secure UDP connection to %s...\n", address)

	// Start client transport
	clientTransport, err := protocol.NewSecureTransport(":0")
	if err != nil {
		return fmt.Errorf("failed to create secure transport: %v", err)
	}
	clientTransport.Start()
	defer clientTransport.Stop()

	// Connect to server
	fmt.Printf("Establishing secure connection to %s...\n", address)
	conn, err := clientTransport.SecureConnect(address)
	if err != nil {
		return fmt.Errorf("secure connection failed: %v", err)
	}
	fmt.Printf("✓ Secure connection established (ChaCha20-Poly1305)\n")

	// Set up message reception
	var received int64
	var wg sync.WaitGroup
	wg.Add(1)

	conn.SetMessageCallback(func(data []byte) {
		count := atomic.AddInt64(&received, 1)
		fmt.Printf("Received encrypted message %d/%d (%d bytes)\n", count, messageCount, len(data))
		if count == int64(messageCount) {
			wg.Done()
		}
	})

	// Generate test data
	testData := make([]byte, messageSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Send test messages
	fmt.Printf("Sending %d encrypted messages of %d bytes each...\n", messageCount, messageSize)
	start := time.Now()

	for i := 0; i < messageCount; i++ {
		if err := clientTransport.SendSecure(conn, testData); err != nil {
			return fmt.Errorf("secure send failed on message %d: %v", i+1, err)
		}
		fmt.Printf("Sent encrypted message %d/%d\n", i+1, messageCount)
		time.Sleep(100 * time.Millisecond) // Small delay between messages
	}

	// Wait for all responses with timeout
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		duration := time.Since(start)
		fmt.Printf("✓ All encrypted messages received in %v\n", duration)
		fmt.Printf("✓ Encrypted throughput: %.2f msg/sec\n", float64(received)/duration.Seconds())
		return nil
	case <-time.After(*timeout):
		return fmt.Errorf("timeout: only received %d/%d encrypted messages", received, messageCount)
	}
}

func main() {
	flag.Parse()

	fmt.Println("UDP Network Remote Test Client")
	fmt.Println("=============================")
	
	address := *host + ":" + *port
	
	fmt.Printf("Target: %s\n", address)
	fmt.Printf("Mode: %s\n", map[bool]string{true: "Secure (Encrypted)", false: "Regular (Unencrypted)"}[*secure])
	fmt.Printf("Messages: %d x %d bytes\n", *messages, *size)
	fmt.Printf("Timeout: %v\n\n", *timeout)

	var err error
	if *secure {
		err = testSecureConnection(address, *messages, *size)
	} else {
		err = testRegularConnection(address, *messages, *size)
	}

	if err != nil {
		fmt.Printf("\n❌ Test failed: %v\n", err)
		log.Fatal(err)
	}

	fmt.Printf("\n✅ Test completed successfully!\n")
	fmt.Printf("The UDP server at %s is working correctly.\n", address)
}
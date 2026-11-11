package main

import (
	"flag"
	"fmt"
	"github.com/DangerosoDavo/udp_network/internal/exampleconfig"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

var (
	host     = flag.String("host", "", "Server hostname or IP address (required)")
	both     = flag.Bool("both", false, "Test both regular (8080) and secure (8443) servers")
	messages = flag.Int("messages", 5, "Number of test messages to send")
	size     = flag.Int("size", 256, "Size of each test message in bytes")
	timeout  = flag.Duration("timeout", 30*time.Second, "Connection timeout")
)

func testConnection(address string, isSecure bool, messageCount int, messageSize int) error {
	connType := "Regular"
	if isSecure {
		connType = "Secure"
	}

	fmt.Printf("\n%s UDP Test to %s\n", connType, address)
	fmt.Printf("%s\n", "===========================================")

	var received int64
	var wg sync.WaitGroup
	wg.Add(1)

	if isSecure {
		// Test secure connection
		clientTransport, err := protocol.NewSecureTransportWithPSK(":0", exampleconfig.PSK())
		if err != nil {
			return fmt.Errorf("failed to create secure transport: %v", err)
		}
		clientTransport.Start()
		defer clientTransport.Stop()

		fmt.Printf("🔐 Establishing encrypted connection...\n")
		conn, err := clientTransport.SecureConnect(address)
		if err != nil {
			return fmt.Errorf("secure connection failed: %v", err)
		}
		fmt.Printf("✅ Secure connection established (ChaCha20-Poly1305)\n")

		conn.SetMessageCallback(func(data []byte) {
			count := atomic.AddInt64(&received, 1)
			fmt.Printf("📨 Received encrypted message %d/%d (%d bytes)\n", count, messageCount, len(data))
			if count == int64(messageCount) {
				wg.Done()
			}
		})

		// Generate and send test data
		testData := make([]byte, messageSize)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		fmt.Printf("📤 Sending %d encrypted messages...\n", messageCount)
		start := time.Now()

		for i := 0; i < messageCount; i++ {
			if err := clientTransport.SendSecure(conn, testData); err != nil {
				return fmt.Errorf("secure send failed: %v", err)
			}
			fmt.Printf("  Sent encrypted message %d/%d\n", i+1, messageCount)
			time.Sleep(200 * time.Millisecond)
		}

		// Wait for responses
		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			duration := time.Since(start)
			fmt.Printf("✅ All encrypted messages received in %v\n", duration)
			return nil
		case <-time.After(*timeout):
			return fmt.Errorf("timeout: only received %d/%d encrypted messages", received, messageCount)
		}

	} else {
		// Test regular connection
		clientTransport, err := protocol.NewTransport(":0")
		if err != nil {
			return fmt.Errorf("failed to create transport: %v", err)
		}
		clientTransport.Start()
		defer clientTransport.Stop()

		fmt.Printf("🔓 Establishing regular connection...\n")
		conn, err := clientTransport.Connect(address)
		if err != nil {
			return fmt.Errorf("connection failed: %v", err)
		}
		fmt.Printf("✅ Regular connection established\n")

		conn.SetMessageCallback(func(data []byte) {
			count := atomic.AddInt64(&received, 1)
			fmt.Printf("📨 Received message %d/%d (%d bytes)\n", count, messageCount, len(data))
			if count == int64(messageCount) {
				wg.Done()
			}
		})

		// Generate and send test data
		testData := make([]byte, messageSize)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		fmt.Printf("📤 Sending %d messages...\n", messageCount)
		start := time.Now()

		for i := 0; i < messageCount; i++ {
			if err := clientTransport.Send(conn, testData); err != nil {
				return fmt.Errorf("send failed: %v", err)
			}
			fmt.Printf("  Sent message %d/%d\n", i+1, messageCount)
			time.Sleep(200 * time.Millisecond)
		}

		// Wait for responses
		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			duration := time.Since(start)
			fmt.Printf("✅ All messages received in %v\n", duration)
			return nil
		case <-time.After(*timeout):
			return fmt.Errorf("timeout: only received %d/%d messages", received, messageCount)
		}
	}
}

func main() {
	flag.Parse()

	if *host == "" {
		fmt.Println("UDP Network Full Remote Test")
		fmt.Println("===========================")
		fmt.Println()
		fmt.Println("Usage: go run tests/full-remote-test/main.go -host <hostname/ip> [options]")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  go run tests/full-remote-test/main.go -host 192.168.1.100 -both")
		fmt.Println("  go run tests/full-remote-test/main.go -host example.com -messages 10")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -host string      Server hostname or IP (required)")
		fmt.Println("  -both            Test both regular (8080) and secure (8443) servers")
		fmt.Println("  -messages int    Number of test messages (default: 5)")
		fmt.Println("  -size int        Message size in bytes (default: 256)")
		fmt.Println("  -timeout duration Connection timeout (default: 30s)")
		fmt.Println()
		log.Fatal("Host is required")
	}

	fmt.Println("🌐 UDP Network Remote Server Test")
	fmt.Println("=================================")
	fmt.Printf("Target Host: %s\n", *host)
	fmt.Printf("Test Messages: %d x %d bytes\n", *messages, *size)
	fmt.Printf("Timeout: %v\n", *timeout)

	var errors []error

	if *both {
		// Test both regular and secure servers
		fmt.Println("\n🔄 Testing both regular and secure servers...")

		// Test regular server (port 8080)
		regularAddr := *host + ":8080"
		if err := testConnection(regularAddr, false, *messages, *size); err != nil {
			fmt.Printf("❌ Regular server test failed: %v\n", err)
			errors = append(errors, err)
		}

		time.Sleep(2 * time.Second) // Brief pause between tests

		// Test secure server (port 8443)
		secureAddr := *host + ":8443"
		if err := testConnection(secureAddr, true, *messages, *size); err != nil {
			fmt.Printf("❌ Secure server test failed: %v\n", err)
			errors = append(errors, err)
		}

	} else {
		// Test single server (auto-detect based on common ports)
		fmt.Println("\n🔄 Auto-detecting server type...")

		// Try secure server first (8443)
		secureAddr := *host + ":8443"
		fmt.Printf("Trying secure server at %s...\n", secureAddr)
		if err := testConnection(secureAddr, true, *messages, *size); err != nil {
			fmt.Printf("Secure server not available, trying regular server...\n")

			// Try regular server (8080)
			regularAddr := *host + ":8080"
			if err := testConnection(regularAddr, false, *messages, *size); err != nil {
				errors = append(errors, err)
			}
		}
	}

	// Summary
	fmt.Println("\n" + strings.Repeat("=", 50))
	if len(errors) == 0 {
		fmt.Println("🎉 All tests completed successfully!")
		fmt.Printf("✅ UDP server at %s is working correctly\n", *host)
	} else {
		fmt.Printf("⚠️  %d test(s) failed:\n", len(errors))
		for i, err := range errors {
			fmt.Printf("  %d. %v\n", i+1, err)
		}
	}
}

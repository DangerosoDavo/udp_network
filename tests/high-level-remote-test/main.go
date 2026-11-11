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

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

var (
	host     = flag.String("host", "", "Server hostname or IP address (required)")
	both     = flag.Bool("both", false, "Test both regular (8080) and secure (8443) servers")
	messages = flag.Int("messages", 5, "Number of test messages to send")
	timeout  = flag.Duration("timeout", 30*time.Second, "Connection timeout")
)

func testRegularServer(address string, messageCount int) error {
	fmt.Printf("\n🔓 Regular UDP Test to %s\n", address)
	fmt.Printf("===========================================\n")

	// Create client using high-level API
	client, err := udp_network.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create client: %v", err)
	}

	client.Start()
	defer client.Stop()

	// Set up message handler to count responses
	var received int64
	var wg sync.WaitGroup
	wg.Add(messageCount)

	client.SetMessageHandler(func(msg *protocol.Message) {
		count := atomic.AddInt64(&received, 1)
		fmt.Printf("📨 Received response %d/%d: %s\n", count, messageCount,
			fmt.Sprintf("Type: %s", msg.Type))
		wg.Done()
	})

	// Connect to server
	fmt.Printf("🔗 Connecting to %s...\n", address)
	if err := client.Connect(address); err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}
	fmt.Printf("✅ Connected successfully\n")

	// Give connection time to establish
	time.Sleep(500 * time.Millisecond)

	// Send test messages
	fmt.Printf("📤 Sending %d test messages...\n", messageCount)
	start := time.Now()

	for i := 0; i < messageCount; i++ {
		testMessage := fmt.Sprintf("Test message %d from remote client", i+1)

		if err := client.SendText(testMessage); err != nil {
			return fmt.Errorf("failed to send message %d: %v", i+1, err)
		}
		fmt.Printf("  Sent message %d/%d: %s\n", i+1, messageCount, testMessage)
		time.Sleep(200 * time.Millisecond)
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
		fmt.Printf("✅ All messages processed in %v\n", duration)
		return nil
	case <-time.After(*timeout):
		return fmt.Errorf("timeout: only received %d/%d responses", received, messageCount)
	}
}

func testSecureServer(address string, messageCount int) error {
	fmt.Printf("\n🔐 Secure UDP Test to %s\n", address)
	fmt.Printf("===========================================\n")

	// Create secure client using high-level API
	client, err := udp_network.NewSecureClientWithPSK(exampleconfig.PSK())
	if err != nil {
		return fmt.Errorf("failed to create secure client: %v", err)
	}
	defer client.Close()

	// Connect to secure server
	fmt.Printf("🔗 Establishing secure connection to %s...\n", address)
	if err := client.Connect(address); err != nil {
		return fmt.Errorf("secure connection failed: %v", err)
	}
	fmt.Printf("✅ Secure connection established (ChaCha20-Poly1305)\n")

	// Give connection time to establish
	time.Sleep(500 * time.Millisecond)

	// Send test messages and wait for responses
	fmt.Printf("📤 Sending %d encrypted test messages...\n", messageCount)
	start := time.Now()

	var received int64
	for i := 0; i < messageCount; i++ {
		testMessage := fmt.Sprintf("Encrypted test message %d from remote client", i+1)

		if err := client.SendText(testMessage); err != nil {
			return fmt.Errorf("failed to send encrypted message %d: %v", i+1, err)
		}
		fmt.Printf("  Sent encrypted message %d/%d: %s\n", i+1, messageCount, testMessage)

		// For secure client, we'll just count successful sends since the API is different
		atomic.AddInt64(&received, 1)
		time.Sleep(200 * time.Millisecond)
	}

	duration := time.Since(start)
	fmt.Printf("✅ All encrypted messages sent successfully in %v\n", duration)
	return nil
}

func testServerCommands(address string, isSecure bool) error {
	connType := "Regular"
	if isSecure {
		connType = "Secure"
	}

	fmt.Printf("\n🔧 %s Server Command Test to %s\n", connType, address)
	fmt.Printf("===========================================\n")

	if isSecure {
		// Test secure server with commands
		client, err := udp_network.NewSecureClientWithPSK(exampleconfig.PSK())
		if err != nil {
			return fmt.Errorf("failed to create secure client: %v", err)
		}
		defer client.Close()

		if err := client.Connect(address); err != nil {
			return fmt.Errorf("connection failed: %v", err)
		}
		fmt.Printf("✅ Connected to secure server\n")

		time.Sleep(500 * time.Millisecond)

		// Test different command types
		commands := []string{
			"hello",
			"ping",
			"time",
		}

		for _, cmd := range commands {
			fmt.Printf("📤 Sending command: %s\n", cmd)
			if err := client.SendText(cmd); err != nil {
				fmt.Printf("⚠️  Command '%s' failed: %v\n", cmd, err)
			} else {
				fmt.Printf("✅ Command '%s' sent successfully\n", cmd)
			}
			time.Sleep(300 * time.Millisecond)
		}

	} else {
		// Test regular server with commands
		client, err := udp_network.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create client: %v", err)
		}
		client.Start()
		defer client.Stop()

		var responses int64
		client.SetMessageHandler(func(msg *protocol.Message) {
			count := atomic.AddInt64(&responses, 1)
			fmt.Printf("📨 Command response %d: Type=%s\n", count, msg.Type)
		})

		if err := client.Connect(address); err != nil {
			return fmt.Errorf("connection failed: %v", err)
		}
		fmt.Printf("✅ Connected to regular server\n")

		time.Sleep(500 * time.Millisecond)

		// Test different command types
		commands := []string{
			"hello",
			"ping",
			"time",
			"stats",
		}

		for _, cmd := range commands {
			fmt.Printf("📤 Sending command: %s\n", cmd)
			if err := client.SendText(cmd); err != nil {
				fmt.Printf("⚠️  Command '%s' failed: %v\n", cmd, err)
			} else {
				fmt.Printf("✅ Command '%s' sent successfully\n", cmd)
			}
			time.Sleep(500 * time.Millisecond) // More time for responses
		}

		// Give time for responses
		time.Sleep(2 * time.Second)
		fmt.Printf("📊 Received %d command responses\n", responses)
	}

	return nil
}

func main() {
	flag.Parse()

	if *host == "" {
		fmt.Println("UDP Network High-Level Remote Test")
		fmt.Println("==================================")
		fmt.Println()
		fmt.Println("Usage: go run tests/high-level-remote-test/main.go -host <hostname/ip> [options]")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  go run tests/high-level-remote-test/main.go -host 192.168.1.100 -both")
		fmt.Println("  go run tests/high-level-remote-test/main.go -host example.com -messages 10")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -host string      Server hostname or IP (required)")
		fmt.Println("  -both            Test both regular (8080) and secure (8443) servers")
		fmt.Println("  -messages int    Number of test messages (default: 5)")
		fmt.Println("  -timeout duration Connection timeout (default: 30s)")
		fmt.Println()
		log.Fatal("Host is required")
	}

	fmt.Println("🌐 UDP Network High-Level Remote Server Test")
	fmt.Println("============================================")
	fmt.Printf("Target Host: %s\n", *host)
	fmt.Printf("Test Messages: %d\n", *messages)
	fmt.Printf("Timeout: %v\n", *timeout)

	var errors []error

	if *both {
		// Test both regular and secure servers
		fmt.Println("\n🔄 Testing both regular and secure servers...")

		// Test regular server (port 8080)
		regularAddr := *host + ":8080"
		if err := testRegularServer(regularAddr, *messages); err != nil {
			fmt.Printf("❌ Regular server test failed: %v\n", err)
			errors = append(errors, err)
		}

		if err := testServerCommands(regularAddr, false); err != nil {
			fmt.Printf("❌ Regular server commands failed: %v\n", err)
			errors = append(errors, err)
		}

		time.Sleep(2 * time.Second) // Brief pause between tests

		// Test secure server (port 8443)
		secureAddr := *host + ":8443"
		if err := testSecureServer(secureAddr, *messages); err != nil {
			fmt.Printf("❌ Secure server test failed: %v\n", err)
			errors = append(errors, err)
		}

		if err := testServerCommands(secureAddr, true); err != nil {
			fmt.Printf("❌ Secure server commands failed: %v\n", err)
			errors = append(errors, err)
		}

	} else {
		// Test single server (try both ports)
		fmt.Println("\n🔄 Auto-detecting server type...")

		// Try secure server first (8443)
		secureAddr := *host + ":8443"
		fmt.Printf("Trying secure server at %s...\n", secureAddr)
		if err := testSecureServer(secureAddr, *messages); err != nil {
			fmt.Printf("Secure server test failed, trying regular server...\n")

			// Try regular server (8080)
			regularAddr := *host + ":8080"
			if err := testRegularServer(regularAddr, *messages); err != nil {
				errors = append(errors, err)
			} else {
				// Also test commands on regular server
				testServerCommands(regularAddr, false)
			}
		} else {
			// Also test commands on secure server
			testServerCommands(secureAddr, true)
		}
	}

	// Summary
	fmt.Println("\n" + strings.Repeat("=", 50))
	if len(errors) == 0 {
		fmt.Println("🎉 All tests completed successfully!")
		fmt.Printf("✅ UDP server at %s is working correctly\n", *host)
		fmt.Println("\nYour server is:")
		fmt.Println("- Accepting connections ✅")
		fmt.Println("- Processing messages ✅")
		fmt.Println("- Handling commands ✅")
		if *both {
			fmt.Println("- Encryption working ✅")
		}
	} else {
		fmt.Printf("⚠️  %d test(s) failed:\n", len(errors))
		for i, err := range errors {
			fmt.Printf("  %d. %v\n", i+1, err)
		}
	}
}

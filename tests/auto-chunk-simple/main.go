package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/DangerosoDavo/udp_network"
)

func main() {
	fmt.Println("🚀 Simple test of automatic chunking for large strings")
	fmt.Println("=" + strings.Repeat("=", 60))

	// Test configurations
	tests := []struct {
		name string
		size int
		desc string
	}{
		{"Small", 100, "Should use regular SendText"},
		{"Medium", 100 * 1024, "Should use regular SendText"},
		{"Threshold", 250 * 1024, "Exactly at threshold - should use regular SendText"},
		{"Large", 300 * 1024, "Should automatically use chunked transfer"},
		{"Very Large", 1024 * 1024, "Should automatically use chunked transfer"},
	}

	// Start server
	server, err := udp_network.NewServer(":8080")
	if err != nil {
		panic(fmt.Sprintf("Failed to create server: %v", err))
	}
	defer server.Stop()
	server.Start()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Run tests
	for _, test := range tests {
		fmt.Printf("\n🧪 Test: %s text (%d bytes)\n", test.name, test.size)
		fmt.Printf("   Expected: %s\n", test.desc)
		fmt.Println("-" + strings.Repeat("-", 50))

		// Create client
		client, err := udp_network.NewClient()
		if err != nil {
			fmt.Printf("❌ Failed to create client: %v\n", err)
			continue
		}

		// Connect
		err = client.Connect("localhost:8080")
		if err != nil {
			fmt.Printf("❌ Failed to connect: %v\n", err)
			continue
		}

		// Start the client
		client.Start()

		// Generate test data
		testData := strings.Repeat("A", test.size)

		// Send the text - this should automatically chunk if > 250KB
		fmt.Printf("📤 Sending %d bytes...\n", test.size)
		err = client.SendText(testData)
		if err != nil {
			fmt.Printf("❌ Failed to send: %v\n", err)
		} else {
			fmt.Printf("✅ Send completed successfully\n")
		}

		// Give time for any messages to complete
		time.Sleep(200 * time.Millisecond)

		client.Stop()
	}

	// Test with secure client
	fmt.Printf("\n🔐 Testing with Secure Client\n")
	fmt.Println("=" + strings.Repeat("=", 60))

	// Start secure server
	secureServer, err := udp_network.NewSecureServer(":8081")
	if err != nil {
		panic(fmt.Sprintf("Failed to create secure server: %v", err))
	}
	defer secureServer.Stop()
	secureServer.Start()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test large secure transfer
	secureClient, err := udp_network.NewSecureClient()
	if err != nil {
		fmt.Printf("❌ Failed to create secure client: %v\n", err)
		return
	}
	defer secureClient.Close()

	err = secureClient.Connect("localhost:8081")
	if err != nil {
		fmt.Printf("❌ Failed to connect secure client: %v\n", err)
		return
	}

	// Test small and large strings
	smallText := strings.Repeat("B", 1024)     // 1KB
	largeText := strings.Repeat("C", 500*1024) // 500KB

	fmt.Printf("\n📤 Sending small text (1KB) via secure client...\n")
	err = secureClient.SendText(smallText)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ Small text sent successfully\n")
	}

	time.Sleep(100 * time.Millisecond)

	fmt.Printf("\n📤 Sending large text (500KB) via secure client...\n")
	err = secureClient.SendText(largeText)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ Large text sent successfully (should have used chunking)\n")
	}

	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n✨ Auto-chunking test completed!")
	fmt.Println("\nNote: Check the console output above to see when chunking was automatically triggered.")
}

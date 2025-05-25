package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/DangerosoDavo/udp_network"
)

func main() {
	fmt.Println("🚀 Testing automatic chunking for large strings (v2)")
	fmt.Println("=" + strings.Repeat("=", 60))

	// Test 1: Regular client and server
	fmt.Println("\n📡 Test 1: Regular Client/Server")
	fmt.Println("-" + strings.Repeat("-", 40))

	server, err := udp_network.NewServer(":8080")
	if err != nil {
		panic(fmt.Sprintf("Failed to create server: %v", err))
	}
	defer server.Stop()
	server.Start()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test different sizes
	testSizes := []struct {
		name string
		size int
	}{
		{"Small (1KB)", 1024},
		{"Medium (100KB)", 100 * 1024},
		{"At threshold (250KB)", 250 * 1024},
		{"Large (300KB)", 300 * 1024},
		{"Very large (1MB)", 1024 * 1024},
	}

	for _, test := range testSizes {
		fmt.Printf("\n  Testing %s:\n", test.name)

		client, err := udp_network.NewClient()
		if err != nil {
			fmt.Printf("    ❌ Failed to create client: %v\n", err)
			continue
		}

		client.Start()
		err = client.Connect("localhost:8080")
		if err != nil {
			fmt.Printf("    ❌ Failed to connect: %v\n", err)
			client.Stop()
			continue
		}

		// Generate test data
		testData := strings.Repeat("A", test.size)

		// Send the text
		fmt.Printf("    📤 Sending %d bytes...\n", test.size)
		err = client.SendText(testData)
		if err != nil {
			fmt.Printf("    ❌ Failed to send: %v\n", err)
		} else {
			fmt.Printf("    ✅ Sent successfully\n")
			if test.size > 250*1024 {
				fmt.Printf("    📦 (Should have used automatic chunking)\n")
			}
		}

		// Give time for transmission
		time.Sleep(200 * time.Millisecond)
		client.Stop()
	}

	// Test 2: Secure client and server
	fmt.Println("\n\n🔐 Test 2: Secure Client/Server")
	fmt.Println("-" + strings.Repeat("-", 40))

	secureServer, err := udp_network.NewSecureServer(":8081")
	if err != nil {
		panic(fmt.Sprintf("Failed to create secure server: %v", err))
	}
	defer secureServer.Stop()
	secureServer.Start()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	for _, test := range testSizes {
		fmt.Printf("\n  Testing %s:\n", test.name)

		client, err := udp_network.NewSecureClient()
		if err != nil {
			fmt.Printf("    ❌ Failed to create secure client: %v\n", err)
			continue
		}

		err = client.Connect("localhost:8081")
		if err != nil {
			fmt.Printf("    ❌ Failed to connect: %v\n", err)
			client.Close()
			continue
		}

		// Generate test data
		testData := strings.Repeat("B", test.size)

		// Send the text
		fmt.Printf("    🔐📤 Sending %d bytes securely...\n", test.size)
		err = client.SendText(testData)
		if err != nil {
			fmt.Printf("    ❌ Failed to send: %v\n", err)
		} else {
			fmt.Printf("    ✅ Sent successfully\n")
			if test.size > 250*1024 {
				fmt.Printf("    🔐📦 (Should have used automatic chunking)\n")
			}
		}

		// Give time for transmission
		time.Sleep(200 * time.Millisecond)
		client.Close()
	}

	fmt.Println("\n\n✨ Test completed!")
	fmt.Println("\nNote: Check console output for automatic chunking messages when size > 250KB")
}

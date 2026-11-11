package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/DangerosoDavo/udp_network"
)

func startServer() {
	fmt.Println("🔓 Starting server with large string support...")

	server, err := udp_network.NewServer(":8091")
	if err != nil {
		log.Fatal(err)
	}

	// Enable large string receiver
	receiver := udp_network.EnableLargeStringReceiver(server)

	// Set handler for completed transfers
	receiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
		fmt.Printf("\n✅ Received complete large string!\n")
		fmt.Printf("   Size: %.2f MB (%d bytes)\n", float64(len(data))/(1024*1024), len(data))
		fmt.Printf("   First 50 chars: %s\n", data[:min(50, len(data))])
		fmt.Printf("   Metadata: %+v\n", metadata)
	})

	server.Start()
	defer server.Stop()

	fmt.Println("✅ Server ready to receive large strings on :8091")

	// Keep running
	select {}
}

func sendLargeString() {
	fmt.Println("\n🔓 Connecting to server...")

	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	client.Start()
	defer client.Stop()

	if err := client.Connect("localhost:8091"); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	fmt.Println("✅ Connected!")

	// Create a large string (1MB)
	fmt.Println("\n📝 Creating 1MB test string...")
	largeString := strings.Repeat("Hello, this is a test of large string transfer! ", 21845) // ~1MB
	fmt.Printf("✅ Created string: %d bytes (%.2f MB)\n", len(largeString), float64(len(largeString))/(1024*1024))

	// Send it
	fmt.Println("\n📤 Sending large string...")

	err = client.SendLargeString(largeString, &udp_network.LargeStringOptions{
		Metadata: map[string]interface{}{
			"filename": "test.txt",
			"type":     "text",
		},
		OnProgress: func(sent, total int) {
			fmt.Printf("\r📊 Progress: %d/%d chunks (%.1f%%)", sent, total, float64(sent)/float64(total)*100)
			if sent == total {
				fmt.Println() // New line after completion
			}
		},
	})

	if err != nil {
		log.Fatal("Failed to send:", err)
	}

	fmt.Println("✅ Large string sent successfully!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	fmt.Println("UDP Network Large String Example")
	fmt.Println("================================")
	fmt.Println()
	fmt.Println("This example demonstrates sending strings larger than")
	fmt.Println("the normal UDP packet size limit using chunked transfer.")
	fmt.Println()

	// Run server in background
	go startServer()
	time.Sleep(2 * time.Second)

	// Send large string
	sendLargeString()

	// Keep running to see results
	time.Sleep(3 * time.Second)
}

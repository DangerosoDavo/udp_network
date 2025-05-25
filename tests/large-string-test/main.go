package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

var (
	mode     = flag.String("mode", "both", "Test mode: send, receive, or both")
	size     = flag.Float64("size", 1.0, "Size in MB of test string")
	port     = flag.String("port", "8090", "Port to use")
	secure   = flag.Bool("secure", false, "Use secure connection")
	compress = flag.Bool("compress", false, "Compress before sending (not implemented)")
)

func generateLargeString(sizeMB float64) string {
	sizeBytes := int(sizeMB * 1024 * 1024)

	// Generate a realistic text pattern
	pattern := "The quick brown fox jumps over the lazy dog. "
	patternLen := len(pattern)

	// Calculate repetitions needed
	repetitions := sizeBytes / patternLen
	remainder := sizeBytes % patternLen

	// Build string efficiently
	var builder strings.Builder
	builder.Grow(sizeBytes)

	for i := 0; i < repetitions; i++ {
		builder.WriteString(pattern)
	}

	if remainder > 0 {
		builder.WriteString(pattern[:remainder])
	}

	return builder.String()
}

func runServer() {
	address := ":" + *port

	if *secure {
		fmt.Printf("🔐 Starting secure large string server on %s...\n", address)
		server, err := udp_network.NewSecureServer(address)
		if err != nil {
			log.Fatal(err)
		}

		// Enable large string receiver
		receiver := udp_network.NewLargeStringReceiver()

		// Set default handler for all transfers
		receiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
			fmt.Printf("\n✅ Received complete large string!\n")
			fmt.Printf("   Size: %.2f MB\n", float64(len(data))/(1024*1024))
			fmt.Printf("   First 100 chars: %s...\n", data[:min(100, len(data))])
			fmt.Printf("   Last 100 chars: ...%s\n", data[max(0, len(data)-100):])

			if metadata != nil {
				fmt.Printf("   Metadata: %+v\n", metadata)
			}
		})

		// Custom message handler
		server.OnMessage(func(conn *protocol.Connection, rawData []byte) {
			// For secure server, we need to parse the message
			handler := protocol.NewMessageHandler(nil)
			msg, err := handler.ParseMessage(rawData)
			if err == nil && msg.Type == protocol.MessageTypeChunked {
				receiver.ProcessChunkedMessage(msg)
			}
		})

		server.Start()
		defer server.Stop()

		fmt.Println("✅ Secure server ready to receive large strings")

	} else {
		fmt.Printf("🔓 Starting large string server on %s...\n", address)
		server, err := udp_network.NewServer(address)
		if err != nil {
			log.Fatal(err)
		}

		// Enable large string receiver
		receiver := udp_network.EnableLargeStringReceiver(server)

		// Set default handler for all transfers
		receiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
			fmt.Printf("\n✅ Received complete large string!\n")
			fmt.Printf("   Size: %.2f MB\n", float64(len(data))/(1024*1024))
			fmt.Printf("   First 100 chars: %s...\n", data[:min(100, len(data))])
			fmt.Printf("   Last 100 chars: ...%s\n", data[max(0, len(data)-100):])

			if metadata != nil {
				fmt.Printf("   Metadata: %+v\n", metadata)
			}
		})

		server.Start()
		defer server.Stop()

		fmt.Println("✅ Server ready to receive large strings")
	}

	// Keep running
	select {}
}

func runClient() {
	serverAddr := "localhost:" + *port

	// Generate large string
	fmt.Printf("📝 Generating %.1f MB test string...\n", *size)
	largeString := generateLargeString(*size)
	fmt.Printf("✅ Generated %d bytes (%.2f MB)\n", len(largeString), float64(len(largeString))/(1024*1024))

	// Calculate expected chunks (using 300KB chunk size)
	chunkSize := 300 * 1024
	expectedChunks := (len(largeString) + chunkSize - 1) / chunkSize
	fmt.Printf("📦 Will send in %d chunks\n", expectedChunks)

	if *secure {
		fmt.Printf("\n🔐 Connecting to secure server at %s...\n", serverAddr)
		client, err := udp_network.NewSecureClient()
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		if err := client.Connect(serverAddr); err != nil {
			log.Fatal("Failed to connect:", err)
		}
		fmt.Println("✅ Secure connection established")

		// Send large string
		fmt.Printf("\n📤 Sending %.1f MB string...\n", *size)
		startTime := time.Now()

		err = client.SendLargeString(largeString, &udp_network.LargeStringOptions{
			Metadata: map[string]interface{}{
				"sender": "test-client",
				"type":   "test-data",
				"size":   len(largeString),
			},
			OnProgress: func(sent, total int) {
				progress := float64(sent) / float64(total) * 100
				fmt.Printf("\r📊 Progress: %d/%d chunks (%.1f%%)", sent, total, progress)
			},
		})

		if err != nil {
			log.Fatal("\n❌ Failed to send:", err)
		}

		duration := time.Since(startTime)
		throughput := float64(len(largeString)) / duration.Seconds() / (1024 * 1024)

		fmt.Printf("\n✅ Large string sent successfully!\n")
		fmt.Printf("   Time: %v\n", duration)
		fmt.Printf("   Throughput: %.2f MB/s\n", throughput)

	} else {
		fmt.Printf("\n🔓 Connecting to server at %s...\n", serverAddr)
		client, err := udp_network.NewClient()
		if err != nil {
			log.Fatal(err)
		}

		client.Start()
		defer client.Stop()

		if err := client.Connect(serverAddr); err != nil {
			log.Fatal("Failed to connect:", err)
		}
		fmt.Println("✅ Connection established")

		// Send large string
		fmt.Printf("\n📤 Sending %.1f MB string...\n", *size)
		startTime := time.Now()

		err = client.SendLargeString(largeString, &udp_network.LargeStringOptions{
			Metadata: map[string]interface{}{
				"sender": "test-client",
				"type":   "test-data",
				"size":   len(largeString),
			},
			OnProgress: func(sent, total int) {
				progress := float64(sent) / float64(total) * 100
				fmt.Printf("\r📊 Progress: %d/%d chunks (%.1f%%)", sent, total, progress)
			},
		})

		if err != nil {
			log.Fatal("\n❌ Failed to send:", err)
		}

		duration := time.Since(startTime)
		throughput := float64(len(largeString)) / duration.Seconds() / (1024 * 1024)

		fmt.Printf("\n✅ Large string sent successfully!\n")
		fmt.Printf("   Time: %v\n", duration)
		fmt.Printf("   Throughput: %.2f MB/s\n", throughput)
	}
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

	fmt.Println("UDP Network Large String Test")
	fmt.Println("=============================")
	fmt.Printf("Mode: %s\n", *mode)
	fmt.Printf("Size: %.1f MB\n", *size)
	fmt.Printf("Port: %s\n", *port)
	fmt.Printf("Secure: %v\n", *secure)

	switch *mode {
	case "server", "receive":
		runServer()

	case "client", "send":
		runClient()

	case "both":
		// Run server in background
		go runServer()
		time.Sleep(2 * time.Second) // Give server time to start

		// Run client
		runClient()

		// Give time to complete
		time.Sleep(2 * time.Second)

	default:
		fmt.Printf("Invalid mode: %s\n", *mode)
		fmt.Println("Use: send, receive, or both")
	}
}

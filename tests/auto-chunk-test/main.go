package main

import (
	"fmt"
	"github.com/DangerosoDavo/udp_network/internal/exampleconfig"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("🚀 Testing automatic chunking for large strings")
	fmt.Println("=" + strings.Repeat("=", 60))

	// Test configurations
	tests := []struct {
		name        string
		size        int
		useSecure   bool
		shouldChunk bool
	}{
		{"Small text (100 bytes)", 100, false, false},
		{"Medium text (100KB)", 100 * 1024, false, false},
		{"Large text (300KB)", 300 * 1024, false, true},
		{"Very large text (1MB)", 1024 * 1024, false, true},
		{"Small secure text (100 bytes)", 100, true, false},
		{"Large secure text (500KB)", 500 * 1024, true, true},
	}

	// Track received messages
	var regularTextReceived atomic.Int32
	var regularChunkReceived atomic.Int32
	var secureTextReceived atomic.Int32
	var secureChunkReceived atomic.Int32

	// Start regular server
	server, err := udp_network.NewServer(":8080")
	if err != nil {
		panic(fmt.Sprintf("Failed to create server: %v", err))
	}
	defer server.Stop()

	// Start secure server
	secureServer, err := udp_network.NewSecureServerWithPSK(":8081", exampleconfig.PSK())
	if err != nil {
		panic(fmt.Sprintf("Failed to create secure server: %v", err))
	}
	defer secureServer.Stop()

	// Enable large string receivers
	regularReceiver := udp_network.EnableLargeStringReceiver(server)
	secureReceiver := udp_network.EnableLargeStringReceiverSecure(secureServer)

	// Set handlers for complete large strings
	regularReceiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
		fmt.Printf("📨 Regular server received complete large string: %d bytes\n", len(data))
	})

	secureReceiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
		fmt.Printf("🔐📨 Secure server received complete large string: %d bytes\n", len(data))
	})

	// Message handlers for regular server
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeText {
			text := msg.Data.(string)
			regularTextReceived.Add(1)
			fmt.Printf("📨 Regular server received text message: %d bytes\n", len(text))
		} else if msg.Type == protocol.MessageTypeChunked {
			regularChunkReceived.Add(1)
			fmt.Printf("📦 Regular server received chunked message part\n")
			// Process through the receiver
			regularReceiver.ProcessChunkedMessage(msg)
		}
	})

	// Message handlers for secure server
	secureServer.OnMessage(func(conn *protocol.Connection, data []byte) {
		// Parse the message
		handler := protocol.NewMessageHandler(secureServer.GetTransport().Transport)
		msg, err := handler.ParseMessage(data)
		if err != nil {
			fmt.Printf("Error parsing message: %v\n", err)
			return
		}

		if msg.Type == protocol.MessageTypeText {
			text := msg.Data.(string)
			secureTextReceived.Add(1)
			fmt.Printf("🔐📨 Secure server received text message: %d bytes\n", len(text))
		} else if msg.Type == protocol.MessageTypeChunked {
			secureChunkReceived.Add(1)
			fmt.Printf("🔐📦 Secure server received chunked message part\n")
			// Process through the receiver
			secureReceiver.ProcessChunkedMessage(msg)
		}
	})

	server.Start()
	secureServer.Start()
	time.Sleep(100 * time.Millisecond)

	// Run tests
	for _, test := range tests {
		fmt.Printf("\n🧪 Test: %s\n", test.name)
		fmt.Println("-" + strings.Repeat("-", 40))

		// Reset counters
		regularTextReceived.Store(0)
		regularChunkReceived.Store(0)
		secureTextReceived.Store(0)
		secureChunkReceived.Store(0)

		// Generate test data
		testData := strings.Repeat("A", test.size)

		if test.useSecure {
			// Test secure client
			client, err := udp_network.NewSecureClientWithPSK(exampleconfig.PSK())
			if err != nil {
				fmt.Printf("❌ Failed to create secure client: %v\n", err)
				continue
			}

			err = client.Connect("localhost:8081")
			if err != nil {
				fmt.Printf("❌ Failed to connect secure client: %v\n", err)
				client.Close()
				continue
			}

			fmt.Printf("📤 Sending %d bytes via secure client...\n", test.size)
			err = client.SendText(testData)
			if err != nil {
				fmt.Printf("❌ Failed to send: %v\n", err)
			}

			// Give time for chunks to be received
			if test.shouldChunk {
				time.Sleep(500 * time.Millisecond)
			} else {
				time.Sleep(100 * time.Millisecond)
			}

			client.Close()

			// Verify results
			if test.shouldChunk {
				if secureChunkReceived.Load() > 0 {
					fmt.Printf("✅ Correctly used chunking (received %d chunks)\n", secureChunkReceived.Load())
				} else {
					fmt.Printf("❌ Expected chunking but none received\n")
				}
			} else {
				if secureTextReceived.Load() == 1 {
					fmt.Printf("✅ Correctly sent as single message\n")
				} else {
					fmt.Printf("❌ Expected single message but got %d\n", secureTextReceived.Load())
				}
			}
		} else {
			// Test regular client
			client, err := udp_network.NewClient()
			if err != nil {
				fmt.Printf("❌ Failed to create client: %v\n", err)
				continue
			}

			err = client.Connect("localhost:8080")
			if err != nil {
				fmt.Printf("❌ Failed to connect client: %v\n", err)
				client.Stop()
				continue
			}

			// Start the client
			client.Start()

			fmt.Printf("📤 Sending %d bytes via regular client...\n", test.size)
			err = client.SendText(testData)
			if err != nil {
				fmt.Printf("❌ Failed to send: %v\n", err)
			}

			// Give time for chunks to be received
			if test.shouldChunk {
				time.Sleep(500 * time.Millisecond)
			} else {
				time.Sleep(100 * time.Millisecond)
			}

			client.Stop()

			// Verify results
			if test.shouldChunk {
				if regularChunkReceived.Load() > 0 {
					fmt.Printf("✅ Correctly used chunking (received %d chunks)\n", regularChunkReceived.Load())
				} else {
					fmt.Printf("❌ Expected chunking but none received\n")
				}
			} else {
				if regularTextReceived.Load() == 1 {
					fmt.Printf("✅ Correctly sent as single message\n")
				} else {
					fmt.Printf("❌ Expected single message but got %d\n", regularTextReceived.Load())
				}
			}
		}
	}

	fmt.Println("\n✨ Auto-chunking test completed!")
}

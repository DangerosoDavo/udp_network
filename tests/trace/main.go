package main

import (
	"fmt"
	"log"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== Message Flow Trace Test ===")

	// Create server
	server, err := udp_network.NewServer(":6666")
	if err != nil {
		log.Fatal(err)
	}

	// Set handler BEFORE starting
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		fmt.Printf("[SERVER] Received message from %s: %v\n", conn.Addr, msg.Data)
	})

	fmt.Println("Starting server...")
	server.Start()
	time.Sleep(100 * time.Millisecond)

	// Create client
	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Starting client...")
	client.Start()

	// Connect
	fmt.Println("Connecting...")
	err = client.Connect("localhost:6666")
	if err != nil {
		log.Fatal("Connect failed:", err)
	}

	// Wait for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	// Try different message types
	fmt.Println("\nSending messages...")

	// 1. Send via SendText
	fmt.Println("1. Sending via SendText...")
	err = client.SendText("Test via SendText")
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Println("   Sent OK")
	}
	time.Sleep(100 * time.Millisecond)

	// 2. Send via SendMessage
	fmt.Println("2. Sending via SendMessage...")
	msg := &protocol.Message{
		Type:      protocol.MessageTypeText,
		Timestamp: time.Now().Unix(),
		Data:      "Test via SendMessage",
	}
	err = client.SendMessage(client.Connection, msg)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Println("   Sent OK")
	}
	time.Sleep(100 * time.Millisecond)

	// 3. Send JSON
	fmt.Println("3. Sending JSON message...")
	jsonMsg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"test": "json data",
			"num":  42,
		},
	}
	err = client.SendMessage(client.Connection, jsonMsg)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Println("   Sent OK")
	}

	// Wait for messages
	time.Sleep(1 * time.Second)

	fmt.Println("\nShutting down...")
	client.Stop()
	server.Stop()
}

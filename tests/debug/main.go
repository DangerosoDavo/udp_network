package main

import (
	"fmt"
	"log"
	"time"
	
	"udp_network"
	"udp_network/protocol"
)

func main() {
	// Test 1: Basic server/client with detailed logging
	fmt.Println("=== UDP Protocol Debug Test ===")
	
	// Start server
	fmt.Println("\n1. Starting server on :7777...")
	server, err := udp_network.NewServer(":7777")
	if err != nil {
		log.Fatal(err)
	}
	
	server.Start()
	time.Sleep(100 * time.Millisecond)
	
	// Start client
	fmt.Println("\n2. Starting client...")
	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	
	client.Start()
	
	// Connect
	fmt.Println("\n3. Connecting to server...")
	err = client.Connect("localhost:7777")
	if err != nil {
		log.Fatal("Connect error:", err)
	}
	fmt.Println("Connected successfully!")
	
	// Wait for connection to stabilize
	time.Sleep(500 * time.Millisecond)
	
	// Send a simple message
	fmt.Println("\n4. Sending test message...")
	msg := &protocol.Message{
		Type:      protocol.MessageTypeText,
		Timestamp: time.Now().Unix(),
		Data:      "Hello from debug test!",
	}
	
	err = client.SendMessage(client.Connection, msg)
	if err != nil {
		log.Printf("Send error: %v", err)
	} else {
		fmt.Println("Message sent successfully!")
	}
	
	// Wait and check
	time.Sleep(1 * time.Second)
	
	fmt.Println("\n5. Shutting down...")
	client.Stop()
	server.Stop()
	
	fmt.Println("\nTest complete!")
}
package main

import (
	"fmt"
	"log"
	"time"
	
	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== Simple Secure Communication Test ===\n")
	
	// Create secure server
	server, err := udpnet.NewSecureServer(":9989")
	if err != nil {
		log.Fatal("Failed to create server:", err)
	}
	
	messageReceived := make(chan string, 1)
	
	server.OnMessage(func(conn *protocol.Connection, data []byte) {
		fmt.Printf("Server received encrypted message: %s\n", string(data))
		
		// Echo back
		response := fmt.Sprintf("Echo: %s", string(data))
		if err := server.SendTo(conn, []byte(response)); err != nil {
			log.Printf("Failed to send response: %v", err)
		}
		
		select {
		case messageReceived <- string(data):
		default:
		}
	})
	
	server.Start()
	defer server.Stop()
	fmt.Println("Secure server running on :9989")
	
	// Give server time to start
	time.Sleep(200 * time.Millisecond)
	
	// Create secure client
	client, err := udpnet.NewSecureClient()
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}
	
	client.OnMessage(func(data []byte) {
		fmt.Printf("Client received response: %s\n", string(data))
	})
	
	fmt.Println("\nConnecting to server...")
	if err := client.Connect("localhost:9989"); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer client.Close()
	
	// Wait for connection and key exchange
	time.Sleep(200 * time.Millisecond)
	
	if !client.IsConnected() {
		log.Fatal("Client is not connected")
	}
	
	fmt.Println("Connected! Sending encrypted message...")
	
	// Send test message
	testMsg := "Hello, this is a secure message!"
	if err := client.Send([]byte(testMsg)); err != nil {
		log.Fatal("Failed to send:", err)
	}
	
	// Wait for server to receive
	select {
	case received := <-messageReceived:
		if received == testMsg {
			fmt.Println("\n✓ SUCCESS: Message was transmitted securely!")
		} else {
			fmt.Printf("\n✗ ERROR: Message mismatch\nSent: %s\nReceived: %s\n", testMsg, received)
		}
	case <-time.After(2 * time.Second):
		fmt.Println("\n✗ ERROR: Message not received within timeout")
	}
	
	// Give time for response
	time.Sleep(500 * time.Millisecond)
	
	fmt.Println("\nSecure communication test completed!")
}
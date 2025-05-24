package main

import (
	"fmt"
	"log"
	"time"

	"udp_network/protocol"
)

func main() {
	fmt.Println("=== Simple Direct Test ===")

	// Create transport directly
	serverTransport, err := protocol.NewTransport(":5555")
	if err != nil {
		log.Fatal(err)
	}

	clientTransport, err := protocol.NewTransport(":0")
	if err != nil {
		log.Fatal(err)
	}

	// Set up server handler
	serverTransport.OnConnect = func(conn *protocol.Connection) {
		fmt.Printf("[SERVER] Client connected: %s\n", conn.Addr)

		conn.SetMessageCallback(func(data []byte) {
			fmt.Printf("[SERVER] Received: %s\n", string(data))
		})
	}

	// Start both
	fmt.Println("Starting transports...")
	serverTransport.Start()
	clientTransport.Start()

	time.Sleep(100 * time.Millisecond)

	// Connect
	fmt.Println("Connecting...")
	conn, err := clientTransport.Connect("localhost:5555")
	if err != nil {
		log.Fatal("Connect failed:", err)
	}
	fmt.Printf("Connected! State: %v\n", conn.GetState())

	// Try to send
	fmt.Println("\nSending data...")
	err = clientTransport.Send(conn, []byte("Hello from client"))
	if err != nil {
		fmt.Printf("Send error: %v\n", err)
	}

	// Wait
	time.Sleep(1 * time.Second)

	fmt.Println("\nShutting down...")
	clientTransport.Stop()
	serverTransport.Stop()
}

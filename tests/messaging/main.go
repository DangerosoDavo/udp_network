package main

import (
	"fmt"
	"log"
	"time"

	"github.com/DangerosoDavo/udp_network"
)

func main() {
	// Start server
	fmt.Println("=== Testing Message Flow ===")
	fmt.Println("\n1. Starting server...")

	server, err := udp_network.NewServer(":9876")
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

	fmt.Println("\n3. Connecting...")
	err = client.Connect("localhost:9876")
	if err != nil {
		log.Fatal(err)
	}

	// Wait for connection
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n4. Sending text message...")
	err = client.SendText("Test message from client")
	if err != nil {
		log.Printf("SendText error: %v", err)
	}

	fmt.Println("\n5. Sending command...")
	err = client.SendCommand("stats", nil)
	if err != nil {
		log.Printf("SendCommand error: %v", err)
	}

	// Wait for messages
	time.Sleep(1 * time.Second)

	fmt.Println("\n6. Checking server state...")
	fmt.Printf("Server has %d clients\n", len(server.Clients()))

	fmt.Println("\nDone!")
	client.Stop()
	server.Stop()
}

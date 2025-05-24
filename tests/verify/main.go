package main

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"
	
	"udp_network"
	"udp_network/protocol"
)

func main() {
	fmt.Println("=== Server Message Reception Test ===")
	
	// Start server with counter
	var serverReceived uint64
	
	server, err := udp_network.NewServer(":4444")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("Setting message handler...")
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		count := atomic.AddUint64(&serverReceived, 1)
		fmt.Printf("[SERVER] Message #%d from %s: %v\n", count, conn.Addr, msg.Data)
		
		// Echo back
		response := &protocol.Message{
			Type:      protocol.MessageTypeText,
			Timestamp: time.Now().Unix(),
			Data:      fmt.Sprintf("ACK #%d", count),
		}
		server.SendMessage(conn, response)
	})
	
	fmt.Println("Starting server...")
	server.Start()
	
	time.Sleep(100 * time.Millisecond)
	
	// Start client
	var clientReceived uint64
	
	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	
	client.SetMessageHandler(func(msg *protocol.Message) {
		count := atomic.AddUint64(&clientReceived, 1)
		fmt.Printf("[CLIENT] Response #%d: %v\n", count, msg.Data)
	})
	
	fmt.Println("Starting client...")
	client.Start()
	
	fmt.Println("Connecting...")
	err = client.Connect("localhost:4444")
	if err != nil {
		log.Fatal(err)
	}
	
	// Send test messages
	fmt.Println("\nSending test messages...")
	for i := 1; i <= 5; i++ {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeText,
			Timestamp: time.Now().Unix(),
			Data:      fmt.Sprintf("Test message %d", i),
		}
		
		err := client.SendMessage(client.Connection, msg)
		if err != nil {
			fmt.Printf("Send error: %v\n", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	
	// Wait and report
	time.Sleep(500 * time.Millisecond)
	
	fmt.Printf("\nResults:\n")
	fmt.Printf("Server received: %d messages\n", serverReceived)
	fmt.Printf("Client received: %d responses\n", clientReceived)
	
	client.Stop()
	server.Stop()
}
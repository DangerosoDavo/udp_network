package main

import (
	"fmt"
	"log"
	"time"
	
	"udp_network"
	"udp_network/protocol"
)

func main() {
	// Start server
	fmt.Println("Starting echo server...")
	server, err := udp_network.NewServer(":9999")
	if err != nil {
		log.Fatal(err)
	}
	
	msgCount := 0
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		msgCount++
		fmt.Printf("Server received message #%d: %v\n", msgCount, msg.Data)
		
		// Echo back
		response := &protocol.Message{
			Type:      protocol.MessageTypeText,
			Timestamp: time.Now().Unix(),
			Data:      fmt.Sprintf("Echo: %v", msg.Data),
		}
		server.SendMessage(conn, response)
	})
	
	server.Start()
	
	// Give server time to start
	time.Sleep(100 * time.Millisecond)
	
	// Start client
	fmt.Println("\nStarting client...")
	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	
	client.Start()
	
	fmt.Println("Connecting to server...")
	if err := client.Connect("localhost:9999"); err != nil {
		log.Fatal(err)
	}
	
	ackCount := 0
	client.SetMessageHandler(func(msg *protocol.Message) {
		ackCount++
		fmt.Printf("Client received response #%d: %v\n", ackCount, msg.Data)
	})
	
	// Send some test messages
	fmt.Println("\nSending test messages...")
	for i := 1; i <= 5; i++ {
		msg := &protocol.Message{
			Type:      protocol.MessageTypeText,
			Timestamp: time.Now().Unix(),
			Data:      fmt.Sprintf("Test message %d", i),
		}
		
		fmt.Printf("Sending: %v\n", msg.Data)
		if err := client.SendMessage(client.Connection, msg); err != nil {
			log.Printf("Send error: %v", err)
		}
		
		time.Sleep(100 * time.Millisecond)
	}
	
	// Wait for responses
	time.Sleep(1 * time.Second)
	
	fmt.Printf("\nResults: Sent 5, Server received %d, Client received %d ACKs\n", msgCount, ackCount)
	
	client.Stop()
	server.Stop()
}
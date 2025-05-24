package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
	
	"udp_network"
	"udp_network/protocol"
)

func main() {
	fmt.Println("=== UDP Protocol Scalability Test ===")
	
	// Start server
	server, err := udp_network.NewServer(":7777")
	if err != nil {
		log.Fatal(err)
	}
	
	// Configure for scalability
	server.SetMaxClients(1000) // Limit connections
	
	var messageCount uint64
	var clientCount int32
	
	// Track messages
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		atomic.AddUint64(&messageCount, 1)
	})
	
	fmt.Println("Starting server with 1000 connection limit...")
	server.Start()
	
	// Monitor connections
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			count := server.GetClientCount()
			msgs := atomic.LoadUint64(&messageCount)
			fmt.Printf("[Monitor] Active connections: %d, Total messages: %d\n", count, msgs)
		}
	}()
	
	// Simulate many clients
	fmt.Println("\nSimulating 100 concurrent clients...")
	var wg sync.WaitGroup
	
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			client, err := udp_network.NewClient()
			if err != nil {
				return
			}
			
			client.Start()
			defer client.Stop()
			
			if err := client.Connect("localhost:7777"); err != nil {
				return
			}
			
			atomic.AddInt32(&clientCount, 1)
			
			// Send a few messages
			for j := 0; j < 10; j++ {
				msg := fmt.Sprintf("Client %d message %d", id, j)
				client.SendText(msg)
				time.Sleep(100 * time.Millisecond)
			}
			
			// Disconnect
			atomic.AddInt32(&clientCount, -1)
		}(i)
		
		// Stagger client connections
		time.Sleep(50 * time.Millisecond)
	}
	
	// Wait for clients
	wg.Wait()
	
	// Final stats
	time.Sleep(2 * time.Second)
	fmt.Printf("\nFinal stats:\n")
	fmt.Printf("Total messages received: %d\n", atomic.LoadUint64(&messageCount))
	fmt.Printf("Active connections: %d\n", server.GetClientCount())
	
	server.Stop()
}
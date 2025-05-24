package main

import (
	"log"
	"github.com/DangerosoDavo/udp_network"
)

func main() {
	log.Println("Starting debug server...")
	
	server, err := udp_network.NewServer(":8080")
	if err != nil {
		log.Fatal(err)
	}
	
	// Add extra logging
	log.Println("Server created, starting...")
	
	server.Start()
	defer server.Stop()
	
	log.Println("UDP server running on :8080")
	log.Println("Waiting for connections...")
	
	// Also log at transport level
	if server.Transport() != nil {
		log.Println("Transport is initialized")
	}
	
	// Keep running
	select {}
}
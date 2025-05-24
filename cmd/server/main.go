package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"github.com/DangerosoDavo/udp_network"
)

func main() {
	log.Println("Starting UDP server...")
	
	// Get port from environment or use default
	port := os.Getenv("UDP_PORT")
	if port == "" {
		port = "8080"
	}
	address := ":" + port
	
	server, err := udp_network.NewServer(address)
	if err != nil {
		log.Fatal("Failed to create server:", err)
	}
	
	server.Start()
	defer server.Stop()
	
	log.Printf("UDP server running on %s", address)
	log.Println("Ready for connections...")
	
	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	<-c
	log.Println("Shutting down server...")
}
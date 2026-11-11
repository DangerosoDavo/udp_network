package main

import (
	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/internal/exampleconfig"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("Starting secure UDP server with ChaCha20-Poly1305 encryption...")

	// Get port from environment or use default
	port := os.Getenv("UDP_PORT")
	if port == "" {
		port = "8443"
	}
	address := ":" + port

	server, err := udp_network.NewSecureServerWithPSK(address, exampleconfig.PSK())
	if err != nil {
		log.Fatal("Failed to create secure server:", err)
	}

	server.Start()
	defer server.Stop()

	log.Printf("Secure UDP server running on %s", address)
	log.Println("All communications encrypted with ChaCha20-Poly1305")
	log.Println("Ready for secure connections...")

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	log.Println("Shutting down secure server...")
}

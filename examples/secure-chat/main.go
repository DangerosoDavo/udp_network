package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/DangerosoDavo/udp_network/internal/exampleconfig"
	"log"
	"os"
	"strings"
	"time"

	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	var isServer bool
	var serverAddr string

	flag.BoolVar(&isServer, "server", false, "Run as server")
	flag.StringVar(&serverAddr, "addr", "localhost:9999", "Server address")
	flag.Parse()

	if isServer {
		runSecureServer(serverAddr)
	} else {
		runSecureClient(serverAddr)
	}
}

func runSecureServer(addr string) {
	server, err := udpnet.NewSecureServerWithPSK(addr, exampleconfig.PSK())
	if err != nil {
		log.Fatal("Failed to create secure server:", err)
	}

	server.OnMessage(func(conn *protocol.Connection, data []byte) {
		msg := string(data)
		fmt.Printf("[%s] %s\n", conn.Addr.String(), msg)

		// Echo back with encryption
		response := fmt.Sprintf("Echo: %s", msg)
		if err := server.SendTo(conn, []byte(response)); err != nil {
			log.Printf("Failed to send response: %v", err)
		}
	})

	server.Start()
	defer server.Stop()

	fmt.Println("Secure server running on", addr)
	fmt.Println("Encryption: ChaCha20-Poly1305")
	fmt.Println("Waiting for encrypted connections...")

	// Keep server running
	select {}
}

func runSecureClient(serverAddr string) {
	client, err := udpnet.NewSecureClientWithPSK(exampleconfig.PSK())
	if err != nil {
		log.Fatal("Failed to create secure client:", err)
	}
	defer client.Close()

	fmt.Println("Connecting to secure server at", serverAddr)
	if err := client.Connect(serverAddr); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	if !client.IsConnected() {
		log.Fatal("Failed to establish secure connection")
	}

	fmt.Println("Connected! Encryption: ChaCha20-Poly1305")
	fmt.Println("Type messages to send (encrypted):")

	// Handle incoming messages
	client.OnMessage(func(data []byte) {
		fmt.Printf("Server: %s\n", string(data))
	})

	// Read user input
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		msg := strings.TrimSpace(scanner.Text())
		if msg == "" {
			continue
		}

		if err := client.Send([]byte(msg)); err != nil {
			log.Printf("Failed to send: %v", err)
		}
	}
}

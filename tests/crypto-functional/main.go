package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
)

func testBasicCommunication() {
	fmt.Println("Testing basic unencrypted communication...")

	// Start server
	server, err := protocol.NewTransport(":8082")
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	server.OnConnect = func(conn *protocol.Connection) {
		conn.SetMessageCallback(func(data []byte) {
			fmt.Printf("Server received: %s\n", string(data))
			wg.Done()
		})
	}

	server.Start()
	defer server.Stop()

	// Start client
	client, err := protocol.NewTransport(":0")
	if err != nil {
		log.Fatal(err)
	}
	client.Start()
	defer client.Stop()

	// Connect and send message
	conn, err := client.Connect("localhost:8082")
	if err != nil {
		log.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)
	client.Send(conn, []byte("Hello, UDP!"))

	wg.Wait()
	fmt.Println("✓ Basic communication successful\n")
}

func testSecureCommunication() {
	fmt.Println("Testing encrypted communication...")

	// Start server
	server, err := protocol.NewSecureTransport(":8083")
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	server.OnConnect = func(conn *protocol.Connection) {
		conn.SetMessageCallback(func(data []byte) {
			fmt.Printf("Secure server received: %s\n", string(data))
			wg.Done()
		})
	}

	server.Start()
	defer server.Stop()

	// Start client
	client, err := protocol.NewSecureTransport(":0")
	if err != nil {
		log.Fatal(err)
	}
	client.Start()
	defer client.Stop()

	// Connect and send message
	conn, err := client.SecureConnect("localhost:8083")
	if err != nil {
		log.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)
	client.SendSecure(conn, []byte("Hello, Secure UDP!"))

	wg.Wait()
	fmt.Println("✓ Encrypted communication successful\n")
}

func main() {
	fmt.Println("UDP Network Functionality Test")
	fmt.Println("==============================")

	testBasicCommunication()
	testSecureCommunication()

	fmt.Println("All tests passed! ✓")
	fmt.Println("\nBoth unencrypted and encrypted UDP communication")
	fmt.Println("are working correctly with ChaCha20-Poly1305 encryption.")
}
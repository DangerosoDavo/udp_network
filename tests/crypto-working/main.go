package main

import (
	"fmt"
	"log"
	"time"
	
	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== Working Crypto Test ===")
	
	// For now, let's use regular server/client and manually encrypt
	server, err := udpnet.NewServer(":9994")
	if err != nil {
		log.Fatal(err)
	}
	
	// Create crypto states
	serverCrypto, _ := protocol.NewCryptoState()
	var clientCrypto *protocol.CryptoState
	
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		fmt.Printf("Server received: %v\n", msg.Data)
		
		// Echo back
		reply := &protocol.Message{
			Type: protocol.MessageTypeText,
			Data: fmt.Sprintf("Echo: %v", msg.Data),
		}
		server.SendMessage(conn, reply)
	})
	
	server.Start()
	defer server.Stop()
	
	time.Sleep(200 * time.Millisecond)
	
	// Create client
	client, err := udpnet.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	
	clientCrypto, _ = protocol.NewCryptoState()
	
	client.SetMessageHandler(func(msg *protocol.Message) {
		fmt.Printf("Client received: %v\n", msg.Data)
	})
	
	client.Start()
	defer client.Stop()
	
	if err := client.Connect("localhost:9994"); err != nil {
		log.Fatal(err)
	}
	
	time.Sleep(200 * time.Millisecond)
	
	// Exchange keys manually
	fmt.Println("\nManual key exchange:")
	fmt.Printf("Server public key: %x\n", serverCrypto.GetPublicKey())
	fmt.Printf("Client public key: %x\n", clientCrypto.GetPublicKey())
	
	// Set remote keys
	serverCrypto.SetRemotePublicKey(clientCrypto.GetPublicKey())
	clientCrypto.SetRemotePublicKey(serverCrypto.GetPublicKey())
	
	fmt.Printf("Server crypto established: %v\n", serverCrypto.IsEstablished())
	fmt.Printf("Client crypto established: %v\n", clientCrypto.IsEstablished())
	
	// Test encryption/decryption
	testData := []byte("Hello, encrypted world!")
	encrypted, err := clientCrypto.Encrypt(testData)
	if err != nil {
		log.Fatal("Encrypt failed:", err)
	}
	fmt.Printf("\nOriginal: %s\n", testData)
	fmt.Printf("Encrypted size: %d bytes\n", len(encrypted))
	
	decrypted, err := serverCrypto.Decrypt(encrypted)
	if err != nil {
		log.Fatal("Decrypt failed:", err)
	}
	fmt.Printf("Decrypted: %s\n", decrypted)
	
	// Send normal message
	client.SendText("Test message over regular connection")
	
	time.Sleep(500 * time.Millisecond)
	
	fmt.Println("\nTest completed successfully!")
}
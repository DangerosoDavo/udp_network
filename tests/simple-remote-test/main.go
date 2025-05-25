package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/DangerosoDavo/udp_network"
)

var (
	host   = flag.String("host", "", "Server hostname or IP address")
	port   = flag.String("port", "8080", "Server port (8080 for regular, 8443 for secure)")
	secure = flag.Bool("secure", false, "Use secure connection")
	count  = flag.Int("count", 5, "Number of messages to send")
)

func main() {
	flag.Parse()

	if *host == "" {
		fmt.Println("Simple UDP Network Remote Test")
		fmt.Println("=============================")
		fmt.Println()
		fmt.Println("Usage: go run tests/simple-remote-test/main.go -host <ip> [options]")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  go run tests/simple-remote-test/main.go -host 192.168.1.100")
		fmt.Println("  go run tests/simple-remote-test/main.go -host 192.168.1.100 -port 8443 -secure")
		fmt.Println()
		log.Fatal("Host is required")
	}

	address := *host + ":" + *port
	fmt.Printf("Testing %s connection to %s\n", map[bool]string{true: "secure", false: "regular"}[*secure], address)

	if *secure {
		// Test secure server
		fmt.Println("🔐 Creating secure client...")
		client, err := udp_network.NewSecureClient()
		if err != nil {
			log.Fatal("Failed to create secure client:", err)
		}
		defer client.Close()

		fmt.Printf("🔗 Connecting to secure server at %s...\n", address)
		if err := client.Connect(address); err != nil {
			log.Fatal("Failed to connect:", err)
		}
		fmt.Println("✅ Secure connection established!")

		// Send test messages
		for i := 1; i <= *count; i++ {
			message := fmt.Sprintf("Secure test message %d from remote client", i)
			fmt.Printf("📤 Sending encrypted message %d: %s\n", i, message)
			
			if err := client.SendText(message); err != nil {
				fmt.Printf("❌ Failed to send message %d: %v\n", i, err)
			} else {
				fmt.Printf("✅ Message %d sent successfully\n", i)
			}
			
			time.Sleep(500 * time.Millisecond)
		}

	} else {
		// Test regular server
		fmt.Println("🔓 Creating regular client...")
		client, err := udp_network.NewClient()
		if err != nil {
			log.Fatal("Failed to create client:", err)
		}
		
		client.Start()
		defer client.Stop()

		fmt.Printf("🔗 Connecting to server at %s...\n", address)
		if err := client.Connect(address); err != nil {
			log.Fatal("Failed to connect:", err)
		}
		fmt.Println("✅ Connection established!")

		// Send test messages
		for i := 1; i <= *count; i++ {
			message := fmt.Sprintf("Test message %d from remote client", i)
			fmt.Printf("📤 Sending message %d: %s\n", i, message)
			
			if err := client.SendText(message); err != nil {
				fmt.Printf("❌ Failed to send message %d: %v\n", i, err)
			} else {
				fmt.Printf("✅ Message %d sent successfully\n", i)
			}
			
			time.Sleep(500 * time.Millisecond)
		}

		// Test some server commands
		fmt.Println("\n🔧 Testing server commands...")
		commands := []string{"ping", "time", "stats"}
		
		for _, cmd := range commands {
			fmt.Printf("📤 Sending command: %s\n", cmd)
			if err := client.SendText(cmd); err != nil {
				fmt.Printf("❌ Command failed: %v\n", err)
			} else {
				fmt.Printf("✅ Command sent successfully\n")
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	fmt.Println("\n🎉 Test completed!")
	fmt.Println("Check your server logs to verify the messages were received.")
}
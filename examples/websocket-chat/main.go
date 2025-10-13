package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"

	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	server := flag.Bool("server", false, "Run as server")
	addr := flag.String("addr", ":8080", "Address to listen/connect")
	flag.Parse()

	if *server {
		runServer(*addr)
	} else {
		runClient(*addr)
	}
}

func runServer(addr string) {
	// Create WebSocket server using unified interface
	server, err := udpnet.NewUnifiedServer(addr, &udpnet.ServerOptions{
		Transport:     udpnet.TransportWebSocket,
		MaxClients:    100,
		EnableLogging: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("🌐 WebSocket Chat Server starting on %s\n", addr)
	fmt.Println("Connect with: go run main.go -addr", addr)
	fmt.Println("Or from browser: ws://"+addr)

	// Handle incoming messages
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		switch msg.Type {
		case protocol.MessageTypeText:
			text, ok := msg.Data.(string)
			if !ok {
				return
			}

			log.Printf("[Chat] %s: %s", conn.Addr.String(), text)

			// Broadcast to all clients
			broadcast := &protocol.Message{
				Type:      protocol.MessageTypeJSON,
				Timestamp: msg.Timestamp,
				Data: map[string]interface{}{
					"type":    "chat",
					"from":    conn.Addr.String(),
					"message": text,
				},
			}
			server.Broadcast(broadcast)

		case protocol.MessageTypeJSON:
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if cmd, ok := data["command"].(string); ok {
					handleCommand(server, conn, cmd, data)
				}
			}
		}
	})

	// Connection callbacks
	server.SetConnectionCallbacks(
		func(conn *protocol.Connection) {
			log.Printf("✅ Client connected: %s", conn.Addr.String())
			count := server.GetClientCount()

			// Notify all clients
			announcement := &protocol.Message{
				Type: protocol.MessageTypeJSON,
				Data: map[string]interface{}{
					"type":    "system",
					"event":   "user_joined",
					"message": fmt.Sprintf("User joined. Total users: %d", count),
				},
			}
			server.Broadcast(announcement)
		},
		func(conn *protocol.Connection) {
			log.Printf("❌ Client disconnected: %s", conn.Addr.String())
			count := server.GetClientCount()

			// Notify remaining clients
			announcement := &protocol.Message{
				Type: protocol.MessageTypeJSON,
				Data: map[string]interface{}{
					"type":    "system",
					"event":   "user_left",
					"message": fmt.Sprintf("User left. Total users: %d", count),
				},
			}
			server.Broadcast(announcement)
		},
	)

	server.Start()
	defer server.Stop()

	fmt.Println("\nServer running. Press Ctrl+C to stop.")
	select {} // Block forever
}

func handleCommand(server udpnet.UnifiedServer, conn *protocol.Connection, cmd string, data map[string]interface{}) {
	switch cmd {
	case "list_users":
		clients := server.Clients()
		users := make([]string, 0, len(clients))
		for addr := range clients {
			users = append(users, addr)
		}

		response := &protocol.Message{
			Type: protocol.MessageTypeJSON,
			Data: map[string]interface{}{
				"type":  "user_list",
				"users": users,
				"count": len(users),
			},
		}
		server.SendMessage(conn, response)

	case "stats":
		response := &protocol.Message{
			Type: protocol.MessageTypeJSON,
			Data: map[string]interface{}{
				"type":      "stats",
				"clients":   server.GetClientCount(),
				"transport": server.GetTransportType(),
			},
		}
		server.SendMessage(conn, response)
	}
}

func runClient(addr string) {
	// Create WebSocket client using unified interface
	client, err := udpnet.NewUnifiedClient(&udpnet.ClientOptions{
		Transport:     udpnet.TransportWebSocket,
		EnableLogging: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Add ws:// if not present for WebSocket
	if len(addr) > 0 && addr[0] != 'w' {
		addr = "ws://" + addr
	}

	err = client.Connect(addr)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Stop()

	fmt.Println("🌐 Connected to WebSocket Chat Server")
	fmt.Println("\nCommands:")
	fmt.Println("  /users    - List online users")
	fmt.Println("  /stats    - Get server stats")
	fmt.Println("  /quit     - Exit")
	fmt.Println("\nType messages to chat:")

	// Handle incoming messages
	client.SetMessageHandler(func(msg *protocol.Message) {
		switch msg.Type {
		case protocol.MessageTypeJSON:
			if data, ok := msg.Data.(map[string]interface{}); ok {
				msgType, _ := data["type"].(string)

				switch msgType {
				case "chat":
					from, _ := data["from"].(string)
					message, _ := data["message"].(string)
					fmt.Printf("\n[%s]: %s\n> ", from, message)

				case "system":
					message, _ := data["message"].(string)
					fmt.Printf("\n[System]: %s\n> ", message)

				case "user_list":
					users, _ := data["users"].([]interface{})
					fmt.Printf("\n📋 Online users (%d):\n", len(users))
					for _, user := range users {
						fmt.Printf("  - %v\n", user)
					}
					fmt.Print("> ")

				case "stats":
					clients, _ := data["clients"].(float64)
					transport, _ := data["transport"].(string)
					fmt.Printf("\n📊 Server Stats:\n")
					fmt.Printf("  Clients: %.0f\n", clients)
					fmt.Printf("  Transport: %s\n", transport)
					fmt.Print("> ")
				}
			}
		}
	})

	// Interactive loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")

	for scanner.Scan() {
		input := scanner.Text()

		if input == "" {
			fmt.Print("> ")
			continue
		}

		switch input {
		case "/quit":
			return

		case "/users":
			client.SendCommand("list_users", nil)

		case "/stats":
			client.SendCommand("stats", nil)

		default:
			if err := client.SendText(input); err != nil {
				fmt.Printf("Error sending: %v\n", err)
			}
		}

		fmt.Print("> ")
	}
}

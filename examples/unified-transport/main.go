package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	server := flag.Bool("server", false, "Run as server")
	addr := flag.String("addr", ":8080", "Address to listen/connect")
	transport := flag.String("transport", "udp", "Transport type: udp or websocket")
	flag.Parse()

	// Determine transport type
	var transportType udpnet.TransportType
	switch *transport {
	case "websocket", "ws":
		transportType = udpnet.TransportWebSocket
	case "udp":
		transportType = udpnet.TransportUDP
	default:
		log.Fatalf("Invalid transport: %s (use 'udp' or 'websocket')", *transport)
	}

	if *server {
		runServer(*addr, transportType)
	} else {
		runClient(*addr, transportType)
	}
}

func runServer(addr string, transportType udpnet.TransportType) {
	fmt.Printf("🚀 Starting server with %s transport on %s\n", transportType, addr)

	// Create server with specified transport
	server, err := udpnet.NewUnifiedServer(addr, &udpnet.ServerOptions{
		Transport:     transportType,
		MaxClients:    1000,
		EnableLogging: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Set up message handler (same code works for both transports!)
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		switch msg.Type {
		case protocol.MessageTypeText:
			text := msg.Data.(string)
			log.Printf("[%s] %s: %s", server.GetTransportType(), conn.Addr.String(), text)

			// Echo back with metadata
			response := &protocol.Message{
				Type: protocol.MessageTypeJSON,
				Data: map[string]interface{}{
					"echo":      text,
					"transport": server.GetTransportType(),
					"timestamp": time.Now().Unix(),
					"length":    len(text),
				},
			}
			server.SendMessage(conn, response)

		case protocol.MessageTypeCommand:
			cmd := msg.Data.(string)
			if cmd == "ping" {
				response := &protocol.Message{
					Type: protocol.MessageTypeText,
					Data: "pong",
				}
				server.SendMessage(conn, response)
			}

		case protocol.MessageTypeJSON:
			if data, ok := msg.Data.(map[string]interface{}); ok {
				handleServerCommand(server, conn, data)
			}
		}
	})

	// Connection callbacks
	server.SetConnectionCallbacks(
		func(conn *protocol.Connection) {
			log.Printf("✅ [%s] Client connected: %s (total: %d)",
				server.GetTransportType(), conn.Addr.String(), server.GetClientCount())
		},
		func(conn *protocol.Connection) {
			log.Printf("❌ [%s] Client disconnected: %s (total: %d)",
				server.GetTransportType(), conn.Addr.String(), server.GetClientCount())
		},
	)

	server.Start()
	defer server.Stop()

	fmt.Printf("\n✅ Server running with %s transport\n", transportType)
	fmt.Printf("   Connect with: go run main.go -transport %s -addr %s\n", transportType, addr)
	fmt.Println("\nPress Ctrl+C to stop.")

	select {} // Block forever
}

func handleServerCommand(server udpnet.UnifiedServer, conn *protocol.Connection, data map[string]interface{}) {
	cmd, ok := data["command"].(string)
	if !ok {
		return
	}

	switch cmd {
	case "stats":
		response := &protocol.Message{
			Type: protocol.MessageTypeJSON,
			Data: map[string]interface{}{
				"clients":   server.GetClientCount(),
				"transport": server.GetTransportType(),
				"timestamp": time.Now().Unix(),
			},
		}
		server.SendMessage(conn, response)

	case "broadcast":
		message, ok := data["message"].(string)
		if !ok {
			return
		}

		broadcast := &protocol.Message{
			Type: protocol.MessageTypeJSON,
			Data: map[string]interface{}{
				"type":      "broadcast",
				"from":      conn.Addr.String(),
				"message":   message,
				"transport": server.GetTransportType(),
			},
		}
		server.Broadcast(broadcast)
	}
}

func runClient(addr string, transportType udpnet.TransportType) {
	fmt.Printf("🔌 Connecting with %s transport to %s\n", transportType, addr)

	// Adjust address format for WebSocket
	if transportType == udpnet.TransportWebSocket && len(addr) > 0 && addr[0] != 'w' {
		addr = "ws://" + addr
	}

	// Create client with specified transport
	client, err := udpnet.NewUnifiedClient(&udpnet.ClientOptions{
		Transport:     transportType,
		EnableLogging: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	err = client.Connect(addr)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Stop()

	fmt.Printf("\n✅ Connected via %s\n", transportType)
	fmt.Println("\nCommands:")
	fmt.Println("  /ping           - Ping server")
	fmt.Println("  /stats          - Get server stats")
	fmt.Println("  /broadcast <msg> - Broadcast message")
	fmt.Println("  /quit           - Exit")
	fmt.Println("\nType messages to send:")

	// Handle incoming messages (same code for both transports!)
	client.SetMessageHandler(func(msg *protocol.Message) {
		switch msg.Type {
		case protocol.MessageTypeText:
			fmt.Printf("\n[Server]: %s\n> ", msg.Data)

		case protocol.MessageTypeJSON:
			if data, ok := msg.Data.(map[string]interface{}); ok {
				handleClientMessage(data)
			}
		}
	})

	// Interactive loop
	client.Start()
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")

	for scanner.Scan() {
		input := scanner.Text()

		if input == "" {
			fmt.Print("> ")
			continue
		}

		// Handle commands
		if input[0] == '/' {
			parts := []rune(input)
			cmd := string(parts[1:])

			switch {
			case cmd == "quit":
				return

			case cmd == "ping":
				msg := &protocol.Message{
					Type: protocol.MessageTypeCommand,
					Data: "ping",
				}
				client.SendMessage(nil, msg)

			case cmd == "stats":
				client.SendCommand("stats", nil)

			case len(cmd) > 10 && string(parts[1:11]) == "broadcast ":
				message := string(parts[11:])
				client.SendCommand("broadcast", map[string]interface{}{
					"message": message,
				})

			default:
				fmt.Printf("Unknown command: %s\n", cmd)
			}
		} else {
			// Send as text
			if err := client.SendText(input); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}

		fmt.Print("> ")
	}
}

func handleClientMessage(data map[string]interface{}) {
	msgType, _ := data["type"].(string)

	switch msgType {
	case "broadcast":
		from, _ := data["from"].(string)
		message, _ := data["message"].(string)
		transport, _ := data["transport"].(string)
		fmt.Printf("\n[Broadcast via %s from %s]: %s\n> ", transport, from, message)

	default:
		// Generic JSON display
		fmt.Printf("\n[Server JSON]: %v\n> ", data)
	}
}

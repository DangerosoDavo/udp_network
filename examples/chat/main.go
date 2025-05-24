package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	var (
		serverMode = flag.Bool("server", false, "Run as server")
		serverAddr = flag.String("addr", "localhost:8080", "Server address")
	)
	flag.Parse()

	if *serverMode {
		runChatServer(*serverAddr)
	} else {
		runChatClient(*serverAddr)
	}
}

func runChatServer(addr string) {
	fmt.Println("Starting UDP Chat Server...")

	server, err := udp_network.NewServer(addr)
	if err != nil {
		log.Fatal(err)
	}

	// Set message handler for chat functionality
	chatHandler := func(conn *protocol.Connection, msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeText {
			// Broadcast chat messages to all clients
			chatMsg := &protocol.Message{
				Type:      protocol.MessageTypeJSON,
				Timestamp: time.Now().Unix(),
				Data: map[string]interface{}{
					"type":    "chat",
					"from":    conn.Addr.String(),
					"message": msg.Data,
					"time":    time.Now().Format("15:04:05"),
				},
			}

			server.Broadcast(chatMsg)
		}
	}

	server.SetMessageHandler(chatHandler)

	server.Start()
	defer server.Stop()

	fmt.Printf("Chat server listening on %s\n", addr)

	// Keep running
	select {}
}

func runChatClient(serverAddr string) {
	fmt.Println("UDP Chat Client")
	fmt.Println("Type messages to send to the chat room")
	fmt.Println("Commands: /quit to exit")
	fmt.Println("----------------------------------------")

	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	client.Start()
	defer client.Stop()

	// Override message handler for chat display
	client.SetMessageHandler(func(msg *protocol.Message) {
		if msg.Type == protocol.MessageTypeJSON {
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if msgType, ok := data["type"].(string); ok && msgType == "chat" {
					from := data["from"].(string)
					message := data["message"].(string)
					time := data["time"].(string)

					fmt.Printf("\r[%s] %s: %s\n> ", time, from, message)
				}
			}
		}
	})

	if err := client.Connect(serverAddr); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Connected to %s\n> ", serverAddr)

	client.RunInteractive()
}

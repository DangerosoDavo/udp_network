package main

import (
	"fmt"
	"log"
	"time"
	
	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== Example Game Using UDP Network Module ===")
	
	// Create game server
	server, err := udpnet.NewServer(":9999")
	if err != nil {
		log.Fatal(err)
	}
	
	// Configure for game use case
	server.SetKeepAlive(10*time.Minute, 1*time.Minute)
	server.SetMaxClients(100)
	
	// Game state
	type Player struct {
		Name  string
		Score int
	}
	
	players := make(map[string]*Player)
	
	// Handle connections
	server.SetConnectionCallbacks(
		func(conn *protocol.Connection) {
			playerID := conn.Addr.String()
			players[playerID] = &Player{
				Name:  fmt.Sprintf("Player%d", len(players)+1),
				Score: 0,
			}
			log.Printf("Player joined: %s", players[playerID].Name)
			
			// Send welcome
			welcome := &protocol.Message{
				Type: protocol.MessageTypeJSON,
				Data: map[string]interface{}{
					"event": "welcome",
					"name":  players[playerID].Name,
				},
			}
			server.SendMessage(conn, welcome)
		},
		func(conn *protocol.Connection) {
			playerID := conn.Addr.String()
			if player, ok := players[playerID]; ok {
				log.Printf("Player left: %s (score: %d)", player.Name, player.Score)
				delete(players, playerID)
			}
		},
	)
	
	// Handle game messages
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		playerID := conn.Addr.String()
		player := players[playerID]
		
		if player == nil {
			return
		}
		
		switch msg.Type {
		case protocol.MessageTypeText:
			// Chat message
			log.Printf("[%s] %v", player.Name, msg.Data)
			
		case protocol.MessageTypeJSON:
			// Game action
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if action, ok := data["action"].(string); ok {
					switch action {
					case "score":
						player.Score++
						response := &protocol.Message{
							Type: protocol.MessageTypeJSON,
							Data: map[string]interface{}{
								"event": "score_update",
								"score": player.Score,
							},
						}
						server.SendMessage(conn, response)
					}
				}
			}
		}
	})
	
	fmt.Println("Game server starting on :9999...")
	server.Start()
	
	// Create a test client
	go func() {
		time.Sleep(1 * time.Second)
		
		client, err := udpnet.NewClient()
		if err != nil {
			log.Printf("Client error: %v", err)
			return
		}
		
		client.Start()
		defer client.Stop()
		
		if err := client.Connect("localhost:9999"); err != nil {
			log.Printf("Connect error: %v", err)
			return
		}
		
		// Send some test messages
		client.SendText("Hello from test client!")
		
		time.Sleep(500 * time.Millisecond)
		
		// Send game action
		client.SendMessage(client.Connection, &protocol.Message{
			Type: protocol.MessageTypeJSON,
			Data: map[string]interface{}{
				"action": "score",
			},
		})
		
		time.Sleep(2 * time.Second)
	}()
	
	// Run for 5 seconds
	time.Sleep(5 * time.Second)
	
	fmt.Println("\nShutting down...")
	server.Stop()
	
	fmt.Println("Example complete!")
}
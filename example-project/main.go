package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	udpnet "github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== Example Game Using UDP Network Module ===")

	// Create game server
	server, err := udpnet.NewServer("127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	// Configure for game use case
	server.SetKeepAlive(10*time.Minute, 1*time.Minute)
	server.SetMaxClients(100)

	// Game state
	type Player struct {
		Name  string
		Score atomic.Int64
	}

	var players sync.Map
	var nextPlayer atomic.Uint64

	// Handle connections
	server.SetConnectionCallbacks(
		func(conn *protocol.Connection) {
			playerID := conn.Addr.String()
			player := &Player{Name: fmt.Sprintf("Player%d", nextPlayer.Add(1))}
			players.Store(playerID, player)
			log.Printf("Player joined: %s", player.Name)

			// Send welcome
			welcome := &protocol.Message{
				Type: protocol.MessageTypeJSON,
				Data: map[string]interface{}{
					"event": "welcome",
					"name":  player.Name,
				},
			}
			server.SendMessage(conn, welcome)
		},
		func(conn *protocol.Connection) {
			playerID := conn.Addr.String()
			if value, ok := players.LoadAndDelete(playerID); ok {
				player := value.(*Player)
				log.Printf("Player left: %s (score: %d)", player.Name, player.Score.Load())
			}
		},
	)

	// Handle game messages
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		playerID := conn.Addr.String()
		value, exists := players.Load(playerID)
		if !exists {
			return
		}
		player := value.(*Player)

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
						score := player.Score.Add(1)
						response := &protocol.Message{
							Type: protocol.MessageTypeJSON,
							Data: map[string]interface{}{
								"event": "score_update",
								"score": score,
							},
						}
						server.SendMessage(conn, response)
					}
				}
			}
		}
	})

	fmt.Printf("Game server starting on %s...\n", server.Transport().LocalAddr())
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

		if err := client.Connect(server.Transport().LocalAddr().String()); err != nil {
			log.Printf("Connect error: %v", err)
			return
		}

		// Send some test messages
		client.SendText("Hello from test client!")

		time.Sleep(500 * time.Millisecond)

		// Send game action
		client.SendMessage(client.CurrentConnection(), &protocol.Message{
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

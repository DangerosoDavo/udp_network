package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

// Player represents a connected game player
type Player struct {
	ID         string
	Name       string
	Connection *protocol.Connection
	JoinedAt   time.Time
	LastAction time.Time
}

// GameServer wraps the UDP server with game logic
type GameServer struct {
	server    *udp_network.Server
	players   map[string]*Player
	playersMu sync.RWMutex
}

func NewGameServer(addr string) (*GameServer, error) {
	server, err := udp_network.NewServer(addr)
	if err != nil {
		return nil, err
	}

	gs := &GameServer{
		server:  server,
		players: make(map[string]*Player),
	}

	// Configure for persistent connections
	// Set 5 minute timeout for game connections
	server.SetKeepAlive(5*time.Minute, 30*time.Second)

	// Set up connection callbacks for game logic
	server.SetConnectionCallbacks(
		gs.onPlayerConnect,
		gs.onPlayerDisconnect,
	)

	// Handle game messages
	server.SetMessageHandler(gs.handleGameMessage)

	return gs, nil
}

func (gs *GameServer) onPlayerConnect(conn *protocol.Connection) {
	playerID := conn.Addr.String()

	gs.playersMu.Lock()
	player := &Player{
		ID:         playerID,
		Name:       fmt.Sprintf("Player_%d", len(gs.players)+1),
		Connection: conn,
		JoinedAt:   time.Now(),
		LastAction: time.Now(),
	}
	gs.players[playerID] = player
	gs.playersMu.Unlock()

	log.Printf("[GAME] Player connected: %s (%s)", player.Name, playerID)

	// Send welcome message
	welcome := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"event":     "welcome",
			"player_id": player.ID,
			"name":      player.Name,
			"message":   "Welcome to the game server!",
		},
	}
	gs.server.SendMessage(conn, welcome)

	// Notify other players
	gs.broadcastEvent("player_joined", map[string]interface{}{
		"player_id": player.ID,
		"name":      player.Name,
	}, player.ID)
}

func (gs *GameServer) onPlayerDisconnect(conn *protocol.Connection) {
	playerID := conn.Addr.String()

	gs.playersMu.Lock()
	player, exists := gs.players[playerID]
	if exists {
		delete(gs.players, playerID)
		gs.playersMu.Unlock()

		log.Printf("[GAME] Player disconnected: %s (%s) - was connected for %v",
			player.Name, playerID, time.Since(player.JoinedAt))

		// Notify other players
		gs.broadcastEvent("player_left", map[string]interface{}{
			"player_id": player.ID,
			"name":      player.Name,
		}, player.ID)

		// Here you would handle game cleanup:
		// - Remove from active games
		// - Save player state
		// - Clean up resources
	} else {
		gs.playersMu.Unlock()
	}
}

func (gs *GameServer) handleGameMessage(conn *protocol.Connection, msg *protocol.Message) {
	playerID := conn.Addr.String()

	gs.playersMu.RLock()
	player, exists := gs.players[playerID]
	gs.playersMu.RUnlock()

	if !exists {
		return
	}

	// Update last action time
	player.LastAction = time.Now()

	// Handle different message types
	switch msg.Type {
	case protocol.MessageTypeCommand:
		if cmd, ok := msg.Data.(string); ok {
			switch cmd {
			case "heartbeat":
				// Client heartbeat - connection stays alive
				// No response needed

			case "status":
				gs.sendPlayerStatus(player)

			case "list_players":
				gs.sendPlayerList(player)
			}
		}

	case protocol.MessageTypeJSON:
		if data, ok := msg.Data.(map[string]interface{}); ok {
			if action, ok := data["action"].(string); ok {
				gs.handleGameAction(player, action, data)
			}
		}
	}
}

func (gs *GameServer) handleGameAction(player *Player, action string, data map[string]interface{}) {
	log.Printf("[GAME] Player %s action: %s", player.Name, action)

	// Example game actions
	switch action {
	case "move":
		// Handle player movement
		gs.broadcastEvent("player_moved", map[string]interface{}{
			"player_id": player.ID,
			"name":      player.Name,
			"position":  data["position"],
		}, "")

	case "chat":
		// Handle chat message
		if message, ok := data["message"].(string); ok {
			gs.broadcastEvent("chat", map[string]interface{}{
				"player_id": player.ID,
				"name":      player.Name,
				"message":   message,
			}, "")
		}
	}
}

func (gs *GameServer) sendPlayerStatus(player *Player) {
	status := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"event":          "status",
			"connected_time": time.Since(player.JoinedAt).Seconds(),
			"last_action":    player.LastAction.Format(time.RFC3339),
		},
	}
	gs.server.SendMessage(player.Connection, status)
}

func (gs *GameServer) sendPlayerList(player *Player) {
	gs.playersMu.RLock()
	playerList := make([]map[string]interface{}, 0, len(gs.players))
	for _, p := range gs.players {
		playerList = append(playerList, map[string]interface{}{
			"id":   p.ID,
			"name": p.Name,
		})
	}
	gs.playersMu.RUnlock()

	list := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"event":   "player_list",
			"players": playerList,
			"count":   len(playerList),
		},
	}
	gs.server.SendMessage(player.Connection, list)
}

func (gs *GameServer) broadcastEvent(event string, data map[string]interface{}, excludeID string) {
	msg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"event": event,
			"data":  data,
		},
	}

	gs.playersMu.RLock()
	defer gs.playersMu.RUnlock()

	for id, player := range gs.players {
		if id != excludeID {
			gs.server.SendMessage(player.Connection, msg)
		}
	}
}

func (gs *GameServer) Start() {
	gs.server.Start()

	// Monitor server stats
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			gs.playersMu.RLock()
			playerCount := len(gs.players)
			gs.playersMu.RUnlock()

			log.Printf("[GAME] Server stats: %d active players", playerCount)
		}
	}()
}

func main() {
	fmt.Println("=== UDP Game Server Example ===")
	fmt.Println("Demonstrates persistent connections for game scenarios")

	gameServer, err := NewGameServer(":8888")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nServer Configuration:")
	fmt.Println("- Connection timeout: 5 minutes")
	fmt.Println("- Keep-alive interval: 30 seconds")
	fmt.Println("- Client heartbeat: 5 seconds")
	fmt.Println("\nStarting game server on :8888...")

	gameServer.Start()

	// Keep running
	select {}
}

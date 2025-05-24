package udp_network

import (
	"log"
	"sync"
	"time"

	"udp_network/protocol"
)

type Server struct {
	transport     *protocol.Transport
	handler       *protocol.MessageHandler
	clients       map[string]*protocol.Connection
	startTime     time.Time
	clientsMu     sync.RWMutex
	customHandler func(*protocol.Connection, *protocol.Message)
}

func NewServer(address string) (*Server, error) {
	transport, err := protocol.NewTransport(address)
	if err != nil {
		return nil, err
	}

	s := &Server{
		transport: transport,
		handler:   protocol.NewMessageHandler(transport),
		clients:   make(map[string]*protocol.Connection),
		startTime: time.Now(),
	}

	// Set up callbacks
	s.transport.OnConnect = s.onClientConnect
	s.transport.OnDisconnect = s.onClientDisconnect

	return s, nil
}

func (s *Server) Start() {
	log.Println("Server starting...")
	s.transport.Start()
}

func (s *Server) Stop() {
	log.Println("Server shutting down...")
	s.transport.Stop()
}

func (s *Server) onClientConnect(conn *protocol.Connection) {
	clientID := conn.Addr.String()

	s.clientsMu.Lock()
	s.clients[clientID] = conn
	s.clientsMu.Unlock()

	log.Printf("Client connected: %s", clientID)

	// Set up message handler
	s.handler.SetMessageCallback(conn, func(msg *protocol.Message) {
		if s.customHandler != nil {
			s.customHandler(conn, msg)
		} else {
			s.handleMessage(conn, msg)
		}
	})

	// Send welcome message
	welcome := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"event":  "welcome",
			"server": "UDP Network Server v1.0",
			"time":   time.Now().Format(time.RFC3339),
		},
	}

	if err := s.handler.SendMessage(conn, welcome); err != nil {
		log.Printf("Error sending welcome: %v", err)
	}
}

func (s *Server) onClientDisconnect(conn *protocol.Connection) {
	clientID := conn.Addr.String()

	s.clientsMu.Lock()
	delete(s.clients, clientID)
	s.clientsMu.Unlock()

	log.Printf("Client disconnected: %s", clientID)
}

func (s *Server) handleMessage(conn *protocol.Connection, msg *protocol.Message) {
	clientID := conn.Addr.String()

	switch msg.Type {
	case protocol.MessageTypeText:
		// Echo text messages with stats
		if text, ok := msg.Data.(string); ok {
			log.Printf("[%s] Text: %s", clientID, text)

			response := &protocol.Message{
				Type:      protocol.MessageTypeJSON,
				ID:        msg.ID,
				Timestamp: time.Now().Unix(),
				Data: map[string]interface{}{
					"echo":       text,
					"length":     len(text),
					"client":     clientID,
					"serverTime": time.Now().Format(time.RFC3339),
				},
			}

			s.handler.SendMessage(conn, response)
		}

	case protocol.MessageTypeJSON:
		// Handle JSON commands
		if data, ok := msg.Data.(map[string]interface{}); ok {
			s.handleJSONCommand(conn, data)
		}

	case protocol.MessageTypeCommand:
		// Handle special commands
		if cmd, ok := msg.Data.(string); ok {
			s.handleCommand(conn, cmd)
		}
	}
}

func (s *Server) handleJSONCommand(conn *protocol.Connection, data map[string]interface{}) {
	if cmd, ok := data["command"].(string); ok {
		switch cmd {
		case "stats":
			s.sendStats(conn)
		case "broadcast":
			if message, ok := data["message"].(string); ok {
				s.broadcast(message, conn)
			}
		case "list_clients":
			s.sendClientList(conn)
		}
	}
}

func (s *Server) handleCommand(conn *protocol.Connection, cmd string) {
	switch cmd {
	case "ping":
		response := &protocol.Message{
			Type:      protocol.MessageTypeText,
			Timestamp: time.Now().Unix(),
			Data:      "pong",
		}
		s.handler.SendMessage(conn, response)

	case "time":
		response := &protocol.Message{
			Type:      protocol.MessageTypeText,
			Timestamp: time.Now().Unix(),
			Data:      time.Now().Format(time.RFC3339),
		}
		s.handler.SendMessage(conn, response)
	}
}

func (s *Server) sendStats(conn *protocol.Connection) {
	stats := map[string]interface{}{
		"clients":    len(s.clients),
		"uptime":     time.Since(s.startTime).Seconds(),
		"serverTime": time.Now().Format(time.RFC3339),
	}

	msg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data:      stats,
	}

	s.handler.SendMessage(conn, msg)
}

func (s *Server) sendClientList(conn *protocol.Connection) {
	clients := make([]string, 0, len(s.clients))
	for addr := range s.clients {
		clients = append(clients, addr)
	}

	msg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"clients": clients,
			"count":   len(clients),
		},
	}

	s.handler.SendMessage(conn, msg)
}

func (s *Server) broadcast(message string, sender *protocol.Connection) {
	senderID := sender.Addr.String()

	broadcastMsg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"type":    "broadcast",
			"from":    senderID,
			"message": message,
			"time":    time.Now().Format(time.RFC3339),
		},
	}

	// Send to all clients except sender
	s.clientsMu.RLock()
	recipients := 0
	for _, client := range s.clients {
		if client != sender {
			s.handler.SendMessage(client, broadcastMsg)
			recipients++
		}
	}
	s.clientsMu.RUnlock()

	// Confirm to sender
	confirmMsg := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"type":       "broadcast_sent",
			"recipients": recipients,
		},
	}

	s.handler.SendMessage(sender, confirmMsg)
}

// SetMessageHandler allows overriding the message handler
func (s *Server) SetMessageHandler(handler func(*protocol.Connection, *protocol.Message)) {
	// Store custom handler
	s.customHandler = handler
	
	// Update OnConnect to use custom handler
	s.transport.OnConnect = func(conn *protocol.Connection) {
		s.clientsMu.Lock()
		s.clients[conn.Addr.String()] = conn
		s.clientsMu.Unlock()
		
		log.Printf("Client connected: %s", conn.Addr.String())

		s.handler.SetMessageCallback(conn, func(msg *protocol.Message) {
			if s.customHandler != nil {
				s.customHandler(conn, msg)
			} else {
				s.handleMessage(conn, msg)
			}
		})
	}
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(msg *protocol.Message) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		s.handler.SendMessage(client, msg)
	}
}

// SendMessage sends a message to a specific client
func (s *Server) SendMessage(conn *protocol.Connection, msg *protocol.Message) error {
	return s.handler.SendMessage(conn, msg)
}

// Transport returns the underlying transport (for debugging)
func (s *Server) Transport() *protocol.Transport {
	return s.transport
}

// Clients returns the current client connections
func (s *Server) Clients() map[string]*protocol.Connection {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	
	// Return a copy to avoid race conditions
	clients := make(map[string]*protocol.Connection)
	for k, v := range s.clients {
		clients[k] = v
	}
	return clients
}

// SetMaxClients sets the maximum number of concurrent client connections
func (s *Server) SetMaxClients(max int) {
	s.transport.SetMaxConnections(max)
}

// GetClientCount returns the current number of connected clients
func (s *Server) GetClientCount() int {
	return s.transport.GetConnectionCount()
}

// SetKeepAlive configures connection timeout and keep-alive settings
func (s *Server) SetKeepAlive(timeout, interval time.Duration) {
	s.transport.ConnTimeout = timeout
	s.transport.KeepAliveInterval = interval
}

// SetConnectionCallbacks allows game logic to hook into connection events
func (s *Server) SetConnectionCallbacks(onConnect, onDisconnect func(*protocol.Connection)) {
	if onConnect != nil {
		existingConnect := s.transport.OnConnect
		s.transport.OnConnect = func(conn *protocol.Connection) {
			if existingConnect != nil {
				existingConnect(conn)
			}
			onConnect(conn)
		}
	}
	
	if onDisconnect != nil {
		existingDisconnect := s.transport.OnDisconnect
		s.transport.OnDisconnect = func(conn *protocol.Connection) {
			if existingDisconnect != nil {
				existingDisconnect(conn)
			}
			onDisconnect(conn)
		}
	}
}

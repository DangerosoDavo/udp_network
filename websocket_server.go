package udp_network

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
	"github.com/gorilla/websocket"
)

// WebSocketServer implements UnifiedServer using WebSockets
type WebSocketServer struct {
	address       string
	server        *http.Server
	upgrader      websocket.Upgrader
	clients       map[string]*WebSocketConnection
	clientsMu     sync.RWMutex
	handler       func(*protocol.Connection, *protocol.Message)
	onConnect     func(*protocol.Connection)
	onDisconnect  func(*protocol.Connection)
	maxClients    int
	timeout       time.Duration
	keepAlive     time.Duration
	startTime     time.Time
	codec         *protocol.MessageCodec
	enableLogging bool
}

// WebSocketConnection wraps a websocket connection to match protocol.Connection interface
type WebSocketConnection struct {
	*protocol.Connection
	ws           *websocket.Conn
	writeMu      sync.Mutex
	lastActivity time.Time
	server       *WebSocketServer
}

// NewWebSocketServer creates a new WebSocket server
func NewWebSocketServer(address string, options *ServerOptions) (*WebSocketServer, error) {
	if options == nil {
		options = &ServerOptions{}
	}

	s := &WebSocketServer{
		address:   address,
		clients:   make(map[string]*WebSocketConnection),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins - configure based on your security needs
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		maxClients:    options.MaxClients,
		timeout:       options.Timeout,
		keepAlive:     options.KeepAlive,
		codec:         protocol.NewMessageCodec(),
		enableLogging: options.EnableLogging,
	}

	// Default timeouts
	if s.timeout == 0 {
		s.timeout = 30 * time.Second
	}
	if s.keepAlive == 0 {
		s.keepAlive = 10 * time.Second
	}

	return s, nil
}

func (s *WebSocketServer) Start() {
	s.startTime = time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	s.server = &http.Server{
		Addr:    s.address,
		Handler: mux,
	}

	go func() {
		if s.enableLogging {
			log.Printf("WebSocket server starting on %s", s.address)
		}
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// Start maintenance loop
	go s.maintenanceLoop()
}

func (s *WebSocketServer) Stop() {
	if s.enableLogging {
		log.Println("WebSocket server shutting down...")
	}

	// Close all client connections
	s.clientsMu.Lock()
	for _, client := range s.clients {
		client.ws.Close()
	}
	s.clients = make(map[string]*WebSocketConnection)
	s.clientsMu.Unlock()

	// Shutdown HTTP server
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
}

func (s *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check connection limit
	s.clientsMu.RLock()
	currentClients := len(s.clients)
	s.clientsMu.RUnlock()

	if s.maxClients > 0 && currentClients >= s.maxClients {
		http.Error(w, "Connection limit reached", http.StatusServiceUnavailable)
		return
	}

	// Upgrade connection
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if s.enableLogging {
			log.Printf("WebSocket upgrade error: %v", err)
		}
		return
	}

	// Create connection wrapper
	remoteAddr := &net.UDPAddr{
		IP:   net.ParseIP(r.RemoteAddr),
		Port: 0, // WebSocket doesn't have meaningful port
	}
	if remoteAddr.IP == nil {
		// Fallback for address parsing
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		remoteAddr.IP = net.ParseIP(host)
	}

	conn := protocol.NewConnection(remoteAddr)
	conn.SetState(protocol.StateConnected)

	wsConn := &WebSocketConnection{
		Connection:   conn,
		ws:           ws,
		lastActivity: time.Now(),
		server:       s,
	}

	clientID := remoteAddr.String()
	s.clientsMu.Lock()
	s.clients[clientID] = wsConn
	s.clientsMu.Unlock()

	if s.enableLogging {
		log.Printf("WebSocket client connected: %s", clientID)
	}

	// Trigger onConnect callback
	if s.onConnect != nil {
		s.onConnect(conn)
	}

	// Start handling messages
	go wsConn.readLoop()

	// Send welcome message
	welcome := &protocol.Message{
		Type:      protocol.MessageTypeJSON,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"event":     "welcome",
			"server":    "UDP Network WebSocket Server v1.0",
			"time":      time.Now().Format(time.RFC3339),
			"transport": "websocket",
		},
	}
	s.SendMessage(conn, welcome)
}

func (wsConn *WebSocketConnection) readLoop() {
	defer func() {
		wsConn.ws.Close()
		wsConn.server.removeClient(wsConn)
	}()

	wsConn.ws.SetReadDeadline(time.Now().Add(wsConn.server.timeout))
	wsConn.ws.SetPongHandler(func(string) error {
		wsConn.ws.SetReadDeadline(time.Now().Add(wsConn.server.timeout))
		wsConn.lastActivity = time.Now()
		return nil
	})

	for {
		messageType, data, err := wsConn.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if wsConn.server.enableLogging {
					log.Printf("WebSocket read error: %v", err)
				}
			}
			break
		}

		wsConn.lastActivity = time.Now()
		wsConn.ws.SetReadDeadline(time.Now().Add(wsConn.server.timeout))

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			// Decode message
			msg, err := wsConn.server.codec.Decode(data)
			if err != nil {
				if wsConn.server.enableLogging {
					log.Printf("Message decode error: %v", err)
				}
				continue
			}

			// Handle message
			if wsConn.server.handler != nil {
				wsConn.server.handler(wsConn.Connection, msg)
			}
		}
	}
}

func (s *WebSocketServer) removeClient(wsConn *WebSocketConnection) {
	clientID := wsConn.Addr.String()

	s.clientsMu.Lock()
	delete(s.clients, clientID)
	s.clientsMu.Unlock()

	if s.enableLogging {
		log.Printf("WebSocket client disconnected: %s", clientID)
	}

	// Trigger onDisconnect callback
	if s.onDisconnect != nil {
		s.onDisconnect(wsConn.Connection)
	}
}

func (s *WebSocketServer) maintenanceLoop() {
	ticker := time.NewTicker(s.keepAlive)
	defer ticker.Stop()

	for range ticker.C {
		s.clientsMu.RLock()
		clients := make([]*WebSocketConnection, 0, len(s.clients))
		for _, client := range s.clients {
			clients = append(clients, client)
		}
		s.clientsMu.RUnlock()

		now := time.Now()
		for _, client := range clients {
			// Check for timeout
			if now.Sub(client.lastActivity) > s.timeout {
				client.ws.Close()
				continue
			}

			// Send ping
			client.writeMu.Lock()
			err := client.ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second))
			client.writeMu.Unlock()

			if err != nil {
				client.ws.Close()
			}
		}
	}
}

func (s *WebSocketServer) SetMessageHandler(handler func(*protocol.Connection, *protocol.Message)) {
	s.handler = handler
}

func (s *WebSocketServer) SendMessage(conn *protocol.Connection, msg *protocol.Message) error {
	// Find the WebSocket connection
	s.clientsMu.RLock()
	wsConn, exists := s.clients[conn.Addr.String()]
	s.clientsMu.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found")
	}

	// Encode message
	data, err := s.codec.Encode(msg)
	if err != nil {
		return fmt.Errorf("encode error: %w", err)
	}

	// Send via WebSocket
	wsConn.writeMu.Lock()
	defer wsConn.writeMu.Unlock()

	wsConn.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return wsConn.ws.WriteMessage(websocket.TextMessage, data)
}

func (s *WebSocketServer) Broadcast(msg *protocol.Message) {
	s.clientsMu.RLock()
	clients := make([]*WebSocketConnection, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.RUnlock()

	// Encode once
	data, err := s.codec.Encode(msg)
	if err != nil {
		return
	}

	// Send to all clients
	for _, client := range clients {
		client.writeMu.Lock()
		client.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		client.ws.WriteMessage(websocket.TextMessage, data)
		client.writeMu.Unlock()
	}
}

func (s *WebSocketServer) Clients() map[string]*protocol.Connection {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	clients := make(map[string]*protocol.Connection)
	for k, v := range s.clients {
		clients[k] = v.Connection
	}
	return clients
}

func (s *WebSocketServer) SetMaxClients(max int) {
	s.maxClients = max
}

func (s *WebSocketServer) GetClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

func (s *WebSocketServer) SetKeepAlive(timeout, interval time.Duration) {
	s.timeout = timeout
	s.keepAlive = interval
}

func (s *WebSocketServer) SetConnectionCallbacks(onConnect, onDisconnect func(*protocol.Connection)) {
	s.onConnect = onConnect
	s.onDisconnect = onDisconnect
}

func (s *WebSocketServer) GetTransportType() TransportType {
	return TransportWebSocket
}

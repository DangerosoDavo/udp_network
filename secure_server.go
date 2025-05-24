package udp_network

import (
	"fmt"
	"log"
	"sync"

	"github.com/DangerosoDavo/udp_network/protocol"
)

type SecureServer struct {
	transport *protocol.SecureTransport
	clients   map[string]*protocol.Connection
	clientsMu sync.RWMutex

	// Message handler
	messageHandler func(*protocol.Connection, []byte)
}

func NewSecureServer(address string) (*SecureServer, error) {
	transport, err := protocol.NewSecureTransport(address)
	if err != nil {
		return nil, err
	}

	s := &SecureServer{
		transport: transport,
		clients:   make(map[string]*protocol.Connection),
	}

	// Set up callbacks
	transport.OnConnect = s.onConnect
	transport.OnDisconnect = s.onDisconnect

	return s, nil
}

func (s *SecureServer) onConnect(conn *protocol.Connection) {
	s.clientsMu.Lock()
	s.clients[conn.Addr.String()] = conn
	s.clientsMu.Unlock()

	log.Printf("Secure client connected: %s", conn.Addr.String())

	// Set up message callback
	conn.SetMessageCallback(func(data []byte) {
		if s.messageHandler != nil {
			s.messageHandler(conn, data)
		}
	})
}

func (s *SecureServer) onDisconnect(conn *protocol.Connection) {
	s.clientsMu.Lock()
	delete(s.clients, conn.Addr.String())
	s.clientsMu.Unlock()

	log.Printf("Secure client disconnected: %s", conn.Addr.String())
}

func (s *SecureServer) Start() {
	s.transport.Start()
	log.Println("Secure UDP server started with ChaCha20-Poly1305 encryption")
}

func (s *SecureServer) Stop() {
	s.transport.Stop()
}

func (s *SecureServer) OnMessage(handler func(*protocol.Connection, []byte)) {
	s.messageHandler = handler
}

func (s *SecureServer) Broadcast(data []byte) error {
	s.clientsMu.RLock()
	clients := make([]*protocol.Connection, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.RUnlock()

	var errors []error
	for _, client := range clients {
		if err := s.transport.SendSecure(client, data); err != nil {
			errors = append(errors, fmt.Errorf("failed to send to %s: %w", client.Addr.String(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("broadcast errors: %v", errors)
	}

	return nil
}

func (s *SecureServer) SendTo(conn *protocol.Connection, data []byte) error {
	return s.transport.SendSecure(conn, data)
}

func (s *SecureServer) GetClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

func (s *SecureServer) SetMaxConnections(max int) {
	s.transport.SetMaxConnections(max)
}

func (s *SecureServer) GetTransport() *protocol.SecureTransport {
	return s.transport
}

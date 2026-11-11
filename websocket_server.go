package udp_network

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network/protocol"
	"github.com/gorilla/websocket"
)

var ErrBackpressure = errors.New("websocket outbound queue full")

// WebSocketServer uses one bounded writer per peer. SendMessage success means
// queued locally, not acknowledged by the remote application.
type WebSocketServer struct {
	address                 string
	server                  *http.Server
	listener                net.Listener
	upgrader                websocket.Upgrader
	clients                 map[string]*WebSocketConnection
	clientsMu               sync.RWMutex
	mu                      sync.RWMutex
	handler                 func(*protocol.Connection, *protocol.Message)
	onConnect, onDisconnect func(*protocol.Connection)
	maxClients              int
	timeout, keepAlive      time.Duration
	startTime               time.Time
	codec                   *protocol.MessageCodec
	enableLogging           bool
	options                 ServerOptions
	started, stopped        bool
	pending                 int
}

type WebSocketConnection struct {
	*protocol.Connection
	ws        *websocket.Conn
	server    *WebSocketServer
	outbound  chan []byte
	done      chan struct{}
	closeOnce sync.Once
	clientID  string
}

func NewWebSocketServer(address string, options *ServerOptions) (*WebSocketServer, error) {
	var opts ServerOptions
	if options != nil {
		opts = *options
	}
	if opts.MaxClients < 0 || opts.Timeout < 0 || opts.KeepAlive < 0 || opts.MaxPendingMessages < 0 || opts.MaxMessageBytes < 0 {
		return nil, fmt.Errorf("websocket limits must be nonnegative")
	}
	if opts.MaxClients == 0 {
		opts.MaxClients = 1024
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.KeepAlive == 0 {
		opts.KeepAlive = 10 * time.Second
	}
	if opts.MaxPendingMessages == 0 {
		opts.MaxPendingMessages = 256
	}
	if opts.MaxMessageBytes == 0 {
		opts.MaxMessageBytes = 1 << 20
	}
	if opts.KeepAlive >= opts.Timeout {
		return nil, fmt.Errorf("keepalive must be shorter than timeout")
	}
	s := &WebSocketServer{address: address, clients: make(map[string]*WebSocketConnection), maxClients: opts.MaxClients, timeout: opts.Timeout, keepAlive: opts.KeepAlive, codec: protocol.NewMessageCodec(), enableLogging: opts.EnableLogging, options: opts}
	s.upgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: opts.CheckOrigin}
	// A nil CheckOrigin retains Gorilla's same-origin default.
	return s, nil
}

func (s *WebSocketServer) Start() {
	if err := s.StartWithError(); err != nil && s.enableLogging {
		log.Printf("websocket start: %v", err)
	}
}

// StartWithError binds synchronously, making readiness and bind failures observable.
func (s *WebSocketServer) StartWithError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return fmt.Errorf("server stopped; construct a new server")
	}
	if s.started {
		return nil
	}
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.listener = listener
	s.startTime = time.Now()
	s.started = true
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: s.timeout, MaxHeaderBytes: 16 << 10}
	if s.options.TLSConfig != nil {
		s.server.TLSConfig = s.options.TLSConfig.Clone()
	}
	httpServer := s.server
	go func() {
		var serveErr error
		if httpServer.TLSConfig != nil {
			serveErr = httpServer.ServeTLS(listener, "", "")
		} else {
			serveErr = httpServer.Serve(listener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && s.enableLogging {
			log.Printf("websocket serve: %v", serveErr)
		}
	}()
	return nil
}

func (s *WebSocketServer) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *WebSocketServer) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	httpServer := s.server
	s.mu.Unlock()
	s.clientsMu.RLock()
	peers := make([]*WebSocketConnection, 0, len(s.clients))
	for _, peer := range s.clients {
		peers = append(peers, peer)
	}
	s.clientsMu.RUnlock()
	for _, peer := range peers {
		peer.close()
	}
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}
}

func (s *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Reserve capacity without holding a global lock during network I/O.
	s.clientsMu.Lock()
	s.mu.RLock()
	stopped, maxClients := s.stopped, s.maxClients
	s.mu.RUnlock()
	if stopped || len(s.clients)+s.pending >= maxClients {
		s.clientsMu.Unlock()
		http.Error(w, "connection limit reached", http.StatusServiceUnavailable)
		return
	}
	s.pending++
	s.clientsMu.Unlock()
	defer func() { s.clientsMu.Lock(); s.pending--; s.clientsMu.Unlock() }()
	upgrader := s.upgrader
	upgrader.HandshakeTimeout = 5 * time.Second
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// Preserve source port: multiple players can share a public IP.
	addr, err := net.ResolveUDPAddr("udp", r.RemoteAddr)
	if err != nil {
		_ = ws.Close()
		return
	}
	conn := protocol.NewConnection(addr)
	conn.SetState(protocol.StateConnected)
	peer := &WebSocketConnection{Connection: conn, ws: ws, server: s, outbound: make(chan []byte, s.options.MaxPendingMessages), done: make(chan struct{}), clientID: r.RemoteAddr}
	s.clientsMu.Lock()
	s.mu.RLock()
	stopped = s.stopped
	s.mu.RUnlock()
	if stopped {
		s.clientsMu.Unlock()
		_ = ws.Close()
		return
	}
	s.clients[peer.clientID] = peer
	s.clientsMu.Unlock()
	go peer.writeLoop()
	s.mu.RLock()
	connected := s.onConnect
	s.mu.RUnlock()
	if connected != nil {
		connected(conn)
	}
	_ = s.SendMessage(conn, &protocol.Message{Type: protocol.MessageTypeJSON, Timestamp: time.Now().Unix(), Data: map[string]interface{}{"event": "welcome", "transport": "websocket"}})
	go peer.readLoop()
}

func (peer *WebSocketConnection) close() {
	peer.closeOnce.Do(func() {
		close(peer.done)
		_ = peer.ws.Close()
		peer.SetState(protocol.StateClosed)
		s := peer.server
		s.clientsMu.Lock()
		if s.clients[peer.clientID] == peer {
			delete(s.clients, peer.clientID)
		}
		s.clientsMu.Unlock()
		s.mu.RLock()
		disconnected := s.onDisconnect
		s.mu.RUnlock()
		if disconnected != nil {
			disconnected(peer.Connection)
		}
	})
}

func (peer *WebSocketConnection) readLoop() {
	defer peer.close()
	s := peer.server
	peer.ws.SetReadLimit(s.options.MaxMessageBytes)
	refresh := func() {
		s.mu.RLock()
		timeout := s.timeout
		s.mu.RUnlock()
		_ = peer.ws.SetReadDeadline(time.Now().Add(timeout))
	}
	refresh()
	peer.ws.SetPongHandler(func(string) error { refresh(); return nil })
	for {
		kind, data, err := peer.ws.ReadMessage()
		if err != nil {
			return
		}
		refresh()
		if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
			continue
		}
		msg, err := s.codec.Decode(data)
		if err != nil {
			return
		}
		s.mu.RLock()
		handler := s.handler
		s.mu.RUnlock()
		if handler != nil {
			handler(peer.Connection, msg)
		}
	}
}

func (peer *WebSocketConnection) writeLoop() {
	defer peer.close()
	peer.server.mu.RLock()
	interval := peer.server.keepAlive
	peer.server.mu.RUnlock()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-peer.done:
			return
		case data := <-peer.outbound:
			_ = peer.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := peer.ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			if err := peer.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

func (s *WebSocketServer) SetMessageHandler(handler func(*protocol.Connection, *protocol.Message)) {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()
}
func (peer *WebSocketConnection) enqueue(data []byte) error {
	select {
	case <-peer.done:
		return fmt.Errorf("connection closed")
	default:
	}
	select {
	case <-peer.done:
		return fmt.Errorf("connection closed")
	case peer.outbound <- data:
		return nil
	default:
		peer.close()
		return ErrBackpressure
	}
}
func (s *WebSocketServer) SendMessage(conn *protocol.Connection, msg *protocol.Message) error {
	if conn == nil {
		return fmt.Errorf("nil connection")
	}
	s.clientsMu.RLock()
	peer := s.clients[conn.Addr.String()]
	s.clientsMu.RUnlock()
	if peer == nil || peer.Connection != conn {
		return fmt.Errorf("connection not found")
	}
	data, err := s.codec.Encode(msg)
	if err != nil {
		return err
	}
	if int64(len(data)) > s.options.MaxMessageBytes {
		return fmt.Errorf("message exceeds websocket limit")
	}
	return peer.enqueue(data)
}
func (s *WebSocketServer) Broadcast(msg *protocol.Message) {
	data, err := s.codec.Encode(msg)
	if err != nil || int64(len(data)) > s.options.MaxMessageBytes {
		return
	}
	s.clientsMu.RLock()
	peers := make([]*WebSocketConnection, 0, len(s.clients))
	for _, peer := range s.clients {
		peers = append(peers, peer)
	}
	s.clientsMu.RUnlock()
	for _, peer := range peers {
		_ = peer.enqueue(data)
	}
}
func (s *WebSocketServer) Clients() map[string]*protocol.Connection {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	clients := make(map[string]*protocol.Connection, len(s.clients))
	for k, v := range s.clients {
		clients[k] = v.Connection
	}
	return clients
}
func (s *WebSocketServer) SetMaxClients(max int) {
	if max <= 0 {
		return
	}
	s.mu.Lock()
	s.maxClients = max
	s.mu.Unlock()
}
func (s *WebSocketServer) GetClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

// SetKeepAlive configures future connections; existing writer timers retain their interval.
func (s *WebSocketServer) SetKeepAlive(timeout, interval time.Duration) {
	if timeout <= 0 || interval <= 0 || interval >= timeout {
		return
	}
	s.mu.Lock()
	s.timeout = timeout
	s.keepAlive = interval
	s.mu.Unlock()
}
func (s *WebSocketServer) SetConnectionCallbacks(connect, disconnect func(*protocol.Connection)) {
	s.mu.Lock()
	s.onConnect = connect
	s.onDisconnect = disconnect
	s.mu.Unlock()
}
func (s *WebSocketServer) GetTransportType() TransportType { return TransportWebSocket }

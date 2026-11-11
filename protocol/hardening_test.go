package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testUDP(t *testing.T) *Transport {
	t.Helper()
	tr, err := NewTransport("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.Stop)
	return tr
}
func connectedTestPeer(t *testing.T, tr *Transport) *Connection {
	t.Helper()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:19001")
	c := NewConnection(addr)
	c.SetState(StateConnected)
	c.setSessionID(99)
	tr.bindConnection(c)
	tr.connMu.Lock()
	tr.connections[addr.String()] = c
	tr.connMu.Unlock()
	return c
}
func TestReliableFragmentedMessageRoundTrip(t *testing.T) {
	server, client := testUDP(t), testUDP(t)
	received := make(chan []byte, 4)
	server.OnConnect = func(c *Connection) { c.SetMessageCallback(func(b []byte) { received <- b }) }
	server.Start()
	client.Start()
	c, err := client.Connect(server.conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte{0x11, 0x44, 0x88, 0xff}, MaxMessageSize/4)
	if err = client.Send(c, data); err != nil {
		t.Fatal(err)
	}
	data[0] = 0 // caller ownership must be independent
	select {
	case got := <-received:
		if len(got) != len(data) || got[0] != 0x11 {
			t.Fatal("message corrupted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fragmented message not delivered")
	}
	if err = client.Send(c, make([]byte, MaxMessageSize+1)); err == nil {
		t.Fatal("oversized message accepted")
	}
}
func TestReliableDuplicatesSuppressedCallbacksSerialized(t *testing.T) {
	tr := testUDP(t)
	c := connectedTestPeer(t, tr)
	received := make(chan byte, 8)
	var active atomic.Int32
	c.SetMessageCallback(func(b []byte) {
		if active.Add(1) != 1 {
			t.Error("concurrent callbacks")
		}
		time.Sleep(time.Millisecond)
		received <- b[0]
		active.Add(-1)
	})
	for _, seq := range []uint32{1, 0, 0, 2, 1} {
		tr.handleData(c, &Packet{Type: TypeData, SeqNum: seq, Payload: []byte{byte(seq)}, SessionID: 99})
	}
	for _, want := range []byte{1, 0, 2} {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("arrival order got %d want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("missing message")
		}
	}
	select {
	case <-received:
		t.Fatal("duplicate delivered")
	case <-time.After(20 * time.Millisecond):
	}
}
func TestUnreliableSequencedHasIndependentSequence(t *testing.T) {
	tr := testUDP(t)
	c := connectedTestPeer(t, tr)
	got := make(chan byte, 4)
	c.SetMessageCallback(func(b []byte) { got <- b[0] })
	for _, seq := range []uint32{10, 9, 10, 11} {
		tr.handleData(c, &Packet{Type: TypeData, Flags: FlagUnreliableSequenced, SeqNum: seq, Payload: []byte{byte(seq)}, SessionID: 99})
	}
	tr.handleData(c, &Packet{Type: TypeData, SeqNum: 0, Payload: []byte{0}, SessionID: 99})
	for _, want := range []byte{10, 11, 0} {
		select {
		case n := <-got:
			if n != want {
				t.Fatalf("got %d want %d", n, want)
			}
		case <-time.After(time.Second):
			t.Fatal("missing update")
		}
	}
	if err := tr.SendUnreliableSequenced(c, []byte("state")); err != nil {
		t.Fatal(err)
	}
	c.sendWindowMu.RLock()
	count := len(c.sendWindow)
	c.sendWindowMu.RUnlock()
	if count != 0 {
		t.Fatal("disposable update entered retransmission window")
	}
	if err := tr.SendUnreliableSequenced(c, make([]byte, MaxPayloadSize+1)); err == nil {
		t.Fatal("oversized disposable accepted")
	}
}
func TestACKCumulativeDuplicateAndRTT(t *testing.T) {
	c := NewConnection(nil)
	c.SetState(StateConnected)
	for i := uint32(0); i < 3; i++ {
		c.addToSendWindow(&Packet{SeqNum: i, Timestamp: time.Now().Add(-20 * time.Millisecond)})
	}
	c.nextSeqNum = 3
	c.ProcessBitmapAck(2, 1)
	if len(c.sendWindow) != 0 {
		t.Fatal("cumulative/selective ACK did not clear window")
	}
	c.mu.RLock()
	window, rtt := c.cwnd, c.rtt
	c.mu.RUnlock()
	if rtt >= 100*time.Millisecond {
		t.Fatal("ACK did not update RTT")
	}
	if c.acknowledge(999) {
		t.Fatal("unknown ACK accepted")
	}
	c.mu.RLock()
	after := c.cwnd
	c.mu.RUnlock()
	if after != window {
		t.Fatal("duplicate/unknown ACK inflated congestion window")
	}
}
func TestReceiveWindowBoundAndWrap(t *testing.T) {
	c := NewConnection(nil)
	for n := uint32(1); n < receiveWindowSize*4; n++ {
		c.TrackReceived(n)
	}
	if len(c.receivedSeqs) > receiveWindowSize {
		t.Fatal("receive state unbounded")
	}
	if !c.TrackReceived(0) {
		t.Fatal("gap not accepted")
	}
	if c.TrackReceived(0) {
		t.Fatal("old duplicate accepted")
	}
	c = NewConnection(nil)
	c.minUnacked = ^uint32(0)
	if !c.TrackReceived(^uint32(0)) || !c.TrackReceived(0) {
		t.Fatal("wrap failed")
	}
}
func TestSendCancellationAndStopWakeWindow(t *testing.T) {
	tr := testUDP(t)
	c := connectedTestPeer(t, tr)
	if err := tr.Send(c, []byte("first")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := tr.SendContext(ctx, c, []byte("blocked")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- tr.SendContext(context.Background(), c, []byte("blocked")) }()
	tr.Stop()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("send succeeded after stop")
		}
	case <-time.After(time.Second):
		t.Fatal("stop stranded sender")
	}
	tr.Start() // no restart after stop
}
func TestUnknownPacketsAndSessionMismatchDoNotAllocateOrDeliver(t *testing.T) {
	tr := testUDP(t)
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:19999")
	for _, kind := range []PacketType{TypeData, TypeAck, TypeKeepAlive, TypeClose} {
		tr.handlePacket(&Packet{Type: kind, SessionID: 9}, addr)
	}
	if tr.GetConnectionCount() != 0 {
		t.Fatal("unsolicited packet allocated connection")
	}
	c := connectedTestPeer(t, tr)
	tr.handlePacket(&Packet{Type: TypeClose, SessionID: 100}, c.Addr)
	if c.GetState() != StateConnected {
		t.Fatal("stale session affected current connection")
	}
}
func TestCallbackBackpressureClosesReliablePeer(t *testing.T) {
	tr := testUDP(t)
	c := connectedTestPeer(t, tr)
	block := make(chan struct{})
	entered := make(chan struct{}, 1)
	c.SetMessageCallback(func([]byte) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-block
	})
	defer close(block)
	tr.handleData(c, &Packet{Type: TypeData, SeqNum: 0, Payload: []byte("x")})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	for seq := uint32(1); seq < 100; seq++ {
		tr.handleData(c, &Packet{Type: TypeData, SeqNum: seq, Payload: []byte("x")})
	}
	if c.GetState() != StateClosed {
		t.Fatal("reliable callback overflow did not close connection")
	}
}
func TestFragmentMetadataAndQuota(t *testing.T) {
	c := NewConnection(nil)
	if _, _, err := c.assemble(&Packet{FragTotal: 2, FragOffset: 2}); err == nil {
		t.Fatal("out of range offset accepted")
	}
	for id := uint16(0); id < maxFragmentTransfers; id++ {
		if _, _, err := c.assemble(&Packet{FragmentID: id, FragTotal: 2, Payload: []byte("x")}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := c.assemble(&Packet{FragmentID: 100, FragTotal: 2}); err == nil {
		t.Fatal("transfer quota ignored")
	}
	if _, _, err := c.assemble(&Packet{FragmentID: 0, FragTotal: 3, FragOffset: 1}); err == nil {
		t.Fatal("inconsistent total accepted")
	}
	if _, _, err := c.assemble(&Packet{FragmentID: 0, FragTotal: 2, FragOffset: 1, SeqNum: 9}); err == nil {
		t.Fatal("mixed fragment sequences accepted")
	}
}
func TestHandshakeRetriesLostRequestAndResponse(t *testing.T) {
	server, client := testUDP(t), testUDP(t)
	var connects atomic.Int32
	server.OnConnect = func(*Connection) { connects.Add(1) }
	server.Start()
	client.Start()
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		buf := make([]byte, 2048)
		var peer *net.UDPAddr
		requestDropped, responseDropped := false, false
		for {
			n, addr, err := relay.ReadFromUDP(buf)
			if err != nil {
				return
			}
			p, err := Unmarshal(buf[:n])
			if err != nil {
				continue
			}
			if addr.String() == server.conn.LocalAddr().String() {
				if p.Type == TypeHandshake && !responseDropped {
					responseDropped = true
					continue
				}
				if peer != nil {
					_, _ = relay.WriteToUDP(buf[:n], peer)
				}
			} else {
				peer = addr
				if p.Type == TypeHandshake && !requestDropped {
					requestDropped = true
					continue
				}
				_, _ = relay.WriteToUDP(buf[:n], server.conn.LocalAddr().(*net.UDPAddr))
			}
		}
	}()
	if _, err = client.Connect(relay.LocalAddr().String()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for connects.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if connects.Load() != 1 {
		t.Fatalf("connect callback count %d", connects.Load())
	}
	relay.Close()
	<-stopped
}
func TestPacketRejectMalformedWire(t *testing.T) {
	for _, p := range []*Packet{{Type: 99}, {Type: TypeData, Flags: 0x80}, {Type: TypeData, Flags: FlagFragment, FragTotal: 1}, {Type: TypeData, Flags: FlagUnreliableSequenced | FlagFragment, FragTotal: 2}, {Type: TypeAck, Payload: []byte{1}}, {Type: TypeBitmapAck}} {
		if _, err := p.Marshal(); err == nil {
			t.Fatalf("malformed packet accepted: %+v", p)
		}
	}
	p := &Packet{Type: TypeData, SessionID: 0x1122334455667788, Payload: []byte("owned")}
	wire, _ := p.Marshal()
	decoded, err := Unmarshal(wire)
	if err != nil || decoded.SessionID != p.SessionID {
		t.Fatal("session roundtrip failed")
	}
	wire[HeaderSize] = 'x'
	if string(decoded.Payload) != "owned" {
		t.Fatal("decoder retains mutable input")
	}
	wire = make([]byte, HeaderSize+MaxWirePayloadSize+1)
	binary.BigEndian.PutUint16(wire, MagicNumber)
	wire[2] = byte(TypeEncrypted)
	if _, err = Unmarshal(wire); err == nil {
		t.Fatal("oversized wire accepted")
	}
}
func TestTokenEnvelopeRoundTripAndMetadata(t *testing.T) {
	codec := NewMessageCodec()
	original := &Message{Type: MessageTypeText, ID: "cmd", Timestamp: 42, Data: strings.Repeat("with", 100), Metadata: map[string]interface{}{"source": "test"}}
	encoded, err := codec.Encode(original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte("\"tk\":true")) {
		t.Fatal("test did not exercise token wrapper")
	}
	msg, err := codec.Decode(encoded)
	if err != nil || msg.Type != original.Type || msg.Data != original.Data || msg.Metadata["source"] != "test" {
		t.Fatalf("roundtrip failed: %+v %v", msg, err)
	}
	for _, wire := range []string{`{"tk":true,"t":"text","d":"AA=="}`, `{"tk":true,"t":"json","d":""}`, `{"tk":true,"t":"text","d":123}`, `{"tk":true,"t":"text","d":"//8="}`, `{}`} {
		if _, err = codec.Decode([]byte(wire)); err == nil {
			t.Fatalf("bad envelope accepted %s", wire)
		}
	}
}
func TestChunkValidationReentryDedupAndStatsConcurrency(t *testing.T) {
	cr := NewChunkReassembler()
	chunks, err := ChunkString(strings.Repeat("x", MaxChunkSize+1), "transfer", nil)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	cr.SetCompleteCallback(func(id string, b []byte, _ map[string]interface{}) {
		calls.Add(1)
		cr.GetStats()
		if len(b) != MaxChunkSize+1 {
			t.Error("corrupt chunk data")
		}
	})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				cr.GetStats()
			}
		}
	}()
	for i := len(chunks) - 1; i >= 0; i-- {
		if err = cr.AddChunk(chunks[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, chunk := range chunks {
		if err = cr.AddChunk(chunk); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate completion")
	}
	invalid := *chunks[0]
	invalid.Metadata.ChunkIndex = -1
	if cr.AddChunk(&invalid) == nil {
		t.Fatal("negative index accepted")
	}
	invalid = *chunks[0]
	invalid.Metadata.TransferID = "other"
	invalid.Metadata.TotalChunks = 999
	if cr.AddChunk(&invalid) == nil {
		t.Fatal("unbounded chunk count accepted")
	}
	cr.Close()
	if cr.AddChunk(chunks[0]) == nil {
		t.Fatal("closed receiver accepted chunk")
	}
	empty, _ := ChunkString("", "empty", nil)
	if len(empty) != 1 {
		t.Fatal("empty transfer cannot complete")
	}
}

func TestChunkStringPreservesUTF8Boundaries(t *testing.T) {
	text := strings.Repeat("宇宙🚀", MaxChunkSize)
	chunks, err := ChunkString(text, "unicode", nil)
	if err != nil {
		t.Fatal(err)
	}
	cr := NewChunkReassembler()
	got := ""
	cr.SetCompleteCallback(func(_ string, b []byte, _ map[string]interface{}) { got = string(b) })
	for _, chunk := range chunks {
		wire, err := NewMessageCodec().Encode(&Message{Type: MessageTypeChunked, Data: chunk})
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Data ChunkedMessage `json:"data"`
		}
		if err = json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		if err = cr.AddChunk(&decoded.Data); err != nil {
			t.Fatal(err)
		}
	}
	if got != text {
		t.Fatal("UTF-8 text changed across JSON chunk roundtrip")
	}
}

func TestClosedConnectionIsTerminal(t *testing.T) {
	c := NewConnection(nil)
	c.SetState(StateClosed)
	c.SetState(StateConnected)
	if c.GetState() != StateClosed {
		t.Fatal("closed connection reopened")
	}
}
func BenchmarkCodecDecodeJSONPayload(b *testing.B) {
	codec := NewMessageCodec()
	wire, _ := codec.Encode(&Message{Type: MessageTypeJSON, Data: map[string]interface{}{"tick": 42, "entities": []interface{}{map[string]interface{}{"id": "18446744073709551615", "x": 1.25, "y": 3.0}}}})
	b.ReportAllocs()
	b.SetBytes(int64(len(wire)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := codec.Decode(wire); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkReceiveDuplicateSuppression(b *testing.B) {
	c := NewConnection(nil)
	c.TrackReceived(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.TrackReceived(0)
	}
}

// newLossRelay forwards one client to server. drop is called by one relay
// goroutine, with fromServer=true for server-to-client traffic.
func newLossRelay(t testing.TB, server *net.UDPAddr, drop func(*Packet, bool) bool) *net.UDPAddr {
	t.Helper()
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	t.Cleanup(func() { relay.Close(); <-stopped })
	go func() {
		defer close(stopped)
		buf := make([]byte, 2048)
		var client *net.UDPAddr
		for {
			n, from, err := relay.ReadFromUDP(buf)
			if err != nil {
				return
			}
			p, err := Unmarshal(buf[:n])
			if err != nil {
				continue
			}
			fromServer := from.String() == server.String()
			if !fromServer {
				client = from
			}
			if drop != nil && drop(p, fromServer) {
				continue
			}
			if fromServer {
				if client != nil {
					_, _ = relay.WriteToUDP(buf[:n], client)
				}
			} else {
				_, _ = relay.WriteToUDP(buf[:n], server)
			}
		}
	}()
	return relay.LocalAddr().(*net.UDPAddr)
}
func TestReliableLossAndLostACKRecoverWithoutDuplicateDelivery(t *testing.T) {
	server, client := testUDP(t), testUDP(t)
	got := make(chan []byte, 8)
	server.OnConnect = func(c *Connection) { c.SetMessageCallback(func(b []byte) { got <- b }) }
	server.Start()
	client.Start()
	dataDropped, ackDropped, bitmapDropped := false, false, false
	relay := newLossRelay(t, server.conn.LocalAddr().(*net.UDPAddr), func(p *Packet, fromServer bool) bool {
		if !fromServer && p.Type == TypeData && p.SeqNum == 1 && !dataDropped {
			dataDropped = true
			return true
		}
		if fromServer && p.Type == TypeAck && p.AckNum == 0 && !ackDropped {
			ackDropped = true
			return true
		}
		if fromServer && p.Type == TypeBitmapAck && !bitmapDropped {
			bitmapDropped = true
			return true
		}
		return false
	})
	c, err := client.Connect(relay.String())
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("z"), MaxPayloadSize*3)
	if err = client.Send(c, data); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-got:
		if !bytes.Equal(result, data) {
			t.Fatal("corrupt message")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lost data not recovered")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		c.sendWindowMu.RLock()
		outstanding := len(c.sendWindow)
		c.sendWindowMu.RUnlock()
		if outstanding == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ACK loss left outstanding packets")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-got:
		t.Fatal("retransmission delivered message twice")
	case <-time.After(30 * time.Millisecond):
	}
}
func TestCanceledPartialFragmentTransferClosesConnection(t *testing.T) {
	tr := testUDP(t)
	c := connectedTestPeer(t, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := tr.SendContext(ctx, c, make([]byte, MaxPayloadSize*2)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if c.GetState() != StateClosed {
		t.Fatal("partially transmitted message abandoned without closing")
	}
}

func FuzzPacketDecode(f *testing.F) {
	seed, _ := (&Packet{Type: TypeData, SessionID: 123, Payload: []byte("hello")}).Marshal()
	f.Add(seed)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := Unmarshal(b)
		if err != nil {
			return
		}
		wire, err := p.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wire, b) {
			t.Fatal("noncanonical packet accepted")
		}
	})
}
func FuzzMessageDecode(f *testing.F) {
	f.Add([]byte(`{"type":"json","ts":1,"data":{"tick":42}}`))
	f.Add([]byte(`{"tk":true,"t":"text","d":"AQEBAQ=="}`))
	codec := NewMessageCodec()
	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := codec.Decode(b)
		if err != nil {
			return
		}
		if m == nil || m.Type == "" {
			t.Fatal("invalid successful decode")
		}
	})
}

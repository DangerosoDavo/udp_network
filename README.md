# UDP Network

Go networking components for bounded reliable UDP messages, disposable sequenced
state updates, authenticated encrypted UDP sessions, and WebSocket messaging.
Requires Go 1.27.4 or newer for the patched standard library. Read [maintenance and migration notes](MAINTENANCE.md) before
upgrading: this revision intentionally changes the UDP wire format and secure
connection setup.

## Delivery contracts

- `protocol.Transport.Send` / `SendContext`: reliable, duplicate-suppressed,
  **unordered** delivery within a live session. Callbacks are serialized per peer
  in arrival or completed-message order. Fragmented messages are reassembled.
- `SendUnreliableSequenced`: disposable updates in a separate sequence space;
  older updates are discarded. There is one such channel per connection.
- Success means locally queued, not acknowledged or applied remotely. Use
  application command IDs, revisions, and durable idempotency records for game
  actions that must survive reconnects and retries.
- Sending, receive history, callback queues, fragments, chunk reassembly and
  connections have finite limits. Backpressure and connection closure are errors
  to handle, not infinite buffering. See [limits](MAINTENANCE.md#limits).

## Choosing a transport

`NewServer` / `NewClient` provide **unauthenticated plaintext UDP**, useful for
controlled tests. Internet-facing game traffic should use authenticated sessions
or WSS. The unified API selects ordinary UDP or WebSockets explicitly:

```go
server, err := udpnet.NewUnifiedServer("127.0.0.1:8080", &udpnet.ServerOptions{
    Transport: udpnet.TransportWebSocket,
    MaxClients: 128,
    MaxPendingMessages: 64,
})
if err != nil { return err }
server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
    // Validate application identity and permissions before processing commands.
})
server.Start()
defer server.Stop()
```

Here `udpnet` is `github.com/DangerosoDavo/udp_network`, and `protocol` is its
`/protocol` package. WebSocket servers offer `StartWithError` and `Addr` for
observable startup. Configure `ServerOptions.TLSConfig` for WSS or terminate TLS
at a trusted reverse proxy; the default listener serves plain WS. Clients accept
`wss://` URLs and `ClientOptions.TLSConfig`. Same-origin checking is enabled by
default; use an explicit `CheckOrigin` policy if browser access needs other origins.

### Typed application WebSocket schemas

`WebSocketClient.SetMessageHandler` retains its decoded `protocol.Message` API.
Applications with their own checked wire schema can avoid generic interface/map
decoding with either raw receive callback:

- `SetRawMessageHandler(func([]byte))` delivers an owned frame; it can be retained.
- `SetBorrowedRawMessageHandler(func([]byte))` delivers a reader-owned frame valid
  only until the callback returns. Decode synchronously or copy before retaining
  or passing it to another goroutine. The reader reuses a buffer bounded by
  `ClientOptions.MaxMessageBytes`.

Borrowed delivery takes precedence over owned raw delivery, which takes precedence
over decoded delivery. Set a handler to `nil` to restore the next configured mode.
Both text and binary WebSocket messages use these callbacks. Raw delivery does
not validate the application envelope: the caller must validate the complete
message, its types, bounds and semantics before using it.

All modes keep ordered synchronous callbacks, frame-size limits, read deadlines
and heartbeat behavior. Callback work consumes the next read's timeout budget
and delays control-frame processing; return promptly. These APIs reduce copying
and decoding work, without changing delivery or timeout guarantees.

## Authenticated UDP

Legacy constructors without credentials now return `protocol.ErrPSKRequired`.
Use `NewSecureServerWithPSK` / `NewSecureClientWithPSK` with at least 32 random
secret bytes, or `WithConfig(protocol.SecureConfig{...})` for admission lookup.
For a game client, issue unique, short-lived session secrets through authenticated
HTTPS and resolve opaque `KeyID` values server-side. A shared secret embedded in
every shipped client does not establish individual player identity.

The v2 secure path authenticates an ephemeral X25519 handshake with the admission
secret, separates directional keys, confirms the transcript, encrypts inner
packet metadata/control messages, and maintains a reorder-tolerant replay window.
It is a custom protocol with regression tests, **not an independently audited
standard transport**. An external security review and comparison with established
authenticated transports remain prerequisites for public production adoption.

## Large messages and schemas

UDP transport fragments messages up to `protocol.MaxMessageSize`; larger text
uses bounded application chunks. `SendLargeString` progress reports local queue
progress. Receivers must scope transfers by authenticated peer/session; the
server helpers isolate by connection address. Asset downloads should use a
separate bulk-transfer channel.

The high-level message codec uses JSON, with an optional fixed text-token
envelope. It is not a compact game-state schema. JSON interface numbers become
`float64`; use typed binary schemas or string IDs for large integer identifiers.
Raw byte APIs are available for application-defined binary messages. An Unreal
C++ implementation and cross-language conformance tests still need to be built.

## WebSocket disconnect diagnostics

`WebSocketClient.LastDisconnectError()` retains the first terminal cause as a
`*WebSocketDisconnectError`, including its operation and wrapped original error.
Use `errors.As` and `errors.Is` to classify read, heartbeat, write and context
failures. The cause is visible before the connection becomes disconnected,
survives `Stop`, and resets when a new `Connect` attempt begins. Applications can
record bounded reason codes while retaining native error types for inspection.

## Validate and measure

```sh
go test ./...
go vet ./...
go test -race ./...
go test -run '^$' -fuzz '^FuzzPacketDecode$' -fuzztime=10s ./protocol
go test -run '^$' -fuzz '^FuzzMessageDecode$' -fuzztime=10s ./protocol
go test -run '^$' -bench '^BenchmarkEncode' -benchmem -count=5 ./tokenizer
```

Tests use local loopback and include loss/duplicate relays. Historical manual
programs under `tests/` are executable examples, not automatically run integration
tests. Secure examples require `UDP_NETWORK_PSK`, a base64-encoded random secret
shared out of band. No default key is provided. See [measured results](PERFORMANCE.md)
and [maintenance notes](MAINTENANCE.md) for reproducible commands and limitations.

## License

The repository's existing [non-commercial license](LICENSE) is unchanged.

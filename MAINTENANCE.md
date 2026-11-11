# Networking maintenance and migration

The November 2026 maintenance changes repair source-confirmed deadlocks, lost
message paths, concurrency defects and cryptographic failures. They establish
tested contracts for further development; they do not establish an audited
internet transport or a measured game-server player capacity.

## Compatibility

The minimum compiler is Go 1.27.4. Earlier Go 1.25.1 networking builds reached
known standard-library advisories; see [security validation](SECURITY.md).

- UDP v2 uses magic `0x5545`, a 24-byte header, and a session identifier. Old peers
  are rejected. Deploy matching client/server implementations together; there is
  no automatic insecure fallback.
- Secure constructors without an explicit secret return `ErrPSKRequired`.
  Use `NewSecureTransportWithConfig`, `NewSecureServerWithConfig` or
  `NewSecureClientWithConfig`. `SecureConfig` accepts `PSK`, `KeyID`, and a server
  `LookupPSK` callback. Secrets need at least 32 random bytes. Resolve and expire
  admission tickets outside this library; callbacks must be bounded and fast.
- `RequireEncryption` is deprecated. Secure v2 always requires encryption.
- Reliable delivery is duplicate-suppressed but unordered. Old documentation
  implying ordered or exactly-once business execution is superseded.
- Public duration/callback fields on low-level transports are startup-only.
  Use `SetKeepAlive` for concurrent timeout configuration and wrapper callback
  setters for supported live updates. Application callbacks should return promptly.
- `Client.CurrentConnection` safely reads the legacy exported connection pointer.
  Do not directly read/write `Client.Connection` concurrently with lifecycle calls.
- A stopped UDP transport is terminal; construct a new transport/client. A
  WebSocketClient supports reconnection on the same object without reusing old
  goroutine cancellation channels or allowing old readers to close the new session.

## Limits

The packet application payload is 1,100 bytes, with space reserved for secure
wrapping. A reliable transport message is at most 255 fragments / 280,500 bytes.
Unreliable-sequenced messages have no fragmentation and fit one application payload.
Application chunks are 32 KiB; a chunk transfer is limited to 128 chunks / 4 MiB.
The reassembler limits simultaneous transfers and total buffered bytes. Transfer
metadata, offsets, checksums and conflicting duplicates are validated before use.

WebSocket defaults are 1,024 peers, 256 pending messages per peer and a 1 MiB
frame-message read limit. The JSON codec's smaller encoded-message limit still
applies. Queue saturation closes the affected peer and returns `ErrBackpressure`;
one slow peer cannot synchronously stall a broadcast to all others. Configure
lower limits where connection count or memory budgets require them. Limits bound
individual allocations/queues; they are not a deployment-wide RAM guarantee.

Successful send/progress callbacks mean local acceptance. Local send deadlines
cancel queue waits; abandoning a partially sent reliable message closes its
session rather than silently leaving a fragmented transfer incomplete. Remote
completion and durable state changes require application acknowledgements.
`LargeStringOptions.ChunkTimeout` bounds queue time per chunk (default 5 seconds).
`RetryAttempts` is deprecated because the transport owns retransmission. Fixed
sleep-based throttling and retry loops have been removed.

## Security migration

The old construction reused a key/nonce bidirectionally, accepted encrypted
replays, lacked peer authentication, allowed plaintext controls into secure
connections and sent high-level secure messages through the plaintext transport.
Regression tests cover these failures. V2 uses authenticated ephemeral handshakes,
directional/session keys, encrypted inner headers and controls, key confirmation,
and a bounded replay window. Retransmissions are freshly encrypted, acknowledged
again as necessary, and suppressed at the application delivery layer.

Static PSKs are suitable only for trusted deployments/prototypes. A public game
needs unique session admission secrets delivered over authenticated HTTPS, an
expiring/revocable identity registry, authorization checks and abuse controls.
The library does not supply identity management, NAT traversal, relays, internet
DDoS protection or a C++ implementation. Independent security assessment and
comparison with standard authenticated transports remain outstanding.

To run matching secure executable examples, generate one local random secret and
set its base64 representation in `UDP_NETWORK_PSK` for both processes. Never
commit that secret. Examples now fail explicitly if the variable is missing.

## Validation and follow-up

Normal and race tests cover loopback roundtrips, same-IP WebSocket clients,
concurrent writers/capacity, WSS, reconnect, malformed bounds, fragmentation,
handshake/data/ACK loss, duplicate suppression, replay, plaintext refusal,
credential lookup and cross-peer chunk isolation. Fuzz targets exercise packet
and message decoders and tokenizer roundtrips. CI runs tests/vet/race on Linux
and Windows and uploads benchmark samples; hosted CI execution must be verified
after these local changes are published.

The standalone `example-project` module now uses the same compiler requirement,
an ephemeral loopback listener, a concurrent player registry and atomic score
updates. Its complete client/server demo was also run with the race detector.

Further transport work should use network impairment matrices (RTT, burst loss,
jitter, reorder, bandwidth throttling), sustained multi-peer fairness/latency
measurements, congestion-controller review, MTU/path changes, resource-exhaustion
testing, and Unreal interoperability. Current RTT/window improvements are not
evidence of TCP-equivalent internet congestion fairness. Preserve raw measurements
before changing queue, window, payload or storage defaults.

# Networking performance measurements

Measured on 11 November 2026 with Go 1.27.4, Windows amd64, Intel i7-6700K,
`GOMAXPROCS=1`, `-cpu=1`, five samples and 300 ms per benchmark. The baseline is
commit `4a49ecd7913deafe6e8f10e962aa0c144c3baaaa`; the exact same benchmark
source and payload fixtures were used before and after. Values below are medians.

| Workload | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| English text tokenization, 1088 bytes | 299353 | 15550 | 3776 | 2304 | 6 | 1 |
| Opaque ASCII tokenization, 1280 bytes | 596258 | 6082 | 6464 | 2688 | 7 | 1 |
| Snapshot-shaped JSON decode, 101 bytes | 30588 | 10348 | 2280 | 1168 | 52 | 24 |

The prefix-index tokenizer replaces up to twenty map lookups per byte with a
short longest-prefix candidate list and one output allocation. The recorded
fixtures show approximately 19x / 98x lower elapsed time, respectively. These
are tokenizer microbenchmarks, not player capacity or full transport throughput.
The high-level JSON codec still pays serialization and envelope costs. A binary
game-state schema can bypass text tokenization entirely.

JSON decoding avoids the former full untyped-map pass followed by another full
parse. Allocation counts fell from 52 to 24 and bytes from 2280 to 1168 on this
fixture. Timing samples vary substantially on the shared machine;
the allocation reduction is stronger evidence than a precise latency claim.

Raw samples are in [benchmarks/](benchmarks/). Reproduce with the same harness,
compiler and host, running revisions sequentially after builds/race tests finish:

```sh
go test ./tokenizer -run '^$' -bench '^BenchmarkEncode' -benchmem -count=5 -benchtime=300ms -cpu=1
go test ./protocol -run '^$' -bench '^BenchmarkMaintenanceCodecDecode$' -benchmem -count=5 -benchtime=300ms -cpu=1
```

Reliable UDP and WebSocket benchmarks now check errors and measure delivered
loopback roundtrips with bounded acknowledgement channels and deadlines. Prior
enqueue-only or ignored-error benchmark rates are not comparable. The original
UDP send deadlock prevents a valid baseline end-to-end transport throughput
comparison. Security benchmarks likewise compare different security guarantees;
the old unauthenticated exchange is not a substitute for the authenticated v2
handshake.

Before selecting this transport for the game, measure sustained peer counts,
per-peer bandwidth, snapshot age, command latency, allocations/GC and fairness
under controlled latency/loss/jitter/reordering and bandwidth caps. Published
microbenchmark gains do not establish internet congestion safety or scalability.

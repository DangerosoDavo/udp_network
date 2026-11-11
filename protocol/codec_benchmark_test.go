package protocol

import "testing"

// Keep this harness identical when comparing an original checkout and a change.
func BenchmarkMaintenanceCodecDecode(b *testing.B) {
	codec := NewMessageCodec()
	wire := []byte(`{"type":"json","ts":1,"data":{"tick":42,"entities":[{"id":"18446744073709551615","x":1.25,"y":3.0}]}}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(wire)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg, err := codec.Decode(wire)
		if err != nil || msg.Type != MessageTypeJSON {
			b.Fatalf("invalid decode: %v", err)
		}
	}
}

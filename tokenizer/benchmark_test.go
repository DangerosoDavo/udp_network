package tokenizer

import (
	"strings"
	"testing"
)

var benchmarkTokens []Token

func BenchmarkEncodeTelemetryText(b *testing.B) {
	t := NewTokenizer()
	text := strings.Repeat("the ship will approach the station with more power and then return. ", 16)
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkTokens = t.Encode(text)
	}
}

func BenchmarkEncodeOpaqueText(b *testing.B) {
	t := NewTokenizer()
	text := strings.Repeat("0123456789XYZ_-:7890", 64)
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkTokens = t.Encode(text)
	}
}

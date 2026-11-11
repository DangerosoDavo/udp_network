package udp_network

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/iotest"

	"github.com/gorilla/websocket"
)

func TestWebSocketReadBufferBoundsAndReuse(t *testing.T) {
	var buffer []byte
	var probe [1]byte
	for _, size := range []int{0, 1, 4096, 4097, 65536, 9, 65536} {
		input := bytes.Repeat([]byte{'x'}, size)
		var err error
		buffer, err = readWebSocketFrame(bytes.NewReader(input), buffer, probe[:], 65536)
		if err != nil || !bytes.Equal(input, buffer) || cap(buffer) > 65536 {
			t.Fatal("frame bytes or buffer bound incorrect", size, len(buffer), cap(buffer), err)
		}
	}
	before := &buffer[0]
	buffer, err := readWebSocketFrame(bytes.NewReader([]byte("next")), buffer, probe[:], 65536)
	if err != nil || &buffer[0] != before {
		t.Fatal("steady-state buffer not reused")
	}
	_, err = readWebSocketFrame(bytes.NewReader(bytes.Repeat([]byte{'x'}, 65537)), buffer, probe[:], 65536)
	if !errors.Is(err, websocket.ErrReadLimit) {
		t.Fatal("oversized frame accepted", err)
	}
	fragmented := bytes.Repeat([]byte{'f'}, 4097)
	buffer, err = readWebSocketFrame(iotest.OneByteReader(bytes.NewReader(fragmented)), buffer, probe[:], 65536)
	if err != nil || !bytes.Equal(buffer, fragmented) {
		t.Fatal("short fragmented reads changed frame", err)
	}
}

type idleFrameReader struct{}

func (idleFrameReader) Read([]byte) (int, error) { return 0, nil }

func TestWebSocketReadBufferRejectsNoProgress(t *testing.T) {
	var probe [1]byte
	if _, err := readWebSocketFrame(idleFrameReader{}, nil, probe[:], 16); !errors.Is(err, io.ErrNoProgress) {
		t.Fatal("non-progressing reader was not bounded", err)
	}
}

func BenchmarkWebSocketReadBuffer(b *testing.B) {
	for _, size := range []int{32 * 1024, 64 * 1024} {
		input := bytes.Repeat([]byte{'x'}, size)
		b.Run(fmt.Sprintf("bytes_%d/ReadAll", size), func(b *testing.B) {
			var reader bytes.Reader
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for i := 0; i < b.N; i++ {
				reader.Reset(input)
				out, err := io.ReadAll(&reader)
				if err != nil || len(out) != len(input) {
					b.Fatal("read failed")
				}
			}
		})
		b.Run(fmt.Sprintf("bytes_%d/reused", size), func(b *testing.B) {
			var reader bytes.Reader
			var probe [1]byte
			reader.Reset(input)
			buffer, err := readWebSocketFrame(&reader, nil, probe[:], int64(size))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader.Reset(input)
				buffer, err = readWebSocketFrame(&reader, buffer, probe[:], int64(size))
				if err != nil || len(buffer) != len(input) {
					b.Fatal("read failed")
				}
			}
		})
	}
}

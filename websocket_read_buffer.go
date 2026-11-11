package udp_network

import (
	"io"

	"github.com/gorilla/websocket"
)

// readWebSocketFrame reuses one reader-owned buffer. Limit every growth and
// individual read even though Gorilla independently enforces its read limit.
// A one-byte probe avoids growing a full buffer when EOF is already next.
func readWebSocketFrame(reader io.Reader, buffer, probe []byte, limit int64) ([]byte, error) {
	buffer = buffer[:0]
	emptyReads := 0
	for {
		var destination []byte
		probing := len(buffer) == cap(buffer)
		if probing {
			destination = probe
		} else {
			available := int64(cap(buffer) - len(buffer))
			if remaining := limit - int64(len(buffer)); remaining < available {
				available = remaining
			}
			if available == 0 {
				probing = true
				destination = probe
			} else {
				destination = buffer[len(buffer) : len(buffer)+int(available)]
			}
		}
		n, err := reader.Read(destination)
		if n > 0 {
			emptyReads = 0
			if int64(len(buffer))+int64(n) > limit {
				return buffer, websocket.ErrReadLimit
			}
			if probing {
				next := cap(buffer) * 2
				if next < 4096 {
					next = 4096
				}
				if int64(next) > limit || next < cap(buffer) {
					next = int(limit)
				}
				grown := make([]byte, len(buffer), next)
				copy(grown, buffer)
				buffer = append(grown, probe[0])
			} else {
				buffer = buffer[:len(buffer)+n]
			}
		} else if err == nil {
			emptyReads++
			if emptyReads >= 100 {
				return buffer, io.ErrNoProgress
			}
		}
		if err != nil {
			if err == io.EOF {
				return buffer, nil
			}
			return buffer, err
		}
	}
}

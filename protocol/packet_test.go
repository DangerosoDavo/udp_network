package protocol

import (
	"bytes"
	"testing"
)

func TestPacketMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		packet  *Packet
		wantErr bool
	}{
		{
			name: "basic data packet",
			packet: &Packet{
				Type:    TypeData,
				Flags:   0,
				SeqNum:  100,
				AckNum:  99,
				Payload: []byte("test data"),
			},
			wantErr: false,
		},
		{
			name: "ack packet",
			packet: &Packet{
				Type:   TypeAck,
				SeqNum: 0,
				AckNum: 500,
			},
			wantErr: false,
		},
		{
			name: "fragmented packet",
			packet: &Packet{
				Type:       TypeData,
				Flags:      FlagFragment | FlagLastFrag,
				SeqNum:     200,
				FragmentID: 12345,
				FragOffset: 2,
				FragTotal:  5,
				Payload:    []byte("fragment data"),
			},
			wantErr: false,
		},
		{
			name: "encrypted packet",
			packet: &Packet{
				Type:    TypeEncrypted,
				Flags:   FlagEncrypted,
				SeqNum:  300,
				Payload: []byte("encrypted payload"),
			},
			wantErr: false,
		},
		{
			name: "oversized payload",
			packet: &Packet{
				Type:    TypeData,
				Payload: make([]byte, MaxPayloadSize+1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := tt.packet.Marshal()
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // Expected error, don't continue
			}

			// Unmarshal
			decoded, err := Unmarshal(data)
			if err != nil {
				t.Errorf("Unmarshal() error = %v", err)
				return
			}

			// Verify fields
			if decoded.Type != tt.packet.Type {
				t.Errorf("Type mismatch: got %v, want %v", decoded.Type, tt.packet.Type)
			}
			if decoded.Flags != tt.packet.Flags {
				t.Errorf("Flags mismatch: got %v, want %v", decoded.Flags, tt.packet.Flags)
			}
			if decoded.SeqNum != tt.packet.SeqNum {
				t.Errorf("SeqNum mismatch: got %v, want %v", decoded.SeqNum, tt.packet.SeqNum)
			}
			if decoded.AckNum != tt.packet.AckNum {
				t.Errorf("AckNum mismatch: got %v, want %v", decoded.AckNum, tt.packet.AckNum)
			}
			if decoded.FragmentID != tt.packet.FragmentID {
				t.Errorf("FragmentID mismatch: got %v, want %v", decoded.FragmentID, tt.packet.FragmentID)
			}
			if decoded.FragOffset != tt.packet.FragOffset {
				t.Errorf("FragOffset mismatch: got %v, want %v", decoded.FragOffset, tt.packet.FragOffset)
			}
			if decoded.FragTotal != tt.packet.FragTotal {
				t.Errorf("FragTotal mismatch: got %v, want %v", decoded.FragTotal, tt.packet.FragTotal)
			}
			if !bytes.Equal(decoded.Payload, tt.packet.Payload) {
				t.Errorf("Payload mismatch: got %v, want %v", decoded.Payload, tt.packet.Payload)
			}
		})
	}
}

func TestPacketMagicNumber(t *testing.T) {
	packet := &Packet{
		Type:    TypeData,
		Payload: []byte("test"),
	}

	data, err := packet.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Check magic number
	if len(data) < 2 {
		t.Fatal("Packet too short")
	}

	magic := uint16(data[0])<<8 | uint16(data[1])
	if magic != MagicNumber {
		t.Errorf("Magic number mismatch: got 0x%04X, want 0x%04X", magic, MagicNumber)
	}
}

func TestUnmarshalInvalidPacket(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "too short",
			data: []byte{0x55, 0x44}, // Only magic number
		},
		{
			name: "wrong magic",
			data: make([]byte, HeaderSize), // Zero-filled
		},
		{
			name: "empty",
			data: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Unmarshal(tt.data)
			if err == nil {
				t.Error("Expected error for invalid packet, got nil")
			}
		})
	}
}

func TestPacketFlags(t *testing.T) {
	tests := []struct {
		name           string
		flags          PacketFlags
		isFragment     bool
		isLastFragment bool
		isTokenized    bool
	}{
		{
			name:           "no flags",
			flags:          0,
			isFragment:     false,
			isLastFragment: false,
			isTokenized:    false,
		},
		{
			name:           "fragment",
			flags:          FlagFragment,
			isFragment:     true,
			isLastFragment: false,
			isTokenized:    false,
		},
		{
			name:           "last fragment",
			flags:          FlagFragment | FlagLastFrag,
			isFragment:     true,
			isLastFragment: true,
			isTokenized:    false,
		},
		{
			name:           "tokenized",
			flags:          FlagTokenized,
			isFragment:     false,
			isLastFragment: false,
			isTokenized:    true,
		},
		{
			name:           "all flags",
			flags:          FlagFragment | FlagLastFrag | FlagTokenized | FlagCompressed | FlagEncrypted,
			isFragment:     true,
			isLastFragment: true,
			isTokenized:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := &Packet{Flags: tt.flags}

			if packet.IsFragment() != tt.isFragment {
				t.Errorf("IsFragment() = %v, want %v", packet.IsFragment(), tt.isFragment)
			}
			if packet.IsLastFragment() != tt.isLastFragment {
				t.Errorf("IsLastFragment() = %v, want %v", packet.IsLastFragment(), tt.isLastFragment)
			}
			if packet.IsTokenized() != tt.isTokenized {
				t.Errorf("IsTokenized() = %v, want %v", packet.IsTokenized(), tt.isTokenized)
			}
		})
	}
}

func TestPacketHeaderSize(t *testing.T) {
	packet := &Packet{
		Type:    TypeData,
		Payload: []byte{},
	}

	data, err := packet.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(data) != HeaderSize {
		t.Errorf("Empty packet size = %d, want %d", len(data), HeaderSize)
	}
}

func BenchmarkPacketMarshal(b *testing.B) {
	packet := &Packet{
		Type:    TypeData,
		SeqNum:  1000,
		Payload: make([]byte, 1000),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = packet.Marshal()
	}
}

func BenchmarkPacketUnmarshal(b *testing.B) {
	packet := &Packet{
		Type:    TypeData,
		SeqNum:  1000,
		Payload: make([]byte, 1000),
	}
	data, _ := packet.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Unmarshal(data)
	}
}

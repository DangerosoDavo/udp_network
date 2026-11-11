package protocol

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	MagicNumber        uint16 = 0x5545 // Wire v2; deliberately incompatible with old peers.
	HeaderSize                = 24
	MaxPayloadSize            = 1100 // Leaves space for authenticated transport wrapping.
	MaxWirePayloadSize        = 1200
	MaxMessageSize            = 255 * MaxPayloadSize
)

type PacketType uint8

const (
	TypeData            PacketType = 0x01
	TypeAck             PacketType = 0x02
	TypeNack            PacketType = 0x03
	TypeKeepAlive       PacketType = 0x04
	TypeHandshake       PacketType = 0x05
	TypeBitmapAck       PacketType = 0x06
	TypeCryptoHandshake PacketType = 0x07
	TypeEncrypted       PacketType = 0x08
	TypeClose           PacketType = 0x09
)

type PacketFlags uint8

const (
	FlagFragment   PacketFlags = 0x01
	FlagCompressed PacketFlags = 0x02
	FlagTokenized  PacketFlags = 0x04
	FlagLastFrag   PacketFlags = 0x08
	FlagEncrypted  PacketFlags = 0x10
	// UnreliableSequenced packets use a separate sequence space and are never ACKed.
	FlagUnreliableSequenced PacketFlags = 0x20
)

type Packet struct {
	Type       PacketType
	Flags      PacketFlags
	SeqNum     uint32
	AckNum     uint32
	FragmentID uint16
	FragOffset uint8
	FragTotal  uint8
	Payload    []byte
	Timestamp  time.Time
	SessionID  uint64
	retries    uint8 // local bookkeeping, never serialized
}

func (p *Packet) validate() error {
	if p.Type < TypeData || p.Type > TypeClose {
		return errors.New("unknown packet type")
	}
	limit := MaxPayloadSize
	if p.Type == TypeEncrypted {
		limit = MaxWirePayloadSize
	}
	if len(p.Payload) > limit {
		return errors.New("payload too large")
	}
	if p.Flags & ^(FlagFragment|FlagCompressed|FlagTokenized|FlagLastFrag|FlagEncrypted|FlagUnreliableSequenced) != 0 {
		return errors.New("unknown packet flags")
	}
	if p.IsFragment() && (p.FragTotal < 2 || p.FragOffset >= p.FragTotal || p.Flags&FlagUnreliableSequenced != 0) {
		return errors.New("invalid fragment metadata")
	}
	if !p.IsFragment() && (p.FragTotal != 0 || p.FragOffset != 0 || p.FragmentID != 0 || p.Flags&FlagLastFrag != 0) {
		return errors.New("unexpected fragment metadata")
	}
	if p.Type != TypeData && p.Type != TypeEncrypted && p.Flags != 0 {
		return errors.New("invalid control flags")
	}
	if (p.Type == TypeAck || p.Type == TypeNack || p.Type == TypeKeepAlive || p.Type == TypeHandshake || p.Type == TypeClose) && len(p.Payload) != 0 {
		return errors.New("unexpected control payload")
	}
	if p.Type == TypeBitmapAck && len(p.Payload) != 8 {
		return errors.New("invalid bitmap ACK")
	}
	return nil
}

func (p *Packet) Marshal() ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	buf := make([]byte, HeaderSize+len(p.Payload))

	binary.BigEndian.PutUint16(buf[0:2], MagicNumber)
	buf[2] = uint8(p.Type)
	buf[3] = uint8(p.Flags)
	binary.BigEndian.PutUint32(buf[4:8], p.SeqNum)
	binary.BigEndian.PutUint32(buf[8:12], p.AckNum)
	binary.BigEndian.PutUint16(buf[12:14], p.FragmentID)
	buf[14] = p.FragOffset
	buf[15] = p.FragTotal
	binary.BigEndian.PutUint64(buf[16:24], p.SessionID)

	copy(buf[HeaderSize:], p.Payload)

	return buf, nil
}

func Unmarshal(data []byte) (*Packet, error) {
	if len(data) < HeaderSize {
		return nil, errors.New("packet too small")
	}

	magic := binary.BigEndian.Uint16(data[0:2])
	if magic != MagicNumber {
		return nil, errors.New("invalid magic number")
	}

	p := &Packet{
		Type:       PacketType(data[2]),
		Flags:      PacketFlags(data[3]),
		SeqNum:     binary.BigEndian.Uint32(data[4:8]),
		AckNum:     binary.BigEndian.Uint32(data[8:12]),
		FragmentID: binary.BigEndian.Uint16(data[12:14]),
		FragOffset: data[14],
		FragTotal:  data[15],
		Timestamp:  time.Now(),
		SessionID:  binary.BigEndian.Uint64(data[16:24]),
	}
	// Validate the length and header before allocating attacker-controlled payload.
	p.Payload = data[HeaderSize:]
	if err := p.validate(); err != nil {
		return nil, err
	}

	if len(data) > HeaderSize {
		p.Payload = make([]byte, len(data)-HeaderSize)
		copy(p.Payload, data[HeaderSize:])
	}

	return p, nil
}

func (p *Packet) IsFragment() bool {
	return p.Flags&FlagFragment != 0
}

func (p *Packet) IsLastFragment() bool {
	return p.Flags&FlagLastFrag != 0
}

func (p *Packet) IsTokenized() bool {
	return p.Flags&FlagTokenized != 0
}

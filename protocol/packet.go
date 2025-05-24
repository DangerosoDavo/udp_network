package protocol

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	MagicNumber uint16 = 0x5544 // "UD" in hex
	HeaderSize  = 16
	MaxPayloadSize = 1400 // Safe UDP payload size
)

type PacketType uint8

const (
	TypeData      PacketType = 0x01
	TypeAck       PacketType = 0x02
	TypeNack      PacketType = 0x03
	TypeKeepAlive PacketType = 0x04
	TypeHandshake PacketType = 0x05
	TypeBitmapAck PacketType = 0x06
)

type PacketFlags uint8

const (
	FlagFragment   PacketFlags = 0x01
	FlagCompressed PacketFlags = 0x02
	FlagTokenized  PacketFlags = 0x04
	FlagLastFrag   PacketFlags = 0x08
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
}

func (p *Packet) Marshal() ([]byte, error) {
	if len(p.Payload) > MaxPayloadSize {
		return nil, errors.New("payload too large")
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
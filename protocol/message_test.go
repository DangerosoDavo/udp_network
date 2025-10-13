package protocol

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestMessageCodecEncodeDecodeText(t *testing.T) {
	codec := NewMessageCodec()

	msg := &Message{
		Type:      MessageTypeText,
		ID:        "test-123",
		Timestamp: 1234567890,
		Data:      "Hello, World!",
	}

	// Encode
	data, err := codec.Encode(msg)
	if err != nil {
		t.Fatalf("Encode() failed: %v", err)
	}

	// Decode
	decoded, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode() failed: %v", err)
	}

	// Verify
	if decoded.Type != msg.Type {
		t.Errorf("Type mismatch: got %v, want %v", decoded.Type, msg.Type)
	}
	if decoded.ID != msg.ID {
		t.Errorf("ID mismatch: got %v, want %v", decoded.ID, msg.ID)
	}
	if decoded.Data != msg.Data {
		t.Errorf("Data mismatch: got %v, want %v", decoded.Data, msg.Data)
	}
}

func TestMessageCodecEncodeDecodeJSON(t *testing.T) {
	codec := NewMessageCodec()

	msg := &Message{
		Type:      MessageTypeJSON,
		Timestamp: 1234567890,
		Data: map[string]interface{}{
			"key1": "value1",
			"key2": float64(42),
			"key3": true,
		},
	}

	// Encode
	data, err := codec.Encode(msg)
	if err != nil {
		t.Fatalf("Encode() failed: %v", err)
	}

	// Decode
	decoded, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode() failed: %v", err)
	}

	// Verify
	if decoded.Type != msg.Type {
		t.Errorf("Type mismatch: got %v, want %v", decoded.Type, msg.Type)
	}

	dataMap, ok := decoded.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Decoded data is not a map")
	}

	if dataMap["key1"] != "value1" {
		t.Errorf("key1 mismatch: got %v, want value1", dataMap["key1"])
	}
	if dataMap["key2"] != float64(42) {
		t.Errorf("key2 mismatch: got %v, want 42", dataMap["key2"])
	}
	if dataMap["key3"] != true {
		t.Errorf("key3 mismatch: got %v, want true", dataMap["key3"])
	}
}

func TestFragmentMessageSingleChunk(t *testing.T) {
	data := []byte("small data")
	maxSize := 100

	fragments, fragID := FragmentMessage(data, maxSize)

	if len(fragments) != 1 {
		t.Errorf("Expected 1 fragment, got %d", len(fragments))
	}

	if fragID != 0 {
		t.Errorf("Expected fragID 0 for single chunk, got %d", fragID)
	}
}

func TestFragmentMessageMultipleChunks(t *testing.T) {
	data := make([]byte, 500)
	for i := range data {
		data[i] = byte(i % 256)
	}
	maxSize := 100

	fragments, fragID := FragmentMessage(data, maxSize)

	expectedFragments := (len(data) + maxSize - 1) / maxSize
	if len(fragments) != expectedFragments {
		t.Errorf("Expected %d fragments, got %d", expectedFragments, len(fragments))
	}

	if fragID == 0 {
		t.Error("Expected non-zero fragID for multiple chunks")
	}

	// Verify total size
	totalSize := 0
	for _, frag := range fragments {
		totalSize += len(frag)
	}

	if totalSize != len(data) {
		t.Errorf("Total fragment size %d != original data size %d", totalSize, len(data))
	}
}

func TestFragmentIDUnique(t *testing.T) {
	// Reset the global counter to ensure test isolation
	atomic.StoreUint32(&globalFragmentID, 0)

	data := make([]byte, 200)
	maxSize := 100

	// Generate multiple fragment IDs
	ids := make(map[uint16]bool)
	for i := 0; i < 100; i++ {
		_, fragID := FragmentMessage(data, maxSize)
		if ids[fragID] {
			t.Errorf("Duplicate fragment ID: %d", fragID)
		}
		ids[fragID] = true
	}

	// Should have 100 unique IDs
	if len(ids) != 100 {
		t.Errorf("Expected 100 unique IDs, got %d", len(ids))
	}
}

func TestFragmentIDConcurrent(t *testing.T) {
	// Reset the global counter
	atomic.StoreUint32(&globalFragmentID, 0)

	data := make([]byte, 200)
	maxSize := 100

	// Generate IDs concurrently
	const goroutines = 100
	ids := make(chan uint16, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, fragID := FragmentMessage(data, maxSize)
			ids <- fragID
		}()
	}

	wg.Wait()
	close(ids)

	// Collect IDs
	idMap := make(map[uint16]bool)
	for id := range ids {
		if idMap[id] {
			t.Errorf("Duplicate fragment ID in concurrent test: %d", id)
		}
		idMap[id] = true
	}

	// Should have unique IDs
	if len(idMap) != goroutines {
		t.Errorf("Expected %d unique IDs, got %d", goroutines, len(idMap))
	}
}

func TestFragmentAssembler(t *testing.T) {
	assembler := NewFragmentAssembler()

	fragID := uint16(12345)
	total := uint8(3)

	// Add fragments out of order
	frag2 := []byte("fragment 2")
	result, complete := assembler.AddFragment(fragID, 1, total, frag2)
	if complete {
		t.Error("Should not be complete after 1/3 fragments")
	}

	frag3 := []byte("fragment 3")
	result, complete = assembler.AddFragment(fragID, 2, total, frag3)
	if complete {
		t.Error("Should not be complete after 2/3 fragments")
	}

	frag1 := []byte("fragment 1")
	result, complete = assembler.AddFragment(fragID, 0, total, frag1)
	if !complete {
		t.Error("Should be complete after 3/3 fragments")
	}

	// Verify reassembled data
	expected := append(append(frag1, frag2...), frag3...)
	if string(result) != string(expected) {
		t.Errorf("Reassembled data mismatch:\ngot  %v\nwant %v", result, expected)
	}
}

func TestFragmentAssemblerMultipleTransfers(t *testing.T) {
	assembler := NewFragmentAssembler()

	// Transfer 1
	fragID1 := uint16(100)
	_, complete := assembler.AddFragment(fragID1, 0, 2, []byte("A"))
	if complete {
		t.Error("Transfer 1 should not be complete")
	}

	// Transfer 2
	fragID2 := uint16(200)
	_, complete = assembler.AddFragment(fragID2, 0, 2, []byte("X"))
	if complete {
		t.Error("Transfer 2 should not be complete")
	}

	// Complete transfer 1
	result1, complete := assembler.AddFragment(fragID1, 1, 2, []byte("B"))
	if !complete {
		t.Error("Transfer 1 should be complete")
	}
	if string(result1) != "AB" {
		t.Errorf("Transfer 1 result: got %s, want AB", string(result1))
	}

	// Complete transfer 2
	result2, complete := assembler.AddFragment(fragID2, 1, 2, []byte("Y"))
	if !complete {
		t.Error("Transfer 2 should be complete")
	}
	if string(result2) != "XY" {
		t.Errorf("Transfer 2 result: got %s, want XY", string(result2))
	}
}

func TestMessageTypes(t *testing.T) {
	types := []MessageType{
		MessageTypeText,
		MessageTypeJSON,
		MessageTypeCommand,
		MessageTypeBinary,
		MessageTypeChunked,
	}

	// Verify all types are distinct
	seen := make(map[MessageType]bool)
	for _, mt := range types {
		if seen[mt] {
			t.Errorf("Duplicate message type: %s", mt)
		}
		seen[mt] = true
	}
}

func BenchmarkMessageEncode(b *testing.B) {
	codec := NewMessageCodec()
	msg := &Message{
		Type:      MessageTypeText,
		Timestamp: 1234567890,
		Data:      "Hello, World!",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.Encode(msg)
	}
}

func BenchmarkMessageDecode(b *testing.B) {
	codec := NewMessageCodec()
	msg := &Message{
		Type:      MessageTypeText,
		Timestamp: 1234567890,
		Data:      "Hello, World!",
	}
	data, _ := codec.Encode(msg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.Decode(data)
	}
}

func BenchmarkFragmentMessage(b *testing.B) {
	data := make([]byte, 10000)
	maxSize := 1400

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = FragmentMessage(data, maxSize)
	}
}

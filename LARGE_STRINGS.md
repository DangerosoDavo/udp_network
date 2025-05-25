# Large String Support in UDP Network

This module now supports sending strings much larger than the UDP packet size limit (65KB) by implementing automatic chunking and reassembly.

## Features

- **Automatic chunking**: Large strings are automatically split into 300KB chunks
- **Reliable delivery**: Each chunk is tracked and can be retried
- **Progress tracking**: Monitor transfer progress with callbacks
- **Metadata support**: Attach metadata to transfers
- **Checksum verification**: Each chunk is verified with SHA256
- **Concurrent transfers**: Multiple large strings can be transferred simultaneously

## Current Limitations

1. **Maximum string size**: Theoretically unlimited, but practically limited by:
   - Available memory (both sender and receiver need to hold the full string)
   - Network bandwidth and latency
   - Transfer timeout (5 minutes by default)

2. **Current implementation**: The fragmentation system uses `uint8` for fragment counts, limiting each message to 255 fragments. However, the chunking system bypasses this by sending multiple independent messages.

## Usage

### Automatic Chunking (NEW!)

The `SendText` method now automatically uses chunked transfer for strings larger than 250KB:

```go
// For strings <= 250KB: uses regular message protocol
client.SendText("Small message")

// For strings > 250KB: automatically uses chunked transfer
client.SendText(largeString) // Automatically chunks if > 250KB
```

This works for both regular and secure clients!

### Manual Large String Sending

For more control over the transfer, you can use `SendLargeString` directly:

```go
import "github.com/DangerosoDavo/udp_network"

// Create client
client, err := udp_network.NewClient()
if err != nil {
    log.Fatal(err)
}
client.Start()
defer client.Stop()

// Connect to server
if err := client.Connect("server:8080"); err != nil {
    log.Fatal(err)
}

// Create a large string (e.g., 8MB)
largeString := strings.Repeat("Hello World! ", 700000) // ~8MB

// Send with progress tracking
err = client.SendLargeString(largeString, &udp_network.LargeStringOptions{
    Metadata: map[string]interface{}{
        "filename": "large_file.txt",
        "type":     "text",
    },
    OnProgress: func(sent, total int) {
        fmt.Printf("Progress: %d/%d chunks (%.1f%%)\n", 
            sent, total, float64(sent)/float64(total)*100)
    },
})
```

### Receiving Large Strings

```go
// Create server
server, err := udp_network.NewServer(":8080")
if err != nil {
    log.Fatal(err)
}

// Enable large string receiver
receiver := udp_network.EnableLargeStringReceiver(server)

// Set handler for completed transfers
receiver.SetDefaultHandler(func(data string, metadata map[string]interface{}) {
    fmt.Printf("Received large string: %.2f MB\n", 
        float64(len(data))/(1024*1024))
    // Process the complete string
})

server.Start()
```

### Secure Large String Transfer

```go
// Secure client
client, err := udp_network.NewSecureClient()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

if err := client.Connect("server:8443"); err != nil {
    log.Fatal(err)
}

// Send large string with encryption
err = client.SendLargeString(largeString, &udp_network.LargeStringOptions{
    OnProgress: func(sent, total int) {
        fmt.Printf("Encrypted chunk %d/%d\n", sent, total)
    },
})
```

## How It Works

1. **Chunking**: Large strings are split into 300KB chunks
2. **Transfer ID**: Each transfer gets a unique ID
3. **Metadata**: Each chunk includes:
   - Transfer ID
   - Chunk index and total count
   - SHA256 checksum
   - Custom metadata
4. **Reassembly**: Receiver collects chunks and reassembles when complete
5. **Cleanup**: Incomplete transfers timeout after 5 minutes

## Performance Considerations

For an 8MB string over a typical network:

- **Chunks required**: ~28 chunks
- **Transfer time**: 1-5 seconds depending on network
- **Memory usage**: 8MB on sender + 8MB on receiver
- **Network overhead**: ~1% for checksums and metadata

## Recommendations

1. **For strings > 10MB**: Consider using external storage (S3, etc.) and sending URLs
2. **For binary data**: Compress before sending to reduce size
3. **For streaming**: Implement a streaming API instead of loading entire string
4. **For production**: Add rate limiting and connection-specific limits

## Example: Sending 8MB String

```go
// Generate 8MB string
size := 8 * 1024 * 1024
largeString := strings.Repeat("A", size)

// Send with retry and progress
options := &udp_network.LargeStringOptions{
    RetryAttempts: 3,
    ChunkTimeout:  10 * time.Second,
    Metadata: map[string]interface{}{
        "content-type": "text/plain",
        "size":         size,
    },
    OnProgress: func(sent, total int) {
        percent := float64(sent) / float64(total) * 100
        fmt.Printf("\rProgress: %.1f%% (%d/%d chunks)", percent, sent, total)
    },
}

if err := client.SendLargeString(largeString, options); err != nil {
    log.Fatal("Failed to send:", err)
}
```

## Limitations and Future Improvements

### Current Limitations:
- No streaming support (entire string must be in memory)
- No compression (could reduce 8MB text to 2-3MB)
- Sequential chunk sending (could parallelize)
- No partial retransmission (resends entire chunk on failure)

### Planned Improvements:
1. Streaming API for very large files
2. Built-in compression options
3. Parallel chunk transmission
4. Adaptive chunk sizing based on network conditions
5. Resume capability for interrupted transfers
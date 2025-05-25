package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/DangerosoDavo/udp_network"
)

var (
	host     = flag.String("host", "", "Server hostname or IP address (required)")
	port     = flag.String("port", "8080", "Server port")
	secure   = flag.Bool("secure", false, "Use secure connection")
	size     = flag.Float64("size", 1.0, "Size in MB to send")
	chunks   = flag.Int("chunks", 0, "Override number of chunks (0 = auto)")
	compress = flag.Bool("compress", false, "Compress before sending (not implemented)")
	verify   = flag.Bool("verify", true, "Verify data integrity")
	timeout  = flag.Duration("timeout", 5*time.Minute, "Transfer timeout")
)

// TestResults stores the test results
type TestResults struct {
	Success      bool
	TransferSize int
	ChunksUsed   int
	TransferTime time.Duration
	Throughput   float64 // MB/s
	Checksum     string
	Errors       []error
}

func generateTestString(sizeMB float64) (string, string) {
	fmt.Printf("📝 Generating %.2f MB test string...\n", sizeMB)

	// Create a realistic text pattern
	patterns := []string{
		"The quick brown fox jumps over the lazy dog. ",
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit. ",
		"This is a test of large string transfer over UDP network. ",
		"0123456789 ABCDEFGHIJKLMNOPQRSTUVWXYZ abcdefghijklmnopqrstuvwxyz. ",
	}

	sizeBytes := int(sizeMB * 1024 * 1024)
	var builder strings.Builder
	builder.Grow(sizeBytes)

	patternIndex := 0
	written := 0

	for written < sizeBytes {
		pattern := patterns[patternIndex%len(patterns)]
		if written+len(pattern) > sizeBytes {
			builder.WriteString(pattern[:sizeBytes-written])
			break
		}
		builder.WriteString(pattern)
		written += len(pattern)
		patternIndex++

		// Progress indicator
		if written%(1024*1024) == 0 {
			fmt.Printf("\r📝 Generating... %.1f%%", float64(written)/float64(sizeBytes)*100)
		}
	}

	result := builder.String()

	// Calculate checksum
	hash := sha256.Sum256([]byte(result))
	checksum := hex.EncodeToString(hash[:])

	fmt.Printf("\r✅ Generated %d bytes (%.2f MB) - Checksum: %s\n",
		len(result), float64(len(result))/(1024*1024), checksum[:16])

	return result, checksum
}

func testRegularLargeString(address string) (*TestResults, error) {
	results := &TestResults{}

	fmt.Printf("\n🔓 Testing regular large string transfer to %s\n", address)
	fmt.Println("=" + strings.Repeat("=", 50))

	// Create client
	client, err := udp_network.NewClient()
	if err != nil {
		return results, fmt.Errorf("failed to create client: %v", err)
	}

	client.Start()
	defer client.Stop()

	// Connect
	fmt.Printf("🔗 Connecting to %s...\n", address)
	if err := client.Connect(address); err != nil {
		return results, fmt.Errorf("connection failed: %v", err)
	}
	fmt.Println("✅ Connected!")

	// Wait for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	// Generate test data
	testString, checksum := generateTestString(*size)
	results.TransferSize = len(testString)
	results.Checksum = checksum

	// Calculate expected chunks
	chunkSize := 300 * 1024
	expectedChunks := (len(testString) + chunkSize - 1) / chunkSize
	if *chunks > 0 {
		expectedChunks = *chunks
	}
	results.ChunksUsed = expectedChunks

	fmt.Printf("\n📊 Transfer Details:\n")
	fmt.Printf("   Size: %.2f MB (%d bytes)\n", float64(len(testString))/(1024*1024), len(testString))
	fmt.Printf("   Chunks: %d × 300KB\n", expectedChunks)
	fmt.Printf("   Checksum: %s\n", checksum[:16])

	// Send with progress tracking
	fmt.Printf("\n📤 Starting transfer...\n")

	var progressMu sync.Mutex
	lastProgress := 0

	startTime := time.Now()

	err = client.SendLargeString(testString, &udp_network.LargeStringOptions{
		Metadata: map[string]interface{}{
			"test":     "remote-large-string",
			"size_mb":  *size,
			"checksum": checksum,
			"sender":   "remote-test-client",
		},
		OnProgress: func(sent, total int) {
			progressMu.Lock()
			defer progressMu.Unlock()

			progress := int(float64(sent) / float64(total) * 100)
			if progress > lastProgress {
				lastProgress = progress
				fmt.Printf("\r📊 Progress: %d/%d chunks (%d%%) - %.2f MB sent",
					sent, total, progress, float64(sent*chunkSize)/(1024*1024))
			}
		},
		RetryAttempts: 3,
		ChunkTimeout:  30 * time.Second,
	})

	transferTime := time.Since(startTime)
	results.TransferTime = transferTime

	if err != nil {
		results.Errors = append(results.Errors, err)
		return results, fmt.Errorf("transfer failed: %v", err)
	}

	fmt.Printf("\n✅ Transfer completed successfully!\n")

	// Calculate throughput
	results.Throughput = float64(len(testString)) / (1024 * 1024) / transferTime.Seconds()
	results.Success = true

	return results, nil
}

func testSecureLargeString(address string) (*TestResults, error) {
	results := &TestResults{}

	fmt.Printf("\n🔐 Testing secure large string transfer to %s\n", address)
	fmt.Println("=" + strings.Repeat("=", 50))

	// Create secure client
	client, err := udp_network.NewSecureClient()
	if err != nil {
		return results, fmt.Errorf("failed to create secure client: %v", err)
	}
	defer client.Close()

	// Connect
	fmt.Printf("🔗 Establishing secure connection to %s...\n", address)
	if err := client.Connect(address); err != nil {
		return results, fmt.Errorf("secure connection failed: %v", err)
	}
	fmt.Println("✅ Secure connection established (ChaCha20-Poly1305)")

	// Wait for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	// Generate test data
	testString, checksum := generateTestString(*size)
	results.TransferSize = len(testString)
	results.Checksum = checksum

	// Calculate expected chunks
	chunkSize := 300 * 1024
	expectedChunks := (len(testString) + chunkSize - 1) / chunkSize
	results.ChunksUsed = expectedChunks

	fmt.Printf("\n📊 Transfer Details:\n")
	fmt.Printf("   Size: %.2f MB (%d bytes)\n", float64(len(testString))/(1024*1024), len(testString))
	fmt.Printf("   Chunks: %d × 300KB\n", expectedChunks)
	fmt.Printf("   Encryption: ChaCha20-Poly1305\n")
	fmt.Printf("   Checksum: %s\n", checksum[:16])

	// Send with progress tracking
	fmt.Printf("\n📤 Starting encrypted transfer...\n")

	var progressMu sync.Mutex
	lastProgress := 0

	startTime := time.Now()

	err = client.SendLargeString(testString, &udp_network.LargeStringOptions{
		Metadata: map[string]interface{}{
			"test":      "remote-secure-large-string",
			"size_mb":   *size,
			"checksum":  checksum,
			"sender":    "remote-test-client",
			"encrypted": true,
		},
		OnProgress: func(sent, total int) {
			progressMu.Lock()
			defer progressMu.Unlock()

			progress := int(float64(sent) / float64(total) * 100)
			if progress > lastProgress {
				lastProgress = progress
				fmt.Printf("\r📊 Progress: %d/%d encrypted chunks (%d%%) - %.2f MB sent",
					sent, total, progress, float64(sent*chunkSize)/(1024*1024))
			}
		},
		RetryAttempts: 3,
		ChunkTimeout:  30 * time.Second,
	})

	transferTime := time.Since(startTime)
	results.TransferTime = transferTime

	if err != nil {
		results.Errors = append(results.Errors, err)
		return results, fmt.Errorf("encrypted transfer failed: %v", err)
	}

	fmt.Printf("\n✅ Encrypted transfer completed successfully!\n")

	// Calculate throughput
	results.Throughput = float64(len(testString)) / (1024 * 1024) / transferTime.Seconds()
	results.Success = true

	return results, nil
}

func printResults(results *TestResults, secure bool) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	if secure {
		fmt.Println("🔐 SECURE LARGE STRING TRANSFER RESULTS")
	} else {
		fmt.Println("📊 LARGE STRING TRANSFER RESULTS")
	}
	fmt.Println(strings.Repeat("=", 60))

	if results.Success {
		fmt.Println("✅ Status: SUCCESS")
	} else {
		fmt.Println("❌ Status: FAILED")
	}

	fmt.Printf("\n📈 Transfer Statistics:\n")
	fmt.Printf("   Size:       %.2f MB (%d bytes)\n",
		float64(results.TransferSize)/(1024*1024), results.TransferSize)
	fmt.Printf("   Chunks:     %d\n", results.ChunksUsed)
	fmt.Printf("   Time:       %v\n", results.TransferTime)
	fmt.Printf("   Throughput: %.2f MB/s\n", results.Throughput)

	if results.TransferTime > 0 {
		fmt.Printf("   Per-chunk:  %.2f ms\n",
			float64(results.TransferTime.Milliseconds())/float64(results.ChunksUsed))
	}

	if *verify && results.Checksum != "" {
		fmt.Printf("\n🔍 Data Integrity:\n")
		fmt.Printf("   Checksum: %s\n", results.Checksum[:16])
		fmt.Println("   Note: Server should verify this matches received data")
	}

	if len(results.Errors) > 0 {
		fmt.Printf("\n⚠️  Errors:\n")
		for i, err := range results.Errors {
			fmt.Printf("   %d. %v\n", i+1, err)
		}
	}

	// Performance assessment
	fmt.Printf("\n🏆 Performance Grade: ")
	if results.Throughput > 10 {
		fmt.Println("EXCELLENT (>10 MB/s)")
	} else if results.Throughput > 5 {
		fmt.Println("GOOD (>5 MB/s)")
	} else if results.Throughput > 1 {
		fmt.Println("FAIR (>1 MB/s)")
	} else {
		fmt.Println("POOR (<1 MB/s)")
	}
}

func main() {
	flag.Parse()

	if *host == "" {
		fmt.Println("UDP Network Remote Large String Test")
		fmt.Println("===================================")
		fmt.Println()
		fmt.Println("Tests sending large strings (MB-sized) over UDP to remote servers.")
		fmt.Println()
		fmt.Println("Usage: go run tests/remote-large-string-test/main.go -host <ip> [options]")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  # Test 1MB string to regular server")
		fmt.Println("  go run tests/remote-large-string-test/main.go -host 192.168.1.100")
		fmt.Println()
		fmt.Println("  # Test 8MB string to secure server")
		fmt.Println("  go run tests/remote-large-string-test/main.go -host 192.168.1.100 -port 8443 -secure -size 8")
		fmt.Println()
		fmt.Println("  # Test 5MB with custom timeout")
		fmt.Println("  go run tests/remote-large-string-test/main.go -host example.com -size 5 -timeout 10m")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -host string     Server IP/hostname (required)")
		fmt.Println("  -port string     Server port (default: 8080)")
		fmt.Println("  -secure         Use secure connection")
		fmt.Println("  -size float     Size in MB (default: 1.0)")
		fmt.Println("  -verify         Verify data integrity (default: true)")
		fmt.Println("  -timeout dur    Transfer timeout (default: 5m)")
		fmt.Println()
		fmt.Println("Notes:")
		fmt.Println("  - Chunk size is fixed at 300KB")
		fmt.Println("  - 8MB requires ~28 chunks")
		fmt.Println("  - Encryption adds ~5-10% overhead")
		fmt.Println()
		log.Fatal("Host is required")
	}

	address := *host + ":" + *port

	fmt.Println("🌐 UDP Network Remote Large String Test")
	fmt.Println("======================================")
	fmt.Printf("Target: %s\n", address)
	fmt.Printf("Size: %.1f MB\n", *size)
	fmt.Printf("Mode: %s\n", map[bool]string{true: "Secure (Encrypted)", false: "Regular"}[*secure])

	// Validate size
	if *size > 100 {
		fmt.Printf("\n⚠️  Warning: %.1f MB is very large. Consider:\n", *size)
		fmt.Println("   - Network timeouts")
		fmt.Println("   - Memory usage")
		fmt.Println("   - Using external storage instead")
		fmt.Println()
	}

	// Run test
	var results *TestResults
	var err error

	if *secure {
		results, err = testSecureLargeString(address)
	} else {
		results, err = testRegularLargeString(address)
	}

	if err != nil {
		fmt.Printf("\n❌ Test failed: %v\n", err)
		if results != nil {
			printResults(results, *secure)
		}
		log.Fatal(err)
	}

	printResults(results, *secure)

	fmt.Println("\n✅ Large string remote test completed successfully!")
	fmt.Println("\nNote: Check server logs to verify the string was received correctly.")
}

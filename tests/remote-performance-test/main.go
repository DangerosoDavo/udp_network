package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

var (
	host       = flag.String("host", "", "Server hostname or IP address (required)")
	port       = flag.String("port", "8080", "Server port (8080 for regular, 8443 for secure)")
	secure     = flag.Bool("secure", false, "Use secure connection")
	messages   = flag.Int("messages", 1000, "Total number of messages to send")
	size       = flag.Int("size", 1024, "Size of each message in bytes")
	rate       = flag.Int("rate", 100, "Messages per second (0 = unlimited)")
	duration   = flag.Duration("duration", 0, "Test duration (0 = send all messages)")
	concurrent = flag.Int("concurrent", 1, "Number of concurrent senders")
	warmup     = flag.Int("warmup", 10, "Number of warmup messages")
	timeout    = flag.Duration("timeout", 30*time.Second, "Connection timeout")
)

type PerformanceStats struct {
	TotalSent        int64
	TotalReceived    int64
	TotalBytes       int64
	StartTime        time.Time
	EndTime          time.Time
	FirstMessageTime time.Time
	LastMessageTime  time.Time
	Latencies        []time.Duration
	mu               sync.Mutex
}

func (ps *PerformanceStats) AddLatency(latency time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Latencies = append(ps.Latencies, latency)
}

func (ps *PerformanceStats) Calculate() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(ps.Latencies) == 0 {
		return
	}

	// Sort latencies for percentile calculations
	sort.Slice(ps.Latencies, func(i, j int) bool {
		return ps.Latencies[i] < ps.Latencies[j]
	})
}

func (ps *PerformanceStats) GetPercentile(p float64) time.Duration {
	if len(ps.Latencies) == 0 {
		return 0
	}
	index := int(float64(len(ps.Latencies)-1) * p / 100.0)
	return ps.Latencies[index]
}

func (ps *PerformanceStats) GetAverage() time.Duration {
	if len(ps.Latencies) == 0 {
		return 0
	}
	var total time.Duration
	for _, l := range ps.Latencies {
		total += l
	}
	return total / time.Duration(len(ps.Latencies))
}

func testPerformance(address string, isSecure bool) (*PerformanceStats, error) {
	stats := &PerformanceStats{
		StartTime: time.Now(),
	}

	connType := "Regular"
	if isSecure {
		connType = "Secure"
	}

	fmt.Printf("\n🚀 %s UDP Performance Test\n", connType)
	fmt.Printf("=====================================\n")
	fmt.Printf("Target: %s\n", address)
	fmt.Printf("Messages: %d x %d bytes\n", *messages, *size)
	fmt.Printf("Rate: %d msg/sec\n", *rate)
	fmt.Printf("Concurrent senders: %d\n", *concurrent)
	fmt.Printf("Warmup messages: %d\n", *warmup)

	// Generate test payload
	testData := make([]byte, *size)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	testMessage := string(testData)

	if isSecure {
		// Test secure connection
		client, err := udp_network.NewSecureClient()
		if err != nil {
			return stats, fmt.Errorf("failed to create secure client: %v", err)
		}
		defer client.Close()

		fmt.Printf("\n🔐 Establishing secure connection...\n")
		if err := client.Connect(address); err != nil {
			return stats, fmt.Errorf("secure connection failed: %v", err)
		}
		fmt.Println("✅ Secure connection established (ChaCha20-Poly1305)")

		// Warmup
		fmt.Printf("\n🔥 Warming up with %d messages...\n", *warmup)
		for i := 0; i < *warmup; i++ {
			client.SendText(fmt.Sprintf("WARMUP_%d_%s", i, testMessage))
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond)

		// Performance test
		fmt.Printf("\n📊 Starting performance test...\n")

		var wg sync.WaitGroup
		messageCount := *messages
		messagesPerSender := messageCount / *concurrent

		// Rate limiter setup
		var rateLimiter *time.Ticker
		if *rate > 0 {
			interval := time.Second / time.Duration(*rate)
			rateLimiter = time.NewTicker(interval)
			defer rateLimiter.Stop()
		}

		startTime := time.Now()
		stats.FirstMessageTime = startTime

		// Launch concurrent senders
		for sender := 0; sender < *concurrent; sender++ {
			wg.Add(1)
			go func(senderID int) {
				defer wg.Done()

				for i := 0; i < messagesPerSender; i++ {
					// Rate limiting
					if rateLimiter != nil {
						<-rateLimiter.C
					}

					msgStart := time.Now()
					message := fmt.Sprintf("PERF_%d_%d_%d_%s", senderID, i, msgStart.UnixNano(), testMessage)

					if err := client.SendText(message); err != nil {
						fmt.Printf("❌ Send error: %v\n", err)
					} else {
						atomic.AddInt64(&stats.TotalSent, 1)
						atomic.AddInt64(&stats.TotalBytes, int64(*size))

						// For secure connections, we estimate latency
						stats.AddLatency(time.Since(msgStart))
					}

					// Check if duration limit reached
					if *duration > 0 && time.Since(startTime) > *duration {
						return
					}
				}
			}(sender)
		}

		// Progress reporter
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					sent := atomic.LoadInt64(&stats.TotalSent)
					elapsed := time.Since(startTime)
					rate := float64(sent) / elapsed.Seconds()
					fmt.Printf("📈 Progress: %d messages sent (%.2f msg/sec)\n", sent, rate)

					if sent >= int64(messageCount) {
						return
					}
				}
			}
		}()

		wg.Wait()
		stats.LastMessageTime = time.Now()
		stats.EndTime = time.Now()

	} else {
		// Test regular connection with response tracking
		client, err := udp_network.NewClient()
		if err != nil {
			return stats, fmt.Errorf("failed to create client: %v", err)
		}
		client.Start()
		defer client.Stop()

		// Message tracking for latency measurement
		messageTimestamps := make(map[string]time.Time)
		var timestampMu sync.Mutex

		// Set up response handler
		client.SetMessageHandler(func(msg *protocol.Message) {
			atomic.AddInt64(&stats.TotalReceived, 1)

			// Try to extract timestamp from message to calculate latency
			if text, ok := msg.Data.(string); ok {
				parts := strings.Split(text, "_")
				if len(parts) >= 4 && parts[0] == "PERF" {
					timestampMu.Lock()
					if sentTime, exists := messageTimestamps[text]; exists {
						latency := time.Since(sentTime)
						stats.AddLatency(latency)
						delete(messageTimestamps, text)
					}
					timestampMu.Unlock()
				}
			}
		})

		fmt.Printf("\n🔗 Connecting to server...\n")
		if err := client.Connect(address); err != nil {
			return stats, fmt.Errorf("connection failed: %v", err)
		}
		fmt.Println("✅ Connection established")

		// Warmup
		fmt.Printf("\n🔥 Warming up with %d messages...\n", *warmup)
		for i := 0; i < *warmup; i++ {
			client.SendText(fmt.Sprintf("WARMUP_%d", i))
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond)

		// Performance test
		fmt.Printf("\n📊 Starting performance test...\n")

		var wg sync.WaitGroup
		messageCount := *messages
		messagesPerSender := messageCount / *concurrent

		// Rate limiter setup
		var rateLimiter *time.Ticker
		if *rate > 0 {
			interval := time.Second / time.Duration(*rate)
			rateLimiter = time.NewTicker(interval)
			defer rateLimiter.Stop()
		}

		startTime := time.Now()
		stats.FirstMessageTime = startTime

		// Launch concurrent senders
		for sender := 0; sender < *concurrent; sender++ {
			wg.Add(1)
			go func(senderID int) {
				defer wg.Done()

				for i := 0; i < messagesPerSender; i++ {
					// Rate limiting
					if rateLimiter != nil {
						<-rateLimiter.C
					}

					msgStart := time.Now()
					message := fmt.Sprintf("PERF_%d_%d_%d_%s", senderID, i, msgStart.UnixNano(), testMessage)

					// Track timestamp for latency calculation
					timestampMu.Lock()
					messageTimestamps[message] = msgStart
					timestampMu.Unlock()

					if err := client.SendText(message); err != nil {
						fmt.Printf("❌ Send error: %v\n", err)
					} else {
						atomic.AddInt64(&stats.TotalSent, 1)
						atomic.AddInt64(&stats.TotalBytes, int64(*size))
					}

					// Check if duration limit reached
					if *duration > 0 && time.Since(startTime) > *duration {
						return
					}
				}
			}(sender)
		}

		// Progress reporter
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			lastSent := int64(0)
			lastReceived := int64(0)

			for {
				select {
				case <-ticker.C:
					sent := atomic.LoadInt64(&stats.TotalSent)
					received := atomic.LoadInt64(&stats.TotalReceived)

					sendRate := float64(sent - lastSent)
					recvRate := float64(received - lastReceived)

					fmt.Printf("📈 Progress: %d sent (%.0f/s), %d received (%.0f/s)\n",
						sent, sendRate, received, recvRate)

					lastSent = sent
					lastReceived = received

					if sent >= int64(messageCount) {
						time.Sleep(2 * time.Second) // Wait for remaining responses
						return
					}
				}
			}
		}()

		wg.Wait()

		// Wait for remaining responses
		fmt.Println("\n⏳ Waiting for remaining responses...")
		time.Sleep(3 * time.Second)

		stats.LastMessageTime = time.Now()
		stats.EndTime = time.Now()
	}

	// Calculate final statistics
	stats.Calculate()
	return stats, nil
}

func printResults(stats *PerformanceStats, isSecure bool) {
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📊 PERFORMANCE TEST RESULTS")
	fmt.Println(strings.Repeat("=", 50))

	duration := stats.EndTime.Sub(stats.StartTime)
	activeDuration := stats.LastMessageTime.Sub(stats.FirstMessageTime)

	fmt.Printf("\n📤 Messages:\n")
	fmt.Printf("   Sent:     %d\n", stats.TotalSent)
	if !isSecure {
		fmt.Printf("   Received: %d (%.1f%%)\n", stats.TotalReceived,
			float64(stats.TotalReceived)/float64(stats.TotalSent)*100)
	}
	fmt.Printf("   Size:     %d bytes each\n", *size)
	fmt.Printf("   Total:    %.2f MB\n", float64(stats.TotalBytes)/(1024*1024))

	fmt.Printf("\n⏱️  Timing:\n")
	fmt.Printf("   Test duration:   %v\n", duration)
	fmt.Printf("   Active duration: %v\n", activeDuration)

	fmt.Printf("\n🚀 Throughput:\n")
	fmt.Printf("   Messages/sec: %.2f\n", float64(stats.TotalSent)/activeDuration.Seconds())
	fmt.Printf("   Bytes/sec:    %.2f MB/s\n", float64(stats.TotalBytes)/(1024*1024)/activeDuration.Seconds())
	fmt.Printf("   Bits/sec:     %.2f Mbps\n", float64(stats.TotalBytes)*8/(1000*1000)/activeDuration.Seconds())

	if len(stats.Latencies) > 0 {
		fmt.Printf("\n📏 Latency (RTT):\n")
		fmt.Printf("   Min:     %v\n", stats.Latencies[0])
		fmt.Printf("   Avg:     %v\n", stats.GetAverage())
		fmt.Printf("   P50:     %v\n", stats.GetPercentile(50))
		fmt.Printf("   P90:     %v\n", stats.GetPercentile(90))
		fmt.Printf("   P95:     %v\n", stats.GetPercentile(95))
		fmt.Printf("   P99:     %v\n", stats.GetPercentile(99))
		fmt.Printf("   Max:     %v\n", stats.Latencies[len(stats.Latencies)-1])
	}

	if !isSecure && stats.TotalReceived < stats.TotalSent {
		lossRate := float64(stats.TotalSent-stats.TotalReceived) / float64(stats.TotalSent) * 100
		fmt.Printf("\n⚠️  Packet Loss: %.2f%% (%d messages)\n",
			lossRate, stats.TotalSent-stats.TotalReceived)
	}

	// Performance grade
	fmt.Printf("\n🏆 Performance Grade: ")
	avgLatency := stats.GetAverage()
	if avgLatency < 10*time.Millisecond {
		fmt.Println("EXCELLENT (< 10ms latency)")
	} else if avgLatency < 50*time.Millisecond {
		fmt.Println("GOOD (< 50ms latency)")
	} else if avgLatency < 100*time.Millisecond {
		fmt.Println("FAIR (< 100ms latency)")
	} else {
		fmt.Println("POOR (> 100ms latency)")
	}

	// Connection quality assessment
	fmt.Printf("\n📡 Connection Quality:\n")
	jitter := stats.GetPercentile(90) - stats.GetPercentile(10)
	fmt.Printf("   Jitter (P90-P10): %v\n", jitter)

	if jitter < 5*time.Millisecond {
		fmt.Println("   Stability: EXCELLENT (very stable)")
	} else if jitter < 20*time.Millisecond {
		fmt.Println("   Stability: GOOD (stable)")
	} else if jitter < 50*time.Millisecond {
		fmt.Println("   Stability: FAIR (some variation)")
	} else {
		fmt.Println("   Stability: POOR (high variation)")
	}
}

func main() {
	flag.Parse()

	if *host == "" {
		fmt.Println("UDP Network Remote Performance Test")
		fmt.Println("==================================")
		fmt.Println()
		fmt.Println("Usage: go run tests/remote-performance-test/main.go -host <ip> [options]")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  # Basic test")
		fmt.Println("  go run tests/remote-performance-test/main.go -host 192.168.1.100")
		fmt.Println()
		fmt.Println("  # Secure server test")
		fmt.Println("  go run tests/remote-performance-test/main.go -host 192.168.1.100 -port 8443 -secure")
		fmt.Println()
		fmt.Println("  # High-load test")
		fmt.Println("  go run tests/remote-performance-test/main.go -host 192.168.1.100 -messages 10000 -rate 1000")
		fmt.Println()
		fmt.Println("  # Concurrent test")
		fmt.Println("  go run tests/remote-performance-test/main.go -host 192.168.1.100 -concurrent 5")
		fmt.Println()
		fmt.Println("  # Large message test")
		fmt.Println("  go run tests/remote-performance-test/main.go -host 192.168.1.100 -size 4096")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -host string      Server IP/hostname (required)")
		fmt.Println("  -port string      Server port (default: 8080)")
		fmt.Println("  -secure          Use secure connection")
		fmt.Println("  -messages int    Total messages to send (default: 1000)")
		fmt.Println("  -size int        Message size in bytes (default: 1024)")
		fmt.Println("  -rate int        Messages per second, 0=unlimited (default: 100)")
		fmt.Println("  -concurrent int  Concurrent senders (default: 1)")
		fmt.Println("  -warmup int      Warmup messages (default: 10)")
		fmt.Println("  -timeout duration Connection timeout (default: 30s)")
		fmt.Println()
		log.Fatal("Host is required")
	}

	address := *host + ":" + *port

	fmt.Println("🌐 UDP Network Remote Performance Test")
	fmt.Println("=====================================")

	stats, err := testPerformance(address, *secure)
	if err != nil {
		fmt.Printf("\n❌ Test failed: %v\n", err)
		log.Fatal(err)
	}

	printResults(stats, *secure)

	fmt.Println("\n✅ Performance test completed successfully!")
}

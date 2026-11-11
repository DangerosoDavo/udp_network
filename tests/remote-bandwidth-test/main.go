package main

import (
	"flag"
	"fmt"
	"github.com/DangerosoDavo/udp_network/internal/exampleconfig"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network"
)

var (
	host      = flag.String("host", "", "Server hostname or IP address (required)")
	port      = flag.String("port", "8080", "Server port")
	secure    = flag.Bool("secure", false, "Use secure connection")
	duration  = flag.Duration("duration", 30*time.Second, "Test duration")
	rampup    = flag.Duration("rampup", 5*time.Second, "Ramp-up time")
	clients   = flag.Int("clients", 1, "Number of concurrent clients")
	startRate = flag.Int("start-rate", 10, "Starting messages per second")
	maxRate   = flag.Int("max-rate", 1000, "Maximum messages per second")
	msgSize   = flag.Int("size", 1024, "Message size in bytes")
)

type BandwidthTest struct {
	TotalSent   int64
	TotalBytes  int64
	CurrentRate int64
	PeakRate    int64
	StartTime   time.Time
	TestPhase   string
	mu          sync.Mutex
}

func (bt *BandwidthTest) UpdatePeakRate(rate int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if rate > bt.PeakRate {
		bt.PeakRate = rate
	}
}

func runBandwidthTest(address string, isSecure bool) error {
	stats := &BandwidthTest{
		StartTime: time.Now(),
		TestPhase: "connecting",
	}

	fmt.Printf("\n🌊 Bandwidth Stress Test\n")
	fmt.Printf("========================\n")
	fmt.Printf("Target: %s (%s)\n", address, map[bool]string{true: "secure", false: "regular"}[isSecure])
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("Clients: %d\n", *clients)
	fmt.Printf("Message size: %d bytes\n", *msgSize)
	fmt.Printf("Rate: %d → %d msg/sec\n", *startRate, *maxRate)

	// Generate test payload
	payload := make([]byte, *msgSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	testMessage := string(payload)

	// Create clients
	var clientList []interface{} // Can hold both regular and secure clients
	var wg sync.WaitGroup

	fmt.Printf("\n📡 Creating %d clients...\n", *clients)

	for i := 0; i < *clients; i++ {
		if isSecure {
			client, err := udp_network.NewSecureClientWithPSK(exampleconfig.PSK())
			if err != nil {
				return fmt.Errorf("failed to create secure client %d: %v", i, err)
			}

			if err := client.Connect(address); err != nil {
				return fmt.Errorf("secure client %d connection failed: %v", i, err)
			}

			clientList = append(clientList, client)
		} else {
			client, err := udp_network.NewClient()
			if err != nil {
				return fmt.Errorf("failed to create client %d: %v", i, err)
			}

			client.Start()

			if err := client.Connect(address); err != nil {
				return fmt.Errorf("client %d connection failed: %v", i, err)
			}

			clientList = append(clientList, client)
		}

		fmt.Printf("✅ Client %d connected\n", i+1)
	}

	// Warmup phase
	fmt.Printf("\n🔥 Warming up...\n")
	stats.TestPhase = "warmup"

	for i := 0; i < 10; i++ {
		for j, client := range clientList {
			msg := fmt.Sprintf("WARMUP_%d_%d", j, i)
			if isSecure {
				client.(*udp_network.SecureClient).SendText(msg)
			} else {
				client.(*udp_network.Client).SendText(msg)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Main test phases
	fmt.Printf("\n📊 Starting bandwidth test...\n")

	// Phase 1: Ramp-up
	stats.TestPhase = "ramp-up"
	currentRate := int64(*startRate)
	atomic.StoreInt64(&stats.CurrentRate, currentRate)

	// Start progress reporter
	stopReporter := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastSent := int64(0)
		lastTime := time.Now()

		for {
			select {
			case <-ticker.C:
				sent := atomic.LoadInt64(&stats.TotalSent)
				bytes := atomic.LoadInt64(&stats.TotalBytes)
				rate := atomic.LoadInt64(&stats.CurrentRate)
				phase := stats.TestPhase

				// Calculate actual rate
				actualRate := float64(sent-lastSent) / time.Since(lastTime).Seconds()
				lastSent = sent
				lastTime = time.Now()

				// Update peak rate
				stats.UpdatePeakRate(int64(actualRate))

				// Calculate bandwidth
				bandwidth := float64(bytes) * 8 / (1000 * 1000) / time.Since(stats.StartTime).Seconds()

				fmt.Printf("[%s] Rate: %d msg/s (actual: %.0f), Total: %d, Bandwidth: %.2f Mbps\n",
					phase, rate, actualRate, sent, bandwidth)

			case <-stopReporter:
				return
			}
		}
	}()

	// Sender workers
	stopSenders := make(chan bool)
	senderWg := sync.WaitGroup{}

	for i, client := range clientList {
		senderWg.Add(1)
		wg.Add(1)

		go func(clientID int, c interface{}) {
			defer senderWg.Done()
			defer wg.Done()

			messageID := 0

			for {
				select {
				case <-stopSenders:
					return
				default:
					// Get current rate limit
					targetRate := atomic.LoadInt64(&stats.CurrentRate)
					messagesPerClient := targetRate / int64(*clients)
					if messagesPerClient < 1 {
						messagesPerClient = 1
					}

					// Send burst of messages
					interval := time.Second / time.Duration(messagesPerClient)

					for j := 0; j < int(messagesPerClient); j++ {
						message := fmt.Sprintf("LOAD_%d_%d_%s", clientID, messageID, testMessage)
						messageID++

						var err error
						if isSecure {
							err = c.(*udp_network.SecureClient).SendText(message)
						} else {
							err = c.(*udp_network.Client).SendText(message)
						}

						if err != nil {
							fmt.Printf("❌ Client %d send error: %v\n", clientID, err)
						} else {
							atomic.AddInt64(&stats.TotalSent, 1)
							atomic.AddInt64(&stats.TotalBytes, int64(*msgSize))
						}

						time.Sleep(interval)
					}
				}
			}
		}(i, client)
	}

	// Test controller
	testStart := time.Now()
	rampupEnd := testStart.Add(*rampup)
	testEnd := testStart.Add(*duration)

	// Ramp-up phase
	fmt.Printf("\n📈 Ramp-up phase (%v)...\n", *rampup)
	rampupTicker := time.NewTicker(1 * time.Second)

	for time.Now().Before(rampupEnd) {
		select {
		case <-rampupTicker.C:
			// Increase rate
			progress := time.Since(testStart).Seconds() / rampup.Seconds()
			newRate := int64(float64(*startRate) + (float64(*maxRate-*startRate) * progress))
			atomic.StoreInt64(&stats.CurrentRate, newRate)
		}
	}
	rampupTicker.Stop()

	// Sustained phase
	stats.TestPhase = "sustained"
	fmt.Printf("\n🔥 Sustained phase at %d msg/s...\n", *maxRate)
	atomic.StoreInt64(&stats.CurrentRate, int64(*maxRate))

	// Wait for test duration
	remaining := time.Until(testEnd)
	if remaining > 0 {
		time.Sleep(remaining)
	}

	// Shutdown
	fmt.Println("\n🛑 Stopping test...")
	stats.TestPhase = "shutdown"
	close(stopSenders)
	senderWg.Wait()
	close(stopReporter)

	// Clean up clients
	for i, client := range clientList {
		if isSecure {
			client.(*udp_network.SecureClient).Close()
		} else {
			client.(*udp_network.Client).Stop()
		}
		fmt.Printf("✅ Client %d disconnected\n", i+1)
	}

	wg.Wait()

	// Final results
	testDuration := time.Since(stats.StartTime)
	totalMB := float64(stats.TotalBytes) / (1024 * 1024)
	avgBandwidth := float64(stats.TotalBytes) * 8 / (1000 * 1000) / testDuration.Seconds()
	peakBandwidth := float64(stats.PeakRate*int64(*msgSize)) * 8 / (1000 * 1000)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📊 BANDWIDTH TEST RESULTS")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Printf("\n📤 Traffic:\n")
	fmt.Printf("   Total messages:  %d\n", stats.TotalSent)
	fmt.Printf("   Total data:      %.2f MB\n", totalMB)
	fmt.Printf("   Test duration:   %v\n", testDuration)

	fmt.Printf("\n🚀 Performance:\n")
	fmt.Printf("   Average rate:    %.2f msg/s\n", float64(stats.TotalSent)/testDuration.Seconds())
	fmt.Printf("   Peak rate:       %d msg/s\n", stats.PeakRate)
	fmt.Printf("   Average bandwidth: %.2f Mbps\n", avgBandwidth)
	fmt.Printf("   Peak bandwidth:    %.2f Mbps\n", peakBandwidth)

	fmt.Printf("\n📡 Network Capability:\n")
	if avgBandwidth > 100 {
		fmt.Println("   Grade: EXCELLENT (>100 Mbps sustained)")
	} else if avgBandwidth > 50 {
		fmt.Println("   Grade: VERY GOOD (>50 Mbps sustained)")
	} else if avgBandwidth > 20 {
		fmt.Println("   Grade: GOOD (>20 Mbps sustained)")
	} else if avgBandwidth > 10 {
		fmt.Println("   Grade: FAIR (>10 Mbps sustained)")
	} else {
		fmt.Println("   Grade: LIMITED (<10 Mbps sustained)")
	}

	fmt.Printf("\n💡 Recommendations:\n")
	if stats.PeakRate < int64(*maxRate) {
		fmt.Printf("   - Server handled up to %d msg/s (target was %d)\n", stats.PeakRate, *maxRate)
		fmt.Println("   - Consider increasing server resources for higher throughput")
	} else {
		fmt.Printf("   - Server successfully handled %d msg/s\n", *maxRate)
		fmt.Println("   - Try increasing -max-rate to find the limit")
	}

	return nil
}

func main() {
	flag.Parse()

	if *host == "" {
		fmt.Println("UDP Network Remote Bandwidth Test")
		fmt.Println("=================================")
		fmt.Println()
		fmt.Println("This test measures maximum sustainable bandwidth and message throughput.")
		fmt.Println()
		fmt.Println("Usage: go run tests/remote-bandwidth-test/main.go -host <ip> [options]")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  # Basic bandwidth test")
		fmt.Println("  go run tests/remote-bandwidth-test/main.go -host 192.168.1.100")
		fmt.Println()
		fmt.Println("  # High-performance test")
		fmt.Println("  go run tests/remote-bandwidth-test/main.go -host 192.168.1.100 -clients 10 -max-rate 5000")
		fmt.Println()
		fmt.Println("  # Secure server test")
		fmt.Println("  go run tests/remote-bandwidth-test/main.go -host 192.168.1.100 -port 8443 -secure")
		fmt.Println()
		fmt.Println("  # Large message test")
		fmt.Println("  go run tests/remote-bandwidth-test/main.go -host 192.168.1.100 -size 4096")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -host string       Server IP/hostname (required)")
		fmt.Println("  -port string       Server port (default: 8080)")
		fmt.Println("  -secure           Use secure connection")
		fmt.Println("  -duration duration Test duration (default: 30s)")
		fmt.Println("  -rampup duration  Ramp-up time (default: 5s)")
		fmt.Println("  -clients int      Concurrent clients (default: 1)")
		fmt.Println("  -start-rate int   Starting msg/s (default: 10)")
		fmt.Println("  -max-rate int     Maximum msg/s (default: 1000)")
		fmt.Println("  -size int         Message size (default: 1024)")
		fmt.Println()
		log.Fatal("Host is required")
	}

	address := *host + ":" + *port

	fmt.Println("🌐 UDP Network Bandwidth Stress Test")
	fmt.Println("===================================")

	if err := runBandwidthTest(address, *secure); err != nil {
		fmt.Printf("\n❌ Test failed: %v\n", err)
		log.Fatal(err)
	}

	fmt.Println("\n✅ Bandwidth test completed successfully!")
}

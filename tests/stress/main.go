package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	var (
		serverMode = flag.Bool("server", false, "Run as server")
		clientAddr = flag.String("addr", "localhost:8080", "Server address")
		duration   = flag.Duration("duration", 30*time.Second, "Test duration")
		packetLoss = flag.Float64("loss", 0.0, "Packet loss rate (0.0-1.0)")
		largeMsg   = flag.Bool("large", false, "Test with large messages (fragmentation)")
		concurrent = flag.Int("concurrent", 10, "Concurrent message streams")
	)
	flag.Parse()

	if *serverMode {
		runStressServer(*packetLoss)
	} else {
		runStressClient(*clientAddr, *duration, *largeMsg, *concurrent)
	}
}

func runStressServer(packetLoss float64) {
	fmt.Printf("Starting stress test server (packet loss: %.1f%%)\n", packetLoss*100)

	var received uint64
	var dropped uint64
	var outOfOrder uint64
	lastSeq := make(map[string]uint64)

	server, err := udp_network.NewServer(":8080")
	if err != nil {
		log.Fatal(err)
	}

	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		// Simulate packet loss
		if packetLoss > 0 && rand.Float64() < packetLoss {
			atomic.AddUint64(&dropped, 1)
			return
		}

		atomic.AddUint64(&received, 1)

		// Check for out-of-order delivery
		if text, ok := msg.Data.(string); ok {
			parts := strings.SplitN(text, "|", 3)
			if len(parts) >= 2 {
				clientID := parts[0]
				var seq uint64
				fmt.Sscanf(parts[1], "%d", &seq)

				if lastSeq[clientID] > 0 && seq < lastSeq[clientID] {
					atomic.AddUint64(&outOfOrder, 1)
				}
				lastSeq[clientID] = seq
			}
		}

		// Echo back for verification
		if msg.Type == protocol.MessageTypeText {
			response := &protocol.Message{
				Type:      protocol.MessageTypeText,
				Timestamp: time.Now().Unix(),
				Data:      fmt.Sprintf("ACK: %v", msg.Data),
			}
			server.SendMessage(conn, response)
		}
	})

	server.Start()
	defer server.Stop()

	// Print stats
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for range ticker.C {
			r := atomic.LoadUint64(&received)
			d := atomic.LoadUint64(&dropped)
			o := atomic.LoadUint64(&outOfOrder)

			fmt.Printf("Stats - Received: %d, Dropped: %d (%.1f%%), Out-of-order: %d\n",
				r, d, float64(d)*100/float64(r+d+1), o)
		}
	}()

	select {}
}

func runStressClient(addr string, duration time.Duration, largeMessages bool, concurrent int) {
	fmt.Printf("Starting stress test client for %v\n", duration)
	fmt.Printf("Large messages: %v, Concurrent streams: %d\n", largeMessages, concurrent)

	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	client.Start()
	defer client.Stop()

	if err := client.Connect(addr); err != nil {
		log.Fatal(err)
	}

	var sent uint64
	var acked uint64
	var errors uint64

	// Track ACKs
	client.SetMessageHandler(func(msg *protocol.Message) {
		if text, ok := msg.Data.(string); ok {
			if strings.HasPrefix(text, "ACK:") {
				atomic.AddUint64(&acked, 1)
			}
		}
	})

	// Start concurrent senders
	stop := make(chan bool)
	time.AfterFunc(duration, func() {
		close(stop)
	})

	for i := 0; i < concurrent; i++ {
		go func(streamID int) {
			seq := uint64(0)

			for {
				select {
				case <-stop:
					return
				default:
					// Generate message
					var data string
					if largeMessages {
						// Create 10KB message to test fragmentation
						data = fmt.Sprintf("%d|%d|%s", streamID, seq,
							strings.Repeat("Large message data. ", 500))
					} else {
						data = fmt.Sprintf("%d|%d|Test message from stream %d, sequence %d",
							streamID, seq, streamID, seq)
					}

					msg := &protocol.Message{
						Type:      protocol.MessageTypeText,
						Timestamp: time.Now().Unix(),
						Data:      data,
					}

					if err := client.SendMessage(client.Connection, msg); err != nil {
						atomic.AddUint64(&errors, 1)
					} else {
						atomic.AddUint64(&sent, 1)
					}

					seq++

					// Vary sending rate
					if seq%100 == 0 {
						time.Sleep(10 * time.Millisecond)
					} else if seq%10 == 0 {
						time.Sleep(1 * time.Millisecond)
					}
				}
			}
		}(i)
	}

	// Print stats
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for range ticker.C {
			s := atomic.LoadUint64(&sent)
			a := atomic.LoadUint64(&acked)
			e := atomic.LoadUint64(&errors)

			loss := float64(s-a) * 100 / float64(s+1)
			fmt.Printf("Sent: %d, ACKed: %d, Errors: %d, Effective loss: %.1f%%\n",
				s, a, e, loss)
		}
	}()

	<-stop

	// Final stats
	time.Sleep(2 * time.Second) // Wait for final ACKs
	s := atomic.LoadUint64(&sent)
	a := atomic.LoadUint64(&acked)
	e := atomic.LoadUint64(&errors)

	fmt.Printf("\nFinal Results:\n")
	fmt.Printf("Total sent: %d\n", s)
	fmt.Printf("Total ACKed: %d\n", a)
	fmt.Printf("Total errors: %d\n", e)
	fmt.Printf("Success rate: %.2f%%\n", float64(a)*100/float64(s))
	fmt.Printf("Message rate: %.2f msg/sec\n", float64(s)/duration.Seconds())
}

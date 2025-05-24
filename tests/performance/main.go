package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DangerosoDavo/udp_network"
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	var (
		serverMode = flag.Bool("server", false, "Run as server")
		clientAddr = flag.String("addr", "localhost:8080", "Server address")
		messages   = flag.Int("messages", 10000, "Number of messages to send")
		msgSize    = flag.Int("size", 100, "Message size in bytes")
		clients    = flag.Int("clients", 1, "Number of concurrent clients")
	)
	flag.Parse()

	if *serverMode {
		runPerfServer()
	} else {
		runPerfClient(*clientAddr, *messages, *msgSize, *clients)
	}
}

func runPerfServer() {
	fmt.Println("Starting performance test server...")

	var received uint64
	var bytes uint64
	startTime := time.Now()

	server, err := udp_network.NewServer(":8080")
	if err != nil {
		log.Fatal(err)
	}

	// Count received messages
	server.SetMessageHandler(func(conn *protocol.Connection, msg *protocol.Message) {
		atomic.AddUint64(&received, 1)
		if data, ok := msg.Data.(string); ok {
			atomic.AddUint64(&bytes, uint64(len(data)))
		}
	})

	server.Start()
	defer server.Stop()

	// Print stats every second
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		lastReceived := uint64(0)
		lastBytes := uint64(0)

		for range ticker.C {
			currentReceived := atomic.LoadUint64(&received)
			currentBytes := atomic.LoadUint64(&bytes)

			deltaMsg := currentReceived - lastReceived
			deltaBytes := currentBytes - lastBytes
			elapsed := time.Since(startTime).Seconds()

			fmt.Printf("Stats: %d msg/sec, %.2f MB/sec, Total: %d messages, %.2f MB in %.1fs\n",
				deltaMsg,
				float64(deltaBytes)/1024/1024,
				currentReceived,
				float64(currentBytes)/1024/1024,
				elapsed)

			lastReceived = currentReceived
			lastBytes = currentBytes
		}
	}()

	select {}
}

func runPerfClient(addr string, totalMessages, msgSize, numClients int) {
	fmt.Printf("Starting performance test client...\n")
	fmt.Printf("Target: %s, Messages: %d, Size: %d bytes, Clients: %d\n",
		addr, totalMessages, msgSize, numClients)

	// Generate test message
	testData := make([]byte, msgSize)
	for i := range testData {
		testData[i] = byte('A' + (i % 26))
	}
	testMessage := string(testData)

	messagesPerClient := totalMessages / numClients
	var wg sync.WaitGroup
	start := time.Now()

	// Launch concurrent clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			client, err := udp_network.NewClient()
			if err != nil {
				log.Printf("Client %d error: %v", clientID, err)
				return
			}

			client.Start()
			defer client.Stop()

			if err := client.Connect(addr); err != nil {
				log.Printf("Client %d connect error: %v", clientID, err)
				return
			}

			// Disable message printing
			client.SetMessageHandler(func(msg *protocol.Message) {})

			// Send messages
			for j := 0; j < messagesPerClient; j++ {
				msg := &protocol.Message{
					Type:      protocol.MessageTypeText,
					Timestamp: time.Now().Unix(),
					Data:      fmt.Sprintf("[%d-%d] %s", clientID, j, testMessage),
				}

				if err := client.SendMessage(client.Connection, msg); err != nil {
					log.Printf("Send error: %v", err)
				}

				// Pace sending to avoid overwhelming
				if j%1000 == 0 {
					time.Sleep(10 * time.Millisecond)
				}
			}

			fmt.Printf("Client %d completed sending %d messages\n", clientID, messagesPerClient)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	totalBytes := totalMessages * msgSize
	rate := float64(totalMessages) / elapsed.Seconds()
	throughput := float64(totalBytes) / elapsed.Seconds() / 1024 / 1024

	fmt.Printf("\nPerformance Test Results:\n")
	fmt.Printf("Total messages: %d\n", totalMessages)
	fmt.Printf("Total data: %.2f MB\n", float64(totalBytes)/1024/1024)
	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Message rate: %.2f msg/sec\n", rate)
	fmt.Printf("Throughput: %.2f MB/sec\n", throughput)
	fmt.Printf("Avg latency: %.2f ms/msg\n", elapsed.Seconds()*1000/float64(totalMessages))
}

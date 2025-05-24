package main

import (
	"fmt"
	"time"
	
	"github.com/DangerosoDavo/udp_network/protocol"
)

func main() {
	fmt.Println("=== Simple Crypto Performance Test ===")
	
	// Create crypto states
	crypto, err := protocol.NewCryptoState()
	if err != nil {
		panic(err)
	}
	
	// Create another for key exchange
	remoteCrypto, err := protocol.NewCryptoState()
	if err != nil {
		panic(err)
	}
	
	// Exchange keys
	crypto.SetRemotePublicKey(remoteCrypto.GetPublicKey())
	remoteCrypto.SetRemotePublicKey(crypto.GetPublicKey())
	
	// Test data
	sizes := []int{100, 500, 1000, 5000, 10000}
	iterations := 10000
	
	fmt.Println("\nBenchmarking ChaCha20-Poly1305 encryption/decryption:")
	fmt.Println("Size (bytes) | Encrypt (µs) | Decrypt (µs) | Overhead | Throughput (MB/s)")
	fmt.Println("-------------|--------------|--------------|----------|------------------")
	
	for _, size := range sizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i % 256)
		}
		
		// Benchmark encryption
		start := time.Now()
		var encrypted []byte
		for i := 0; i < iterations; i++ {
			encrypted, _ = crypto.Encrypt(data)
		}
		encryptTime := time.Since(start)
		
		// Benchmark decryption
		start = time.Now()
		for i := 0; i < iterations; i++ {
			_, _ = remoteCrypto.Decrypt(encrypted)
		}
		decryptTime := time.Since(start)
		
		// Calculate metrics
		encryptPerOp := encryptTime.Nanoseconds() / int64(iterations) / 1000 // microseconds
		decryptPerOp := decryptTime.Nanoseconds() / int64(iterations) / 1000 // microseconds
		overhead := len(encrypted) - size
		throughput := float64(size*iterations) / (1024 * 1024) / encryptTime.Seconds()
		
		fmt.Printf("%-12d | %-12d | %-12d | %-8d | %.2f\n", 
			size, encryptPerOp, decryptPerOp, overhead, throughput)
	}
	
	fmt.Println("\nNotes:")
	fmt.Println("- Overhead is constant at 28 bytes (12-byte nonce + 16-byte tag)")
	fmt.Println("- ChaCha20-Poly1305 is optimized for software implementation")
	fmt.Println("- Performance scales linearly with message size")
}